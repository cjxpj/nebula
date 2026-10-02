package build

import "strings"

// ============== .n 词库源码 → 词库 JSON 中间表示（IR） ==============
//
// 积木编程打开已有 .n 文件时，需要把源码反向解析为 IR 再转成积木显示。
// 这里手写解析器，与 IRToNebula 构成往返，保证「文件 → 积木 → 再生成代码」不丢内容：
//
//   - 每一行都有归属：能对应积木的行映射为对应语句，无法识别的行降级为
//     output（原样输出文本）或 raw（原样语句）——两者序列化时都按原文本重发，
//     因此内容与顺序天然保真；
//   - 块（如果 / 匹配 / 循环 / 遍历 / 文本 / JSON）只在「开启标记与关闭标记都齐备、
//     且分支能完整建模」时才转成积木，否则整块逐行降级为 raw，绝不错拆结构；
//   - 行内文本统一去掉行首空白（与运行时 strings.TrimLeft 口径一致），
//     序列化时会重新按层级加缩进，故缩进可无损还原；
//   - 头部 / 正文的切分、块开启与关闭的识别直接复用同包 formatOpen /
//     formatCloseKind / formatLeafClosed / formatBranch，与格式化、编译保持一致。

// NebulaToIR 把 .n 词库源码反向解析为 IR。
func NebulaToIR(src string) IR {
	lines := splitSourceLines(src)
	if len(lines) == 0 {
		return IR{}
	}

	// 头部 = 文件开头到第一个空行之间（与 run 包、FormatDic 的判定一致）：
	// 空行下标大于 0 时前面是头部，等于 0 时无头部；全文无空行且开头像初始化脚本时整篇按头部。
	blank := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			blank = i
			break
		}
	}

	var ir IR
	bodyStart := 0
	switch {
	case blank > 0:
		ir.Header = parseStmts(lines[:blank])
		bodyStart = skipBlankRun(lines, blank)
	case blank == 0:
		bodyStart = skipBlankRun(lines, 0)
	default:
		if FirstHeadLikeLine(lines) {
			ir.Header = parseStmts(lines)
			return ir
		}
	}

	ir.Entries = parseEntries(lines[bodyStart:])
	return ir
}

// splitSourceLines 按行切分源码：CRLF / 单个 CR 统一为 LF，并去掉末尾空行。
func splitSourceLines(text string) []string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}

// skipBlankRun 跳过从 i 起的连续空行，返回首个非空行下标。
func skipBlankRun(lines []string, i int) int {
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	return i
}

// parseEntries 把正文行解析为词条列表。
func parseEntries(lines []string) []Entry {
	var entries []Entry
	for _, chunk := range splitEntryChunks(lines) {
		entries = append(entries, chunkToEntries(chunk)...)
	}
	return entries
}

// splitEntryChunks 按「空行 + 多行块（#{...}# / <?n...?> / 多行注释）」把正文切成词条块。
// 一个词条块内的行共享同一条触发词（与运行时以空行分隔词条的口径一致）。
func splitEntryChunks(lines []string) [][]string {
	var chunks [][]string
	var cur []string
	state := "normal" // normal | hashbrace | qmark | blockcomment

	flush := func() {
		if len(cur) > 0 {
			chunks = append(chunks, cur)
			cur = nil
		}
	}

	for _, raw := range lines {
		t := strings.TrimSpace(raw)

		switch state {
		case "hashbrace":
			cur = append(cur, raw)
			if t == "}#" {
				flush()
				state = "normal"
			}
			continue
		case "qmark":
			cur = append(cur, raw)
			if t == "?>" {
				flush()
				state = "normal"
			}
			continue
		case "blockcomment":
			// 多行注释只吞空行、不切分词条：注释整体是词条正文的一部分，
			// 闭合后仍要与其后的行同属一个词条（由空行触发 flush）。
			cur = append(cur, raw)
			if strings.HasSuffix(t, "*/") {
				state = "normal"
			}
			continue
		}

		if t == "" {
			flush()
			continue
		}
		if t == "<?n" {
			cur = append(cur, raw)
			state = "qmark"
			continue
		}
		if isBlockCommentOpen(t) {
			cur = append(cur, raw)
			state = "blockcomment"
			continue
		}
		cur = append(cur, raw)
		if strings.HasSuffix(t, " #{") {
			state = "hashbrace"
		}
	}
	flush()
	return chunks
}

