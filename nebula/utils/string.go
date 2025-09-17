package utils

import (
	"bytes"
	"errors"
	"io"
	"regexp"
	"strconv"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

var (
	commentBlock1 = regexp.MustCompile(`/\*([^*]|\*+[^*/])*\*+/`)
	commentBlock2 = regexp.MustCompile(`/\*[^*]*\*+([^*/][^*]*\*+)*/`)
	commentLine   = regexp.MustCompile(`//.*`)
)

func RemoveComments(text string) string {
	// 去除 /* */ 注释
	text = commentBlock1.ReplaceAllString(text, "")
	text = commentBlock2.ReplaceAllString(text, "")
	// 去除 // 注释
	text = commentLine.ReplaceAllString(text, "")
	return text
}

// 直接转换为字符串，如果转换失败则返回空字符串
func AnyIsString(text any) string {
	if res, ok := text.(string); ok {
		return res
	}
	return ""
}

// 统一将 any 类型转换为字符串（优先 JSON 编码）
func AnyToString(data any) string {
	if str, ok := data.(string); ok {
		return str
	}

	// 打印类型
	// fmt.Println("type", reflect.TypeOf(data))

	// 小数
	if num, ok := data.(float64); ok {
		return strconv.FormatFloat(num, 'f', -1, 64)
	}

	// 整数
	if num, ok := data.(int); ok {
		return strconv.Itoa(num)
	}

	b, err := json.Marshal(data)
	if err != nil {
		return "" // 或者返回 fmt.Sprintf("%v", data) 做 fallback
	}
	return string(b)
}

// 解码
func DecodeType(t string, raw []byte) (string, error) {
	switch t {
	case "UTF-8", "utf-8":
		return string(raw), nil

	// 简体中文
	case "GB-18030", "GB18030", "GBK", "gb18030", "gbk":
		reader := transform.NewReader(bytes.NewReader(raw), simplifiedchinese.GB18030.NewDecoder())
		decoded, _ := io.ReadAll(reader)
		return string(decoded), nil

	// 繁体中文
	case "Big5", "big5", "big-5":
		reader := transform.NewReader(bytes.NewReader(raw), traditionalchinese.Big5.NewDecoder())
		decoded, _ := io.ReadAll(reader)
		return string(decoded), nil

	// 日文
	case "Shift-JIS", "SJIS", "shift_jis", "cp932":
		reader := transform.NewReader(bytes.NewReader(raw), japanese.ShiftJIS.NewDecoder())
		decoded, _ := io.ReadAll(reader)
		return string(decoded), nil
	case "EUC-JP", "euc-jp":
		reader := transform.NewReader(bytes.NewReader(raw), japanese.EUCJP.NewDecoder())
		decoded, _ := io.ReadAll(reader)
		return string(decoded), nil
	case "ISO-2022-JP", "iso-2022-jp":
		reader := transform.NewReader(bytes.NewReader(raw), japanese.ISO2022JP.NewDecoder())
		decoded, _ := io.ReadAll(reader)
		return string(decoded), nil

	// 韩文
	case "EUC-KR", "euc-kr":
		reader := transform.NewReader(bytes.NewReader(raw), korean.EUCKR.NewDecoder())
		decoded, _ := io.ReadAll(reader)
		return string(decoded), nil

	// 西欧/拉丁编码
	case "ISO-8859-1", "iso-8859-1":
		reader := transform.NewReader(bytes.NewReader(raw), charmap.ISO8859_1.NewDecoder())
		decoded, _ := io.ReadAll(reader)
		return string(decoded), nil
	case "Windows-1252", "windows-1252", "cp1252":
		reader := transform.NewReader(bytes.NewReader(raw), charmap.Windows1252.NewDecoder())
		decoded, _ := io.ReadAll(reader)
		return string(decoded), nil
	}

	return "", errors.New("不存在类型")
}
