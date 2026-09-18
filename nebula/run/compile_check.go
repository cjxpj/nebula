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
// 避免「变量:$引入」形式下同一批函数同时出现在全局函数表与类函数表中被重复检查。
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
	// 变量:函数> / 变量:执行函数> 开头的函数框（赋予值形式）；
	// 变量:文本> / 变量:纯文本> 开头的赋值文本框（赋予值形式）。
	if vt, vp, vs := build.ValTextTest(line); vt == 6 && vp != "" {
		switch {
		case strings.HasPrefix(vs, "函数>"), strings.HasPrefix(vs, "执行函数>"):
			return blkFunc, true
		case strings.HasPrefix(vs, "文本>"), strings.HasPrefix(vs, "纯文本>"):
			return blkText, true
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
	// 头部已作为首个词块并入 Dic，随 allBuildDics 统一检查，与正文词条口径一致。
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
		case vp != "" && (strings.HasPrefix(vs, "文本>") || strings.HasPrefix(vs, "纯文本>")):
			// 变量:文本> / 变量:纯文本> 赋值文本框，内容不做 $函数$ 插值。
			return funcSkipFrame{byMark: true, mark: "<文本"}, true
		}
	}
	return funcSkipFrame{}, false
}

// checkFuncClosed 静态检查「$函数$」是否缺少结尾 $。
// 缺结尾 $ 时 parseFuncSegments 会把该行剩余部分整段按字面量输出：函数不执行，也没有任何提示；
// 这里在编译期补一条警告，定位这种静默失效。
func checkFuncClosed(v *dto.BuildValue, stack *importStack) {
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
// 依据词库变量命名规范：键名只能是中英文/数字/下划线，且长度不超过 32 字节（UTF-8）。
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
// 保留的生命周期钩子（_初始化/_中间件）不属于可调用函数，跳过检查。
func checkFuncNameConflict(v *dto.BuildValue, stack *importStack) {
	for _, e := range v.DicFuncs["函数"] {
		if dto.IsReservedTrigger(e.Trigger) {
			continue
		}
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

	// 生命周期钩子（[f]_初始化 / [f]_中间件，其中 _中间件 已含编译期并入的头部内容）
	// 在正文前执行（初始化只首次、中间件每次），其赋值对正文可见；先检查并累积，再逐个检查正文词条。
	headDefined := make(map[string]bool)
	// 编译期资源变量（//@资源 / //@一次性资源）在运行时注入局部变量表，视为已定义，避免误报「变量不存在」。
	for name := range v.Resources {
		headDefined[name] = true
	}
	for _, e := range v.DicFuncs["函数"] {
		if e == nil || !dto.IsReservedTrigger(e.Trigger) {
			continue
		}
		checkUndefinedVarsLines(e.Text, e.LineNums, headDefined, funcOutVars, stack)
	}

	for _, e := range allBuildDics(v) {
		if e == nil || e.Trigger == dto.HeaderTrigger || dto.IsReservedTrigger(e.Trigger) {
			continue
		}
		checkUndefinedVarsEntry(e, headDefined, funcOutVars, stack)
	}
}

// checkUndefinedVarsLines 顺序检查行序列中的变量引用并累积赋值变量到 defined。
// 框内容行（文本/JSON/JS/链式/三引号等）不参与赋值收集：它们不是变量赋值；
// 其中做 %变量% 插值的框（文本>、变量:"""、变量:>>>、JSON>）仍按引用检查变量是否存在。
func checkUndefinedVarsLines(lines []string, lineNums []int, defined map[string]bool, funcOutVars map[string]map[string]bool, stack *importStack) {
	var blocks contentBlockTracker
	for i, line := range lines {
		ln := 0
		if i < len(lineNums) {
			ln = lineNums[i]
		}
		if blocks.step(line) {
			seen := make(map[string]bool)
			for _, name := range blocks.refs(line) {
				if name == "" || seen[name] || defined[name] || isMagicVar(name) {
					continue
				}
				seen[name] = true
				stack.addWarning(ln, "变量不存在："+name)
			}
			continue
		}
		checkUndefinedVarsLine(line, ln, defined, stack)
		collectAssignedVars(line, defined)
		collectBlockVars(line, defined)
		collectFuncOutVarsFromLine(line, funcOutVars, defined)
	}
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
// 空函数「$:$」或空变量「%:%」，故返回操作符后的值；
// 纯文本赋值(::)的值原样写入、不执行 $函数$ 与 %变量%，返回空串表示该值不参与检查；
// 其余行返回原行。
func assignOpValue(line string) string {
	vt, _, vs := build.ValTextTest(line)
	switch vt {
	case 3, 4:
		return vs
	case 5:
		return ""
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

// contentFrame 记录「内容行不是变量赋值」的框的闭合方式与取值规则。
type contentFrame struct {
	byMark bool   // true：按关闭标记行闭合；false：按括号平衡闭合
	mark   string // 关闭标记行
	depth  int    // 括号平衡深度
	interp bool   // 内容行做 %变量% 插值，其中的 %变量% 是变量引用
	json   bool   // JSON 键值框：键名不是变量名，值按 NewJson 前缀语法读取变量
}

// contentBlockTracker 跟踪「内容行不是变量赋值」的框（文本/JSON/JS/链式/三引号等）的嵌套状态。
// 这类框的内容行会被 ValTextTest 误认为「变量名 + 冒号」的赋值，静态检查需跳过：
//   - 文本>/纯文本>/三引号文本块（变量: 加三个引号）：内容按文本输出或赋值（行内形如 `a: 文本` 是文本）；
//   - --js ... --end：内容是 JS 脚本；
//   - JSON>{/[、JSON> ... <JSON、变量:{/[：内容是 JSON 键值（key=value / key:=value）；
//   - 变量:>>>/#:>>> ... <<<：内容是写回链式变量的取值表达式。
type contentBlockTracker struct {
	frames []contentFrame
}

// step 更新框状态，返回该行是否属于框内部（含关闭标记行；开启行本身返回 false）。
func (t *contentBlockTracker) step(line string) bool {
	if n := len(t.frames); n > 0 {
		f := &t.frames[n-1]
		if f.byMark {
			if line == f.mark {
				t.frames = t.frames[:n-1]
			}
			return true
		}
		if strings.HasSuffix(line, "{") || strings.HasSuffix(line, "[") {
			f.depth++
		}
		if line == "}" || line == "]" || line == "}," || line == "]," {
			f.depth--
			if f.depth == 0 {
				t.frames = t.frames[:n-1]
			}
		}
		return true
	}
	if f, ok := contentBlockOpen(line); ok {
		t.frames = append(t.frames, f)
	}
	return false
}

// refs 提取当前框内容行里被读取的变量名。
func (t *contentBlockTracker) refs(line string) []string {
	if len(t.frames) == 0 {
		return nil
	}
	f := t.frames[len(t.frames)-1]
	var names []string
	if f.json {
		// JSON>{/[ 与 变量:{/[ 的值按 dic.NewJson 的前缀语法（"s%变量名"/"%变量名"）读取变量。
		names = append(names, jsonValueRefs(line)...)
	}
	if f.interp {
		// JSON> 框的值与 文本>/变量:"""/变量:>>> 的内容经 r.Val.Text 做 %变量% 插值，
		// 与正文行不同：内容行不会被切成赋值操作符，直接按 %变量% 成对切分。
		names = append(names, textBlockVarRefs(line)...)
	}
	return names
}

// contentBlockOpen 识别「内容行不是变量赋值」的框开启行，判定与 blockOpen/funcSkipOpen 一致。
func contentBlockOpen(line string) (contentFrame, bool) {
	switch {
	case strings.HasPrefix(line, "纯文本>"):
		return contentFrame{byMark: true, mark: "<文本"}, true
	case strings.HasPrefix(line, "文本>"):
		return contentFrame{byMark: true, mark: "<文本", interp: true}, true
	case line == "JSON>[" || line == "JSON>{":
		return contentFrame{depth: 1, json: true}, true
	case strings.HasPrefix(line, "JSON>"):
		return contentFrame{byMark: true, mark: "<JSON", interp: true, json: true}, true
	case line == "--js":
		return contentFrame{byMark: true, mark: "--end"}, true
	case strings.HasPrefix(line, "#:>>>"):
		return contentFrame{byMark: true, mark: "<<<", interp: true}, true
	}
	if vt, vp, vs := build.ValTextTest(line); vt == 6 {
		switch {
		case vs == `"""`:
			return contentFrame{byMark: true, mark: `"""`, interp: true}, true
		case vs == `'''`:
			return contentFrame{byMark: true, mark: `'''`}, true
		case vs == ">>>":
			return contentFrame{byMark: true, mark: "<<<", interp: true}, true
		case (vs == "{" || vs == "[") && !strings.Contains(vp, "->"):
			// 变量:{ / 变量:[ 多行 JSON 赋值框（含 -> 路径的是单行 JSON 赋值，非框）。
			return contentFrame{depth: 1, json: true}, true
		case vp != "" && strings.HasPrefix(vs, "纯文本>"):
			// 变量:纯文本> 赋值框：内容原样，%变量% 不插值。
			return contentFrame{byMark: true, mark: "<文本"}, true
		case vp != "" && strings.HasPrefix(vs, "文本>"):
			// 变量:文本> 赋值框：内容做 %变量% 插值。
			return contentFrame{byMark: true, mark: "<文本", interp: true}, true
		}
	}
	return contentFrame{}, false
}

// jsonValueRefs 提取 JSON 字符串值形式的变量引用。
// dic.NewJson 的替换规则：字符串值以 "s%变量名" 或 "%变量名" 开头即读取该变量
// （如 "a": "%ww" 读取变量 ww，值不带闭合 %）。静态检查需按同一语法识别，
// 否则 JSON 框内以该语法读取的变量会被误判为「未使用」。
func jsonValueRefs(line string) []string {
	var names []string
	for i := 0; i < len(line); i++ {
		if line[i] != '"' {
			continue
		}
		end := strings.IndexByte(line[i+1:], '"')
		if end < 0 {
			break
		}
		end += i + 1
		val := line[i+1 : end]
		// 仅取值位置的字符串参与替换，键名不受影响。
		rest := strings.TrimRight(line[:i], " \t")
		i = end
		if rest == "" {
			continue
		}
		if c := rest[len(rest)-1]; c != ':' && c != ',' && c != '[' {
			continue
		}
		switch {
		case strings.HasPrefix(val, "s%") && len(val) > 2 && !strings.Contains(val[2:], "%"):
			names = append(names, val[2:])
		case strings.HasPrefix(val, "%") && len(val) > 1 && !strings.Contains(val[1:], "%"):
			names = append(names, val[1:])
		}
	}
	return filterVarNames(names)
}

// textBlockVarRefs 提取文本框内容行里的 %变量% 引用（含 %实例.成员% 按 . 拆分的实例变量），
// 并剔除 % 切分产生的噪声（如 JSON 文本值 `a="50%", b="60%"` 会被切成 `", b="60`）。
func textBlockVarRefs(line string) []string {
	var names []string
	for _, name := range extractVarRefs(line) {
		if strings.Contains(name, ".") {
			names = append(names, strings.Split(name, ".")...)
			continue
		}
		names = append(names, name)
	}
	return filterVarNames(names)
}

// filterVarNames 过滤出符合变量名规则的引用，剔除 % 切分噪声。
func filterVarNames(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		if !isPlainVarName(name) {
			continue
		}
		out = append(out, name)
	}
	return out
}

// isPlainVarName 判断字符串是否为变量名（不含空白、引号、逗号、等号、括号等 % 切分噪声字符）。
func isPlainVarName(name string) bool {
	if name == "" {
		return false
	}
	return !strings.ContainsAny(name, " \t\r\n\"',=<>{}[]()%/\\")
}

// unusedAssign 记录一条赋值行（目标变量名、行号与原始行文本）。
type unusedAssign struct {
	name string
	line int
	text string
}

// checkUnusedAssignment 静态检查赋值行：目标变量在整个编译产物中都未被引用时给出警告。
// 正文行一旦以「名字+半角冒号」开头就会被解析成赋予值，该行不输出；
// 若本意是输出带冒号的文本，这是一种静默失效，提示时一并给出转义冒号的修正写法。
// 告警定位在最后一次赋值行：该处才是变量的最终值，此前的赋值都被覆盖。
func checkUnusedAssignment(v *dto.BuildValue, stack *importStack) {
	checkUnusedAssignmentWith(v, stack, isMagicVar, nil)
}

// checkUnusedAssignmentWith 与 checkUnusedAssignment 相同，只是把「哪些变量名不算死代码」交给调用方决定：
// 网页词库脚本块没有编译环节、isMagicVar 也不覆盖运行时注入的全局变量，需要额外豁免；
// extraUsed 是调用方已知的外部引用（如页面 HTML 里用 {{.键}} 取过值的变量），这些同样不算死代码。
func checkUnusedAssignmentWith(v *dto.BuildValue, stack *importStack, exempt func(string) bool, extraUsed map[string]bool) {
	used := make(map[string]bool, len(extraUsed)) // 全产物中被引用过的变量名
	for name := range extraUsed {
		used[name] = true
	}
	last := make(map[string]unusedAssign) // 变量 -> 最后一次赋值（最终值所在行）
	var order []string                    // 变量首次出现的顺序，保证告警顺序稳定

	scan := func(lines []string, lineNums []int) {
		var blocks contentBlockTracker
		for i, line := range lines {
			ln := 0
			if i < len(lineNums) {
				ln = lineNums[i]
			}
			// 框内容行不是变量赋值：文本/JSON/JS/链式/三引号等框内的 `key: 值` 会被
			// ValTextTest 误认为变量赋值。内容行仍会读取变量（%变量% 或 NewJson 的 %变量名），需计入引用。
			if blocks.step(line) {
				for _, name := range blocks.refs(line) {
					used[name] = true
				}
				continue
			}
			if !strings.HasPrefix(line, "//") {
				vt, vp, _ := build.ValTextTest(line)
				// 行内判断（如果:/否则如果:/if:/elif:）由运行时按前缀拦截，不是变量赋值。
				isAssign := vt != 0 && vp != "" && !isInlineControlLine(line)
				// 自引用（赋值行右侧读取赋值目标自身，如 lis:%lis%%appid%）不算「使用」：
				// 变量只有在自身赋值行之外被读取，才算真正被引用。
				for _, name := range lineVarRefs(line) {
					if isAssign && name == vp {
						continue
					}
					used[name] = true
				}
				if isAssign && !exempt(vp) {
					// 告警定位到最后一次赋值：变量的最终值死在那里，此前的赋值都被覆盖了。
					if _, ok := last[vp]; !ok {
						order = append(order, vp)
					}
					last[vp] = unusedAssign{name: vp, line: ln, text: line}
				}
			}
		}
	}

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
		a := last[name]
		msg := "变量未使用：" + name + " 赋值后未被任何地方引用"
		// 块开启行（key:{/[、key:"""/'''、key:>>>）本身就是块结构声明，冒号不是「想输出文本」的误写，
		// 给出转义写法反而误导，只提示变量未被引用。
		if !isBlockAssign(a.text, name) {
			msg += "；若此处是要输出文本，应写成 " + escapeAssignColon(a.text, name)
		}
		stack.addWarning(a.line, msg)
	}
}

// isBlockAssign 判断赋值行是否为块开启声明（多行 JSON 框 key:{ / key:[、三引号文本块、链式块 key:>>>），
// 判定与 build.formatOpen、contentBlockOpen 保持一致。
func isBlockAssign(line, key string) bool {
	if key == "" || strings.Contains(key, "->") || !strings.HasPrefix(line, key) {
		return false
	}
	vt, _, vs := build.ValTextTest(line)
	if vt != 6 {
		return false
	}
	switch vs {
	case `"""`, `'''`, ">>>":
		return true
	}
	return strings.HasPrefix(vs, "{") || strings.HasPrefix(vs, "[")
}

// isInlineControlLine 判断该行是否为行内判断语句（如果:/if: 起始，否则如果:/elif: 分支）。
// 运行时由 dic/bc 的 inlineIfStart/inlineElifCond 按前缀拦截，不会走赋值路径；
// 静态检查需保持一致，否则「如果:/if:」会被当成名为「如果」「if」的变量赋值而误告警。
func isInlineControlLine(line string) bool {
	for _, p := range [...]string{"否则如果:", "如果:", "elif:", "if:"} {
		if strings.HasPrefix(line, p) && len(line) > len(p) {
			return true
		}
	}
	return false
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
