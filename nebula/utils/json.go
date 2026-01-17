package utils

import (
	jsoniter "github.com/json-iterator/go"
)

var Json = jsoniter.Config{
	EscapeHTML: false, // 禁用 HTML 转义
}.Froze()

// 判断字符串是否为 JSON 格式
func IsJSON(s string) bool {
	var js map[string]any
	var jss []any
	if Json.Unmarshal([]byte(s), &js) == nil || Json.Unmarshal([]byte(s), &jss) == nil {
		return true
	}
	return false
}

// 是json就返回
func IsJSONResult(s string) any {
	var js map[string]any
	var jss []any
	if Json.Unmarshal([]byte(s), &js) == nil {
		return js
	}
	if Json.Unmarshal([]byte(s), &jss) == nil {
		return jss
	}
	return nil
}

// 编码
func Marshal(v any) ([]byte, error) {
	return Json.Marshal(v)
}
