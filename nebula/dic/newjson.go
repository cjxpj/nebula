package dic

import (
	"fmt"
	"strconv"
	"strings"

	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/dto"
)

// NewJson 方法将 JSON 字符串转换为 map，遍历并替换包含 % 的字符串
func NewJson(r *dic_dto.DicEntry, v *dto.Val, jsonStr string) string {
	// 解析 JSON 字符串为 map
	var data any
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		debugLog.Infof("Error parsing JSON:%v", err)
		return "error"
	}

	// 递归遍历 map 并替换 % 中的内容
	var replace func(any) any
	replace = func(value any) any {
		switch value := value.(type) {
		case string:
			// 如果是字符串，检查是否以 s% 或 % 开头
			if strings.HasPrefix(value, "s%") {
				// 如果以 s% 开头，去掉前缀 s%，并从 dto 中获取值
				key := value[2:]
				if rawValue := v.Get(key); rawValue != nil {
					// 如果获取到的值是字符串，直接返回；否则，转换为字符串
					if _, ok := rawValue.(string); ok {
						return rawValue
					} else {
						return fmt.Sprintf("%v", value)
					}
				}
				return value // 如果没有找到对应的键，保持原值
			} else if strings.HasPrefix(value, "%") {
				// 如果以 % 开头，去掉前缀 %，并从 dto 中获取值
				key := value[1:]
				if rawValue := v.Get(key); rawValue != nil {
					// 如果获取到的值是字符串，根据内容类型处理
					if strValue, ok := rawValue.(string); ok {
						// 检查是否为纯数字
						if intValue, err := strconv.Atoi(strValue); err == nil {
							return intValue // 转换为 int
						}
						// 检查是否为 JSON 格式
						if jsonValue := tryParseJSON(strValue); jsonValue != nil {
							return jsonValue // 解析为 JSON 对象或数组
						}
						// 其他情况直接返回字符串
						return strValue
					}
					// 如果不是字符串，直接返回原始值
					return rawValue
				}
				return value // 如果没有找到对应的键，保持原值
			}
			// 如果不符合上述规则，直接返回原始值
			return value
		case map[string]any:
			// 如果是 map，递归遍历其值
			for k := range value {
				value[k] = replace(value[k])
			}
			return value
		case []any:
			// 如果是切片，递归遍历其元素
			for i := range value {
				value[i] = replace(value[i])
			}
			return value
		default:
			return value
		}
	}

	// 调用递归函数并更新 data
	data = replace(data)

	// 将更新后的 map 转换回 JSON 字符串
	newJsonStr, err := json.Marshal(data)
	if err != nil {
		debugLog.Infof("Error marshaling JSON:%v", err)
		return ""
	}

	return string(newJsonStr)
}

// 尝试解析字符串为 JSON
func tryParseJSON(str string) any {
	var result any
	if err := json.Unmarshal([]byte(str), &result); err == nil {
		return result
	}
	return nil
}
