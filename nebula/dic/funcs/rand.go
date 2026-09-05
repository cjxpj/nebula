package funcs

import (
	"math/rand"
	"strconv"
	"strings"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// 随机大小写字母和数字
func (f *DicFunc) RandLetter(mod int) string {
	if f.Len != 1 {
		return "参数错误"
	}
	num := f.Inputs.Int(1)
	if num <= 0 {
		return ""
	}

	var charset string
	switch mod {
	case 0:
		charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	case 1:
		charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	case 2:
		charset = "abcdefghijklmnopqrstuvwxyz"
	case 3:
		charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	case 4:
		charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	case 5:
		charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	case 6:
		charset = "0123456789"
	default:
		charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ" // 默认大小写
	}

	var result strings.Builder
	for range num {
		index := rand.Intn(len(charset))
		result.WriteByte(charset[index])
	}

	return result.String()
}

// 随机文本
func randString(d *dto.DicInputs) (any, error) {
	if d.Inputs.LenOk(1) {
		r := []rune(d.Inputs.String(1))
		min := 0
		max := len(r) - 1

		rN := utils.RandNum(min, max)
		if rN == min-1 {
			return "", nil
		}

		randomChar := r[rN]
		resStr := string(randomChar)
		return resStr, nil
	}
	r := strings.Split(d.Inputs.String(2), d.Inputs.String(1))
	min := 0
	max := len(r) - 1

	rN := utils.RandNum(min, max)
	if rN == min-1 {
		return "", nil
	}

	resStr := r[rN]
	return resStr, nil
}

func (f *DicFunc) RandNum() string {
	if f.Len == 2 {
		if min, err := strconv.Atoi(f.Inputs.String(1)); err == nil {
			if max, err := strconv.Atoi(f.Inputs.String(2)); err == nil {
				rN := utils.RandNum(min, max)
				if rN == min-1 {
					return ""
				}
				return strconv.Itoa(rN)
			}
		}
	}
	return ""
}

func doRandNum(d *dto.DicInputs) (any, error) {
	if min, err := strconv.Atoi(d.Inputs.String(1)); err == nil {
		if max, err := strconv.Atoi(d.Inputs.String(2)); err == nil {
			rN := utils.RandNum(min, max)
			if rN == min-1 {
				return "", nil
			}
			return strconv.Itoa(rN), nil
		}
	}
	return "", nil
}

func randLetterHelper(d *dto.DicInputs, mod int) (any, error) {
	num := d.Inputs.Int(1)
	if num <= 0 {
		return "", nil
	}

	var charset string
	switch mod {
	case 0:
		charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	case 1:
		charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	case 2:
		charset = "abcdefghijklmnopqrstuvwxyz"
	case 3:
		charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	case 4:
		charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	case 5:
		charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	case 6:
		charset = "0123456789"
	default:
		charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	}

	var result strings.Builder
	for range num {
		index := rand.Intn(len(charset))
		result.WriteByte(charset[index])
	}

	return result.String(), nil
}

func randLetterUpperLower(d *dto.DicInputs) (any, error)    { return randLetterHelper(d, 0) }
func randLetterUpper(d *dto.DicInputs) (any, error)         { return randLetterHelper(d, 1) }
func randLetterLower(d *dto.DicInputs) (any, error)         { return randLetterHelper(d, 2) }
func randLetterUpperLowerNum(d *dto.DicInputs) (any, error) { return randLetterHelper(d, 3) }
func randLetterLowerNum(d *dto.DicInputs) (any, error)      { return randLetterHelper(d, 4) }
func randLetterUpperNum(d *dto.DicInputs) (any, error)      { return randLetterHelper(d, 5) }
func randNumber(d *dto.DicInputs) (any, error)              { return randLetterHelper(d, 6) }

// 设置随机种子：使后续随机数序列可复现。
func randSeed(d *dto.DicInputs) (any, error) {
	rand.Seed(d.Inputs.Int64(1))
	return "", nil
}

// 随机小数：无参返回 [0,1) 均匀分布；两个参数返回 [min,max) 均匀分布。
func randFloat(d *dto.DicInputs) (any, error) {
	if d.Inputs.Len() >= 2 {
		min := d.Inputs.Float64(1)
		max := d.Inputs.Float64(2)
		if min > max {
			min, max = max, min
		}
		return min + rand.Float64()*(max-min), nil
	}
	return rand.Float64(), nil
}

// 正态分布：无参返回标准正态（均值 0、标准差 1）；两个参数返回 N(均值, 标准差)。
func randNormal(d *dto.DicInputs) (any, error) {
	mean := 0.0
	std := 1.0
	if d.Inputs.Len() >= 2 {
		mean = d.Inputs.Float64(1)
		std = d.Inputs.Float64(2)
	}
	return rand.NormFloat64()*std + mean, nil
}

// 指数分布：参数 λ（率参数）可省略（默认 1），均值 = 1/λ。
func randExpDist(d *dto.DicInputs) (any, error) {
	lambda := 1.0
	if d.Inputs.Len() >= 1 {
		if v := d.Inputs.Float64(1); v > 0 {
			lambda = v
		}
	}
	return rand.ExpFloat64() / lambda, nil
}
