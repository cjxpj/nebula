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
			"%f%", // 引用函数框变量，避免触发「变量未使用」告警
		},
		LineNums: []int{2, 3, 4, 5, 6, 7, 8, 9, 10},
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

func TestCheckBlockPairsTextBlockVarPrefix(t *testing.T) {
	// 变量名:文本> / 变量名:纯文本> 属叶子框：<文本 关闭，内容行的 `a: 文本` 不参与框识别。
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text: []string{
			"a:文本>",
			"第一行",
			"<文本",
			"b:纯文本>|",
			"第二行",
			"<文本",
			"%a%%b%",
		},
		LineNums: []int{2, 3, 4, 5, 6, 7, 8},
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
		Text:        []string{"x:$:$执行函数$", "%x%"},
		LineNums:    []int{3, 4},
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
		Text:        []string{"y:1", "x:%:%y%", "%x%"},
		LineNums:    []int{2, 3, 4},
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
			"pa:纯文本>|", // 变量前缀原样赋值框：内容不做插值
			"%不存在5%",
			"<文本",
			"pb:文本>%换行%", // 变量前缀插值赋值框：内容做插值，应检查
			"%不存在6%",
			"<文本",
		},
		LineNums: []int{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19},
	}}
	s := newTestStack()
	runCompileChecks(v, s)

	// 原样文本框内的 %不存在1% / %不存在2% / %不存在4% / %不存在5% 不应告警；
	// 插值文本框内的 %不存在3% / %不存在6% 应告警。
	for _, w := range s.warnings {
		if strings.Contains(w.Text, "变量不存在：不存在1") ||
			strings.Contains(w.Text, "变量不存在：不存在2") ||
			strings.Contains(w.Text, "变量不存在：不存在4") ||
			strings.Contains(w.Text, "变量不存在：不存在5") {
			t.Fatalf("原样文本框内的变量不应告警，实际：%v", warningsText(s.warnings))
		}
	}
	if !containsText(s.warnings, "变量不存在：不存在3") {
		t.Fatalf("插值文本框内的未定义变量应告警，实际：%v", warningsText(s.warnings))
	}
	if !containsText(s.warnings, "变量不存在：不存在6") {
		t.Fatalf("变量前缀插值文本框内的未定义变量应告警，实际：%v", warningsText(s.warnings))
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
	v.DicFuncs["函数"] = []*dto.BuildDic{{
		Trigger: dto.MiddlewareTrigger,
		Text: []string{
			"纯文本>%换行%", // 中间件原样框：内容不做插值
			"%不存在1%",
			"<文本",
			"变量:'''", // 中间件原样赋值框：内容不做插值
			"%不存在2%",
			"'''",
		},
		LineNums: []int{1, 2, 3, 4, 5, 6},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	for _, w := range s.warnings {
		if strings.Contains(w.Text, "变量不存在：不存在1") ||
			strings.Contains(w.Text, "变量不存在：不存在2") {
			t.Fatalf("中间件原样文本框内的变量不应告警，实际：%v", warningsText(s.warnings))
		}
	}
}

func TestCheckFuncClosed(t *testing.T) {
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text: []string{
			"$线程变量 对局中 1$",  // 闭合，正常
			"$线程变量 模式 %模式%", // 缺结尾 $，应告警
			"$复读 你好$，共 $价格 1$ 元",
		},
		LineNums: []int{2, 3, 4},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if !containsText(s.warnings, "函数未闭合") {
		t.Fatalf("缺结尾 $ 应告警，实际：%v", warningsText(s.warnings))
	}
	// 只应有一条：第 3 行的两个调用都闭合，第 4 行也闭合。
	closeWarn := 0
	for _, w := range s.warnings {
		if strings.Contains(w.Text, "函数未闭合") {
			closeWarn++
			if w.Line != 3 {
				t.Fatalf("告警行号应为 3，实际：%d", w.Line)
			}
		}
	}
	if closeWarn != 1 {
		t.Fatalf("期望仅 1 条未闭合告警，实际：%d，全部：%v", closeWarn, warningsText(s.warnings))
	}
}

func TestCheckFuncClosedSkipRawBlocks(t *testing.T) {
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text: []string{
			"文本>",
			"价格$100",
			"<文本",
			"纯文本>",
			"$复读 你好$",
			"<文本",
			"JSON>a",
			"k=值$",
			"<JSON",
			`变量:"""`,
			"$未闭合",
			`"""`,
			`变量:'''`,
			"$未闭合",
			`'''`,
			"a:{",
			`"k":"v$"`,
			"}",
			"--js",
			"const s = `$ {x}`",
			"--end",
			`价格\$100`, // 转义 $：不算函数起始，不告警
		},
		LineNums: []int{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	for _, w := range s.warnings {
		if strings.Contains(w.Text, "函数未闭合") {
			t.Fatalf("原样框内/转义 $ 不应告警，实际：%v", warningsText(s.warnings))
		}
	}
}

func TestCheckFuncClosedInFuncBlock(t *testing.T) {
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text: []string{
			"f:函数>",
			"$线程变量 模式 %模式",
			"<函数",
		},
		LineNums: []int{2, 3, 4},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if !containsText(s.warnings, "函数未闭合") {
		t.Fatalf("函数框内缺结尾 $ 应告警，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckAssignKeySpecialChar(t *testing.T) {
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text: []string{
			"$a:a",      // 键名混入 $：报键名特殊字符，不报函数未闭合
			"%随机数%:abc", // 键名混入 %：报键名特殊字符
			"价格:100",    // 正常赋值，不告警
			"%价格%元",     // 正常插值，不告警
			"我的变量名字超过了三十二个字节上限:100",    // 键名超 32 字节：报变量名过长
			"这是一段很长的普通文本没有冒号所以不会被当成赋值", // 无操作符，不告警
		},
		LineNums: []int{2, 3, 4, 5, 6, 7},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if containsText(s.warnings, "函数未闭合") {
		t.Fatalf("$a:a 不应报函数未闭合，实际：%v", warningsText(s.warnings))
	}
	markerLines := map[int]bool{}
	longLines := map[int]bool{}
	for _, w := range s.warnings {
		if strings.Contains(w.Text, "键名不能包含特殊字符") {
			markerLines[w.Line] = true
		}
		if strings.Contains(w.Text, "变量名过长") {
			longLines[w.Line] = true
		}
	}
	if len(markerLines) != 2 || !markerLines[2] || !markerLines[3] {
		t.Fatalf("期望第 2、3 行各一条键名特殊字符告警，实际：%v", warningsText(s.warnings))
	}
	if len(longLines) != 1 || !longLines[6] {
		t.Fatalf("期望第 6 行一条变量名过长告警，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckUnusedAssignment(t *testing.T) {
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text: []string{
			"伤害:%伤害%",     // 赋值后右侧仅读取自身，且变量全程未被引用：应告警并给出转义写法
			"未被引用的变量:100", // 赋值后未被引用：应告警
			"被引用变量:1",     // 赋值后被 %被引用变量% 引用：不告警
			"%被引用变量%",
		},
		LineNums: []int{2, 3, 4, 5},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if !containsText(s.warnings, "变量未使用：伤害") {
		t.Fatalf("赋值后未被引用的变量应告警，实际：%v", warningsText(s.warnings))
	}
	if !containsText(s.warnings, `伤害\:%伤害%`) {
		t.Fatalf("告警应给出转义冒号的修正写法，实际：%v", warningsText(s.warnings))
	}
	if !containsText(s.warnings, "变量未使用：未被引用的变量") {
		t.Fatalf("未被引用的赋值应告警，实际：%v", warningsText(s.warnings))
	}
	if containsText(s.warnings, "变量未使用：被引用变量") {
		t.Fatalf("被引用的变量不应告警，实际：%v", warningsText(s.warnings))
	}
	for _, w := range s.warnings {
		if strings.Contains(w.Text, "变量未使用") && w.Line != 2 && w.Line != 3 {
			t.Fatalf("告警行号应为 2/3，实际：%v", warningsText(s.warnings))
		}
	}
}

func TestCheckUnusedAssignmentSkips(t *testing.T) {
	v := newTestBuildValue()
	// 函数传出变量由运行时写回，函数体内赋值即隐式被使用。
	v.DicFuncs["函数"] = []*dto.BuildDic{{
		Trigger:   "f->结果",
		ParamRule: "0",
		Text:      []string{"结果:1"},
		LineNums:  []int{2},
	}}
	v.Dic = []*dto.BuildDic{
		{
			Trigger:     "甲",
			TriggerLine: 1,
			Text:        []string{"跨词条变量:1"},
			LineNums:    []int{2},
		},
		{
			Trigger:     "乙",
			TriggerLine: 5,
			Text: []string{
				"实例:x",
				"$实例.方法$", // 实例方法调用：实例变量被使用
				"%跨词条变量%", // 跨词条引用
			},
			LineNums: []int{6, 7, 8},
		},
	}
	s := newTestStack()
	runCompileChecks(v, s)
	for _, w := range s.warnings {
		if strings.Contains(w.Text, "变量未使用") {
			t.Fatalf("函数传出变量/跨词条引用/实例方法调用不应告警，实际：%v", warningsText(s.warnings))
		}
	}
}

func TestCheckUnusedAssignmentSkipsJsonBlockContent(t *testing.T) {
	// JSON 键值框内容行是 JSON 键值设置而不是变量赋值（键名会被 ValTextTest 误认为赋值目标），
	// 其中的值仍会读取变量：JSON> 框按 %变量% 插值，变量:{ 框按 dic.NewJson 的 %变量名 前缀替换。
	newCase := func(texts []string, nums []int) *importStack {
		v := newTestBuildValue()
		v.DicFuncs["函数"] = []*dto.BuildDic{
			{Trigger: dto.MiddlewareTrigger, Text: []string{"b:", "ww:ok"}, LineNums: []int{1, 2}},
		}
		v.Dic = []*dto.BuildDic{{
			Trigger:     "测试",
			TriggerLine: 3,
			Text:        texts,
			LineNums:    nums,
		}}
		s := newTestStack()
		runCompileChecks(v, s)
		return s
	}

	s := newCase([]string{
		"JSON>{}",
		"b:=%b%", // 读变量 b；键名 b 不是赋值目标
		"<JSON",
		"a:{",
		`    "a": "%ww"`, // NewJson 前缀语法读变量 ww（值不带闭合 %）
		"}",
		"%a%",
	}, []int{4, 5, 6, 7, 8, 9, 10})
	for _, w := range s.warnings {
		if strings.Contains(w.Text, "变量未使用") {
			t.Fatalf("JSON 框内容行不应产生「变量未使用」告警，实际：%v", warningsText(s.warnings))
		}
	}

	// 框开启行本身仍是赋值：变量全程未被引用时应照常告警。
	s = newCase([]string{"a:{", `    "a": 1`, "}"}, []int{4, 5, 6})
	if !containsText(s.warnings, "变量未使用：a") {
		t.Fatalf("JSON 框赋值后未被引用的变量应告警，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckUnusedAssignmentSkipsContentBlockLines(t *testing.T) {
	// 文本框/JS 框/链式框等内容行不是变量赋值（行首形如 `key: 值` 的是文本或取值表达式），
	// 不应产生「变量未使用」告警。
	cases := []struct {
		name string
		text []string
	}{
		{"文本块", []string{"文本>", "a: 这是文本", "<文本"}},
		{"纯文本块", []string{"纯文本>", "a: 这是文本", "<文本"}},
		{"变量三引号", []string{`x:"""`, "a: 这是文本", `"""`, "%x%"}},
		{"变量三单引号", []string{`x:'''`, "a: 这是文本", `'''`, "%x%"}},
		{"变量前缀文本块", []string{"x:文本>", "a: 这是文本", "<文本", "%x%"}},
		{"变量前缀纯文本块", []string{"x:纯文本>", "a: 这是文本", "<文本", "%x%"}},
		{"JS框", []string{"--js", "const o = {a: 1};", "foo: bar", "--end"}},
		{"链式框", []string{"v:>>>", "a: 1", "<<<", "%v%"}},
		{"异步链式框", []string{"#:>>>", "a: 1", "<<<"}},
	}
	for _, c := range cases {
		v := newTestBuildValue()
		nums := make([]int, len(c.text))
		for i := range nums {
			nums[i] = i + 2
		}
		v.Dic = []*dto.BuildDic{{Trigger: "测试", TriggerLine: 1, Text: c.text, LineNums: nums}}
		s := newTestStack()
		runCompileChecks(v, s)
		if containsText(s.warnings, "变量未使用") {
			t.Fatalf("%s：内容行不应产生「变量未使用」告警，实际：%v", c.name, warningsText(s.warnings))
		}
	}
}

func TestCheckUndefinedVarSkipsJsonPercentNoise(t *testing.T) {
	// JSON 框内容行的文本值含 % 时（如 a="50%", b="60%"），% 切分产生的噪声片段
	// 不应被当成变量引用报「变量不存在」。
	v := newTestBuildValue()
	v.DicFuncs["函数"] = []*dto.BuildDic{
		{Trigger: dto.MiddlewareTrigger, Text: []string{"b:1"}, LineNums: []int{1}},
	}
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 2,
		Text:        []string{"JSON>{}", `a="50%", b="60%"`, "<JSON", "%b%"},
		LineNums:    []int{3, 4, 5, 6},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if containsText(s.warnings, "变量不存在") {
		t.Fatalf("JSON 框内容行的 %% 不应产生噪声引用告警，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckUndefinedVarInContentBlocks(t *testing.T) {
	// 框内容行不参与赋值收集，但插值框里的 %变量% 仍是引用，应照常检查变量是否存在。
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text: []string{
			"V:>>>", // 链式框内容为取值表达式
			"%不存在1%",
			"<<<",
			"%V%",
			"--js", // JS 框内容不做 %变量% 插值
			"%不存在2%",
			"--end",
		},
		LineNums: []int{2, 3, 4, 5, 6, 7, 8},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if !containsText(s.warnings, "变量不存在：不存在1") {
		t.Fatalf("链式框内容里的未定义变量应告警，实际：%v", warningsText(s.warnings))
	}
	if containsText(s.warnings, "变量不存在：不存在2") {
		t.Fatalf("JS 框内容不做插值，不应告警，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckRawAssignSkipsFuncAndVar(t *testing.T) {
	// :: 纯文本赋值（绝对文本）的值原样写入，不执行 $函数$、不解析 %变量%，
	// 故其中的 $...$ / %...% 不应被误报为「函数不存在」「变量不存在」。
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text: []string{
			"a::$不存在的函数 参数$%未定义%",
			"%a%", // 引用 a，避免触发「变量未使用」
		},
		LineNums: []int{2, 3},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if containsText(s.warnings, "函数不存在：不存在的函数") {
		t.Fatalf(":: 赋值不应报函数不存在，实际：%v", warningsText(s.warnings))
	}
	if containsText(s.warnings, "变量不存在：未定义") {
		t.Fatalf(":: 赋值不应报变量不存在，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckFuncClosedSkipsRawAssign(t *testing.T) {
	// :: 纯文本赋值的值不解析 $，即使缺少结尾 $ 也不应报「函数未闭合」。
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text:        []string{"a::$价格100", "%a%"},
		LineNums:    []int{2, 3},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	for _, w := range s.warnings {
		if strings.Contains(w.Text, "函数未闭合") {
			t.Fatalf(":: 赋值值不解析 $，不应报函数未闭合，实际：%v", warningsText(s.warnings))
		}
	}
}

func TestCheckNormalAssignStillParsed(t *testing.T) {
	// 对照：普通赋值(:)仍会解析 $...$ 与 %...%，误报修复不应削弱该检查。
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text: []string{
			"a:$不存在的函数$", // 普通赋值：应报函数不存在
			"b:%未定义%",    // 普通赋值：应报变量不存在
			"%a%%b%",     // 引用 a/b，避免触发「变量未使用」
		},
		LineNums: []int{2, 3, 4},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if !containsText(s.warnings, "函数不存在：不存在的函数") {
		t.Fatalf("普通赋值仍应报函数不存在，实际：%v", warningsText(s.warnings))
	}
	if !containsText(s.warnings, "变量不存在：未定义") {
		t.Fatalf("普通赋值仍应报变量不存在，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckUnusedAssignmentSkipsRawTextAndComment(t *testing.T) {
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text: []string{
			"a:1",
			"纯文本>",
			"%a%", // 原样框内容行不做插值，%a% 不算引用
			"<文本",
			"// %a%", // 注释不执行，%a% 不算引用
		},
		LineNums: []int{2, 3, 4, 5, 6},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if !containsText(s.warnings, "变量未使用：a") {
		t.Fatalf("原样框/注释中的 %%a%% 不应算作引用，实际：%v", warningsText(s.warnings))
	}
}
