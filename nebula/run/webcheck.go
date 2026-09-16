package run

import (
	"regexp"
	"sort"
	"strings"

	"github.com/cjxpj/nebula/dto"
)

// webDicGlobalVars 网页词库在 HTTP 链路里由 serveHTTP 注入的全局变量。
// 它们既不出现在脚本块的赋值里，也不在 isMagicVar 名单中，需要在检查前预置，否则会被误报。
var webDicGlobalVars = []string{"响应状态", "输出头部", "COOKIE", "网站根目录", "访问数据"}

var (
	// webDicScriptOpenRe 匹配 <script ...> 开始标签（含属性，忽略大小写与换行）。
	webDicScriptOpenRe = regexp.MustCompile(`(?is)<script\b[^>]*>`)
	// webDicScriptCloseRe 匹配 </script> 结束标签。
	webDicScriptCloseRe = regexp.MustCompile(`(?is)</script\s*>`)
	// webDicTypeAttrRe / webDicIDAttrRe 取开始标签里的 type / id 属性值。
	// 属性名要求位于标签内空白之后，避免把 data-id 之类的属性误当成 id。
	webDicTypeAttrRe = regexp.MustCompile(`(?is)(?:^|\s)type\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
	webDicIDAttrRe   = regexp.MustCompile(`(?is)(?:^|\s)id\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
	// webDicActionRe 匹配 Go 模板动作 {{...}}，webDicKeyRe 从动作里取 {{.键}} / {{$.键}} 的首段键名。
	// 前面的字符必须不是标识符字符，这样 {{.A.B}} 只取 A 而不把 B 也当成键。
	webDicActionRe = regexp.MustCompile(`(?s)\{\{-?\s*.*?\s*-?\}\}`)
	webDicKeyRe    = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])\.([\p{L}\p{N}_]+)`)
	// webDicStyleRe / webDicScriptAnyRe 用于屏蔽 CSS 与 JS 区域：它们不经 Nebula 解析，
	// 内容里满是 `属性: 值;`、$、% 这类形状，与 Nebula 语法相像，直接扫描会误报。
	webDicStyleRe     = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style\s*>`)
	webDicScriptAnyRe = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script\s*>`)
	// webDicVarReadRe 匹配 %变量% 取值（变量名必须以字母或下划线开头，避免把 %20% 这类编码误判成变量）。
	webDicVarReadRe = regexp.MustCompile(`%(\p{L}|_)[\p{L}\p{N}_.]*%`)
	// webDicInlineRe 匹配内联执行块 <?n ... ?>：整块按 Nebula 语法就地执行，结果插回块所在位置。
	// 非贪婪匹配，遇到最近的 ?> 结束；块内允许为空（空块执行结果为空串）。
	webDicInlineRe = regexp.MustCompile(`(?s)<\?n(.*?)\?>`)
)

// webDicBlock 原文里的一个执行块：<script type="nebula"> 脚本块或 <?n ... ?> 内联块。
type webDicBlock struct {
	id        string   // 块的 id 属性；空表示无 id（块内变量会整体并入模板数据）。内联块没有 id
	lines     []string // 块正文，首尾空白按运行时的 TrimSpace 语义裁掉
	lineNums  []int    // lines 每行对应的原文行号（1-based）
	openStart int      // 块起始标记在原文中的起始偏移
	closeEnd  int      // 块结束标记在原文中的结束偏移
}

// WebDicCheck 对网页词库（.wn）原文做三类静态检查：
//  1. 执行块变量检查：$变量$ 误用、变量不存在、变量未使用（赋值了但页面没用到）；
//  2. 模板键核对：{{.键}} 在脚本块结果里不存在时提示——Go 模板遇到缺失键会静默渲染成空；
//  3. 执行块外 Nebula 语法：语句没写进执行块（<?n ... ?> 或 <script type="nebula">）时不会执行，会原样输出到页面。
func WebDicCheck(text string) []dto.BuildWarning {
	blocks := webDicBlocks(text)

	keys := webDicTemplateKeys(blocks)
	templateWarnings, pageKeys := webDicTemplateWarnings(text, blocks, keys)

	var warnings []dto.BuildWarning
	for _, b := range blocks {
		warnings = append(warnings, webDicCheckLines(b.lines, b.lineNums, pageKeys)...)
	}
	warnings = append(warnings, templateWarnings...)
	warnings = append(warnings, webDicOutsideSyntaxWarnings(text, blocks)...)

	return webDicDedupeWarnings(warnings)
}

// WebDicRenderInfo 汇总网页词库的执行块数量与它们提供的模板键。
// 供运行工具给出渲染摘要：模型据此不必通读整页 HTML，就能判断执行块是否真的存在、
// {{.键}} 是否真的能取到值（执行块数为 0 时页面不会执行任何 Nebula 逻辑）。
func WebDicRenderInfo(text string) (blockCount int, keys []string) {
	blocks := webDicBlocks(text)
	set := webDicTemplateKeys(blocks)
	keys = make([]string, 0, len(set))
	for name := range set {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	return len(blocks), keys
}

// webDicBlocks 按原文扫描网页词库里的全部执行块，并按出现顺序返回。
func webDicBlocks(text string) []webDicBlock {
	blocks := webDicScriptBlocks(text)
	blocks = append(blocks, webDicInlineBlocks(text)...)
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].openStart < blocks[j].openStart })
	return blocks
}

// webDicTemplateKeys 收集执行块提供的模板键：有 id 的块提供该 id，无 id 的块提供块内所有被赋值的变量。
func webDicTemplateKeys(blocks []webDicBlock) map[string]bool {
	keys := make(map[string]bool)
	for _, b := range blocks {
		if b.id != "" {
			keys[b.id] = true
			continue
		}
		for _, name := range WebDicAssignedVars(b.lines) {
			keys[name] = true
		}
	}
	return keys
}

// webDicCheckLines 对网页词库的单个执行块（<?n ... ?> 内联块 / <script type="nebula"> 脚本块）做变量类静态检查。
// 只保留与变量有关的告警：$变量$ 误用、变量不存在、变量未使用；
// 不做函数存在性/参数数量/触发词检查——脚本块允许调用 $GET$ / $POST$ / $设置头部$ 等运行时注入的函数。
// lineNums 为该块每行对应的 .wn 原文行号（1-based），长度不足或为 nil 时对应行不带行号。
// pageKeys 为页面 HTML 里被 {{.键}} 引用过的变量名：脚本块赋值、页面取值是 .wn 的正常用法，
// 这些变量不能判成「赋值后未被引用」。
func webDicCheckLines(lines []string, lineNums []int, pageKeys map[string]bool) []dto.BuildWarning {
	stack := newImportStack()

	checkFuncClosedLines(lines, lineNums, stack)

	defined := make(map[string]bool, len(webDicGlobalVars))
	for _, name := range webDicGlobalVars {
		defined[name] = true
	}
	checkUndefinedVarsLines(lines, lineNums, defined, nil, stack)

	checkUnusedAssignmentWith(&dto.BuildValue{Head: lines, HeadLineNums: lineNums}, stack, isWebDicVar, pageKeys)

	return stack.warnings
}

// WebDicAssignedVars 返回脚本块里被赋过值的变量名，供模板键核对使用。
// 与变量检查保持一致：框内容行不是赋值，只有框声明行（文本>/JSON>/循环> 等）里的键算赋值。
func WebDicAssignedVars(lines []string) []string {
	defined := make(map[string]bool)
	var blocks contentBlockTracker
	for _, line := range lines {
		if blocks.step(line) {
			continue
		}
		collectAssignedVars(line, defined)
		collectBlockVars(line, defined)
	}
	out := make([]string, 0, len(defined))
	for name := range defined {
		out = append(out, name)
	}
	return out
}

// isWebDicVar 在 isMagicVar 的基础上补上网页词库运行时注入的全局变量。
func isWebDicVar(name string) bool {
	if isMagicVar(name) {
		return true
	}
	for _, n := range webDicGlobalVars {
		if name == n {
			return true
		}
	}
	return false
}

// webDicScriptBlocks 按原文扫描 <script type="nebula"> 块并记录每行的原文行号。
// 运行时用 html 解析树、只认 head/body 的直接子节点 script；这里按原文顺序扫描，
// 覆盖范围更大（放在嵌套位置或 </html> 之后的块同样会被扫到）。
// type 属性值要求精确等于 nebula，与运行时 isNebulaScript 保持一致。
func webDicScriptBlocks(text string) []webDicBlock {
	var blocks []webDicBlock

	for _, loc := range webDicScriptOpenRe.FindAllStringIndex(text, -1) {
		tag := text[loc[0]:loc[1]]
		if tp, _ := webDicAttrValue(webDicTypeAttrRe, tag); tp != "nebula" {
			continue
		}

		// 结束标签之后的内容留给下一轮扫描，未闭合的块一直取到文末。
		contentStart := loc[1]
		contentEnd, closeEnd := len(text), len(text)
		if rel := webDicScriptCloseRe.FindStringIndex(text[contentStart:]); rel != nil {
			contentEnd = contentStart + rel[0]
			closeEnd = contentStart + rel[1]
		}

		// 换算成逐行文本与原文行号
		rawLines := strings.Split(text[contentStart:contentEnd], "\n")
		nums := make([]int, len(rawLines))
		base := 1 + strings.Count(text[:contentStart], "\n")
		for i := range rawLines {
			nums[i] = base + i
		}

		id, _ := webDicAttrValue(webDicIDAttrRe, tag)
		lines, lineNums := webDicTrimBlockLines(rawLines, nums)
		blocks = append(blocks, webDicBlock{
			id:        id,
			lines:     lines,
			lineNums:  lineNums,
			openStart: loc[0],
			closeEnd:  closeEnd,
		})
	}

	return blocks
}

// webDicInlineBlocks 按原文扫描 <?n ... ?> 内联执行块并记录每行的原文行号。
// 内联块没有 id：块内赋值同样会并入模板数据，页面可以用 {{.键}} 取到。
func webDicInlineBlocks(text string) []webDicBlock {
	var blocks []webDicBlock
	for _, loc := range webDicInlineRe.FindAllStringSubmatchIndex(text, -1) {
		// loc[2]/loc[3] 为 <\?n 之后、?> 之前的正文区间
		rawLines := strings.Split(text[loc[2]:loc[3]], "\n")
		nums := make([]int, len(rawLines))
		base := 1 + strings.Count(text[:loc[2]], "\n")
		for i := range rawLines {
			nums[i] = base + i
		}
		lines, lineNums := webDicTrimBlockLines(rawLines, nums)
		blocks = append(blocks, webDicBlock{
			lines:     lines,
			lineNums:  lineNums,
			openStart: loc[0],
			closeEnd:  loc[1],
		})
	}
	return blocks
}

// webDicAttrValue 从开始标签里取属性值。
// 引号取值可能落在任一捕获组上，全部为空说明匹配到的属性值为空串（如 id=""）。
func webDicAttrValue(re *regexp.Regexp, tag string) (string, bool) {
	m := re.FindStringSubmatch(tag)
	if m == nil {
		return "", false
	}
	for _, v := range m[1:] {
		if v != "" {
			return v, true
		}
	}
	return "", true
}

// webDicTrimBlockLines 按运行时的 strings.TrimSpace(块文本) 语义裁掉首尾空白行，
// 再按 TrimWebScriptIndent 去掉每行行首空白，行号随行一起移动。
func webDicTrimBlockLines(lines []string, nums []int) ([]string, []int) {
	start, end := 0, len(lines)
	for start < end && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	if start == end {
		return nil, nil
	}
	return TrimWebScriptIndent(lines[start:end]), nums[start:end]
}

// TrimWebScriptIndent 去掉网页词库脚本块每行行首的空白，与运行时解析前的处理保持一致
// （解释器识别块开启行/赋值行时不做 TrimLeft，缩进排版后的脚本必须先去掉行首空白）。
// //@关闭缩进 到 //@启用缩进 之间的行首空白有语义，保持原样，与 web 对 .n 头部的处理一致。
func TrimWebScriptIndent(lines []string) []string {
	out := make([]string, len(lines))
	suojin := false
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		switch trimmed {
		case "//@关闭缩进":
			suojin = true
			out[i] = trimmed
			continue
		case "//@启用缩进":
			suojin = false
			out[i] = trimmed
			continue
		}
		if suojin {
			out[i] = line
			continue
		}
		out[i] = trimmed
	}
	return out
}

// webDicTemplateWarnings 核对 HTML 里的 {{.键}}，并返回页面引用到的键名集合。
// 键既不是某个脚本块的 id、也不是无 id 块里赋值过的变量时，Go 模板会把它渲染成空串，属静默失效。
// 只检查脚本块之外的模板动作：脚本块在模板解析前就被移除了，块内的模板语法不会生效。
// 返回的 used 交给变量检查使用：页面既然用 {{.键}} 取了这个变量，脚本块里的赋值就不是死代码。
func webDicTemplateWarnings(text string, blocks []webDicBlock, keys map[string]bool) (warnings []dto.BuildWarning, used map[string]bool) {
	masked := webDicMaskScripts(text, blocks)

	used = make(map[string]bool)
	for _, loc := range webDicActionRe.FindAllStringIndex(masked, -1) {
		action := masked[loc[0]:loc[1]]
		for _, km := range webDicKeyRe.FindAllStringSubmatchIndex(action, -1) {
			name := action[km[2]:km[3]]
			used[name] = true
			if keys[name] {
				continue
			}
			warnings = append(warnings, dto.BuildWarning{
				Line:  1 + strings.Count(masked[:loc[0]+km[2]], "\n"),
				Text:  "模板键不存在：" + name + "，脚本块没有提供该键，页面渲染时会输出为空",
				Level: "warning",
			})
		}
	}
	return warnings, used
}

// webDicOutsideSyntaxWarnings 检查执行块之外出现的 Nebula 语法。
// 执行块之外的 Nebula 语句不会被执行：赋值行 / 框开启行会成为页面上的可见文字，
// $函数$ 调用与 %变量% 取值会原样输出。这正是「文件看着像词库、页面却什么都没渲染」的常见原因
// （例如把 gridContent:文本> ... <文本 直接写在 HTML 里，或漏写 <?n ... ?> / <script type="nebula"> 标记）。
// 只报告形状确定的 Nebula 行，CSS/JS 区域已屏蔽，避免把 `width: 300px;` / `$()` 之类判成词库语句。
func webDicOutsideSyntaxWarnings(text string, blocks []webDicBlock) []dto.BuildWarning {
	masked := webDicMaskScripts(text, blocks)
	masked = webDicMaskRegions(masked, webDicStyleRe, webDicScriptAnyRe)

	var warnings []dto.BuildWarning
	add := func(line int, msg string) {
		warnings = append(warnings, dto.BuildWarning{Line: line, Text: msg, Level: "warning"})
	}
	for i, raw := range strings.Split(masked, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		ln := i + 1
		if _, ok := blockClose(line); ok {
			add(ln, "脚本块外的 Nebula 语法："+line+" 是框的结束标记，但不在 <?n ... ?> 内联块或 <script type=\"nebula\"> 脚本块内，会作为普通文字输出到页面；请把这段语句包进执行块")
			continue
		}
		if _, ok := blockOpen(line); ok {
			add(ln, "脚本块外的 Nebula 语法："+line+" 是词库语句（赋值 / 框开启），但不在 <?n ... ?> 内联块或 <script type=\"nebula\"> 脚本块内，不会执行、会作为普通文字输出到页面；请把这段语句包进执行块")
			continue
		}
		if webDicFuncCallLine(line) {
			add(ln, "脚本块外的 Nebula 语法："+line+" 是函数调用，写在这里不会执行、会原样输出到页面；请把它放进 <?n ... ?> 内联块执行，再用 {{.键}} 把结果渲染到页面")
			continue
		}
		if loc := webDicVarReadRe.FindStringIndex(line); loc != nil {
			add(ln, "脚本块外的 Nebula 语法："+line[loc[0]:loc[1]]+" 不会被替换（模板只认 {{.键}}），会原样输出到页面；变量请在 <?n ... ?> 内联块里赋值，页面用 {{.键}} 取值")
		}
	}
	return warnings
}

// webDicFuncCallLine 判断整行是否为一个 $函数 …$ 调用（形如 $设置头部 Content-Type text/plain$）。
// 要求整行就是一个调用，避免把 JS / 文案里零散的 $ 误判成调用。
func webDicFuncCallLine(line string) bool {
	return len(line) > 2 && strings.HasPrefix(line, "$") && strings.HasSuffix(line, "$") && !strings.ContainsAny(line, "<>")
}

// webDicMaskRegions 按正则把若干区域逐字节替换成空格（保留换行），长度不变。
func webDicMaskRegions(text string, res ...*regexp.Regexp) string {
	buf := []byte(text)
	for _, re := range res {
		for _, loc := range re.FindAllStringIndex(text, -1) {
			for i := loc[0]; i < loc[1] && i < len(buf); i++ {
				if buf[i] != '\n' {
					buf[i] = ' '
				}
			}
		}
	}
	return string(buf)
}

// webDicMaskScripts 把执行块（脚本块 / 内联块）区域逐字节替换成空格（保留换行），长度不变，便于后续按偏移定位行号。
func webDicMaskScripts(text string, blocks []webDicBlock) string {
	if len(blocks) == 0 {
		return text
	}
	buf := []byte(text)
	for _, b := range blocks {
		for i := b.openStart; i < b.closeEnd && i < len(buf); i++ {
			if buf[i] != '\n' {
				buf[i] = ' '
			}
		}
	}
	return string(buf)
}

// webDicDedupeWarnings 去掉同一行同一文案的重复告警，并按行号稳定排序。
func webDicDedupeWarnings(in []dto.BuildWarning) []dto.BuildWarning {
	type wkey struct {
		line int
		text string
	}
	seen := make(map[wkey]bool, len(in))
	out := make([]dto.BuildWarning, 0, len(in))
	for _, w := range in {
		k := wkey{line: w.Line, text: w.Text}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, w)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Line < out[j].Line })
	return out
}
