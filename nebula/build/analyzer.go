package build

import "unicode"

/*
* 判断是否为赋予值 类型，赋予值键名，赋予值内容
* 0 非赋予值
* 1 自减
* 2 自增
* 3 执行函数
* 4 执行变量
* 5 纯文本赋予值
* 6 普通赋予值
 */
func ValTextTest(text string) (int8, string, string) {
	textLen := len(text)
	jsonHead := false
	for i, r := range text {
		// if i == 0 {
		// 	continue
		// }
		if r == '$' {
			break
		}
		endIdx := i + 2
		if textLen >= endIdx {
			if r == '-' && text[i+1] == ':' {
				prefix := text[:i]
				suffix := text[endIdx:]
				return 1, prefix, suffix
			}
			if r == '+' && text[i+1] == ':' {
				prefix := text[:i]
				suffix := text[endIdx:]
				return 2, prefix, suffix
			}
		}
		if r == ':' {
			endIdx++ // 3
			if textLen >= endIdx {
				if text[i+1] == '$' && text[i+2] == ':' {
					prefix := text[:i]
					suffix := text[endIdx:]
					return 3, prefix, suffix
				}
				if text[i+1] == '%' && text[i+2] == ':' {
					prefix := text[:i]
					suffix := text[endIdx:]
					return 4, prefix, suffix
				}
			}
			endIdx-- // 2
			if textLen >= endIdx && text[i+1] == ':' {
				prefix := text[:i]
				suffix := text[endIdx:]
				return 5, prefix, suffix
			}
			endIdx-- // 1
			prefix := text[:i]
			suffix := text[endIdx:]
			return 6, prefix, suffix
		}

		// 匹配json多键
		if jsonHead {
			if r == '>' {
				jsonHead = false
				continue
			}
			break
		}
		if r == '-' && !jsonHead {
			jsonHead = true
			continue
		}

		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Scripts["Han"], r) || r == '_') {
			break
		}
	}
	return 0, "", ""
}

// 匹配自己想要的字符第一个
func MatchFirst(text string) (int, rune) {
	// 匹配:跟+跟-
	for i, r := range text {
		if r == ':' || r == '+' || r == '-' {
			return i, r
		}
	}
	return -1, 0
}
