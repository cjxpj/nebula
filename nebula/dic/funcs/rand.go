package funcs

import (
	"math/rand"
	"strconv"
	"strings"

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

func (f *DicFunc) RandString() string {
	if f.Len == 1 {
		r := []rune(f.Inputs.String(1))
		min := 0
		max := len(r) - 1

		rN := utils.RandNum(min, max)
		if rN == min-1 {
			return ""
		}

		randomChar := r[rN]
		resStr := string(randomChar)
		return resStr
	}
	if f.Len == 2 {
		r := strings.Split(f.Inputs.String(2), f.Inputs.String(1))
		min := 0
		max := len(r) - 1

		rN := utils.RandNum(min, max)
		if rN == min-1 {
			return ""
		}

		resStr := r[rN]
		return resStr
	}
	return ""
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
