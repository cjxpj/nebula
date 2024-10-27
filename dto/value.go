package dto

import (
	"fmt"
	"net/url"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/utils"
)

// value变量
type Val struct {
	mutex   sync.RWMutex
	objlock map[string]bool
	obj     sync.Map
}

// 线程变量
var GV *Val = NewVal()

// NewVal 初始化 Val 对象
func NewVal() *Val {
	v := &Val{
		objlock: make(map[string]bool),
	}
	return v
}

// 生成参数跟括号
func RunTrigger(msg, trigger string, v *Val) {
	regex := regexp.MustCompile("^" + trigger + "$")
	matches := regex.FindStringSubmatch(msg)
	for i, val := range matches {
		key := fmt.Sprintf("括号%d", i)
		v.Set(key, val)
	}
	triggerSplit := strings.Split(msg, " ")
	for i, val := range triggerSplit {
		key := fmt.Sprintf("参数%d", i)
		v.Set(key, val)
	}
}

// 变量生成参数跟括号
func ValRunTrigger(msg, trigger string, setV, v *Val) {
	regex := regexp.MustCompile("^" + trigger + "$")
	matches := regex.FindStringSubmatch(msg)
	for i, val := range matches {
		key := fmt.Sprintf("括号%d", i)
		setV.Set(key, v.Text(val))
	}
	triggerSplit := strings.Split(msg, " ")
	for i, val := range triggerSplit {
		key := fmt.Sprintf("参数%d", i)
		setV.Set(key, v.Text(val))
	}
}

// Get 返回指定键的值
func (v *Val) Get(key string) interface{} {
	value, _ := v.obj.Load(key)
	return value
}

// GetObj 返回指定键的值
func (v *Val) GetObj(key string) map[string]interface{} {
	value, _ := v.obj.Load(key)
	if value, ok := value.(map[string]interface{}); ok {
		return value
	}
	return make(map[string]interface{})
}

// GetAll 返回全部对象
func (v *Val) GetAll() map[string]interface{} {
	all := make(map[string]interface{})
	v.obj.Range(func(key, value interface{}) bool {
		if k, ok := key.(string); ok {
			all[k] = value
		}
		return true
	})
	return all
}

// NewObj 添加新对象
func (v *Val) NewObj(val map[string]interface{}) {
	for k, newVal := range val {
		v.obj.Store(k, newVal)
	}
}

// 覆盖obj
func (v *Val) AddObjs(key string, mapV []map[string]interface{}) {
	value, _ := v.obj.Load(key)
	var obj []map[string]interface{}
	if m, ok := value.([]map[string]interface{}); ok {
		obj = m
	}
	obj = append(obj, mapV...)
	v.obj.Store(key, obj)
}

// Reset 重新设置对象
func (v *Val) Reset(val map[string]interface{}) {
	v.obj = sync.Map{}
	for k, newVal := range val {
		v.obj.Store(k, newVal)
	}
}

// SetObj 设置指定键的值，如果操作成功返回 true，否则返回 false
func (v *Val) SetObj(key string, objkey string, val interface{}) bool {
	value, _ := v.obj.Load(key)
	if m, ok := value.(map[string]interface{}); ok {
		m[objkey] = val
		v.obj.Store(key, m)
		return true
	}
	return false
}

// Set 设置指定键的值
func (v *Val) SetLock(key string, val bool) *Val {
	v.mutex.Lock()
	defer v.mutex.Unlock()
	v.objlock[key] = val
	return v
}

// Set 设置指定键的值
func (v *Val) Set(key string, val interface{}) *Val {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	if !v.objlock[key] {
		v.obj.Store(key, val)
	}
	return v
}

// Add 将值添加到指定键的值后面
func (v *Val) Add(key string, val interface{}) {
	value, _ := v.obj.Load(key)
	if existingVal, ok := value.(string); ok {
		v.obj.Store(key, existingVal+val.(string))
	} else {
		v.obj.Store(key, val)
	}
}

// HeaderAdd 将值添加到指定键的值前面
func (v *Val) HeaderAdd(key string, val interface{}) {
	value, _ := v.obj.Load(key)
	if existingVal, ok := value.(string); ok {
		v.obj.Store(key, val.(string)+existingVal)
	} else {
		v.obj.Store(key, val)
	}
}

// Text 读取变量
func (v *Val) Text(str string) string {
	return v.Texts(str, "%", "%")
}

func (v *Val) Texts(str, start, end string) string {
	result := replaceProcessedContent(str, start, end, func(val string) string {

		strLen := len(val)

		if strLen > 4 && val[0] == 'U' && val[1] == 'R' && val[2] == 'L' && val[3] == '_' {
			value, _ := v.obj.Load(val[4:])
			if strValue, isString := value.(string); isString {
				return url.QueryEscape(strValue)
			}
			return ""
		}

		if strLen > 1 && val[0] == '!' {
			value, _ := v.obj.Load(val[1:])
			if strValue, isString := value.(string); isString {
				if strValue == "true" {
					return "false"
				}
				if strValue == "false" {
					return "true"
				}
				if strValue == "1" {
					return "0"
				}
				if strValue == "0" {
					return "1"
				}
				return strValue
			}
			return ""
		}

		switch val {
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

		value, _ := v.obj.Load(val)
		if strValue, isString := value.(string); isString {
			return strValue
		}

		if strLen > 6 && val[:6] == "时间" {
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

		if strLen == 4 && val[:3] == "val" {
			getstr := val[3:]
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
			}
		}

		return start + val + end
	})

	return result
}

// replaceProcessedContent 接受一个字符串、开始和结束的子串，以及一个处理函数作为参数
func replaceProcessedContent(str, strStart, strEnd string, process func(string) string) string {
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

		result.WriteString(processedContent)

		start = closeIndex + len(strEnd)
	}

	result.WriteString(str[start:])

	return result.String()
}
