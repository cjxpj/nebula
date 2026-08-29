package dic

import (
	"github.com/cjxpj/nebula/count"
	"github.com/cjxpj/nebula/dic/cond"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/utils"
)

// Pd 判断条件表达式真假，复用词库「如果>」的完整条件语法（等于/不等于/大于/小于/或/且 等）。
// 操作数经 %变量%/[算术]/$函数$ 解析后参与比较。
func Pd(dic *dic_dto.DicFunc, str string) bool {
	res, err := cond.Eval(str, func(operand string) string {
		return utils.AnyToString(Runs(dic, utils.AnyToString(count.RunCountText(dic.Val, operand))))
	})
	if err != nil {
		dic.Output.Add(err.Error())
		dic.Sys.Stop.Store(true)
		return false
	}
	return res
}
