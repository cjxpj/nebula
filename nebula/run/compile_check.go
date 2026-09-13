package run

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/cjxpj/nebula/build"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// runCompileChecks 对编译产物执行编译期静态检查，发现的警告统一追加到引入链的警告切片。
func runCompileChecks(v *dto.BuildValue, stack *importStack) {
	checkBlockPairs(v, stack)
	checkTriggerRegex(v, stack)
	checkFuncClosed(v, stack)
	checkFuncParams(v, stack)
	checkFuncNameConflict(v, stack)
	checkUndefinedVars(v, stack)
	checkUnusedAssignment(v, stack)
}

// allBuildDics 收集编译产物中全部词条（正文、全局函数、类内函数），按指针去重，
// 避免「变量:#引入=」形式下同一批函数同时出现在全局函数表与类函数表中被重复检查。
func allBuildDics(v *dto.BuildValue) []*dto.BuildDic {
	seen := make(map[*dto.BuildDic]bool)
	var out []*dto.BuildDic
	add := func(list []*dto.BuildDic) {
		for _, e := range list {
			if e == nil || seen[e] {
				continue
			}
			seen[e] = true
			out = append(out, e)
		}
	}
	add(v.Dic)
	for _, list := range v.DicFuncs {
		add(list)
	}
	for _, c := range v.Class {
		if c == nil {
			continue
		}
		for _, list := range c.DicFuncs {
			add(list)
		}
	}
	return out
}

// ============ 框配对/嵌套检查 ============

// blockKind 框类型，与 vm.go 的 State* 一一对应。
type blockKind uint8

const (
	blkFunc    blockKind = iota // 函数> ... <函数
	blkIf                       // 如果> ... <如果
	blkMatch                    // 匹配> ... <匹配
	blkFor                      // 循环> ... <循环
	blkForEach                  // 遍历> ... <遍历
	blkText                     // 文本>/纯文本> ... <文本
	blkJson                     // JSON> ... <JSON
	blkNewJson                  // JSON>{ / JSON>[ ... 平衡括号
)

// blockFrame 栈中的一帧。
type blockFrame struct {
	kind  blockKind
	line  int // 开启行号（1-based）
	depth int // 仅新建 JSON 框使用：括号平衡深度
}

func blockKindOpen(k blockKind) string {
	switch k {
	case blkFunc:
		return "函数框"
	case blkIf:
		return "如果>"
	case blkMatch:
		return "匹配>"
	case blkFor:
		return "循环>"
	case blkForEach:
		return "遍历>"
	case blkText:
		return "文本>"
	case blkJson:
		return "JSON>"
	case blkNewJson:
		return "JSON>{/["
	}
	return "?"
}

// blockOpen 识别「框开启」行，返回框类型与是否开启；语义与 vm.go 的 classifyLine/op 分发一致。
func blockOpen(line string) (blockKind, bool) {
	switch {
	case len(line) > 7 && strings.HasPrefix(line, "如果>"):
		return blkIf, true
	case len(line) > 7 && strings.HasPrefix(line, "匹配>"):
		return blkMatch, true
	case strings.HasPrefix(line, "判断循环>") && len(line) > len("判断循环>"):
		return blkFor, true
	case strings.HasPrefix(line, "循环>"):
		return blkFor, true
	case strings.HasPrefix(line, "遍历>"):
		return blkForEach, true
	case strings.HasPrefix(line, "纯文本>"):
		return blkText, true
	case strings.HasPrefix(line, "文本>"):
		return blkText, true
	case line == "JSON>[":
		return blkNewJson, true
	case line == "JSON>{":
		return blkNewJson, true
	case strings.HasPrefix(line, "JSON>"):
		return blkJson, true
	case strings.HasPrefix(line, "#:执行函数>"):
		return blkFunc, true
	case strings.HasPrefix(line, "执行函数>"):
		return blkFunc, true
	}
	// 变量:函数> / 变量:执行函数> 开头的函数框（赋予值形式）。
	if vt, vp, vs := build.ValTextTest(line); vt == 6 && vp != "" {
		if strings.HasPrefix(vs, "函数>") || strings.HasPrefix(vs, "执行函数>") {
			return blkFunc, true
		}
	}
	return 0, false
}

