package run

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// runCompileChecks 对编译产物执行编译期静态检查，发现的警告统一追加到引入链的警告切片。
func runCompileChecks(v *dto.BuildValue, stack *importStack) {
	checkBlockPairs(v, stack)
	checkTriggerRegex(v, stack)
	checkFuncParams(v, stack)
	checkFuncNameConflict(v, stack)
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
	blkFunc blockKind = iota // 函数> ... <函数
	blkIf                    // 如果> ... <如果
	blkFor                   // 循环> ... <循环
	blkForEach               // 遍历> ... <遍历
	blkText                  // 文本>/纯文本> ... <文本
	blkJson                  // JSON> ... <JSON
	blkNewJson               // JSON>{ / JSON>[ ... 平衡括号
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
		return "函数>"
	case blkIf:
		return "如果>"
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
	case strings.HasPrefix(line, "函数>"):
		return blkFunc, true
	case len(line) > 7 && strings.HasPrefix(line, "如果>"):
		return blkIf, true
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
	for _, e := range allBuildDics(v) {
		checkBlockPairsEntry(e, stack)
	}
}

func checkBlockPairsEntry(e *dto.BuildDic, stack *importStack) {
	if len(e.Text) == 0 {
		return
	}

	frames := make([]blockFrame, 0, 4)
	for i, line := range e.Text {
		ln := 0
		if i < len(e.LineNums) {
			ln = e.LineNums[i]
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

// ============ 函数参数数量检查 ============

func checkFuncParams(v *dto.BuildValue, stack *importStack) {
	funcIndex := v.GetFuncIndex()
	for _, e := range allBuildDics(v) {
		checkFuncParamsEntry(e, v, funcIndex, stack)
	}
}

func checkFuncParamsEntry(e *dto.BuildDic, v *dto.BuildValue, funcIndex map[string][]*dto.BuildDic, stack *importStack) {
	for i, line := range e.Text {
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
	}
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
