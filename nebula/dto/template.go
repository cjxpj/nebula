package dto

import (
	"encoding/base64"
	"net/url"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/utils"
)

// textTemplateCache 缓存字符串的预编译分段，避免每次变量替换都重复扫描 % 位置。
var textTemplateCache sync.Map

// varSlotRegistry 变量名 -> 槽号（进程级驻留，所有 Val 共享；槽号单调递增不复用）。
// 同一变量名在所有词库脚本中映射到同一槽号，各 Val 的 slots 数组按槽号各自存储。
// varSlotNames 为槽号 -> 变量名的反向表，供 FlushSlotsToMap 将无锁槽同步回 num/obj 映射。
var (
	varSlotRegistry sync.Map      // string -> int32
	varSlotNames    []string      // int32 -> string（槽号反向表）
	varSlotMu       sync.RWMutex  // 保护 varSlotNames 追加与读取（FlushSlotsToMap 读锁）
	varSlotCount    int32
)

// internVar 返回变量名对应的槽号；首次出现时分配新槽（线程安全）。
func internVar(name string) int32 {
	if v, ok := varSlotRegistry.Load(name); ok {
		return v.(int32)
	}
	varSlotMu.Lock()
	if v, ok := varSlotRegistry.Load(name); ok {
		varSlotMu.Unlock()
		return v.(int32)
	}
	n := varSlotCount
	varSlotCount++
	varSlotRegistry.Store(name, n)
	varSlotNames = append(varSlotNames, name)
	varSlotMu.Unlock()
	return n
}

// isPlainVarName 判断变量名是否为「普通变量」：即命中 renderVar 末尾的纯 GetVal 分支，
// 不匹配任何内置/修饰符/JSON 路径/取反/时间/随机数/转义/类成员等特殊形式。
// 仅此类变量名可安全槽化；其余回退 renderVar 保持语义完全一致。
func isPlainVarName(name string) bool {
	if name == "" || strings.Contains(name, ".") {
		return false
	}
	switch name {
	case "时间", "时间戳", "毫秒时间戳", "微秒时间戳", "纳秒时间戳", "空格", "换行", "系统", "版本",
		"val0", "val1", "val2", "val3", "val4", "val5", "val6", "val7", "val8", "val9", "val10":
		return false
	}
	for _, p := range [...]string{"URL编码@", "B64编码@", "URL@", "B64@", "TYPE@", "@", "?", "!", "时间", "随机数"} {
		if strings.HasPrefix(name, p) {
			return false
		}
	}
	return true
}

// slotForName 普通变量名驻留为槽号，特殊形式返回 -1（回退 renderVar）。
func slotForName(name string) int32 {
	if isPlainVarName(name) {
		return internVar(name)
	}
	return -1
}

// SlotForName 对外暴露：普通变量名驻留为槽号，特殊形式返回 -1。供编译期（dic 包）预计算指令槽号。
func SlotForName(name string) int32 {
	return slotForName(name)
}

// getVarSegments 返回字符串的预编译分段（带缓存）。
func getVarSegments(s string) []varSegment {
	if v, ok := textTemplateCache.Load(s); ok {
		return v.([]varSegment)
	}
	segs := compileVarSegments(s)
	actual, _ := textTemplateCache.LoadOrStore(s, segs)
	return actual.([]varSegment)
}

// varSegment 预编译后的模板分段：%...% 之间为变量段，其余为字面量段。
type varSegment struct {
	isVar bool
	text  string // 字面量文本，或变量名（% 中间的内容）
	slot  int32  // isVar=true 时：普通变量名驻留的槽号；-1 表示回退 renderVar
}

// compileVarSegments 把字符串按 %...% 预切分为分段序列。
// 仅做「找 % 位置」这一静态切分，变量求值仍由 renderVar 完成，保证与原语义完全一致。
func compileVarSegments(s string) []varSegment {
	if !strings.Contains(s, "%") {
		return []varSegment{{isVar: false, text: s}}
	}
	segs := make([]varSegment, 0, 8)
	start := 0
	for {
		open := strings.Index(s[start:], "%")
		if open == -1 {
			break
		}
		open += start
		close := strings.Index(s[open+1:], "%")
		if close == -1 {
			break
		}
		close += open + 1
		if open > start {
			segs = append(segs, varSegment{isVar: false, text: s[start:open], slot: -1})
		}
		name := s[open+1 : close]
		segs = append(segs, varSegment{isVar: true, text: name, slot: slotForName(name)})
		start = close + 1
	}
	if start < len(s) {
		segs = append(segs, varSegment{isVar: false, text: s[start:]})
	}
	return segs
}