// blockClose 识别「框关闭」标记行。
func blockClose(line string) (blockKind, bool) {
	switch line {
	case "<函数":
		return blkFunc, true
	case "<如果":
		return blkIf, true
	case "<匹配":
		return blkMatch, true
	case "<循环":
		return blkFor, true
	case "<遍历":
		return blkForEach, true
	case "<文本":
		return blkText, true
	case "<JSON":
		return blkJson, true
	}
	return 0, false
}

func checkBlockPairs(v *dto.BuildValue, stack *importStack) {
	// 头部虽无触发词，但同样会经 DicRunLine 线性执行，框结构缺失闭合应在编译期报错，
	// 与正文词条保持一致的检查口径。
	checkBlockPairsLines(v.Head, v.HeadLineNums, stack)
	for _, e := range allBuildDics(v) {
		checkBlockPairsLines(e.Text, e.LineNums, stack)
	}
}

func checkBlockPairsLines(lines []string, lineNums []int, stack *importStack) {
	if len(lines) == 0 {
		return
	}

	frames := make([]blockFrame, 0, 4)
	for i, line := range lines {
		ln := 0
		if i < len(lineNums) {
			ln = lineNums[i]
		}

		// 叶子框（文本/JSON/新建JSON）只识别各自的关闭，内部不再识别其他框。
		if len(frames) > 0 {
			top := &frames[len(frames)-1]
			switch top.kind {
			case blkText:
				if line == "<文本" {
					frames = frames[:len(frames)-1]
				}
				continue
			case blkJson:
				if line == "<JSON" {
					frames = frames[:len(frames)-1]
				}
				continue
			case blkNewJson:
				if strings.HasSuffix(line, "{") || strings.HasSuffix(line, "[") {
					top.depth++
				}
				if line == "}" || line == "]" || line == "}," || line == "]," {
					top.depth--
					if top.depth == 0 && (line == "}" || line == "]") {
						frames = frames[:len(frames)-1]
					}
				}
				continue
			}
		}

		if k, ok := blockClose(line); ok {
			if len(frames) == 0 {
				stack.addError(ln, "框配对错误：多余的关闭标记 "+line)
				continue
			}
			top := frames[len(frames)-1]
			if top.kind != k {
				stack.addError(ln, "框配对错误："+line+" 无法关闭 "+blockKindOpen(top.kind)+" 开启的框")
				frames = frames[:len(frames)-1]
				continue
			}
			frames = frames[:len(frames)-1]
			continue
		}

		if k, ok := blockOpen(line); ok {
			frames = append(frames, blockFrame{kind: k, line: ln, depth: 1})
			continue
		}
	}

	for _, f := range frames {
		stack.addError(f.line, "框配对错误："+blockKindOpen(f.kind)+" 开启的框未闭合")
	}
}

// ============ 触发词正则语法检查 ============

func checkTriggerRegex(v *dto.BuildValue, stack *importStack) {
	for _, e := range v.Dic {
		t := e.Trigger
		if isPlainTrigger(t) {
			continue
		}
		if _, err := regexp.Compile("^(" + t + ")$"); err != nil {
			stack.addError(e.TriggerLine, "触发词正则语法错误："+err.Error())
		}
	}
}

// ============ 函数 $ 闭合检查 ============

// funcSkipFrame 记录「内容行不做 $函数$ 插值」的叶子框，用于跳过其内部行。
type funcSkipFrame struct {
	byMark bool   // true：按关闭标记行闭合；false：按括号平衡闭合
	mark   string // 关闭标记行
	depth  int    // 括号平衡深度
}

