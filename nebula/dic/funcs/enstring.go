package funcs

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func (f *DicFunc) EnUtf8() string {
	if f.Len == 3 && f.Inputs.String(1) == "ANSI" {
		text := f.Inputs.String(3)     // 要着色的文本
		colorStr := f.Inputs.String(2) // 颜色参数，如 "红" 或 "31"

		// 支持的颜色映射
		colorMap := map[string]int{
			"黑": 30, "红": 31, "绿": 32, "黄": 33,
			"蓝": 34, "紫": 35, "青": 36, "白": 37,
		}

		var colorCode int
		if code, ok := colorMap[colorStr]; ok {
			colorCode = code
		} else if c, err := strconv.Atoi(colorStr); err == nil {
			colorCode = c
		} else {
			return "颜色无效"
		}

		return fmt.Sprintf("\033[%dm%s\033[0m", colorCode, text)
	}
	if f.Len != 2 {
		return ""
	}

	utf8Str := f.Inputs.String(2)

	switch f.Inputs.String(1) {
	case "GBK":
		encoder := simplifiedchinese.GBK.NewEncoder()
		encodedStr, _, err := transform.String(encoder, utf8Str)
		if err != nil {
			return "error"
		}
		return encodedStr

	case "GB18030":
		encoder := simplifiedchinese.GB18030.NewEncoder()
		encodedStr, _, err := transform.String(encoder, utf8Str)
		if err != nil {
			return "error"
		}
		return encodedStr

	case "HZGB2312":
		encoder := simplifiedchinese.HZGB2312.NewEncoder()
		encodedStr, _, err := transform.String(encoder, utf8Str)
		if err != nil {
			return "error"
		}
		return encodedStr

	case "ASCII":
		asciiCodes := make([]string, 0, len([]rune(utf8Str)))
		for _, c := range utf8Str {
			// 获取字符的ASCII码值
			asciiValue := int(c)
			// 将ASCII码值转换为字符串，并添加到切片中
			asciiCodes = append(asciiCodes, strconv.Itoa(asciiValue))
		}
		return strings.Join(asciiCodes, " ")

	case "ISO-8859-1":
		encoder := charmap.ISO8859_1.NewEncoder()
		encodedStr, _, err := transform.String(encoder, utf8Str)
		if err != nil {
			return "error"
		}
		return encodedStr

	case "十六进制":
		encodedStr := hex.EncodeToString([]byte(utf8Str))
		return encodedStr

	case "二进制":
		var binaryStr strings.Builder
		binaryStr.Grow(len(utf8Str) * 8)
		for i := 0; i < len(utf8Str); i++ {
			// 将每个字节转换为二进制字符串，保证每个字节是8位
			binaryStr.WriteString(binTable[utf8Str[i]])
		}
		encodedStr := binaryStr.String()
		return encodedStr

	default:
		return "未知编码"
	}
}
