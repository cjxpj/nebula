package funcs

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/cjxpj/nebula/dto"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

var binTable [256]string

func init() {
	for i := 0; i < 256; i++ {
		binTable[i] = fmt.Sprintf("%08b", i)
	}
}

// ========== 编码/解码 ==========

func enUtf8(d *dto.DicInputs) (any, error) {
	if !d.Inputs.LenOk(2) {
		return "", nil
	}

	utf8Str := d.Inputs.String(2)

	switch d.Inputs.String(1) {
	case "GBK":
		encoder := simplifiedchinese.GBK.NewEncoder()
		encodedStr, _, err := transform.String(encoder, utf8Str)
		if err != nil {
			return "error", nil
		}
		return encodedStr, nil

	case "GB18030":
		encoder := simplifiedchinese.GB18030.NewEncoder()
		encodedStr, _, err := transform.String(encoder, utf8Str)
		if err != nil {
			return "error", nil
		}
		return encodedStr, nil

	case "HZGB2312":
		encoder := simplifiedchinese.HZGB2312.NewEncoder()
		encodedStr, _, err := transform.String(encoder, utf8Str)
		if err != nil {
			return "error", nil
		}
		return encodedStr, nil

	case "ASCII":
		asciiCodes := make([]string, 0, len([]rune(utf8Str)))
		for _, c := range utf8Str {
			asciiValue := int(c)
			asciiCodes = append(asciiCodes, strconv.Itoa(asciiValue))
		}
		return strings.Join(asciiCodes, " "), nil

	case "ISO-8859-1":
		encoder := charmap.ISO8859_1.NewEncoder()
		encodedStr, _, err := transform.String(encoder, utf8Str)
		if err != nil {
			return "error", nil
		}
		return encodedStr, nil

	case "十六进制":
		encodedStr := hex.EncodeToString([]byte(utf8Str))
		return encodedStr, nil

	case "二进制":
		var binaryStr strings.Builder
		binaryStr.Grow(len(utf8Str) * 8)
		for i := 0; i < len(utf8Str); i++ {
			binaryStr.WriteString(binTable[utf8Str[i]])
		}
		return binaryStr.String(), nil

	default:
		return "未知编码", nil
	}
}

func deUtf8(d *dto.DicInputs) (any, error) {
	if !d.Inputs.LenOk(2) {
		return "", nil
	}

	utf8Str := d.Inputs.String(2)

	switch d.Inputs.String(1) {
	case "GBK":
		decoder := simplifiedchinese.GBK.NewDecoder()
		decodedStr, _, err := transform.String(decoder, utf8Str)
		if err != nil {
			return "error", nil
		}
		return decodedStr, nil

	case "GB18030":
		decoder := simplifiedchinese.GB18030.NewDecoder()
		decodedStr, _, err := transform.String(decoder, utf8Str)
		if err != nil {
			return "error", nil
		}
		return decodedStr, nil

	case "HZGB2312":
		decoder := simplifiedchinese.HZGB2312.NewDecoder()
		decodedStr, _, err := transform.String(decoder, utf8Str)
		if err != nil {
			return "error", nil
		}
		return decodedStr, nil

	case "ASCII":
		asciiArray := strings.Split(utf8Str, " ")
		var resultStr strings.Builder
		for _, codeStr := range asciiArray {
			code, err := strconv.Atoi(codeStr)
			if err != nil {
				return "", nil
			}
			resultStr.WriteRune(rune(code))
		}
		return resultStr.String(), nil

	case "ISO-8859-1":
		decoder := charmap.ISO8859_1.NewDecoder()
		decodedStr, _, err := transform.String(decoder, utf8Str)
		if err != nil {
			return "error", nil
		}
		return decodedStr, nil

	case "十六进制":
		decodedStr, err := hex.DecodeString(utf8Str)
		if err != nil {
			return "error", nil
		}
		return string(decodedStr), nil

	case "二进制":
		if len(utf8Str)%8 != 0 {
			return "error1", nil
		}
		var bytess = make([]byte, 0, len(utf8Str)/8)
		for i := 0; i < len(utf8Str); i += 8 {
			byteStr := utf8Str[i : i+8]
			b, err := strconv.ParseUint(byteStr, 2, 8)
			if err != nil {
				return "error2", nil
			}
			bytess = append(bytess, byte(b))
		}
		return string(bytess), nil

	default:
		return "未知编码", nil
	}
}
