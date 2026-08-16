package dic

import (
	"testing"

	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
)

// TestFuncAfterLoopBlock 回归测试：循环/遍历/判断块执行完毕后，
// 局部函数（含 #引入 注入到「函数」类别的词条）仍应可被 $函数$ 调用。
func TestFuncAfterLoopBlock(t *testing.T) {
	chdirToAppWin()

	const funcDef = "\n[函数]test\n引入函数可用\n\nMain\n"

	cases := []struct {
		name string
		body string
	}{
		{"循环", "循环>i\n>终止循环\n<循环\n$test$"},
		{"遍历", "遍历>i=[1]\n>终止遍历\n<遍历\n$test$"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			D := dic_dto.NewDic("t.n", funcDef+c.body)
			got := dic_api.Api.DicRun(D, "Main")
			if got != "引入函数可用" {
				t.Errorf("%s 块后函数失效，期望 引入函数可用，实际 %q", c.name, got)
			}
		})
	}
}
