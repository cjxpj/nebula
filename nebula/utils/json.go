package utils

import (
	jsoniter "github.com/json-iterator/go"
)

var Json = jsoniter.Config{
	EscapeHTML: false, // 禁用 HTML 转义
}.Froze()

// isJSONStart 判断字符串去掉前导空白后是否以 [ 或 { 开头，用于快速排除非 JSON 输入，
// 避免对纯数字、普通文本等做无谓的 JSON 反序列化（自增/累加等热点路径每轮都会调用）。
func isJSONStart(s string) bool {
	i := 0
	for i < len(s) {
		switch s[i] {
		case ' ', '\t', '\n', '\r':
			i++
			continue
		}
		break
	}
	return i < len(s) && (s[i] == '[' || s[i] == '{')
}

// 判断字符串是否为 JSON 格式
func IsJSON(s string) bool {
	if !isJSONStart(s) {
		return false
	}
	var js map[string]any
	var jss []any
	if Json.Unmarshal([]byte(s), &js) == nil || Json.Unmarshal([]byte(s), &jss) == nil {
		return true
	}
	return false
}

// 是json就返回
func IsJSONResult(s string) any {
	if !isJSONStart(s) {
		return nil
	}
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
