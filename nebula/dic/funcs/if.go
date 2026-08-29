package funcs

import (
	"github.com/cjxpj/nebula/count"
	"github.com/cjxpj/nebula/dic/cond"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// ifCondition 通用判断：$判断 a 等于 a 或 true$，参数以空格区分，
// 支持 等于/不等于/大于/小于/大于等于/小于等于 与 或/且/或者/并且、()、! 等条件语法。
// 优先用未展开的原始参数（Raw）拼接条件，再按 operand 逐个求值 %变量%/[算术]，
// 避免变量值本身含判断符号（如 ==、>、或 等）被当作运算符误拆。
func ifCondition(d *dto.DicInputs) (any, error) {
	raw := d.Raw
	if raw == nil {
		raw = d.Inputs
	}
	res, err := cond.Eval(raw.StringAfter(1), func(operand string) string {
		return utils.AnyToString(d.V.Text(count.RunCountText(d.V, operand)))
	})
	if err != nil {
		return "", err
	}
	if res {
		return "true", nil
	}
	return "false", nil
}

func ifNull(d *dto.DicInputs) (any, error) {
	switch d.Inputs.String(1) {
	case "null",
		"nil",
		"false",
		"{}",
		"[]",
		"空",
		"NaN",
		"undefined",
		" ",
		"":
		return "true", nil
	}
	return "false", nil
}

func ifNONull(d *dto.DicInputs) (any, error) {
	switch d.Inputs.String(1) {
	case "null",
		"nil",
		"false",
		"{}",
		"[]",
		"空",
		"NaN",
		"undefined",
		" ",
		"":
		return "false", nil
	}
	return "true", nil
}
