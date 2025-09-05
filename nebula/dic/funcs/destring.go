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

func (f *DicFunc) DeUtf8() string {
	if f.Len != 2 {
		return ""
	}

	utf8Str := f.Inputs.String(2)

	switch f.Inputs.String(1) {
	case "GBK":
		decoder := simplifiedchinese.GBK.NewDecoder()
		decodedStr, _, err := transform.String(decoder, utf8Str)
		if err != nil {
			return "error"
		}
		return decodedStr

	case "GB18030":
		decoder := simplifiedchinese.GB18030.NewDecoder()
		decodedStr, _, err := transform.String(decoder, utf8Str)
		if err != nil {
			return "error"
		}
		return decodedStr

	case "HZGB2312":
		decoder := simplifiedchinese.HZGB2312.NewDecoder()
		decodedStr, _, err := transform.String(decoder, utf8Str)
		if err != nil {
			return "error"
		}
		return decodedStr

	case "ASCII":
		// 使用空格分割字符串，获取ASCII码值数组
		asciiArray := strings.Split(utf8Str, " ")

		// 初始化结果字符串
		var resultStr strings.Builder

		// 遍历ASCII码值数组
		for _, codeStr := range asciiArray {
			// 将字符串码值转换为整数
			code, err := strconv.Atoi(codeStr)
			if err != nil {
				return ""
			}
			// 将ASCII码值转换为字符，并拼接到结果字符串
			resultStr.WriteRune(rune(code))
		}

		// 返回解码后的字符串
		return resultStr.String()

	case "ISO-8859-1":
		decoder := charmap.ISO8859_1.NewDecoder()
		decodedStr, _, err := transform.String(decoder, utf8Str)
		if err != nil {
			return "error"
		}
		return decodedStr

	case "十六进制":
		decodedStr, err := hex.DecodeString(utf8Str)
		if err != nil {
			return "error"
		}
		return string(decodedStr)

	case "二进制":
		if len(utf8Str)%8 != 0 {
			fmt.Println(len(utf8Str))
			return "error1"
		}
		var bytes []byte
		for i := 0; i < len(utf8Str); i += 8 {
			byteStr := utf8Str[i : i+8]
			b, err := strconv.ParseUint(byteStr, 2, 8)
			if err != nil {
				return "error2"
			}
			bytes = append(bytes, byte(b))
		}

		decodedStr := string(bytes)
		return decodedStr

	default:
		return "未知编码"
	}
}