// chunkToEntries 把一个词条块切成条目：块首的纯注释行单独成条，
// 其余以首个非注释行作为触发词。
func chunkToEntries(chunk []string) []Entry {
	if len(chunk) == 0 {
		return nil
	}
	j := firstNonCommentLine(chunk)
	if j < 0 {
		return []Entry{commentEntry(chunk)}
	}
	if j > 0 {
		return append([]Entry{commentEntry(chunk[:j])}, buildEntryFromChunk(chunk[j:])...)
	}
	return buildEntryFromChunk(chunk)
}

// firstNonCommentLine 返回块内首个非注释行的下标（整块都是注释行时返回 -1）。
// 多行注释（/* ... */）整体按注释行看待，其内部行不会被误当成触发词。
func firstNonCommentLine(chunk []string) int {
	for i := 0; i < len(chunk); i++ {
		t := strings.TrimSpace(chunk[i])
		if !isCommentLine(t) {
			return i
		}
		if isBlockCommentOpen(t) {
			for i < len(chunk) && !strings.Contains(strings.TrimSpace(chunk[i]), "*/") {
				i++
			}
			if i >= len(chunk) {
				return -1
			}
		}
	}
	return -1
}

// commentEntry 把一段纯注释行转成词条：首行作触发词（原样输出），其余行按注释语句解析
// （与积木层 commentEntriesOf 互为逆操作，保证 /* ... */ 这类注释原样保留、往返无损）。
// 运行时会把 // 开头的行整体跳过（只收集为函数说明），故此形态语义与原文件一致。
func commentEntry(lines []string) Entry {
	if len(lines) == 0 {
		return Entry{Kind: "normal"}
	}
	body := make([]Stmt, 0, len(lines)-1)
	for _, l := range lines[1:] {
		body = append(body, commentStmtOf(trimLeadingBlank(l)))
	}
	return Entry{Kind: "normal", Name: trimLeadingBlank(lines[0]), Body: body}
}

// isCommentLine 判断是否为注释行（// 、/* 、*/ 开头）。
func isCommentLine(t string) bool {
	return strings.HasPrefix(t, "//") || strings.HasPrefix(t, "/*") || strings.HasPrefix(t, "*/")
}

// isBlockCommentOpen 判断是否为「未在本行闭合的跨行注释起始行」（即 /* 之后同行没有 */）。
func isBlockCommentOpen(t string) bool {
	t = strings.TrimLeft(t, " \t")
	return strings.HasPrefix(t, "/*") && !strings.Contains(t[2:], "*/")
}

// buildEntryFromChunk 由词条块生成词条：首行是触发词，其余行为正文。
func buildEntryFromChunk(chunk []string) []Entry {
	if len(chunk) == 0 {
		return nil
	}
	trigger := trimLeadingBlank(chunk[0])
	kind, className, name, rule := parseEntryTrigger(trigger)
	return []Entry{{Kind: kind, ClassName: className, Name: name, Rule: rule, Body: parseStmts(chunk[1:])}}
}

// parseEntryTrigger 解析触发词行的 [类别] 前缀。
// 类别缩写与运行时（run.parseTriggerPrefix）口径一致：[F]/[f] 归一为 [函数]，[L]/[l] 归一为 [内部]。
// 函数类别的 [|参数规则] 后缀（如 [F|2]、[函数|1|2]、[函数:类|2]）解析为 Entry.Rule；
// 保守策略：只对 [函数] / [内部]（含缩写）/ [函数:类名] / [内部:类名] 建模，
// 其余（[内部|规则]、[群事件] 等特殊类别）一律按普通触发词原样保留，
// 保证 IRToNebula 重发时文本完全一致（参数规则仅对函数类别生效，与运行时一致）。
func parseEntryTrigger(line string) (kind, className, name, rule string) {
	t := trimLeadingBlank(line)
	if !strings.HasPrefix(t, "[") {
		return "normal", "", t, ""
	}
	end := strings.Index(t, "]")
	if end <= 1 {
		return "normal", "", t, ""
	}
	tag := t[1:end]
	rest := t[end+1:]
	if before, after, ok := strings.Cut(tag, "|"); ok {
		tag, rule = before, after
	}
	category, class := tag, ""
	if before, after, ok := strings.Cut(tag, ":"); ok {
		category, class = before, after
	}
	switch normalizeTriggerCategory(category) {
	case "函数":
		if class != "" {
			return "func_class", class, rest, rule
		}
		return "func", "", rest, rule
	case "内部":
		if rule != "" {
			// 参数规则仅对函数类别生效，内部类别带规则时原样保留
			return "normal", "", t, ""
		}
		if class != "" {
			return "inner_class", class, rest, ""
		}
		return "inner", "", rest, ""
	}
	return "normal", "", t, ""
}