// funcSkipOpen 识别「内容行不做 $函数$ 插值」的框开启行：
// 文本>/纯文本>/JSON>/JSON>{/[ /变量:{/[ /变量:"""/”' /--js。
// 这些框的内容原样（或仅做 %变量% 插值）输出，$ 不参与函数解析，故其中的 $ 不应报未闭合。
func funcSkipOpen(line string) (funcSkipFrame, bool) {
	switch {
	case strings.HasPrefix(line, "纯文本>"):
		return funcSkipFrame{byMark: true, mark: "<文本"}, true
	case strings.HasPrefix(line, "文本>"):
		return funcSkipFrame{byMark: true, mark: "<文本"}, true
	case line == "JSON>[" || line == "JSON>{":
		return funcSkipFrame{depth: 1}, true
	case strings.HasPrefix(line, "JSON>"):
		return funcSkipFrame{byMark: true, mark: "<JSON"}, true
	case line == "--js":
		return funcSkipFrame{byMark: true, mark: "--end"}, true
	}
	if vt, vp, vs := build.ValTextTest(line); vt == 6 {
		switch {
		case vs == `"""`:
			return funcSkipFrame{byMark: true, mark: `"""`}, true
		case vs == `'''`:
			return funcSkipFrame{byMark: true, mark: `'''`}, true
		case (vs == "{" || vs == "[") && !strings.Contains(vp, "->"):
			// 变量:{ / 变量:[ 多行 JSON 赋值框（含 -> 路径的是单行 JSON 赋值，非框）。
			return funcSkipFrame{depth: 1}, true
		}
	}
	return funcSkipFrame{}, false
}

// checkFuncClosed 静态检查「$函数$」是否缺少结尾 $。
// 缺结尾 $ 时 parseFuncSegments 会把该行剩余部分整段按字面量输出：函数不执行，也没有任何提示；
// 这里在编译期补一条警告，定位这种静默失效。
func checkFuncClosed(v *dto.BuildValue, stack *importStack) {
	checkFuncClosedLines(v.Head, v.HeadLineNums, stack)
	for _, e := range allBuildDics(v) {
		checkFuncClosedLines(e.Text, e.LineNums, stack)
	}
}

func checkFuncClosedLines(lines []string, lineNums []int, stack *importStack) {
	var frames []funcSkipFrame
	for i, line := range lines {
		ln := 0
		if i < len(lineNums) {
			ln = lineNums[i]
		}

		// 原样框内：内容不做 $ 解析，只识别各自的关闭（叶子框语义）。
		if n := len(frames); n > 0 {
			top := &frames[n-1]
			if top.byMark {
				if line == top.mark {
					frames = frames[:n-1]
				}
			} else {
				if strings.HasSuffix(line, "{") || strings.HasSuffix(line, "[") {
					top.depth++
				}
				if line == "}" || line == "]" || line == "}," || line == "]," {
					top.depth--
					if top.depth == 0 && (line == "}" || line == "]") {
						frames = frames[:n-1]
					}
				}
			}
			continue
		}

		if f, ok := funcSkipOpen(line); ok {
			frames = append(frames, f)
			continue
		}

		issue := assignKeyIssue(line)
		if issue == "" && findUnclosedFuncOpen(assignOpValue(line)) >= 0 {
			stack.addWarning(ln, "函数未闭合：$ 缺少配对的结尾 $，该行会按原样输出且函数不会执行")
			continue
		}
		if issue != "" {
			stack.addWarning(ln, issue)
		}
	}
}

// assignKeyIssue 判断一行是否为「疑似赋值但键名不符合变量命名规范」，返回告警文案；空串表示不是。
// 依据 dic.md 变量命名规范：键名只能是中英文/数字/下划线，且长度不超过 32 字节（UTF-8）。
// 这类行既不被识别为赋值也不是函数调用，编译后按原样输出、变量不会被赋值，属于静默失效。
func assignKeyIssue(line string) string {
	if marker := headVarMarker(line); marker != "" {
		return "赋予值键名不能包含特殊字符 " + marker + "，该行会按原样输出、变量不会被赋值"
	}
	key, rest := build.ValTextKeyScan(line)
	if len(key) > 32 && isAssignOpPrefix(rest) {
		return "变量名过长：键名最多 32 字节（UTF-8），该行会按原样输出、变量不会被赋值"
	}
	return ""
}

