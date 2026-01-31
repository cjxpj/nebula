package build

import (
	"unicode"
	"unicode/utf8"
)

/*
* 判断是否为赋予值 类型，赋予值键名，赋予值内容
* 0 非赋予值
* 1 自减        -:
* 2 自增        +:
* 3 执行函数    :$:
* 4 执行变量    :%:
* 5 纯文本赋值  ::
* 6 普通赋值    :
* 7 乘法        *:
* 8 除法        /:
 */
func ValTextTest(text string) (int8, string, string) {
	n := len(text)
	if n == 0 {
		return 0, "", ""
	}

	// 1️⃣ 扫描键名
	i := 0
	jsonHead := false

	for i < n {
		c := text[i]

		// JSON 多键 -xxx>
		if jsonHead {
			if c == '>' {
				jsonHead = false
				i++
				continue
			}
			break
		}
		if c == '-' {
			jsonHead = true
			i++
			continue
		}

		// ASCII 快速路径
		if (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '_' {
			i++
			continue
		}

		// Unicode 兜底（中文等）
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && size == 1 {
			break
		}
		if unicode.Is(unicode.Scripts["Han"], r) || unicode.IsLetter(r) || unicode.IsDigit(r) {
			i += size
			continue
		}

		break
	}

	// 没有操作符
	if i >= n {
		return 0, "", ""
	}

	prefix := text[:i]

	// 2️⃣ prefix 长度限制
	if len(prefix) > 18 {
		return 0, "", ""
	}

	rest := text[i:]

	// 3️⃣ 操作符解析（最短路径）
	switch {
	case len(rest) >= 2 && rest[0] == '-' && rest[1] == ':':
		return 1, prefix, rest[2:]
	case len(rest) >= 2 && rest[0] == '+' && rest[1] == ':':
		return 2, prefix, rest[2:]
	case len(rest) >= 2 && rest[0] == '*' && rest[1] == ':':
		return 7, prefix, rest[2:]
	case len(rest) >= 2 && rest[0] == '/' && rest[1] == ':':
		return 8, prefix, rest[2:]

	case len(rest) >= 3 && rest[0] == ':' && rest[1] == '$' && rest[2] == ':':
		return 3, prefix, rest[3:]
	case len(rest) >= 3 && rest[0] == ':' && rest[1] == '%' && rest[2] == ':':
		return 4, prefix, rest[3:]

	case len(rest) >= 2 && rest[0] == ':' && rest[1] == ':':
		return 5, prefix, rest[2:]

	case rest[0] == ':':
		return 6, prefix, rest[1:]
	}

	return 0, "", ""
}
