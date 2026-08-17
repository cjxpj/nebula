package dto

import (
	"encoding/base64"
	"fmt"
	"maps"
	"net/url"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/utils"
)

// 词库变量
type DicVal struct {
	// 全局变量
	G *Val
	// 局部变量
	P *Val
}

// NewDicVal 初始化 DicVal 对象
func NewDicVal() *DicVal {
	return &DicVal{
		G: NewVal(),
		P: NewVal(),
	}
}

// value变量
type Val struct {
	objlock sync.Map
	obj     sync.Map
	// Class 变量表：类名 -> 类变量，供 %类名.变量% 解析
	Class map[string]*Val `json:"-"`
}

// 回收词库变量
func (v *DicVal) Close() {
	v.G.Close()
	v.P.Close()
}

// 回收变量
func (v *Val) Close() {
	v.objlock.Range(func(key, value any) bool {
		v.obj.Delete(key)
		return true
	})
}

// 线程变量
var GV *Val = NewVal()

// ClearThreadVars 清空线程变量，保留系统内部变量（键名以 _ 开头）
func ClearThreadVars() {
	GV.obj.Range(func(key, _ any) bool {
		if k, ok := key.(string); ok && !strings.HasPrefix(k, "_") {
			GV.obj.Delete(key)
		}
		return true
	})
}

// SetThreadVar 设置线程变量（键名以 _ 开头为系统内部变量，不允许修改）
func SetThreadVar(key, val string) {
	if strings.HasPrefix(key, "_") {
		return
	}
	GV.Set(key, val)
}

// DeleteThreadVar 删除指定线程变量（键名以 _ 开头为系统内部变量，不允许删除）
func DeleteThreadVar(key string) {
	if strings.HasPrefix(key, "_") {
		return
	}
	GV.obj.Delete(key)
}

// NewVal 初始化 Val 对象
func NewVal() *Val {
	return &Val{}
}

// 生成参数跟括号
func RunTrigger(msg, trigger string, v *Val) {
	v.Set("括号0", msg)
	regex, err := regexp.Compile("^" + trigger + "$")
	if err == nil {
		matches := regex.FindStringSubmatch(msg)
		for i, val := range matches {
			if i == 0 {
				continue
			}
			key := fmt.Sprintf("括号%d", i)
			v.Set(key, val)
		}
	} else {
		debugLog.Infof("正则语法错误:%v", err)
	}
	triggerSplit := strings.Split(msg, " ")
	for i, val := range triggerSplit {
		key := fmt.Sprintf("参数%d", i)
		v.Set(key, val)
	}
}

// 变量生成参数跟括号
func ValRunTrigger(msg, trigger string, setV, v *DicVal) {
	setV.P.Set("括号0", v.Text(msg))
	regex, err := regexp.Compile("^" + trigger + "$")
	if err == nil {
		matches := regex.FindStringSubmatch(msg)
		for i, val := range matches {
			if i == 0 {
				continue
			}
			key := fmt.Sprintf("括号%d", i)
			setV.P.Set(key, v.Text(val))
		}
	} else {
		debugLog.Infof("正则语法错误:%v", err)
	}
	triggerSplit := strings.Split(msg, " ")
	for i, val := range triggerSplit {
		key := fmt.Sprintf("参数%d", i)
		setV.P.Set(key, v.Text(val))
	}
}

// 词库获取变量
func (v *DicVal) Get(key string) any {
	res := v.P.Get(key)
	if res == nil {
		return v.G.Get(key)
	}
	return res
}

// 获取全部变量
func (v *DicVal) GetAll() map[string]any {
	res := v.P.GetAll()
	gv := v.G.GetAll()
	maps.Copy(res, gv)
	return res
}

// Get 返回指定键的值
func (v *Val) Get(key string) any {
	if name, ok := strings.CutPrefix(key, "__"); ok && name != "" {
		value, _ := GV.obj.Load(name)
		return value
	}
	value, _ := v.obj.Load(key)
	return value
}

// GetStr 返回指定键的值
func (v *Val) GetStr(key string) string {
	value, _ := v.obj.Load(key)
	if value, ok := value.(string); ok {
		return value
	}
	return ""
}

// GetINT 返回指定键的值
func (v *Val) GetINT(key string) int {
	value, _ := v.obj.Load(key)
	if value, ok := value.(int); ok {
		return value
	}
	return 0
}

// GetObj 返回指定键的值
func (v *Val) GetObj(key string) map[string]any {
	value, _ := v.obj.Load(key)
	if value, ok := value.(map[string]any); ok {
		return value
	}
	return make(map[string]any)
}

// GetAll 返回全部对象
func (v *Val) GetAll() map[string]any {
	all := make(map[string]any)
	v.obj.Range(func(key, value any) bool {
		if k, ok := key.(string); ok {
			all[k] = value
		}
		return true
	})
	return all
}

// NewObj 添加新对象
func (v *Val) NewObj(val map[string]any) {
	for k, newVal := range val {
		v.obj.Store(k, newVal)
	}
}

