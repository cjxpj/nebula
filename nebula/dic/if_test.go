package dic

import (
	"sync"
	"testing"

	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
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

// TestLifecycleHooks 生命周期钩子执行模型：
//   - [f]_初始化：首次加载该词库内容时执行一次，变量快照带入后续每次执行；
//   - 头部 与 [f]_中间件 二选一：已定义 [f]_中间件 时覆盖头部，头部运行时语句不执行；
//   - [f]_中间件：每次执行都会经过；
//   - 触发词/正文正常匹配，初始化快照不覆盖本次的触发词。
func TestLifecycleHooks(t *testing.T) {
	chdirToAppWin()
	initOutputCache = sync.Map{} // 重置缓存，避免与其他测试相互影响

	const path = "lifecycle_test.n"
	// 头部写赋值 + 计数；_中间件 也写赋值 + 计数；初始化只执行一次。
	// 已定义 _中间件 时头部被覆盖，故头部赋值/计数不应发生。
	const text = "$计数头部$\n" +
		"头部前缀:head\n" +
		"\n" +
		"[f]_初始化\n" +
		"$计数初始化$\n" +
		"初始化前缀:init\n" +
		"\n" +
		"[f]_中间件\n" +
		"$计数中间件$\n" +
		"中间件前缀:mid\n" +
		"\n" +
		"Main\n" +
		"%头部前缀%|%初始化前缀%|%中间件前缀%\n" +
		"\n" +
		"第二\n" +
		"%触发词%|%头部前缀%|%初始化前缀%|%中间件前缀%"

	var headRuns, initRuns, midRuns int
	mkCounter := func(p *int) dto.DicFunc {
		return dto.DicFunc{L: "0", Fn: func(d *dto.DicInputs) (any, error) {
			*p++
			return "", nil
		}}
	}
	newDic := func() *dic_dto.Dic {
		return dic_dto.NewDic(path, text).
			SetFunc("计数头部", mkCounter(&headRuns)).
			SetFunc("计数初始化", mkCounter(&initRuns)).
			SetFunc("计数中间件", mkCounter(&midRuns))
	}

	// 头部被覆盖：%头部前缀% 未赋值，保持字面量原样输出；只有初始化与中间件赋值生效。
	got1 := dic_api.Api.DicRun(newDic(), "Main")
	if got1 != "%头部前缀%|init|mid" {
		t.Fatalf("首次执行期望 %%头部前缀%%|init|mid（头部被覆盖），实际 %q", got1)
	}
	got2 := dic_api.Api.DicRun(newDic(), "第二")
	if got2 != "第二|%头部前缀%|init|mid" {
		t.Fatalf("第二次执行期望 第二|%%头部前缀%%|init|mid，实际 %q", got2)
	}
	if initRuns != 1 {
		t.Fatalf("_初始化 应只执行一次，实际 %d 次", initRuns)
	}
	if headRuns != 0 {
		t.Fatalf("头部被 _中间件 覆盖，应不执行，实际 %d 次", headRuns)
	}
	if midRuns != 2 {
		t.Fatalf("_中间件 应每次执行，实际 %d 次", midRuns)
	}
}

// TestHeadOnlyAsMiddleware 只写头部、未定义 [f]_中间件 时，头部内容即中间件，每次执行都会经过。
func TestHeadOnlyAsMiddleware(t *testing.T) {
	chdirToAppWin()
	initOutputCache = sync.Map{}

	const path = "head_only_middleware_test.n"
	const text = "$计数头部$\n" +
		"头部前缀:head\n" +
		"\n" +
		"Main\n" +
		"%头部前缀%"

	var headRuns int
	newDic := func() *dic_dto.Dic {
		return dic_dto.NewDic(path, text).
			SetFunc("计数头部", dto.DicFunc{L: "0", Fn: func(d *dto.DicInputs) (any, error) {
				headRuns++
				return "", nil
			}})
	}

	if got := dic_api.Api.DicRun(newDic(), "Main"); got != "head" {
		t.Fatalf("只写头部时头部内容应作为中间件执行，期望 head，实际 %q", got)
	}
	if got := dic_api.Api.DicRun(newDic(), "Main"); got != "head" {
		t.Fatalf("头部中间件应每次执行，期望 head，实际 %q", got)
	}
	if headRuns != 2 {
		t.Fatalf("头部中间件应每次执行，实际 %d 次", headRuns)
	}
}

// TestInitHaltPersists [f]_初始化 里的 >终止 应持久生效：初始化只执行一次，
// 命中缓存后仍要恢复「终止」状态，避免首次执行被终止、后续执行又继续跑正文的不一致行为。
func TestInitHaltPersists(t *testing.T) {
	chdirToAppWin()
	initOutputCache = sync.Map{}

	const path = "init_halt_test.n"
	const text = "\n[f]_初始化\n>终止\n\nMain\n正文"

	newDic := func() *dic_dto.Dic {
		return dic_dto.NewDic(path, text)
	}

	if got := dic_api.Api.DicRun(newDic(), "Main"); got != "" {
		t.Fatalf("首次执行应被初始化 >终止，期望空输出，实际 %q", got)
	}
	if got := dic_api.Api.DicRun(newDic(), "Main"); got != "" {
		t.Fatalf("命中初始化缓存后仍应被 >终止，期望空输出，实际 %q", got)
	}
}