// normalizeTriggerCategory 把触发词类别缩写归一为全称，与 run.parseTriggerPrefix 保持一致。
func normalizeTriggerCategory(category string) string {
	switch category {
	case "F", "f":
		return "函数"
	case "L", "l":
		return "内部"
	}
	return category
}

// parseStmts 把若干行解析为语句列表。
func parseStmts(lines []string) []Stmt {
	return parseStmtsCtx(lines, false)
}

// parseStmtsCtx 解析语句列表；inJson 表示当前处于 JSON 框内（此时「键=值 / 键:=值」按 kv 处理）。
func parseStmtsCtx(lines []string, inJson bool) []Stmt {
	var out []Stmt
	for i := 0; i < len(lines); {
		raw := lines[i]
		t := trimLeadingBlank(raw)

		if strings.TrimSpace(t) == "" {
			out = append(out, Stmt{T: "raw", Text: ""})
			i++
			continue
		}

		// 多行块注释：整段原样保留
		if isBlockCommentOpen(t) {
			j := i + 1
			for j < len(lines) && !strings.Contains(trimLeadingBlank(lines[j]), "*/") {
				j++
			}
			if j >= len(lines) {
				j = len(lines) - 1
			}
			out = appendRaw(out, lines, i, j)
			i = j + 1
			continue
		}

		if f, ok := formatOpen(t); ok {
			if end := findBlockEnd(lines, i); end > i {
				if s, ok2 := buildBlockStmt(f, lines, i, end); ok2 {
					out = append(out, s)
				} else {
					out = appendRaw(out, lines, i, end)
				}
				i = end + 1
				continue
			}
			// 未闭合的框：按普通行处理，交由 leafStmt 原样保留
		}

		if inJson {
			if s, ok := kvStmt(t); ok {
				out = append(out, s)
				i++
				continue
			}
		}

		out = append(out, leafStmt(t))
		i++
	}
	return out
}

// appendRaw 把 lines[from..to] 逐行原样追加为 raw 语句。
func appendRaw(out []Stmt, lines []string, from, to int) []Stmt {
	for k := from; k <= to; k++ {
		out = append(out, Stmt{T: "raw", Text: trimLeadingBlank(lines[k])})
	}
	return out
}

// kvStmt 把 JSON 框内的「键=值 / 键:=值」解析为 kv 语句。
func kvStmt(t string) (Stmt, bool) {
	key, rest := ValTextKeyScan(t)
	if key == "" {
		return Stmt{}, false
	}
	if strings.HasPrefix(rest, ":=") {
		return Stmt{T: "kv", Key: key, Mode: ":=", V: rest[2:]}, true
	}
	if strings.HasPrefix(rest, "=") {
		return Stmt{T: "kv", Key: key, Mode: "=", V: rest[1:]}, true
	}
	return Stmt{}, false
}

