package dic

import (
	"strconv"
	"strings"

	"github.com/cjxpj/nebula/count"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/utils"
)

// 执行
func (m *dicImpl) DicRunLine(r *dic_dto.DicEntry, txt []string) string {
	return m.dicRunLineBytecode(r, txt)
}

// SplitValChain 按 >>> 分割赋予值内容，跳过 $...$ 函数块内部的 >>>，用于赋予值连续执行
func SplitValChain(text string) []string {
	var parts []string
	var b strings.Builder
	inFunc := false
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c == '$' {
			// 统计前置反斜杠，判断是否被转义，\$ 不参与块状态切换
			bs := 0
			for j := i - 1; j >= 0 && text[j] == '\\'; j-- {
				bs++
			}
			if bs%2 == 0 {
				inFunc = !inFunc
			}
			b.WriteByte(c)
			continue
		}
		if !inFunc && c == '>' && i+2 < len(text) && text[i+1] == '>' && text[i+2] == '>' {
			parts = append(parts, b.String())
			b.Reset()
			i += 2
			continue
		}
		b.WriteByte(c)
	}
	// 忽略末尾空段（如 "a>>>"），保持返回结果整洁
	if b.Len() > 0 {
		parts = append(parts, b.String())
	}
	return parts
}

// unescapeText 处理文本中的转义序列：
// \r → 换行、\: → 冒号、\\ → 反斜杠（因此 \\r 表示字面 \r）。
func unescapeText(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'r':
				b.WriteByte('\n')
				i++
				continue
			case ':':
				b.WriteByte(':')
				i++
				continue
			case '\\':
				b.WriteByte('\\')
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// parseTernary 解析三目表达式 条件?真值:假值。
// 返回条件、真值、假值；不满足三目形态（? 紧跟 : 的 ?: 回退、无 ?、? 位于开头或 ? 后无 :）返回 ok=false。
func parseTernary(value string) (cond, trueVal, falseVal string, ok bool) {
	q := strings.IndexByte(value, '?')
	if q <= 0 {
		return "", "", "", false
	}
	// ? 后面紧跟 : 属于 ?: 回退，不是三目
	if q+1 < len(value) && value[q+1] == ':' {
		return "", "", "", false
	}
	rest := value[q+1:]
	c := strings.IndexByte(rest, ':')
	if c < 0 {
		return "", "", "", false
	}
	return value[:q], rest[:c], rest[c+1:], true
}

// runValSet 执行赋予值内容并写回变量，支持 三目 条件?真值:假值、?: 回退 与 @json路径
func runValSet(r *dic_dto.DicEntry, funcV *dic_dto.DicFunc, vPrefix, value string) {
	value = unescapeText(value)

	// 三目：条件?真值:假值（? 后紧跟 : 的 ?: 属于回退，走下方逻辑）
	if cond, trueVal, falseVal, ok := parseTernary(value); ok {
		if Pd(funcV, cond) {
			if runText, stopSetVal := RunsVal(funcV, trueVal, vPrefix); !stopSetVal {
				r.Val.P.Set(vPrefix, runText)
			}
		} else {
			if runText, stopSetVal := RunsVal(funcV, falseVal, vPrefix); !stopSetVal {
				r.Val.P.Set(vPrefix, runText)
			}
		}
		return
	}

	GetIfKeys := strings.Split(value, "?:")
	var runText any
questionCycle:
	for _, GetIfKey := range GetIfKeys {
		if strings.HasPrefix(GetIfKey, "@") {
			keys := strings.Split(GetIfKey, "->")
			if len(keys) < 2 {
				runText, stopSetVal := RunsVal(funcV, utils.AnyToString(count.RunCountText(r.Val, GetIfKey)), vPrefix)
				if stopSetVal {
					break
				}
				switch runText {
				case "", "null", "NULL", "Null", "false", "False", "FALSE":
					r.Val.P.Set(vPrefix, runText)
					continue
				}
				r.Val.P.Set(vPrefix, runText)
				break
			}
			for RunI, key := range keys {
				// 第一次加载解析数据
				if RunI == 0 {
					// 读取数据去除@
					runTexts := RunsAny(funcV, key[1:])
					// 推断数据map
					if rJ, ok := runTexts.(map[string]string); ok {
						runText = rJ
						continue
					}
					if rJ, ok := runTexts.(map[string]any); ok {
						runText = rJ
						continue
					}
					// 字符串转换后解析数据
					if runTexts, ok := runTexts.(string); ok {
						if rJ := utils.IsJSONResult(runTexts); rJ != nil {
							runText = rJ
							continue
						}
					}
					continue
				}
				// 解析数据
				switch objData := runText.(type) {
				case map[string]string:
					if rD, ok := objData[key]; ok {
						runText = rD
					} else {
						runText = ""
						break
					}
				case map[string]any:
					if rD, ok := objData[key]; ok {
						switch num := rD.(type) {
						case int:
							runText = strconv.FormatInt(int64(num), 10)
						case int64:
							runText = strconv.FormatInt(num, 10)
						case float64:
							runText = strconv.FormatFloat(num, 'f', -1, 64)
						default:
							runText = num
						}
					} else {
						runText = ""
						break
					}
				case []any:
					if num, err := strconv.Atoi(key); err == nil {
						if num >= 0 && num < len(objData) {
							rD := objData[num]
							runText = rD
						} else {
							runText = ""
							break
						}
					}
				}
				if objData, ok := runText.(string); ok {
					runText = objData
					break
				}
			}
			// 判断是否为空
			if rStr, ok := runText.(string); ok {
				switch rStr {
				case "", "null", "NULL", "Null", "false", "False", "FALSE":
					r.Val.P.Set(vPrefix, runText)
				default:
					r.Val.P.Set(vPrefix, runText)
					break questionCycle
				}
			}
			if runText != nil {
				// 最后一个直接设置
				r.Val.P.Set(vPrefix, utils.AnyToString(runText))
				continue
			}
		} else {
			runText, stopSetVal := RunsVal(funcV, GetIfKey, vPrefix)
			if stopSetVal {
				break
			}
			switch runText {
			case "", "null", "NULL", "Null", "false", "False", "FALSE":
				r.Val.P.Set(vPrefix, runText)
			default:
				r.Val.P.Set(vPrefix, runText)
				break questionCycle
			}
			continue
		}
	}
}