// headVarMarker 判断行首是否多写了变量标记，命中返回该标记（$ 或 %）：
// $名:值、%名:值、%名%:值（如 $a:a、%随机数%:abc）。
func headVarMarker(line string) string {
	if strings.HasPrefix(line, "$") {
		if vt, _, _ := build.ValTextTest(line[1:]); vt != 0 {
			return "$"
		}
	}
	if strings.HasPrefix(line, "%") {
		rest := line[1:]
		if vt, _, _ := build.ValTextTest(rest); vt != 0 {
			return "%"
		}
		if j := strings.IndexByte(rest, '%'); j > 0 {
			if vt, _, _ := build.ValTextTest(rest[j+1:]); vt != 0 {
				return "%"
			}
		}
	}
	return ""
}

// isAssignOpPrefix 判断 rest 是否以赋值操作符开头：: / :: / :$: / :%: / -: / +: / *: / /:。
func isAssignOpPrefix(rest string) bool {
	if rest == "" {
		return false
	}
	if rest[0] == ':' {
		return true
	}
	if len(rest) >= 2 && rest[1] == ':' {
		switch rest[0] {
		case '-', '+', '*', '/':
			return true
		}
	}
	return false
}

// ============ 函数参数数量检查 ============

func checkFuncParams(v *dto.BuildValue, stack *importStack) {
	funcIndex := v.GetFuncIndex()
	for _, e := range allBuildDics(v) {
		checkFuncParamsEntry(e, v, funcIndex, stack)
	}
}

func checkFuncParamsEntry(e *dto.BuildDic, v *dto.BuildValue, funcIndex map[string][]*dto.BuildDic, stack *importStack) {
	for i, line := range e.Text {
		line = assignOpValue(line)
		if !strings.Contains(line, "$") {
			continue
		}
		ln := 0
		if i < len(e.LineNums) {
			ln = e.LineNums[i]
		}
		for _, seg := range parseFuncSegments(line) {
			if !seg.isFunc || len(seg.args) == 0 {
				continue
			}
			checkFuncCall(seg.args[0], len(seg.args)-1, ln, v, funcIndex, stack)
		}
	}
}

func checkFuncCall(name string, argCount, line int, v *dto.BuildValue, funcIndex map[string][]*dto.BuildDic, stack *importStack) {
	// $!函数名 参数$：捕获报错调用，! 为前缀，需去掉后再解析函数名。
	name = strings.TrimPrefix(name, "!")
	if name == "" || name == "new" {
		return
	}
	// %变量% 函数框引用、$.类名 / %类名 类方法调用、$变量.方法$ 实例方法调用：
	// 这些由运行时动态解析，编译期不做静态参数检查。
	if len(name) > 2 && name[0] == '%' && name[len(name)-1] == '%' {
		return
	}
	if name[0] == '.' || name[0] == '%' || strings.Contains(name, ".") {
		return
	}

	// 局部函数 [函数|规则]
	if items := funcIndex[name]; len(items) > 0 {
		rule := items[0].ParamRule
		if rule == "" {
			rule = "0"
		}
		if !utils.MatchLenRule(argCount, rule) {
			stack.addError(line, fmt.Sprintf("函数参数数量错误：$%s$ 需要 %s 个参数，实际 %d 个", name, rule, argCount))
		}
		return
	}

	// 自定义函数（含 bot 注入）
	if fn, ok := v.MyFunc[name]; ok {
		if !utils.MatchLenRule(argCount, fn.L) {
			stack.addError(line, fmt.Sprintf("函数参数数量错误：$%s$ 需要 %s 个参数，实际 %d 个", name, fn.L, argCount))
		}
		return
	}

	// 内置注册函数
	if rule, ok := dto.GetFuncRule(name); ok {
		if !utils.MatchLenRule(argCount, rule) {
			stack.addError(line, fmt.Sprintf("函数参数数量错误：$%s$ 需要 %s 个参数，实际 %d 个", name, rule, argCount))
		}
		return
	}

	// 未定义的函数：非局部函数、非自定义函数、非内置函数，运行时将原样返回 $...$ 文本。
	stack.addWarning(line, "函数不存在："+name)
}

// ============ 函数名与内置函数冲突检查 ============