// leafStmt 把单行解析为语句：识别赋值、引入、动作、注释等，
// 无法识别的行降级为 output（原样输出文本），保证内容不丢。
func leafStmt(t string) Stmt {
	if strings.TrimSpace(t) == "" {
		return Stmt{T: "raw", Text: ""}
	}
	// 多行块、旧式块标记：原样保留
	if t == "}#" || t == "<?n" || t == "?>" {
		return Stmt{T: "raw", Text: t}
	}
	if strings.HasPrefix(t, "/*") || strings.HasPrefix(t, "*/") {
		return Stmt{T: "raw", Text: t}
	}
	if strings.HasPrefix(t, "// ") {
		return Stmt{T: "comment", Text: strings.TrimPrefix(t, "// ")}
	}
	if strings.HasPrefix(t, "//") {
		return Stmt{T: "raw", Text: t}
	}
	for _, p := range []string{"如果:", "否则如果:", "否则:", "否则", "如果尾", "#:"} {
		if strings.HasPrefix(t, p) {
			return Stmt{T: "raw", Text: t}
		}
	}
	// 全局引入：旧写法 #引入=目标，对应「全局引入」积木。
	if strings.HasPrefix(t, "#引入=") {
		if p := strings.TrimSpace(t[len("#引入="):]); p != "" {
			return Stmt{T: "importGlobal", Path: p}
		}
	}
	// 引入：规范形式 $引入 目标$（IRToNebula 重发时逐字一致）。
	if strings.HasPrefix(t, "$引入 ") && strings.HasSuffix(t, "$") {
		p := t[len("$引入 ") : len(t)-1]
		if p != "" && p == strings.TrimSpace(p) {
			return Stmt{T: "import", Path: p}
		}
	}
	// 赋予值形式引入：变量:$引入 目标$（把整个包打包成实例赋给变量，之后用 $变量.方法$ 调用）。
	if i := strings.Index(t, ":$引入 "); i > 0 && strings.HasSuffix(t, "$") {
		name := t[:i]
		p := t[i+len(":$引入 ") : len(t)-1]
		if name == strings.TrimSpace(name) && p != "" && p == strings.TrimSpace(p) {
			return Stmt{T: "import", Name: name, Path: p}
		}
	}
	if isFuncCallLine(t) {
		return Stmt{T: "raw", Text: t}
	}
	switch t {
	case ">中断":
		return Stmt{T: "break"}
	case ">跳过":
		return Stmt{T: "continue"}
	case ">终止":
		return Stmt{T: "stop"}
	case ">终止循环":
		return Stmt{T: "stopLoop"}
	case ">终止遍历":
		return Stmt{T: "stopForeach"}
	}
	if vt, key, val := ValTextTest(t); vt != 0 && key != "" {
		switch vt {
		case 1:
			return Stmt{T: "assign", Name: key, Op: "sub", V: val}
		case 2:
			return Stmt{T: "assign", Name: key, Op: "add", V: val}
		case 5:
			return Stmt{T: "assign", Name: key, Op: "raw", V: val}
		case 6:
			return Stmt{T: "assign", Name: key, Op: "set", V: val}
		}
		return Stmt{T: "raw", Text: t}
	}
	return Stmt{T: "output", V: t}
}

// findBlockEnd 从 start 处的框开启行出发，返回与之配对的关闭行下标；
// 未找到返回 -1。栈机与 formatIndent 完全一致，保证与排版口径相同。
func findBlockEnd(lines []string, start int) int {
	f, ok := formatOpen(trimLeadingBlank(lines[start]))
	if !ok {
		return -1
	}
	f.depth = 1
	stack := []formatFrame{f}

	for i := start + 1; i < len(lines); i++ {
		line := trimLeadingBlank(lines[i])
		depth := len(stack)
		if depth == 0 {
			return i - 1
		}
		if stack[depth-1].leaf {
			if formatLeafClosed(&stack[depth-1], line) {
				stack = stack[:depth-1]
			}
			if len(stack) == 0 {
				return i
			}
			continue
		}
		if kind := formatCloseKind(line); kind != "" && stack[depth-1].kind == kind {
			stack = stack[:depth-1]
			if len(stack) == 0 {
				return i
			}
			continue
		}
		if formatBranch(&stack[depth-1], line) {
			continue
		}
		if nf, ok := formatOpen(line); ok {
			nf.depth = 1
			stack = append(stack, nf)
		}
	}
	return -1
}

