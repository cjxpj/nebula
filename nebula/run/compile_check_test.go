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
			"函数>f",
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
		Text:        []string{"函数>f", "循环>i", "<循环"},
		LineNums:    []int{2, 3, 4},
	}}
	s := newTestStack()
	runCompileChecks(v, s)
	if !containsText(s.warnings, "函数>") || !containsText(s.warnings, "未闭合") {
		t.Fatalf("期望报告未闭合的函数框，实际：%v", warningsText(s.warnings))
	}
}

func TestCheckBlockPairsMismatch(t *testing.T) {
	v := newTestBuildValue()
	v.Dic = []*dto.BuildDic{{
		Trigger:     "测试",
		TriggerLine: 1,
		Text:        []string{"函数>f", "循环>i", "<函数", "<循环"},
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