// checkFuncNameConflict 检查全局 [函数] 定义是否与系统内置函数重名。
// 内置函数为只读，词库内 [函数] 定义不得覆盖；冲突时编译警告 + 运行时调用报错。
func checkFuncNameConflict(v *dto.BuildValue, stack *importStack) {
	for _, e := range v.DicFuncs["函数"] {
		name := e.Trigger
		if i := strings.LastIndex(name, "->"); i != -1 {
			name = name[:i]
		}
		if _, ok := dto.GetFuncRule(name); ok {
			stack.addError(e.TriggerLine, fmt.Sprintf("禁止覆盖系统内置函数：%s", name))
		}
	}
}

// ============ 变量引用存在性检查 ============

// checkUndefinedVars 静态检查变量引用：%变量% 引用了「到当前行为止仍未赋值」、且非魔术/消息变量时给出警告。
// 按词条正文顺序分析：赋值行/框声明即时生效，函数传出变量仅在 $函数$ 调用写回后才视为已定义。
func checkUndefinedVars(v *dto.BuildValue, stack *importStack) {
	funcOutVars := collectFuncOutVars(v)

	// 头部初始化语句：顺序检查并累积赋值，结果作为所有词条正文的初始已定义变量。
	headDefined := make(map[string]bool)
	// 编译期资源变量（//@资源 / //@一次性资源）在运行时注入局部变量表，视为已定义，避免误报「变量不存在」。
	for name := range v.Resources {
		headDefined[name] = true
	}
	checkUndefinedVarsLines(v.Head, v.HeadLineNums, headDefined, funcOutVars, stack)

	for _, e := range allBuildDics(v) {
		checkUndefinedVarsEntry(e, headDefined, funcOutVars, stack)
	}
}

// checkUndefinedVarsLines 顺序检查行序列中的变量引用并累积赋值变量到 defined。
// 原样文本框（纯文本>/变量:”'）内的内容不做 %变量% 插值，跳过引用检查与赋值收集。
func checkUndefinedVarsLines(lines []string, lineNums []int, defined map[string]bool, funcOutVars map[string]map[string]bool, stack *importStack) {
	var rawClosers []string
	for i, line := range lines {
		ln := 0
		if i < len(lineNums) {
			ln = lineNums[i]
		}
		// 关闭原样文本框
		if n := len(rawClosers); n > 0 && line == rawClosers[n-1] {
			rawClosers = rawClosers[:n-1]
			continue
		}
		closer, isRawOpen := rawTextCloser(line)
		// 原样文本框内容行：原样输出，跳过引用检查与赋值收集
		if !isRawOpen && len(rawClosers) > 0 {
			continue
		}
		checkUndefinedVarsLine(line, ln, defined, stack)
		collectAssignedVars(line, defined)
		collectBlockVars(line, defined)
		collectFuncOutVarsFromLine(line, funcOutVars, defined)
		if isRawOpen {
			rawClosers = append(rawClosers, closer)
		}
	}
}

// rawTextCloser 判断行是否为「原样文本」框的开启行（内容不做 %变量% 插值），
// 返回其关闭标记；非开启行返回 ok=false。
//   - 纯文本> ... <文本：内容原样输出
//   - 变量:”' ... ”'：内容原样赋值
func rawTextCloser(line string) (closer string, ok bool) {
	if strings.HasPrefix(line, "纯文本>") {
		return "<文本", true
	}
	if vt, _, vs := build.ValTextTest(line); vt == 6 && vs == `'''` {
		return `'''`, true
	}
	return "", false
}

// checkUndefinedVarsEntry 顺序检查单个词条正文里的变量引用：
// 每行先按「当前已定义变量」检查引用，再把本行的赋值/框声明/函数调用写回加入已定义集合。
func checkUndefinedVarsEntry(e *dto.BuildDic, headDefined map[string]bool, funcOutVars map[string]map[string]bool, stack *importStack) {
	if e == nil {
		return
	}
	defined := make(map[string]bool, len(headDefined)+4)
	for k := range headDefined {
		defined[k] = true
	}
	checkUndefinedVarsLines(e.Text, e.LineNums, defined, funcOutVars, stack)
}