// topLevelSplit 在顶层按 isMarker 切分段：命中行作为 marker 单独返回，不进入任何段。
// 段数为 len(markers)+1，第 i 段为第 i 个 marker 之前的内容（首段在 markers 之前）。
func topLevelSplit(lines []string, isMarker func(string) bool) (segs [][]string, markers []string) {
	segs = [][]string{{}}
	stack := make([]formatFrame, 0, 4)
	for _, raw := range lines {
		line := trimLeadingBlank(raw)
		if len(stack) == 0 && isMarker(line) {
			markers = append(markers, line)
			segs = append(segs, []string{})
			continue
		}
		segs[len(segs)-1] = append(segs[len(segs)-1], raw)

		depth := len(stack)
		if depth > 0 && stack[depth-1].leaf {
			if formatLeafClosed(&stack[depth-1], line) {
				stack = stack[:depth-1]
			}
			continue
		}
		if kind := formatCloseKind(line); kind != "" && depth > 0 && stack[depth-1].kind == kind {
			stack = stack[:depth-1]
			continue
		}
		if depth > 0 && formatBranch(&stack[depth-1], line) {
			continue
		}
		if f, ok := formatOpen(line); ok {
			f.depth = 1
			stack = append(stack, f)
		}
	}
	return segs, markers
}

// buildBlockStmt 尝试把 lines[start..end]（含开闭标记）转成一条块语句；
// 无法完整建模时返回 false，由调用方整块降级为 raw。
func buildBlockStmt(f formatFrame, lines []string, start, end int) (Stmt, bool) {
	switch f.kind {
	case "if":
		return buildIfStmt(lines, start, end)
	case "match":
		return buildMatchStmt(lines, start, end)
	case "for":
		return buildLoopStmt(lines, start, end)
	case "foreach":
		return buildForeachStmt(lines, start, end)
	case "text":
		return buildTextStmt(lines, start, end)
	case "json":
		return buildJsonStmt(lines, start, end)
	}
	return Stmt{}, false
}

// buildIfStmt 构建判断框语句。
// 结构：如果>条件 / (若干) >否则如果:条件 / (至多一个) >否则 / <如果。
// >否则如果: 必须全部位于 >否则 之前；>否则 若有分支体则不能为空（与序列化对称，
// 否则重发时会丢行）。「否则如果」分支体允许为空。
func buildIfStmt(lines []string, start, end int) (Stmt, bool) {
	open := trimLeadingBlank(lines[start])
	inner := lines[start+1 : end]
	segs, markers := topLevelSplit(inner, func(l string) bool {
		return l == ">否则" || strings.HasPrefix(l, ">否则如果:")
	})

	s := Stmt{T: "if", Cond: strings.TrimPrefix(open, "如果>")}
	if len(markers) == 0 {
		s.Do = parseStmts(inner)
		return s, true
	}
	s.Do = parseStmts(segs[0])
	for i, m := range markers {
		body := segs[i+1]
		if m == ">否则" {
			// >否则 必须是最后一个分支，且分支体不能为空（空体会在重发时丢行）。
			if i != len(markers)-1 || len(trimBlankLines(body)) == 0 {
				return Stmt{}, false
			}
			s.Else = parseStmts(body)
			continue
		}
		cond, ok := strings.CutPrefix(m, ">否则如果:")
		if !ok {
			return Stmt{}, false
		}
		s.Elifs = append(s.Elifs, IfBranch{Cond: cond, Body: parseStmts(body)})
	}
	return s, true
}

// trimBlankLines 去掉首尾空行；用于判断分支体是否「实质为空」。
func trimBlankLines(lines []string) []string {
	i, j := 0, len(lines)
	for i < j && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	for j > i && strings.TrimSpace(lines[j-1]) == "" {
		j--
	}
	return lines[i:j]
}

