package dic

import (
	"testing"

	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
)

// TestContinueTrigger 验证 $继续执行$：从当前触发词命中的下一个词条，按正常触发词匹配规则
// （纯文本优先、正则按序）只执行紧邻的下一个命中词条。
func TestContinueTrigger(t *testing.T) {
	chdirToAppWin()

	// 两个 Main 词条：第一个 Main 调用 $继续执行$ 触发第二个 Main
	D := dic_dto.NewDic("t.n", "\nMain\n第一个$继续执行$\n\nMain\n第二个")
	if got := dic_api.Api.DicRun(D, "Main"); got != "第一个第二个" {
		t.Fatalf("继续执行级联错误，期望 第一个第二个，实际 %q", got)
	}

	// 两个相同正则触发词词条：都匹配同一输入，第一个调用 $继续执行$ 触发第二个
	D2 := dic_dto.NewDic("t.n", "\n测试 .+\n第一个$继续执行$\n\n测试 .+\n第二个")
	if got := dic_api.Api.DicRun(D2, "测试 abc"); got != "第一个第二个" {
		t.Fatalf("正则触发词继续执行错误，期望 第一个第二个，实际 %q", got)
	}

	// 后面跟正则 .*（也匹配 Main）：$继续执行$ 只往下走一个，命中第二个 Main，不误执行 .*
	D3 := dic_dto.NewDic("t.n", "\nMain\na\\r$继续执行$\n\nMain\nok2\\r\n\n.*\nok3")
	if got := dic_api.Api.DicRun(D3, "Main"); got != "a\nok2\n" {
		t.Fatalf("继续执行不应跨到 .*，期望 a\\nok2\\n，实际 %q", got)
	}

	// 三个 Main：$继续执行$ 只触发紧邻的下一个，第三个需第二个再调用才执行
	D4 := dic_dto.NewDic("t.n", "\nMain\na\\r$继续执行$\n\nMain\nok2\\r\n\nMain\nok3\\r")
	if got := dic_api.Api.DicRun(D4, "Main"); got != "a\nok2\n" {
		t.Fatalf("继续执行应只触发下一个，期望 a\\nok2\\n，实际 %q", got)
	}

	// 正则触发词 M.* 也匹配 Main：$继续执行$ 按正则匹配，命中紧邻的 M.*
	D5 := dic_dto.NewDic("t.n", "\nMain\na\\r$继续执行$\n\nM.*\nok5\n\nMain\nok2\\r")
	if got := dic_api.Api.DicRun(D5, "Main"); got != "a\nok5" {
		t.Fatalf("继续执行应正则匹配 M.*，期望 a\\nok5，实际 %q", got)
	}

	// M.* 里也带 $继续执行$：级联往下逐级执行，且下标正确递增、不无限递归
	D6 := dic_dto.NewDic("t.n", "\nMain\na\\r$继续执行$\n\nM.*\nok5$继续执行$\n\nMain\nok2\\r")
	if got := dic_api.Api.DicRun(D6, "Main"); got != "a\nok5ok2\n" {
		t.Fatalf("继续执行逐级往下错误，期望 a\\nok5ok2\\n，实际 %q", got)
	}
}