// checkUndefinedVarsLine 检查单行里的变量引用；同一行内重复引用去重。
func checkUndefinedVarsLine(line string, ln int, defined map[string]bool, stack *importStack) {
	line = assignOpValue(line)
	if !strings.Contains(line, "%") {
		return
	}
	// $%名字%...$ 的首个 token 是函数框引用（调用变量里存的函数），而非变量读取，跳过变量检查。
	funcBoxNames := make(map[string]bool)
	if strings.Contains(line, "$") {
		for _, seg := range parseFuncSegments(line) {
			if seg.isFunc && len(seg.args) > 0 {
				name := seg.args[0]
				if len(name) > 2 && name[0] == '%' && name[len(name)-1] == '%' {
					funcBoxNames[name[1:len(name)-1]] = true
				}
			}
		}
	}

	seen := make(map[string]bool)
	for _, name := range extractVarRefs(line) {
		if name == "" || seen[name] || funcBoxNames[name] {
			continue
		}
		seen[name] = true
		if defined[name] || isMagicVar(name) {
			continue
		}
		stack.addWarning(ln, "变量不存在："+name)
	}
}

// extractVarRefs 提取字符串中 %...% 之间的变量名（静态切分，与运行时 % 切分语义一致）。
func extractVarRefs(s string) []string {
	if !strings.Contains(s, "%") {
		return nil
	}
	var names []string
	start := 0
	for {
		open := strings.Index(s[start:], "%")
		if open == -1 {
			break
		}
		open += start
		close := strings.Index(s[open+1:], "%")
		if close == -1 {
			break
		}
		close += open + 1
		names = append(names, s[open+1:close])
		start = close + 1
	}
	return names
}

// assignOpValue 返回赋予值行中需参与函数/变量静态检查的「值」部分。
// 执行函数(:$:)与只读取变量(:%:)的操作符内含有相邻的 $/% 与 :，整行按 $/% 切分会被误判为
// 空函数「$:$」或空变量「%:%」，故返回操作符后的值；其余行返回原行。
func assignOpValue(line string) string {
	vt, _, vs := build.ValTextTest(line)
	if vt == 3 || vt == 4 {
		return vs
	}
	return line
}

// collectFuncOutVars 建立「函数名 → 传出变量集合」映射，供 $函数$ 调用写回分析使用。
func collectFuncOutVars(v *dto.BuildValue) map[string]map[string]bool {
	out := make(map[string]map[string]bool)
	for _, e := range v.DicFuncs["函数"] {
		if e == nil {
			continue
		}
		t := e.Trigger
		i := strings.LastIndex(t, "->")
		if i == -1 {
			continue
		}
		name := strings.TrimSpace(t[:i])
		if name == "" {
			continue
		}
		if out[name] == nil {
			out[name] = make(map[string]bool)
		}
		for _, vname := range strings.Split(t[i+2:], ",") {
			vname = strings.TrimSpace(vname)
			if vname != "" {
				out[name][vname] = true
			}
		}
	}
	return out
}

// collectFuncOutVarsFromLine 识别一行中的 $函数$ 调用，把对应函数的传出变量加入 defined。
func collectFuncOutVarsFromLine(line string, funcOutVars map[string]map[string]bool, defined map[string]bool) {
	line = assignOpValue(line)
	if !strings.Contains(line, "$") || len(funcOutVars) == 0 {
		return
	}
	for _, seg := range parseFuncSegments(line) {
		if !seg.isFunc || len(seg.args) == 0 {
			continue
		}
		name := seg.args[0]
		name = strings.TrimPrefix(name, "!")
		if name == "" || name == "new" {
			continue
		}
		// 动态调用（类方法、%变量% 函数框、实例方法）无法静态确定传出变量，跳过。
		if name[0] == '.' || name[0] == '%' || strings.Contains(name, ".") {
			continue
		}
		for vname := range funcOutVars[name] {
			defined[vname] = true
		}
	}
}

