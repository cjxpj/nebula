package utils

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"math/big"
	"regexp"
	"strconv"
	"strings"

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

// 直接转换字节
func AnyIsStringAndBytes(text any) []byte {
	if res, ok := text.(string); ok {
		return []byte(res)
	}
	if res, ok := text.([]byte); ok {
		return res
	}
	return []byte{}
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

	b, err := Json.Marshal(data)
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

// 随机字符
func RandomString(str string, n int) string {
	if str == "大小字母" {
		str = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	}
	b := make([]byte, n)
	l := big.NewInt(int64(len(str)))
	for i := range b {
		idx, _ := rand.Int(rand.Reader, l)
		b[i] = str[idx.Int64()]
	}
	return string(b)
}

var mdEscaper = strings.NewReplacer(
	`\`, `\\`, // 反斜杠本身
	`*`, `\*`, // 强调、列表
	`_`, `\_`, // 强调
	`[`, `\[`, // 链接、图片
	`]`, `\]`,
	`(`, `\(`,
	`)`, `\)`,
	`#`, `\#`, // 标题
	`+`, `\+`, // 列表
	`-`, `\-`, // 列表、分隔线
	`.`, `\.`, // 有序列表、段号
	`!`, `\!`, // 图片
	`<`, `\<`, // 自动链接、HTML
	`>`, `\>`,
	`|`, `\|`, // 表格
	`{`, `\{`, // 某些扩展语法
	`}`, `\}`,
)

// MDEscape 对任意文本做 Markdown 字符转义，返回可直接插入 .md 的字符串
func MDEscape(s string) string {
	return mdEscaper.Replace(s)
}
