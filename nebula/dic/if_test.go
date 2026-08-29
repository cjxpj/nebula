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

// TestIfBlockCnLogicWord 判断条件中文逻辑连接词：或/或者 → |，且/并且 → &。
func TestIfBlockCnLogicWord(t *testing.T) {
	cases := []struct {
		name string
		cond string
		want string
	}{
		{"中文或", `如果>%x%==1 或 %x%==2`, "是"},
		{"中文或false", `如果>%x%==3 或 %x%==2`, "否"},
		{"中文或者", `如果>%x%==3 或者 %x%==1`, "是"},
		{"中文且", `如果>%x%==1 且 %y%==2`, "是"},
		{"中文且false", `如果>%x%==1 且 %y%==3`, "否"},
		{"中文并且", `如果>%x%==1 并且 %y%==2`, "是"},
		{"混用竖线", `如果>%x%==3|%x%==1 或 %x%==9`, "是"},
		{"括号分组", `如果>%x%==9 或 (%x%==1 且 %y%==2)`, "是"},
		{"首部空格", `如果> %x%==1 或 %x%==2`, "是"},
		{"尾部空格", `如果>%x%==1 或 %x%==2 `, "是"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runIf(t, "x:1\ny:2\n"+c.cond+"\n是\n>否则\n否\n<如果")
			if got != c.want {
				t.Errorf("条件 %q 期望输出 %q，实际 %q", c.cond, c.want, got)
			}
		})
	}
}

// TestIfBlockCnWordNoSpace 无空格包裹的中文字不被当作逻辑连接词，按字面比较。
func TestIfBlockCnWordNoSpace(t *testing.T) {
	got := runIf(t, `s:黑或白
如果>%s%==黑或白
是
>否则
否
<如果`)
	if got != "是" {
		t.Errorf("无空格包裹的「或」应保持字面比较，期望 是，实际 %q", got)
	}
}

// TestIfBlockCnCmpWord 判断条件中文比较运算符：等于/不等于/大于/小于/大于等于/小于等于 → 符号。
func TestIfBlockCnCmpWord(t *testing.T) {
	cases := []struct {
		name string
		cond string
		want string
	}{
		{"中文等于", `如果>%x% 等于 1`, "是"},
		{"中文等于false", `如果>%x% 等于 2`, "否"},
		{"中文不等于", `如果>%x% 不等于 2`, "是"},
		{"中文不等于false", `如果>%x% 不等于 1`, "否"},
		{"中文大于", `如果>%x% 大于 0`, "是"},
		{"中文大于false", `如果>%x% 大于 1`, "否"},
		{"中文小于", `如果>%x% 小于 2`, "是"},
		{"中文小于false", `如果>%x% 小于 1`, "否"},
		{"中文大于等于", `如果>%x% 大于等于 1`, "是"},
		{"中文小于等于", `如果>%x% 小于等于 1`, "是"},
		{"中文比较混用或", `如果>%x% 等于 3 或 %x% 等于 1`, "是"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runIf(t, "x:1\n"+c.cond+"\n是\n>否则\n否\n<如果")
			if got != c.want {
				t.Errorf("条件 %q 期望输出 %q，实际 %q", c.cond, c.want, got)
			}
		})
	}
}

// TestIfBlockCnCmpWordNoSpace 无空格包裹的中文比较词不被当作运算符，按字面比较。
func TestIfBlockCnCmpWordNoSpace(t *testing.T) {
	got := runIf(t, `s:等于
如果>%s%==等于
是
>否则
否
<如果`)
	if got != "是" {
		t.Errorf("无空格包裹的「等于」应保持字面比较，期望 是，实际 %q", got)
	}
}

// TestIfConditionFunc 通用判断函数 $判断$：空格区分参数，支持比较与逻辑运算。
func TestIfConditionFunc(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"等于或true", `$判断 a 等于 a 或 true$`, "true"},
		{"等于false", `$判断 a 等于 b$`, "false"},
		{"不等于", `$判断 1 不等于 2$`, "true"},
		{"大于", `$判断 3 大于 2$`, "true"},
		{"小于等于", `$判断 2 小于等于 2$`, "true"},
		{"逻辑且", `$判断 1 等于 1 且 2 等于 2$`, "true"},
		{"括号分组", `$判断 1 等于 2 或 (1 等于 1 且 2 等于 2)$`, "true"},
		{"变量比较", "x:1\n$判断 %x% 等于 1$", "true"},
		{"变量括号", "x:1\ny:2\n$判断 1 等于 2 或 (%x% 等于 1 且 %y% 等于 2)$", "true"},
		{"算术参数", `$判断 [1+1] 等于 2$`, "true"},
		{"取反为真", `$判断 !1 等于 2$`, "true"},
		{"取反为假", `$判断 !1 等于 1$`, "false"},
		{"变量值含判断符号相等", "a:1==2\nb:1==2\n$判断 %a% 等于 %b%$", "true"},
		{"变量值含判断符号不等", "a:1==2\nb:1==3\n$判断 %a% 等于 %b%$", "false"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runIf(t, c.body)
			if got != c.want {
				t.Errorf("期望输出 %q，实际 %q", c.want, got)
			}
		})
	}
}
