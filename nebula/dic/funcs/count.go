package funcs

import (
	"math"
	"math/big"

	"github.com/cjxpj/nebula/dto"
)

// 四舍五入
func round(d *dto.DicInputs) (any, error) {
	one := d.Inputs.BigFloat(1)
	two := d.Inputs.BigFloat(2)

	// 小数位数
	twoInt64, _ := two.Int64()
	// factor = 10 ^ digits
	factor := new(big.Float).SetFloat64(
		math.Pow(10, float64(twoInt64)),
	)

	// scaled = one * factor
	scaled := new(big.Float).Mul(one, factor)

	// +0.5 实现四舍五入
	rounded := new(big.Float).Add(scaled, big.NewFloat(0.5))

	// 转整数
	roundedInt := new(big.Int)
	rounded.Int(roundedInt)

	// 再除回去
	result := new(big.Float).SetInt(roundedInt)
	result.Quo(result, factor)

	return result.Text('f', -1), nil
}

func (f *DicFunc) Count() string {
	if f.Len == 3 {

		// 检查并转换为大整数或大浮点数
		parseBigFloat := func(input string) (*big.Float, bool) {
			floatVal := new(big.Float).SetPrec(128) // 设置一个合理的默认精度
			_, ok := floatVal.SetString(input)
			return floatVal, ok
		}

		// 获取输入的值
		oneFloat, ok1 := parseBigFloat(f.Inputs.String(1))
		twoFloat, ok2 := parseBigFloat(f.Inputs.String(3))

		if !ok1 || !ok2 {
			return "Error"
		}

		// 默认使用 big.Float 进行运算
		var sumFloat big.Float
		switch f.Inputs.String(2) {
		case "加", "+":
			sumFloat.Add(oneFloat, twoFloat)
		case "减", "-":
			sumFloat.Sub(oneFloat, twoFloat)
		case "乘", "*":
			sumFloat.Mul(oneFloat, twoFloat)
		case "除", "/":
			if twoFloat.Cmp(big.NewFloat(0)) == 0 {
				return "0"
			}
			sumFloat.Quo(oneFloat, twoFloat)
		case "整除":
			oneInt := new(big.Int)
			twoInt := new(big.Int)
			oneFloat.Int(oneInt)
			twoFloat.Int(twoInt)
			sumInt := new(big.Int).Div(oneInt, twoInt)
			return sumInt.String()
		case "除余", "%":
			oneInt := new(big.Int)
			twoInt := new(big.Int)
			oneFloat.Int(oneInt)
			twoFloat.Int(twoInt)
			sumInt := new(big.Int).Mod(oneInt, twoInt)
			return sumInt.String()
		case "四舍五入":
			twoInt64, _ := twoFloat.Int64()
			factor := new(big.Float).SetFloat64(math.Pow(10, float64(twoInt64)))
			scaled := new(big.Float).Mul(oneFloat, factor)
			rounded := new(big.Float).Add(scaled, big.NewFloat(0.5))
			roundedInt := new(big.Int)
			rounded.Int(roundedInt)
			rounded.SetInt(roundedInt)
			rounded.Quo(rounded, factor)
			return rounded.Text('f', -1)
		case "按位或", "|": // 按位或运算
			oneInt := new(big.Int)
			twoInt := new(big.Int)
			oneFloat.Int(oneInt)
			twoFloat.Int(twoInt)
			sumInt := new(big.Int).Or(oneInt, twoInt)
			return sumInt.String()
		case "按位与", "&": // 按位与运算
			oneInt := new(big.Int)
			twoInt := new(big.Int)
			oneFloat.Int(oneInt)
			twoFloat.Int(twoInt)
			sumInt := new(big.Int).And(oneInt, twoInt)
			return sumInt.String()
		case "右移", ">>": // 右移运算
			oneInt := new(big.Int)
			twoInt := new(big.Int)
			oneFloat.Int(oneInt)
			twoFloat.Int(twoInt)
			sumInt := new(big.Int).Rsh(oneInt, uint(twoInt.Uint64()))
			return sumInt.String()
		case "左移", "<<": // 左移运算
			oneInt := new(big.Int)
			twoInt := new(big.Int)
			oneFloat.Int(oneInt)
			twoFloat.Int(twoInt)
			sumInt := new(big.Int).Lsh(oneInt, uint(twoInt.Uint64()))
			return sumInt.String()
		case "根号", "sqrt": // 平方根运算
			if oneFloat.Cmp(big.NewFloat(0)) < 0 {
				return "Error" // 平方根不支持负数
			}
			sumFloat.Sqrt(oneFloat)
		default:
			return "未知算法"
		}

		return sumFloat.Text('f', -1)
	}
	return ""
}
