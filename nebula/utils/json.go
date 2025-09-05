package utils

import jsoniter "github.com/json-iterator/go"

var json = jsoniter.Config{
	EscapeHTML: false, // 禁用 HTML 转义
}.Froze()

// 判断字符串是否为 JSON 格式
func IsJSON(s string) bool {
	var js map[string]any
	var jss []any
	if json.Unmarshal([]byte(s), &js) == nil || json.Unmarshal([]byte(s), &jss) == nil {
		return true
	}
	return false
}

// 是json就返回
func IsJSONResult(s string) any {
	var js map[string]any
	var jss []any
	if json.Unmarshal([]byte(s), &js) == nil {
		return js
	}
	if json.Unmarshal([]byte(s), &jss) == nil {
		return jss
	}
	return nil
}
