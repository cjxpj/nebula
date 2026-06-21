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
		case "^", "幂", "次方":
			// 幂运算：oneFloat ^ round(twoFloat)

			// 1. 指数四舍五入
			roundedExp := new(big.Float).SetPrec(128).Set(twoFloat)
			roundedExp.Add(roundedExp, big.NewFloat(0.5))

			expInt := new(big.Int)
			roundedExp.Int(expInt)

			// 2. 判断负指数
			isNegative := expInt.Sign() < 0
			if isNegative {
				expInt.Abs(expInt)
			}

			// 3. 快速幂
			result := new(big.Float).SetPrec(128).SetFloat64(1)
			base := new(big.Float).SetPrec(128).Set(oneFloat)

			for expInt.Sign() > 0 {
				if expInt.Bit(0) == 1 {
					result.Mul(result, base)
				}
				base.Mul(base, base)
				expInt.Rsh(expInt, 1)
			}

			// 4. 负指数取倒数
			if isNegative {
				if result.Cmp(big.NewFloat(0)) == 0 {
					return "Error"
				}
				result.Quo(big.NewFloat(1), result)
			}

			return result.Text('f', -1)

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

func doCount(d *dto.DicInputs) (any, error) {
	if d.Inputs.Len() == 3 {
		parseBigFloat := func(input string) (*big.Float, bool) {
			floatVal := new(big.Float).SetPrec(128)
			_, ok := floatVal.SetString(input)
			return floatVal, ok
		}

		oneFloat, ok1 := parseBigFloat(d.Inputs.String(1))
		twoFloat, ok2 := parseBigFloat(d.Inputs.String(3))

		if !ok1 || !ok2 {
			return "Error", nil
		}

		var sumFloat big.Float
		switch d.Inputs.String(2) {
		case "加", "+":
			sumFloat.Add(oneFloat, twoFloat)
		case "减", "-":
			sumFloat.Sub(oneFloat, twoFloat)
		case "乘", "*":
			sumFloat.Mul(oneFloat, twoFloat)
		case "除", "/":
			if twoFloat.Cmp(big.NewFloat(0)) == 0 {
				return "0", nil
			}
			sumFloat.Quo(oneFloat, twoFloat)
		case "^", "幂", "次方":
			roundedExp := new(big.Float).SetPrec(128).Set(twoFloat)
			roundedExp.Add(roundedExp, big.NewFloat(0.5))

			expInt := new(big.Int)
			roundedExp.Int(expInt)

			isNegative := expInt.Sign() < 0
			if isNegative {
				expInt.Abs(expInt)
			}

			result := new(big.Float).SetPrec(128).SetFloat64(1)
			base := new(big.Float).SetPrec(128).Set(oneFloat)

			for expInt.Sign() > 0 {
				if expInt.Bit(0) == 1 {
					result.Mul(result, base)
				}
				base.Mul(base, base)
				expInt.Rsh(expInt, 1)
			}

			if isNegative {
				if result.Cmp(big.NewFloat(0)) == 0 {
					return "Error", nil
				}
				result.Quo(big.NewFloat(1), result)
			}

			return result.Text('f', -1), nil
		case "整除":
			oneInt := new(big.Int)
			twoInt := new(big.Int)
			oneFloat.Int(oneInt)
			twoFloat.Int(twoInt)
			sumInt := new(big.Int).Div(oneInt, twoInt)
			return sumInt.String(), nil
		case "除余", "%":
			oneInt := new(big.Int)
			twoInt := new(big.Int)
			oneFloat.Int(oneInt)
			twoFloat.Int(twoInt)
			sumInt := new(big.Int).Mod(oneInt, twoInt)
			return sumInt.String(), nil
		case "四舍五入":
			twoInt64, _ := twoFloat.Int64()
			factor := new(big.Float).SetFloat64(math.Pow(10, float64(twoInt64)))
			scaled := new(big.Float).Mul(oneFloat, factor)
			rounded := new(big.Float).Add(scaled, big.NewFloat(0.5))
			roundedInt := new(big.Int)
			rounded.Int(roundedInt)
			rounded.SetInt(roundedInt)
			rounded.Quo(rounded, factor)
			return rounded.Text('f', -1), nil
		case "按位或", "|":
			oneInt := new(big.Int)
			twoInt := new(big.Int)
			oneFloat.Int(oneInt)
			twoFloat.Int(twoInt)
			sumInt := new(big.Int).Or(oneInt, twoInt)
			return sumInt.String(), nil
		case "按位与", "&":
			oneInt := new(big.Int)
			twoInt := new(big.Int)
			oneFloat.Int(oneInt)
			twoFloat.Int(twoInt)
			sumInt := new(big.Int).And(oneInt, twoInt)
			return sumInt.String(), nil
		case "右移", ">>":
			oneInt := new(big.Int)
			twoInt := new(big.Int)
			oneFloat.Int(oneInt)
			twoFloat.Int(twoInt)
			sumInt := new(big.Int).Rsh(oneInt, uint(twoInt.Uint64()))
			return sumInt.String(), nil
		case "左移", "<<":
			oneInt := new(big.Int)
			twoInt := new(big.Int)
			oneFloat.Int(oneInt)
			twoFloat.Int(twoInt)
			sumInt := new(big.Int).Lsh(oneInt, uint(twoInt.Uint64()))
			return sumInt.String(), nil
		case "根号", "sqrt":
			if oneFloat.Cmp(big.NewFloat(0)) < 0 {
				return "Error", nil
			}
			sumFloat.Sqrt(oneFloat)
		default:
			return "未知算法", nil
		}

		return sumFloat.Text('f', -1), nil
	}
	return "", nil
}
