package funcs

import (
	"errors"
	"strconv"
	"strings"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"

	"github.com/cjxpj/hajimimanbo"
	"github.com/mozillazg/go-pinyin"
)

const (
	hajimimanbo_key = "cjxpj"
)

// 加密
func hajimimanboEncrypt(d *dto.DicInputs) (any, error) {
	res, err := hajimimanbo.Encrypt(d.Inputs.String(1), d.Inputs.StringDefault(2, hajimimanbo_key))
	if err != nil {
		return "", nil
	}
	return res, nil
}

// 解密
func hajimimanboDecrypt(d *dto.DicInputs) (any, error) {
	res, err := hajimimanbo.Decrypt(d.Inputs.String(1), d.Inputs.StringDefault(2, hajimimanbo_key))
	if err != nil {
		return "", nil
	}
	return res, nil
}

// 写图片
func writeImage(d *dto.DicInputs) (any, error) {
	path1 := utils.NewFileQueue(d.Inputs.String(1)).FileName
	imgdata, err := utils.SetImgData(path1, []byte(d.Inputs.String(2)))
	if err != nil {
		return "", err
	}
	return string(imgdata), nil
}

// 读图片
func readImage(d *dto.DicInputs) (any, error) {
	path1 := utils.NewFileQueue(d.Inputs.String(1)).FileName
	res, err := utils.ReadImgData(path1)
	if err != nil {
		return d.Inputs.String(2), nil
	}
	return string(res), nil
}

func repeat(d *dto.DicInputs) (any, error) {
	ff := d.Inputs.IntDefault(2, 2)
	return strings.Repeat(d.Inputs.String(1), ff), nil
}

func numberFormatting(d *dto.DicInputs) (any, error) {
	var FType byte = 'f'
	if d.Inputs.LenOk(3) {
		str := d.Inputs.String(3)
		switch str {
		case "b", "e", "E", "f", "g", "G":
			FType = str[0]
		default:
			return "", errors.New("参数3未知格式")
		}
	}
	ff, ok := d.Inputs.Float64Ok(1)
	if !ok {
		return "", errors.New("参数1非数字")
	}
	fff, ok := d.Inputs.IntOk(2)
	if !ok {
		return "", errors.New("参数2非数字")
	}
	return strconv.FormatFloat(ff, FType, fff, 64), nil
}

// 左右
func removeLR(d *dto.DicInputs) (any, error) {
	if d.Inputs.LenOk(2) {
		return strings.Trim(d.Inputs.String(1), d.Inputs.String(2)), nil
	}
	return strings.TrimSpace(d.Inputs.String(1)), nil
}

// 左
func removeL(d *dto.DicInputs) (any, error) {
	return strings.TrimLeft(d.Inputs.String(1), d.Inputs.StringDefault(2, " ")), nil
}

// 右
func removeR(d *dto.DicInputs) (any, error) {
	return strings.TrimRight(d.Inputs.String(1), d.Inputs.StringDefault(2, " ")), nil
}

// 拼接
func join(d *dto.DicInputs) (any, error) {
	var strSlice []string
	err := json.Unmarshal([]byte(d.Inputs.String(2)), &strSlice)
	if err != nil {
		return "", nil
	}
	resS := strings.Join(strSlice, d.Inputs.String(1))
	return resS, nil
}

// 拼音
func pinYin(d *dto.DicInputs) (any, error) {
	a := pinyin.NewArgs()
	a.Heteronym = true
	a.Style = pinyin.Tone2
	py := pinyin.Pinyin(d.Inputs.String(1), a)
	jsonData, err := json.Marshal(py)
	if err != nil {
		return "[]", nil
	}
	return string(jsonData), nil
}

func takeTheMiddle(d *dto.DicInputs) (any, error) {
	str := d.Inputs.String(1)
	startStr := d.Inputs.String(2)

	startIndex := strings.Index(str, startStr)
	if startIndex == -1 {
		return "", nil
	}

	startIndex += len(startStr)
	resStr := str[startIndex:]

	if d.Inputs.LenOk(3) {
		endStr := d.Inputs.String(3)
		endIndex := strings.Index(resStr, endStr)
		if endIndex == -1 {
			return "", nil
		}
		resStr = resStr[:endIndex]
	}

	return resStr, nil
}

