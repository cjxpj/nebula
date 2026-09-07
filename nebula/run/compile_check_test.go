package run

import (
	"strings"
	"testing"

	"github.com/cjxpj/nebula/dto"
)

func newTestStack() *importStack {
	return newImportStack()
}

func newTestBuildValue() *dto.BuildValue {
	return &dto.BuildValue{
		Dic:      []*dto.BuildDic{},
		DicFuncs: map[string][]*dto.BuildDic{},
		Class:    map[string]*dto.DicClass{},
		MyFunc:   map[string]dto.DicFunc{},
	}
}

func warningsText(w []dto.BuildWarning) []string {
	out := make([]string, len(w))
	for i, v := range w {
		out[i] = v.Text
	}
	return out
}

func containsText(w []dto.BuildWarning, substr string) bool {
	for _, v := range w {
		if strings.Contains(v.Text, substr) {
			return true
		}
	}
	return false
}

func TestCheckBlockPairsBalanced(t *testing.T) {
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text: []string{
			"f:函数>",
			"循环>i",
			"遍历>x",
			"如果>条件",
			"<如果",
			"<遍历",
			"<循环",
			"<函数",
		},
		LineNums: []int{2, 3, 4, 5, 6, 7, 8, 9},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if len(s.warnings) != 0 {
		t.Fatalf("期望无警告，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckBlockPairsUnclosed(t *testing.T) {
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text:        []string{"f:函数>", "循环>i", "<循环"},
		LineNums:    []int{2, 3, 4},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if !containsText(s.warnings, "函数框") || !containsText(s.warnings, "未闭合") {
		t.Fatalf("期望报告未闭合的函数框，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckBlockPairsMismatch(t *testing.T) {
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text:        []string{"f:函数>", "循环>i", "<函数", "<循环"},
		LineNums:    []int{2, 3, 4, 5},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if !containsText(s.warnings, "无法关闭") {
		t.Fatalf("期望报告不匹配的关闭标记，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckBlockPairsExtraClose(t *testing.T) {
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text:        []string{"<函数"},
		LineNums:    []int{2},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if !containsText(s.warnings, "多余的关闭标记") {
		t.Fatalf("期望报告多余关闭标记，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckBlockPairsNewJson(t *testing.T) {
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text: []string{
			"JSON>{",
			"\"a\":1",
			"}",
		},
		LineNums: []int{2, 3, 4},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if len(s.warnings) != 0 {
		t.Fatalf("期望无警告，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckTriggerRegex(t *testing.T) {
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{
		{Trigger: "abc.+", TriggerLine: 2},
		{Trigger: "a(b", TriggerLine: 4},
	}
	s := newTestStack()
	runCompileChecks(v, s)
	if len(s.warnings) != 1 || !containsText(s.warnings, "触发词正则语法错误") {
		t.Fatalf("期望仅报告 1 条正则错误，实际：%v", warningsText(s.warnings))
	}
	if s.warnings[0].Line != 4 {
		t.Fatalf("期望错误行号为 4，实际 %d", s.warnings[0].Line)
	}
}

func TestCheckFuncParamsLocal(t *testing.T) {
	v := newTestBuildValue()
	v.DicFuncs["函数"] = []*dto.BuildDic{{
		Trigger:   "打招呼",
		ParamRule: "2",
	}}
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text:        []string{"$打招呼 你好$"},
		LineNums:    []int{3},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if !containsText(s.warnings, "函数参数数量错误") {
		t.Fatalf("期望报告参数数量错误，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckFuncParamsMyFunc(t *testing.T) {
	v := newTestBuildValue()
	v.MyFunc["自定义"] = dto.DicFunc{L: "1"}
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text:        []string{"$自定义$"},
		LineNums:    []int{3},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if !containsText(s.warnings, "函数参数数量错误") {
		t.Fatalf("期望报告 MyFunc 参数数量错误，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckFuncParamsBuiltin(t *testing.T) {
	dto.RegisterFuncRule("测试内置函数", "3")
	defer dto.UnregisterFuncRule("测试内置函数")

	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text:        []string{"$测试内置函数 a$"},
		LineNums:    []int{3},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if !containsText(s.warnings, "函数参数数量错误") {
		t.Fatalf("期望报告内置函数参数数量错误，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckFuncParamsSkipsDynamic(t *testing.T) {
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text:        []string{"$.类 方法$", "$%变量%$", "$new 类$", "$变量.方法 参数$"},
		LineNums:    []int{3, 4, 5, 6},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if len(s.warnings) != 0 {
		t.Fatalf("动态调用不应触发静态参数检查，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckFuncNameConflict(t *testing.T) {
	dto.RegisterFuncRule("内置冲突函数", "0")
	defer dto.UnregisterFuncRule("内置冲突函数")

	v := newTestBuildValue()
	v.DicFuncs["函数"] = []*dto.BuildDic{
		{Trigger: "内置冲突函数", TriggerLine: 10, Text: []string{"x"}, LineNums: []int{11}},
		{Trigger: "普通函数", TriggerLine: 20, Text: []string{"y"}, LineNums: []int{21}},
	}
	s := newTestStack()
	runCompileChecks(v, s)
	if !containsText(s.warnings, "禁止覆盖系统内置函数：内置冲突函数") {
		t.Fatalf("期望报告内置函数冲突，实际：%v", warningsText(s.warnings))
	}
	if containsText(s.warnings, "普通函数") {
		t.Fatalf("普通函数不应触发冲突警告，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckUndefinedFunc(t *testing.T) {
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text:        []string{"$不存在的函数 参数$"},
		LineNums:    []int{3},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if !containsText(s.warnings, "函数不存在：不存在的函数") {
		t.Fatalf("期望报告未定义函数，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckUndefinedVar(t *testing.T) {
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text:        []string{"你好%未定义变量%"},
		LineNums:    []int{3},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if !containsText(s.warnings, "变量不存在：未定义变量") {
		t.Fatalf("期望报告未定义变量，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckFuncParamsExecFuncAssign(t *testing.T) {
	// :$: 执行函数赋值：操作符 $:$ 不应被误判为空函数。
	v := newTestBuildValue()
	v.DicFuncs["函数"] = []*dto.BuildDic{{Trigger: "执行函数", ParamRule: "0", Text: []string{"a"}, LineNums: []int{1}}}
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text:        []string{"x:$:$执行函数$"},
		LineNums:    []int{3},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if len(s.warnings) != 0 {
		t.Fatalf(":$: 执行函数赋值不应产生警告，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckFuncParamsExecFuncAssignUndefined(t *testing.T) {
	// :$: 执行函数赋值：应报告右侧真实函数名，而不是把 $:$ 误判为名为 ":" 的空函数。
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text:        []string{"x:$:$不存在的函数$"},
		LineNums:    []int{3},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if !containsText(s.warnings, "函数不存在：不存在的函数") {
		t.Fatalf("期望报告右侧真实函数不存在，实际：%v", warningsText(s.warnings))
	}
	for _, w := range s.warnings {
		if w.Text == "函数不存在：" {
			t.Fatalf("不应把 $:$ 误判为空函数，实际：%v", warningsText(s.warnings))
		}
	}
}

func TestCheckUndefinedVarExecVarAssign(t *testing.T) {
	// :%: 只读取变量赋值：操作符 %:% 不应被误判为空变量，右侧变量已定义时不应告警。
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text:        []string{"y:1", "x:%:%y%"},
		LineNums:    []int{2, 3},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if len(s.warnings) != 0 {
		t.Fatalf(":%%: 只读取变量赋值不应产生警告，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckUndefinedVarExecVarAssignUndefined(t *testing.T) {
	// :%: 只读取变量赋值：应报告右侧真实变量名，而不是把 %:% 误判为名为 ":" 的空变量。
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text:        []string{"x:%:%y%"},
		LineNums:    []int{3},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if !containsText(s.warnings, "变量不存在：y") {
		t.Fatalf("期望报告右侧真实变量不存在，实际：%v", warningsText(s.warnings))
	}
	for _, w := range s.warnings {
		if w.Text == "变量不存在：" {
			t.Fatalf("不应把 %%:%% 误判为空变量，实际：%v", warningsText(s.warnings))
		}
	}
}

func TestCheckUndefinedVarSkipsDefined(t *testing.T) {
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text: []string{
			"a:1",
			"%a%",
			"循环>i=3",
			"%i%",
			"<循环",
			"遍历>k,v=[\"x\"]",
			"%k%%v%",
			"<遍历",
			"foo:函数>",
			"$%foo%$",
			"<函数",
		},
		LineNums: []int{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if len(s.warnings) != 0 {
		t.Fatalf("已定义变量不应告警，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckUndefinedVarJsonBlockAssign(t *testing.T) {
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text: []string{
			"JSON>a={}",
			"a=ok",
			"<JSON",
			"%a%",
		},
		LineNums: []int{2, 3, 4, 5},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if len(s.warnings) != 0 {
		t.Fatalf("JSON> 框赋值变量不应告警，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckUndefinedVarJsonBlockNoAssign(t *testing.T) {
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text: []string{
			"JSON>[]",
			"0=a",
			"<JSON",
			"%a%", // JSON>[] 直接输出，未声明变量 a，应告警
		},
		LineNums: []int{2, 3, 4, 5},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if !containsText(s.warnings, "变量不存在：a") {
		t.Fatalf("JSON>[] 直接输出不声明变量，%%a%% 应告警，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckUndefinedVarSkipsMagic(t *testing.T) {
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text:        []string{"%参数1%%QQ%%时间%%换行%%__词库路径__%"},
		LineNums:    []int{3},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if len(s.warnings) != 0 {
		t.Fatalf("魔术/消息变量不应告警，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckUndefinedVarFuncOutNotAssigned(t *testing.T) {
	v := newTestBuildValue()
	v.DicFuncs["函数"] = []*dto.BuildDic{{
		Trigger:   "a->aa",
		ParamRule: "1",
		Text:      []string{"a"}, // 函数正文仅输出，未给传出变量 aa 赋值
		LineNums:  []int{2},
	}}
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text:        []string{"%aa%"},
		LineNums:    []int{3},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if !containsText(s.warnings, "变量不存在：aa") {
		t.Fatalf("函数传出变量未被赋值时应告警，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckUndefinedVarFuncOutAssigned(t *testing.T) {
	v := newTestBuildValue()
	v.DicFuncs["函数"] = []*dto.BuildDic{{
		Trigger:   "a->aa",
		ParamRule: "1",
		Text:      []string{"aa:1"},
		LineNums:  []int{2},
	}}
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text:        []string{"$a x$", "%aa%"},
		LineNums:    []int{3, 4},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if containsText(s.warnings, "变量不存在：aa") {
		t.Fatalf("函数正文已赋值传出变量，不应告警，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckUndefinedVarFuncOutBeforeCall(t *testing.T) {
	v := newTestBuildValue()
	v.DicFuncs["函数"] = []*dto.BuildDic{{
		Trigger:   "a->aa",
		ParamRule: "1",
		Text:      []string{"aa:ok"},
		LineNums:  []int{2},
	}}
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text:        []string{"%aa%", "$a x$", "%aa%"},
		LineNums:    []int{3, 4, 5},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	// 只有调用 $a$ 之前（第 3 行）的 %aa% 应告警，调用之后（第 5 行）不应告警
	var warnedLines []int
	for _, w := range s.warnings {
		if strings.Contains(w.Text, "变量不存在：aa") {
			warnedLines = append(warnedLines, w.Line)
		}
	}
	if len(warnedLines) != 1 || warnedLines[0] != 3 {
		t.Fatalf("期望仅第 3 行 %%aa%% 告警，实际告警行：%v，全部：%v", warnedLines, warningsText(s.warnings))
	}
}

func TestCheckUndefinedVarSkipRawTextBlocks(t *testing.T) {
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text: []string{
			"纯文本>%换行%", // 原样框：内容不做插值
			"%不存在1%",
			"<文本",
			"变量:'''", // 原样赋值框：内容不做插值
			"%不存在2%",
			"'''",
			"文本>%换行%", // 插值框：内容做插值，应检查
			"%不存在3%",
			"<文本",
			"a:'''", // 再验证原样框不影响后续行检查
			"%不存在4%",
			"'''",
		},
		LineNums: []int{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13},
	}}
	s := newTestStack()
	runCompileChecks(v, s)

	// 原样文本框内的 %不存在1% / %不存在2% / %不存在4% 不应告警；
	// 插值文本框内的 %不存在3% 应告警。
	for _, w := range s.warnings {
		if strings.Contains(w.Text, "变量不存在：不存在1") ||
			strings.Contains(w.Text, "变量不存在：不存在2") ||
			strings.Contains(w.Text, "变量不存在：不存在4") {
			t.Fatalf("原样文本框内的变量不应告警，实际：%v", warningsText(s.warnings))
		}
	}
	if !containsText(s.warnings, "变量不存在：不存在3") {
		t.Fatalf("插值文本框内的未定义变量应告警，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckUndefinedVarSkipRawTextBlocksInFunc(t *testing.T) {
	v := newTestBuildValue()
	v.DicFuncs["函数"] = []*dto.BuildDic{{
		Trigger:   "f->r",
		ParamRule: "0",
		Text: []string{
			"纯文本>%换行%", // 函数体内原样框：内容不做插值
			"%不存在1%",
			"<文本",
			"r:'''", // 函数体内原样赋值框：内容不做插值
			"%不存在2%",
			"'''",
			"r:1",
		},
		LineNums: []int{2, 3, 4, 5, 6, 7, 8},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	for _, w := range s.warnings {
		if strings.Contains(w.Text, "变量不存在：不存在1") ||
			strings.Contains(w.Text, "变量不存在：不存在2") {
			t.Fatalf("函数体内原样文本框内的变量不应告警，实际：%v", warningsText(s.warnings))
		}
	}
}

func TestCheckUndefinedVarSkipRawTextBlocksInHead(t *testing.T) {
	v := newTestBuildValue()
	v.Head = []string{
		"纯文本>%换行%", // 头部原样框：内容不做插值
		"%不存在1%",
		"<文本",
		"变量:'''", // 头部原样赋值框：内容不做插值
		"%不存在2%",
		"'''",
	}
	v.HeadLineNums = []int{1, 2, 3, 4, 5, 6}
	s := newTestStack()
	runCompileChecks(v, s)
	for _, w := range s.warnings {
		if strings.Contains(w.Text, "变量不存在：不存在1") ||
			strings.Contains(w.Text, "变量不存在：不存在2") {
			t.Fatalf("头部原样文本框内的变量不应告警，实际：%v", warningsText(s.warnings))
		}
	}
}