// 新建词库对象
func (dv *DicVal) NewDicVal(v *Val) *DicVal {
	return &DicVal{
		G: dv.G,
		P: v,
	}
}

// 覆盖obj
func (v *Val) AddObjs(key string, mapV []map[string]any) {
	value, _ := v.obj.Load(key)
	var obj []map[string]any
	if m, ok := value.([]map[string]any); ok {
		obj = m
	}
	obj = append(obj, mapV...)
	v.obj.Store(key, obj)
}

// Reset 重新设置对象
func (v *Val) Reset(val map[string]any) *Val {
	v.obj = sync.Map{}
	for k, newVal := range val {
		v.obj.Store(k, newVal)
	}
	return v
}

// SetObj 设置指定键的值，如果操作成功返回 true，否则返回 false
func (v *Val) SetObj(key string, objkey string, val any) bool {
	value, _ := v.obj.Load(key)
	if m, ok := value.(map[string]any); ok {
		m[objkey] = val
		v.obj.Store(key, m)
		return true
	}
	return false
}

// SetLock 设置指定键的锁定状态
func (v *Val) SetLock(key string, val bool) *Val {
	v.objlock.Store(key, val)
	return v
}

// Set 设置指定键的值，只有在键未被锁定时才设置
func (v *Val) Set(key string, val any) *Val {
	if name, ok := strings.CutPrefix(key, "__"); ok && name != "" {
		GV.obj.Store(name, val)
		return v
	}
	value, ok := v.objlock.Load(key)
	isLocked, typeOk := value.(bool)
	if !ok || (ok && !typeOk) || (ok && !isLocked) {
		v.obj.Store(key, val)
	}
	return v
}

// Add 将值添加到指定键的值后面
func (v *Val) Add(key string, val any) {
	value, _ := v.obj.Load(key)
	if existingVal, ok := value.(string); ok {
		if newVal, ok := val.(string); ok {
			v.obj.Store(key, existingVal+newVal)
		} else {
			v.obj.Store(key, val)
		}
	} else {
		v.obj.Store(key, val)
	}
}

// HeaderAdd 将值添加到指定键的值前面
func (v *Val) HeaderAdd(key string, val any) {
	value, _ := v.obj.Load(key)
	if existingVal, ok := value.(string); ok {
		if newVal, ok := val.(string); ok {
			v.obj.Store(key, newVal+existingVal)
		} else {
			v.obj.Store(key, val)
		}
	} else {
		v.obj.Store(key, val)
	}
}

// 获取变量值，优先从 P，再从 G
func (v *DicVal) GetVal(key string) (any, bool) {
	if name, ok := strings.CutPrefix(key, "__"); ok && name != "" {
		return GV.obj.Load(name)
	}
	value, ok := v.P.obj.Load(key)
	if !ok && v.G != nil {
		value, ok = v.G.obj.Load(key)
	}
	return value, ok
}

// 获取变量值，优先从 P，再从 G
func (v *Val) GetVal(vv *Val, key string) (any, bool) {
	if name, ok := strings.CutPrefix(key, "__"); ok && name != "" {
		return GV.obj.Load(name)
	}
	value, ok := v.obj.Load(key)
	if !ok && vv != nil {
		value, ok = vv.obj.Load(key)
	}
	return value, ok
}

// 词库变量
func (v *DicVal) Text(content any) any {
	return v.P.Text(v.G, content)
}

func (v *Val) Text(vv *Val, content any) any {
	str, ok := content.(string)
	if !ok {
		return content
	}
	result := replaceProcessedContent(str, "%", "%", func(val string) any {
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
									// 不存在键返回
									return ""
								}
							case []any:
								// 尝试把 key 转成索引
								idx, err := strconv.Atoi(key)
								if err != nil || idx < 0 || idx >= len(curr) {
									// 不存在键返回
									return ""
								}
								res = curr[idx]
							default:
								// 遇到不可再下探的类型
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
								// 不存在键返回
								return ""
							}
						}
					}
					return utils.AnyToString(res)
				}
			}
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
			} else {
				return value
			}
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
	})

	return result
}

// replaceProcessedContent 接受一个字符串、开始和结束的子串，以及一个处理函数作为参数
func replaceProcessedContent(str, strStart, strEnd string, process func(string) any) any {
	var result strings.Builder
	start := 0

	for {
		openIndex := strings.Index(str[start:], strStart)
		if openIndex == -1 {
			break
		}
		openIndex += start

		closeIndex := strings.Index(str[openIndex+len(strStart):], strEnd)
		if closeIndex == -1 {
			break
		}
		closeIndex += openIndex + len(strStart)

		result.WriteString(str[start:openIndex])

		content := str[openIndex+len(strStart) : closeIndex]
		processedContent := process(content)

		if resStr, ok := processedContent.(string); ok {
			result.WriteString(resStr)
		} else {
			return processedContent
		}

		start = closeIndex + len(strEnd)
	}

	result.WriteString(str[start:])

	return result.String()
}
