package funcs

import (
	"math/rand"
	"strings"

	"github.com/cjxpj/nebula/dto"
)

// mcColorToAnsi MC 颜色字符(0-9/a-f) -> ANSI 前景色码
var mcColorToAnsi = map[byte]string{
	'0': "30", '1': "34", '2': "32", '3': "36",
	'4': "31", '5': "35", '6': "33", '7': "37",
	'8': "90", '9': "94", 'a': "92", 'b': "96",
	'c': "91", 'd': "95", 'e': "93", 'f': "97",
}

// obfChars §k 乱码使用的可见 ASCII 字符集
const obfChars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz!#$%&()*+,-./:;<=>?@[]^_{|}~"

// mcAnsiCode 返回单个 MC 格式字符对应的 ANSI 序列（§k 由调用方单独处理），无效返回空
func mcAnsiCode(ch rune) string {
	if ch == 'r' || ch == 'R' {
		return "\x1b[0m"
	}
	if ch >= 'A' && ch <= 'Z' {
		ch += 'a' - 'A'
	}
	if code, ok := mcColorToAnsi[byte(ch)]; ok {
		return "\x1b[" + code + "m"
	}
	switch ch {
	case 'l': // 粗体
		return "\x1b[1m"
	case 'm': // 删除线
		return "\x1b[9m"
	case 'n': // 下划线
		return "\x1b[4m"
	case 'o': // 斜体
		return "\x1b[3m"
	}
	return ""
}

// mcInlineToAnsi 解析文本中的 §x 格式码并转为 ANSI；§k 段替换为固定随机乱码，无效码原样保留
func mcInlineToAnsi(text string) string {
	runes := []rune(text)
	var b strings.Builder
	colored := false
	obf := false
	for i := 0; i < len(runes); i++ {
		if runes[i] == '§' && i+1 < len(runes) {
			next := runes[i+1]
			if next == 'k' || next == 'K' {
				obf = true
				i++
				continue
			}
			if code := mcAnsiCode(next); code != "" {
				if next == 'r' || next == 'R' {
					obf = false
				}
				b.WriteString(code)
				colored = true
				i++
				continue
			}
			// 无效格式码：原样保留 §x
			b.WriteRune(runes[i])
			b.WriteRune(next)
			i++
			continue
		}
		if obf {
			if runes[i] == ' ' || runes[i] == '\t' {
				b.WriteRune(runes[i])
			} else {
				b.WriteByte(obfChars[rand.Intn(len(obfChars))])
			}
		} else {
			b.WriteRune(runes[i])
		}
	}
	if colored {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

// MC终端颜色 将文本中的 MC 颜色码(§x)转为终端 ANSI 颜色文字，配合「实时终端」显示彩色输出。
// 例：$MC终端颜色 §e你好§c世界$
func mcTerminalColor(d *dto.DicInputs) (any, error) {
	return mcInlineToAnsi(d.Inputs.String(1)), nil
}