// 截取
func intercept(d *dto.DicInputs) (any, error) {
	str := d.Inputs.String(1)
	strLen := len(str)
	var start int
	if num, ok := d.Inputs.IntOk(2); ok {
		start = num
	} else {
		return "", errors.New("参数2所需为数字")
	}

	if start < 0 || start >= strLen {
		return "", nil
	}

	if d.Inputs.LenOk(3) {
		var length int
		if num, ok := d.Inputs.IntOk(3); ok {
			length = num
		} else {
			length = strLen - start
		}

		if length < 0 || start+length > strLen {
			return str[start:], nil
		}
		return str[start : start+length], nil
	}
	return str[start:], nil
}

// SubStrHead 取前 n 个字符
func subStrHead(d *dto.DicInputs) (any, error) {
	n := d.Inputs.IntDefault(1, 1) // 截取长度，默认 1
	str := d.Inputs.String(2)      // 待处理的字符串

	if n <= 0 {
		return "", nil
	}

	// 防止越界
	if n > len([]rune(str)) {
		n = len([]rune(str))
	}

	// 用 rune 保证中文不会被截断
	runes := []rune(str)
	return string(runes[:n]), nil
}

// SubStrTail 取后 n 个字符
func subStrTail(d *dto.DicInputs) (any, error) {
	n := d.Inputs.IntDefault(1, 1) // 截取长度，默认 1
	str := d.Inputs.String(2)      // 待处理的字符串

	if n <= 0 {
		return "", nil
	}

	runes := []rune(str)
	l := len(runes)

	// 防止越界
	if n > l {
		n = l
	}

	return string(runes[l-n:]), nil
}

func find(d *dto.DicInputs) (any, error) {
	return strconv.Itoa(strings.Index(d.Inputs.String(1), d.Inputs.String(2))), nil
}

// NumToString 将数字字符串转化为中文表示（支持无限大数）
func numToString(d *dto.DicInputs) (any, error) {
	strNum := d.Inputs.String(1)

	numMap := []string{"零", "一", "二", "三", "四", "五", "六", "七", "八", "九"}
	unitSmall := []string{"", "十", "百", "千"}

	// 常见大数单位表（按4位分组）
	unitBig := []string{"", "万", "亿", "兆", "京", "垓", "秭", "穰", "沟", "涧", "正", "载"}

	// 超出已知范围的自动生成，比如 "万^12"
	getBigUnit := func(idx int) string {
		if idx < len(unitBig) {
			return unitBig[idx]
		}
		return "万^" + strconv.Itoa(idx)
	}

	isNegative := false
	if strings.HasPrefix(strNum, "-") {
		isNegative = true
		strNum = strNum[1:]
	}

	var integerPart, decimalPart string
	if dotIdx := strings.Index(strNum, "."); dotIdx != -1 {
		integerPart = strNum[:dotIdx]
		decimalPart = strNum[dotIdx+1:]
	} else {
		integerPart = strNum
	}

	if len(integerPart) == 0 {
		return "", nil
	}

	var result strings.Builder
	if isNegative {
		result.WriteString("负")
	}

	// 处理整数部分（分组处理，每四位一个节）
	groupCount := (len(integerPart) + 3) / 4
	parts := make([]string, groupCount)

	for g := 0; g < groupCount; g++ {
		start := len(integerPart) - (g+1)*4
		if start < 0 {
			start = 0
		}
		end := len(integerPart) - g*4
		group := integerPart[start:end]

		groupStr := ""
		zeroFlag := false
		for i, c := range group {
			num := int(c - '0')
			unitIdx := len(group) - i - 1
			if num == 0 {
				zeroFlag = true
			} else {
				if zeroFlag {
					groupStr += "零"
					zeroFlag = false
				}
				groupStr += numMap[num] + unitSmall[unitIdx]
			}
		}

		if groupStr != "" {
			groupStr += getBigUnit(g)
		}
		parts[groupCount-g-1] = groupStr
	}

	// 拼接所有部分
	finalStr := strings.Join(parts, "")

	// 处理 "一十" 开头的情况
	if strings.HasPrefix(finalStr, "一十") {
		finalStr = finalStr[3:]
	}
	if strings.HasPrefix(finalStr, "负一十") {
		finalStr = "负" + finalStr[6:]
	}

	// 小数部分
	if len(decimalPart) > 0 {
		finalStr += "点"
		for _, c := range decimalPart {
			num := int(c - '0')
			finalStr += numMap[num]
		}
	}

	return finalStr, nil
}