// numericString 若值为数值类型则返回其字符串表示（用于数值直存直算后仍能正确拼接文本）。
func numericString(val any) (string, bool) {
	switch n := val.(type) {
	case int:
		return strconv.Itoa(n), true
	case int8:
		return strconv.FormatInt(int64(n), 10), true
	case int16:
		return strconv.FormatInt(int64(n), 10), true
	case int32:
		return strconv.FormatInt(int64(n), 10), true
	case int64:
		return strconv.FormatInt(n, 10), true
	case uint:
		return strconv.FormatUint(uint64(n), 10), true
	case uint8:
		return strconv.FormatUint(uint64(n), 10), true
	case uint16:
		return strconv.FormatUint(uint64(n), 10), true
	case uint32:
		return strconv.FormatUint(uint64(n), 10), true
	case uint64:
		return strconv.FormatUint(n, 10), true
	case float32:
		return strconv.FormatFloat(float64(n), 'f', -1, 32), true
	case float64:
		return strconv.FormatFloat(n, 'f', -1, 64), true
	}
	return "", false
}

// renderVarSegments 执行预编译分段：字面量直接拼接，变量段调用 renderVar 求值。
// 数值类型转为字符串继续拼接；其余非字符串值则整个模板返回该值（与 replaceProcessedContent 行为一致）。
func (v *Val) renderVarSegments(vv *Val, segs []varSegment) any {
	// 快速路径：无 % 的纯文本段直接返回原串，避免 Builder 复制（最常见的输出路径）。
	if len(segs) == 1 && !segs[0].isVar {
		return segs[0].text
	}
	var result strings.Builder
	for _, seg := range segs {
		if !seg.isVar {
			result.WriteString(seg.text)
			continue
		}
		var processed any
		if seg.slot >= 0 {
			// 普通变量槽快速路径：无锁按槽号直取，P 未命中再查 G。
			// 槽未命中时回退 renderVar 查 map（obj/num 为权威存储）：以 Reset/NewObj 等
			// map 方式构造的 Val（如 [内部] 回调经 GetAll+Reset 复制调用方变量）未写槽，
			// 直接返回字面量会导致这些变量读不到（变量互通失效）。
			if val, ok := v.slotGet(seg.slot); ok {
				processed = val
			} else if vv != nil {
				if val, ok := vv.slotGet(seg.slot); ok {
					processed = val
				} else {
					processed = v.renderVar(vv, seg.text)
				}
			} else {
				processed = v.renderVar(vv, seg.text)
			}
		} else {
			processed = v.renderVar(vv, seg.text)
		}
		if resStr, ok := processed.(string); ok {
			result.WriteString(resStr)
		} else if numStr, ok := numericString(processed); ok {
			result.WriteString(numStr)
		} else {
			return processed
		}
	}
	return result.String()
}