// collectAssignedVars 收集赋值行（变量:值、变量:$函数$、变量:%变量%、变量:、变量+/-/*// 等）的变量名。
func collectAssignedVars(line string, defined map[string]bool) {
	vt, vp, _ := build.ValTextTest(line)
	if vt != 0 && vp != "" {
		defined[vp] = true
	}
}

// collectBlockVars 收集块开启行声明的变量：
// 循环>变量、遍历>k,v、文本>/纯文本>变量=...（赋值目标）。
// 函数框（变量:函数>）由 collectAssignedVars 以赋值形式收集变量名。
func collectBlockVars(line string, defined map[string]bool) {
	var rest string
	isTextBlock := false
	switch {
	case strings.HasPrefix(line, "纯文本>"):
		rest = line[len("纯文本>"):]
		isTextBlock = true
	case strings.HasPrefix(line, "文本>"):
		rest = line[len("文本>"):]
		isTextBlock = true
	case strings.HasPrefix(line, "循环>"):
		rest = line[len("循环>"):]
	case strings.HasPrefix(line, "遍历>"):
		rest = line[len("遍历>"):]
	case strings.HasPrefix(line, "JSON>"):
		rest = line[len("JSON>"):]
		i := strings.IndexByte(rest, '=')
		if i < 0 {
			// 无 =：JSON>{/[ 新建、JSON>[]、JSON> 直接输出，均不声明变量。
			return
		}
		rest = rest[:i]
	default:
		return
	}

	if i := strings.IndexByte(rest, '='); i >= 0 {
		rest = rest[:i]
	} else if isTextBlock {
		// 文本>/纯文本> 无 = 时是直接输出，不声明变量。
		return
	}

	for _, name := range strings.Split(rest, ",") {
		name = strings.TrimSpace(name)
		if name == "" || strings.ContainsAny(name, " %$[]<>.") {
			continue
		}
		defined[name] = true
	}
}

// ============ 赋值后变量未被使用检查 ============

// unusedAssign 记录一条赋值行（目标变量名、行号与原始行文本）。
type unusedAssign struct {
	name string
	line int
	text string
}

// checkUnusedAssignment 静态检查赋值行：目标变量在整个编译产物中都未被引用时给出警告。
// 正文行一旦以「名字+半角冒号」开头就会被解析成赋予值（见 dic.md「赋予值」），该行不输出；
// 若本意是输出带冒号的文本，这是一种静默失效，提示时一并给出转义冒号的修正写法。
func checkUnusedAssignment(v *dto.BuildValue, stack *importStack) {
	used := make(map[string]bool)          // 全产物中被引用过的变量名
	first := make(map[string]unusedAssign) // 变量 -> 首次赋值
	var order []string                     // 赋值出现顺序，保证告警顺序稳定

	scan := func(lines []string, lineNums []int) {
		var rawClosers []string
		for i, line := range lines {
			ln := 0
			if i < len(lineNums) {
				ln = lineNums[i]
			}
			// 关闭原样文本框
			if n := len(rawClosers); n > 0 && line == rawClosers[n-1] {
				rawClosers = rawClosers[:n-1]
				continue
			}
			closer, isRawOpen := rawTextCloser(line)
			// 原样文本框内容行：原样输出，%变量% 不做插值，既不算引用也不是赋值。
			if !isRawOpen && len(rawClosers) > 0 {
				continue
			}
			if !strings.HasPrefix(line, "//") {
				vt, vp, _ := build.ValTextTest(line)
				isAssign := vt != 0 && vp != ""
				// 赋值行右侧读取自身时不算该变量的「引用」：若再无其他用途即视为未被使用。
				for _, name := range lineVarRefs(line) {
					if isAssign && name == vp {
						continue
					}
					used[name] = true
				}
				if isAssign && !isMagicVar(vp) {
					if _, ok := first[vp]; !ok {
						first[vp] = unusedAssign{name: vp, line: ln, text: line}
						order = append(order, vp)
					}
				}
			}
			if isRawOpen {
				rawClosers = append(rawClosers, closer)
			}
		}
	}

	scan(v.Head, v.HeadLineNums)
	for _, e := range allBuildDics(v) {
		if e == nil {
			continue
		}
		scan(e.Text, e.LineNums)
	}

	// 函数传出变量（[函数]触发词的 name->变量）由运行时写回，隐式被使用。
	outVars := make(map[string]bool)
	for _, m := range collectFuncOutVars(v) {
		for name := range m {
			outVars[name] = true
		}
	}

	for _, name := range order {
		if used[name] || outVars[name] {
			continue
		}
		a := first[name]
		stack.addWarning(a.line, "变量未使用："+name+" 赋值后未被任何地方引用；若此处是要输出文本，应写成 "+escapeAssignColon(a.text, name))
	}
}

