package dic

import (
	"testing"

	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
)

// runIf 执行一段以 Main 为触发词的脚本（判断框专项），返回输出文本。
func runIf(t *testing.T, body string) string {
	t.Helper()
	chdirToAppWin()
	D := dic_dto.NewDic("t.n", "Main\n"+body)
	return dic_api.Api.DicRun(D, "Main")
}

// TestIfBlockBasic 判断框基本命中：条件为真走 if 分支。
func TestIfBlockBasic(t *testing.T) {
	got := runIf(t, `x:1
如果>%x%==1
是
>否则
否
<如果`)
	if got != "是" {
		t.Errorf("条件为真应命中 if 分支，期望 是，实际 %q", got)
	}
}

// TestIfBlockElse 判断框 else 分支：条件为假走 >否则。
func TestIfBlockElse(t *testing.T) {
	got := runIf(t, `x:2
如果>%x%==1
是
>否则
否
<如果`)
	if got != "否" {
		t.Errorf("条件为假应命中 else 分支，期望 否，实际 %q", got)
	}
}

// TestIfBlockElseIf 判断框 >否则如果 命中中间分支。
func TestIfBlockElseIf(t *testing.T) {
	got := runIf(t, `x:2
如果>%x%==1
一
>否则如果:%x%==2
二
>否则
三
<如果`)
	if got != "二" {
		t.Errorf("应命中 >否则如果 分支，期望 二，实际 %q", got)
	}
}

// TestIfBlockElseIfFallthrough 判断框所有条件都不满足时走 else。
func TestIfBlockElseIfFallthrough(t *testing.T) {
	got := runIf(t, `x:3
如果>%x%==1
一
>否则如果:%x%==2
二
>否则
三
<如果`)
	if got != "三" {
		t.Errorf("条件均不满足应走 else，期望 三，实际 %q", got)
	}
}

// TestIfBlockNested 判断框套娃：内层判断框关键字作为内容收集并在执行期递归处理。
func TestIfBlockNested(t *testing.T) {
	got := runIf(t, `x:1
如果>%x%==1
外层
如果>%x%==1
内层
<如果
>否则
否
<如果`)
	if got != "外层内层" {
		t.Errorf("嵌套判断框执行错误，期望 外层内层，实际 %q", got)
	}
}

// TestIfBlockLogicAnd 判断条件逻辑与 &。
func TestIfBlockLogicAnd(t *testing.T) {
	got := runIf(t, `x:1
y:2
如果>%x%==1&%y%==2
是
>否则
否
<如果`)
	if got != "是" {
		t.Errorf("逻辑与为真应命中 if，期望 是，实际 %q", got)
	}
}

// TestIfBlockLogicOr 判断条件逻辑或 |。
func TestIfBlockLogicOr(t *testing.T) {
	got := runIf(t, `x:1
y:3
如果>%x%==1|%y%==2
是
>否则
否
<如果`)
	if got != "是" {
		t.Errorf("逻辑或为真应命中 if，期望 是，实际 %q", got)
	}
}
