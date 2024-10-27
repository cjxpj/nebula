package utils

import "encoding/json"

// 判断字符串是否为 JSON 格式
func IsJSON(s string) bool {
	var js map[string]interface{}
	var jss []interface{}
	if json.Unmarshal([]byte(s), &js) == nil || json.Unmarshal([]byte(s), &jss) == nil {
		return true
	}
	return false
}