// lineVarRefs 提取一行中被引用的变量名：
//   - %变量% 形式（含 %实例.成员% 按 . 拆分，实例变量本身也算被引用）
//   - $实例.方法$ 形式的实例方法调用（点号前的实例变量名）
func lineVarRefs(line string) []string {
	var names []string
	add := func(name string) {
		if name == "" {
			return
		}
		if strings.Contains(name, ".") {
			for _, part := range strings.Split(name, ".") {
				if part != "" {
					names = append(names, part)
				}
			}
			return
		}
		names = append(names, name)
	}
	// 执行函数(:$:)/只读取变量(:%:)操作符内含相邻的 $/% 与 :，需取操作符后的值再切分。
	val := assignOpValue(line)
	for _, name := range extractVarRefs(val) {
		add(name)
	}
	if strings.Contains(val, "$") {
		for _, seg := range parseFuncSegments(val) {
			if !seg.isFunc || len(seg.args) == 0 {
				continue
			}
			name := strings.TrimPrefix(seg.args[0], "!")
			if !strings.Contains(name, ".") {
				continue
			}
			for _, part := range strings.Split(name, ".") {
				add(part)
			}
		}
	}
	return names
}

// escapeAssignColon 在赋值行的操作符冒号前插入转义反斜杠，得到「按原样文本输出」的写法。
func escapeAssignColon(line, key string) string {
	if key == "" || !strings.HasPrefix(line, key) {
		return line
	}
	rest := line[len(key):]
	if strings.HasPrefix(rest, ":") {
		return key + `\:` + rest[1:]
	}
	if len(rest) >= 2 && rest[1] == ':' {
		return key + rest[:1] + `\:` + rest[2:]
	}
	return line
}

// isParamVar 判断变量名是否为 参数N / 括号N 形式的运行时参数变量。
func isParamVar(name string) bool {
	for _, prefix := range [...]string{"参数", "括号"} {
		if after, ok := strings.CutPrefix(name, prefix); ok && after != "" {
			if _, err := strconv.Atoi(after); err == nil {
				return true
			}
		}
	}
	return false
}

// isMagicVar 判断变量名是否为运行时自动注入或内置的「魔术变量」，这些引用无需预先赋值。
func isMagicVar(name string) bool {
	if name == "" {
		return false
	}
	switch name {
	case "时间", "时间戳", "毫秒时间戳", "微秒时间戳", "纳秒时间戳", "空格", "换行", "系统", "版本",
		"val0", "val1", "val2", "val3", "val4", "val5", "val6", "val7", "val8", "val9", "val10",
		// 运行时上下文/系统变量
		"触发词", "触发", "报错", "Class", "自己", "类型",
		// 各机器人注入的消息变量
		"来源", "昵称", "群号", "群名", "QQ", "qq", "uid", "主人", "MsgId", "MessageID",
		"Op", "robot", "Robot", "data", "GolineMode", "文件数据", "撤回消息", "AT0",
		"子群号", "管理", "头像", "robot_appid":
		return true
	}
	// 系统魔术变量：_xxx_
	if strings.HasPrefix(name, "_") {
		return true
	}
	if isParamVar(name) {
		return true
	}
	// 编码/类型/JSON路径/取反前缀
	for _, p := range [...]string{"URL编码@", "B64编码@", "URL@", "B64@", "TYPE@", "@", "!"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	// 时间格式化（如 时间yyyy-MM-dd）与随机数（如 随机数1-100）
	if strings.HasPrefix(name, "时间") || strings.HasPrefix(name, "随机数") {
		return true
	}
	return false
}
