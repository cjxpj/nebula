package dic

import (
	"testing"

	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/run"
)

func BenchmarkLoopArith(b *testing.B) {
	chdirToAppWin()
	body := "Main\n循环>i=500000\nn+:%i%\n<循环\n%n%"

	for b.Loop() {
		D := dic_dto.NewDic("t.n", body)
		dic_api.Api.DicRun(D, "Main")
	}
}

// BenchmarkRealDebugA 真实 debug.n 的 a 触发词（循环>i=5000000）。
func BenchmarkRealDebugA(b *testing.B) {
	chdirToAppWin()

	for b.Loop() {
		D := dic_dto.NewDic("t.n", "")
		// 直接以 500 万循环体构造等价词库，避免读盘/缓存干扰纯执行耗时
		D.Data = run.BuildDicLinesWithRaw("t.n",
			[]string{"Main", "循环>i=5000000", "n+:%i%", "<循环", "%n%"},
			[]byte("Main\n循环>i=5000000\nn+:%i%\n<循环\n%n%"))
		dic_api.Api.DicRun(D, "Main")
	}
}