// renderVar 对 %val% 中的 val 求值。逻辑与原 Val.Text 内联闭包逐字对应，
// 保留所有 fall-through 控制流（部分分支在未命中时会继续向下求值）。
func (v *Val) renderVar(vv *Val, val string) any {
	// url编码
	if strings.HasPrefix(val, "URL编码@") {
		if value, ok := v.GetVal(vv, val[10:]); ok {
			if strValue, isString := value.(string); isString {
				return url.QueryEscape(strValue)
			}
		}
		return ""
	}
	// B64编码
	if strings.HasPrefix(val, "B64编码@") {
		if value, ok := v.GetVal(vv, val[10:]); ok {
			if strValue, isString := value.(string); isString {
				return base64.StdEncoding.EncodeToString([]byte(strValue))
			}
		}
		return ""
	}
	// url解码
	if strings.HasPrefix(val, "URL@") {
		if value, ok := v.GetVal(vv, val[4:]); ok {
			if strValue, isString := value.(string); isString {
				decoded, err := url.QueryUnescape(strValue)
				if err != nil {
					return strValue
				}
				return decoded
			}
		}
		return ""
	}
	// B64解码
	if strings.HasPrefix(val, "B64@") {
		if value, ok := v.GetVal(vv, val[4:]); ok {
			if strValue, isString := value.(string); isString {
				decoded, err := base64.StdEncoding.DecodeString(strValue)
				if err != nil {
					return ""
				}
				return string(decoded)
			}
		}
		return ""
	}

	// 类型
	if strings.HasPrefix(val, "TYPE@") {
		if value, ok := v.GetVal(vv, val[5:]); ok {
			return reflect.TypeOf(value).String()
		}
		return ""
	}

	if strings.HasPrefix(val, "@") {
		list := strings.Split(val[1:], ".")
		if len(list) > 1 {
			// 先取第一个变量
			value, _ := v.GetVal(vv, list[0])
			switch valueStr := value.(type) {
			case string:
				if j := utils.IsJSONResult(valueStr); j != nil {
					res := j
					for _, key := range list[1:] {
						switch curr := res.(type) {
						case map[string]any:
							if v, ok := curr[key]; ok {
								res = v
							} else {
								return ""
							}
						case []any:
							idx, err := strconv.Atoi(key)
							if err != nil || idx < 0 || idx >= len(curr) {
								return ""
							}
							res = curr[idx]
						default:
							return utils.AnyToString(res)
						}
					}
					return utils.AnyToString(res)
				}
			case map[string]any:
				var res any = valueStr
				for _, key := range list[1:] {
					if v, ok := res.(map[string]any); ok {
						if val, ok := v[key]; ok {
							res = val
						} else {
							return ""
						}
					}
				}
				return utils.AnyToString(res)
			}
		}
	}

	// ? 前缀：变量存在则输出值，否则输出空字符串（区别于普通 %变量% 不存在时原样输出 %变量%）。
	if strings.HasPrefix(val, "?") {
		if value, ok := v.GetVal(vv, val[1:]); ok && value != nil {
			if strValue, isString := value.(string); isString {
				return strValue
			}
			return value
		}
		return ""
	}

	if strings.HasPrefix(val, "!") {
		value, _ := v.GetVal(vv, val[1:])
		if strValue, isString := value.(string); isString {
			switch strValue {
			case "true":
				return "false"
			case "false":
				return "true"
			case "1":
				return "0"
			case "0":
				return "1"
			}
			return strValue
		}
		return ""
	}

	// 词库路径：从当前词库实例读取（SetRaw 绕过线程变量全局映射，避免并发串线）
	if val == "__词库路径__" {
		if vv != nil {
			if p, ok := vv.GetRaw("_词库路径_"); ok {
				if s, isStr := p.(string); isStr {
					return s
				}
			}
		}
		if p, ok := v.GetRaw("_词库路径_"); ok {
			if s, isStr := p.(string); isStr {
				return s
			}
		}
		return ""
	}

	switch val {
	case "时间":
		return time.Now()
	case "时间戳":
		return strconv.FormatInt(time.Now().Unix(), 10)
	case "毫秒时间戳":
		return strconv.FormatInt(time.Now().UnixNano()/1e6, 10)
	case "微秒时间戳":
		return strconv.FormatInt(time.Now().UnixNano()/1e3, 10)
	case "纳秒时间戳":
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	case "空格":
		return " "
	case "换行":
		return "\n"
	case "系统":
		return runtime.GOOS
	case "版本":
		return appfiles.Version
	}

	value, valueOk := v.GetVal(vv, val)
	if valueOk {
		strValue, isString := value.(string)
		if isString {
			return strValue
		}
		return value
	}

	if strings.HasPrefix(val, "时间") {
		getstr := val[6:]
		replacements := map[string]string{
			"yyyy":   "2006",
			"MM":     "01",
			"dd":     "02",
			"hh":     "03",
			"HH":     "15",
			"mm":     "04",
			"ss":     "05",
			"Mon":    "Mon",
			"Monday": "Monday",
		}
		for key, value := range replacements {
			getstr = strings.ReplaceAll(getstr, key, value)
		}
		return time.Now().Format(getstr)
	}

	if strings.HasPrefix(val, "随机数") {
		lval := val[9:]
		if dashIndex := strings.Index(lval, "-"); dashIndex != -1 {
			minStr := lval[:dashIndex]
			maxStr := lval[dashIndex+1:]
			if min, err := strconv.Atoi(minStr); err == nil {
				if max, err := strconv.Atoi(maxStr); err == nil {
					rN := utils.RandNum(min, max)
					if rN == min-1 {
						return ""
					}
					return strconv.Itoa(rN)
				}
			}
		}
	}

	if getstr, ok := strings.CutPrefix(val, "val"); ok && getstr != "" {
		switch getstr {
		case "0":
			return "$"
		case "1":
			return "%"
		case "2":
			return ":"
		case "3":
			return " "
		case "4":
			return "\t"
		case "5":
			return "\n"
		case "6":
			return ";"
		case "7":
			return "["
		case "8":
			return "]"
		case "9":
			return "\r\n"
		case "10":
			return "\r"
		}
	}

	// Class 变量：%类名.变量% / %变量.变量%（变量值为类名或实例）
	if dot := strings.Index(val, "."); dot > 0 {
		key := val[dot+1:]
		className := val[:dot]

		classMap := v.Class
		if classMap == nil && vv != nil {
			classMap = vv.Class
		}

		var cv *Val
		if className == "自己" {
			switch c := v.Get("Class").(type) {
			case string:
				if c != "" && classMap != nil {
					cv = classMap[c]
				}
			case *DicClass:
				cv = c.LocalValue
			}
		} else if classMap != nil {
			cv = classMap[className]
		}

		// 变量间接：className 是变量，值为类名或实例
		if cv == nil {
			if value, ok := v.GetVal(vv, className); ok {
				switch inst := value.(type) {
				case string:
					if inst != "" && inst != className && classMap != nil {
						cv = classMap[inst]
					}
				case *DicClass:
					cv = inst.LocalValue
				}
			}
		}

		if cv != nil {
			if value, ok := cv.GetVal(nil, key); ok {
				if strValue, isString := value.(string); isString {
					return strValue
				}
				return value
			}
		}

		if classMap != nil {
			return ""
		}
	}

	return "%" + val + "%"
}
