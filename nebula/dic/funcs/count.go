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

func doCount(d *dto.DicInputs) (any, error) {
	l := d.Inputs.Len()
	if l < 2 {
		return "", nil
	}

	parseBigFloat := func(input string) (*big.Float, bool) {
		floatVal := new(big.Float).SetPrec(128)
		_, ok := floatVal.SetString(input)
		return floatVal, ok
	}

	// 解析第一个操作数
	result, ok := parseBigFloat(d.Inputs.String(1))
	if !ok {
		return "Error", nil
	}

	// 依次处理运算符和操作数
	for i := 2; i <= l; i++ {
		op := d.Inputs.String(i)

		// 一元运算：根号
		if op == "根号" || op == "sqrt" {
			if result.Cmp(big.NewFloat(0)) < 0 {
				return "Error", nil
			}
			result.Sqrt(result)
			continue
		}

		// 四舍五入：下一个参数是小数位数
		if op == "四舍五入" {
			i++
			var digits *big.Float
			if i <= l {
				var ok bool
				digits, ok = parseBigFloat(d.Inputs.String(i))
				if !ok {
					return "Error", nil
				}
			} else {
				digits = big.NewFloat(0)
			}
			twoInt64, _ := digits.Int64()
			factor := new(big.Float).SetFloat64(math.Pow(10, float64(twoInt64)))
			scaled := new(big.Float).Mul(result, factor)
			rounded := new(big.Float).Add(scaled, big.NewFloat(0.5))
			roundedInt := new(big.Int)
			rounded.Int(roundedInt)
			rounded.SetInt(roundedInt)
			rounded.Quo(rounded, factor)
			result = rounded
			continue
		}

		// 二元运算：需要操作数
		i++
		if i > l {
			return "Error", nil
		}

		operand, ok := parseBigFloat(d.Inputs.String(i))
		if !ok {
			return "Error", nil
		}

		switch op {
		case "加", "+":
			result.Add(result, operand)
		case "减", "-":
			result.Sub(result, operand)
		case "乘", "*":
			result.Mul(result, operand)
		case "除", "/":
			if operand.Cmp(big.NewFloat(0)) == 0 {
				return "0", nil
			}
			result.Quo(result, operand)
		case "^", "幂", "次方":
			roundedExp := new(big.Float).SetPrec(128).Set(operand)
			roundedExp.Add(roundedExp, big.NewFloat(0.5))
			expInt := new(big.Int)
			roundedExp.Int(expInt)
			isNegative := expInt.Sign() < 0
			if isNegative {
				expInt.Abs(expInt)
			}
			powResult := new(big.Float).SetPrec(128).SetFloat64(1)
			base := new(big.Float).SetPrec(128).Set(result)
			for expInt.Sign() > 0 {
				if expInt.Bit(0) == 1 {
					powResult.Mul(powResult, base)
				}
				base.Mul(base, base)
				expInt.Rsh(expInt, 1)
			}
			if isNegative {
				if powResult.Cmp(big.NewFloat(0)) == 0 {
					return "Error", nil
				}
				powResult.Quo(big.NewFloat(1), powResult)
			}
			result = powResult
		case "整除":
			oneInt := new(big.Int)
			twoInt := new(big.Int)
			result.Int(oneInt)
			operand.Int(twoInt)
			sumInt := new(big.Int).Div(oneInt, twoInt)
			result.SetInt(sumInt)
		case "除余", "%":
			oneInt := new(big.Int)
			twoInt := new(big.Int)
			result.Int(oneInt)
			operand.Int(twoInt)
			sumInt := new(big.Int).Mod(oneInt, twoInt)
			result.SetInt(sumInt)
		case "按位或", "|":
			oneInt := new(big.Int)
			twoInt := new(big.Int)
			result.Int(oneInt)
			operand.Int(twoInt)
			sumInt := new(big.Int).Or(oneInt, twoInt)
			result.SetInt(sumInt)
		case "按位与", "&":
			oneInt := new(big.Int)
			twoInt := new(big.Int)
			result.Int(oneInt)
			operand.Int(twoInt)
			sumInt := new(big.Int).And(oneInt, twoInt)
			result.SetInt(sumInt)
		case "右移", ">>":
			oneInt := new(big.Int)
			twoInt := new(big.Int)
			result.Int(oneInt)
			operand.Int(twoInt)
			sumInt := new(big.Int).Rsh(oneInt, uint(twoInt.Uint64()))
			result.SetInt(sumInt)
		case "左移", "<<":
			oneInt := new(big.Int)
			twoInt := new(big.Int)
			result.Int(oneInt)
			operand.Int(twoInt)
			sumInt := new(big.Int).Lsh(oneInt, uint(twoInt.Uint64()))
			result.SetInt(sumInt)
		default:
			return "未知算法", nil
		}
	}

	return result.Text('f', -1), nil
}