// buildMatchStmt 构建匹配框语句。
func buildMatchStmt(lines []string, start, end int) (Stmt, bool) {
	open := trimLeadingBlank(lines[start])
	inner := lines[start+1 : end]
	segs, markers := topLevelSplit(inner, func(l string) bool {
		t := strings.TrimPrefix(l, ">")
		return strings.HasPrefix(t, "如果是:") || t == "如果不是"
	})
	if len(segs[0]) != 0 {
		// 首个分支标记之前有内容，结构无法建模
		return Stmt{}, false
	}

	var cases []Case
	var def []Stmt
	for i, m := range markers {
		body := segs[i+1]
		if m == "如果不是" {
			if i != len(markers)-1 || len(def) > 0 || len(body) == 0 {
				return Stmt{}, false
			}
			def = parseStmts(body)
			continue
		}
		if !strings.HasPrefix(m, "如果是:") || len(def) > 0 {
			return Stmt{}, false
		}
		cases = append(cases, Case{V: strings.TrimPrefix(m, "如果是:"), Body: parseStmts(body)})
	}
	if len(cases) == 0 && len(def) == 0 {
		return Stmt{}, false
	}
	return Stmt{T: "match", Expr: strings.TrimPrefix(open, "匹配>"), Cases: cases, Def: def}, true
}

// buildLoopStmt 构建循环框语句（判断循环 / 次数循环 / 区间循环）。
func buildLoopStmt(lines []string, start, end int) (Stmt, bool) {
	open := trimLeadingBlank(lines[start])
	inner := lines[start+1 : end]

	var s Stmt
	if after, ok := strings.CutPrefix(open, "判断循环>"); ok {
		s = Stmt{T: "loopWhile", Cond: after}
	} else {
		rest := strings.TrimPrefix(open, "循环>")
		eq := strings.Index(rest, "=")
		if eq < 0 {
			return Stmt{}, false
		}
		varName, val := rest[:eq], rest[eq+1:]
		if before, after, ok := strings.Cut(val, "~"); ok {
			s = Stmt{T: "loopRange", Var: varName, From: before, To: after}
		} else {
			s = Stmt{T: "loopCount", Var: varName, Count: val}
		}
	}
	s.Body = parseStmts(inner)
	return s, true
}

// buildForeachStmt 构建遍历框语句。
func buildForeachStmt(lines []string, start, end int) (Stmt, bool) {
	open := trimLeadingBlank(lines[start])
	rest := strings.TrimPrefix(open, "遍历>")
	before, after, ok := strings.Cut(rest, "=")
	if !ok {
		return Stmt{}, false
	}
	head, target := before, after

	s := Stmt{T: "foreach", Target: target, Body: parseStmts(lines[start+1 : end])}
	if c := strings.Index(head, ","); c >= 0 {
		key, val := head[:c], head[c+1:]
		if key == "" || val == "" {
			return Stmt{}, false
		}
		s.Mode = "kv"
		s.Key = key
		s.Val = val
	} else {
		s.Mode = "v"
		s.Val = head
	}
	return s, true
}

// buildTextStmt 构建文本框语句（变量名:文本>分隔符 ... <文本）。
func buildTextStmt(lines []string, start, end int) (Stmt, bool) {
	open := trimLeadingBlank(lines[start])
	vt, name, val := ValTextTest(open)
	if vt != 6 || name == "" {
		return Stmt{}, false
	}
	var sep string
	switch {
	case strings.HasPrefix(val, "文本>"):
		sep = strings.TrimPrefix(val, "文本>")
	case strings.HasPrefix(val, "纯文本>"):
		sep = strings.TrimPrefix(val, "纯文本>")
	default:
		// 裸 文本> / 纯文本> 无变量名，无法建模
		return Stmt{}, false
	}
	return Stmt{T: "textblock", Name: name, Sep: sep, Body: parseStmts(lines[start+1 : end])}, true
}

// buildJsonStmt 构建 JSON 框语句（JSON>变量名={} / JSON>变量名=[]）。
func buildJsonStmt(lines []string, start, end int) (Stmt, bool) {
	open := trimLeadingBlank(lines[start])
	rest := strings.TrimPrefix(open, "JSON>")
	eq := strings.Index(rest, "=")
	if eq <= 0 {
		return Stmt{}, false
	}
	name, kindStr := rest[:eq], rest[eq+1:]
	var kind string
	switch kindStr {
	case "{}":
		kind = "obj"
	case "[]":
		kind = "arr"
	default:
		return Stmt{}, false
	}
	return Stmt{T: "json", Name: name, Kind: kind, Body: parseStmtsCtx(lines[start+1:end], true)}, true
}
