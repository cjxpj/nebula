package dto

import "sync"

// FuncRules 内置函数参数数量规则注册表（函数名 -> 参数规则，如 "1|2"）。
// 由 funcs.Register 在注册时写入，供 run 编译期做函数参数数量静态检查，
// 避免 run 直接 import funcs 造成循环依赖。
var FuncRules sync.Map

// RegisterFuncRule 写入内置函数参数规则。
func RegisterFuncRule(name, l string) {
	FuncRules.Store(name, l)
}

// UnregisterFuncRule 移除内置函数参数规则。
func UnregisterFuncRule(name string) {
	FuncRules.Delete(name)
}

// GetFuncRule 读取内置函数参数规则；未注册返回 ("", false)。
func GetFuncRule(name string) (string, bool) {
	v, ok := FuncRules.Load(name)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// 函数框
type FuncBox struct {
	// Default 调用时的默认参数：$%变量%$ 无参调用时以该值作为触发词，留空默认 "0"。
	Default  string
	Content  []string
	LineNums []int // 每行内容对应的原始文件行号（1-based），用于调试报错定位
}

// 单个函数
type DicFunc struct {
	// 长度
	L string
	// 函数
	Fn func(d *DicInputs) (any, error)
}

// 一次性注册函数
type RegisterDicFunc struct {
	Name string
	L    string
	Fn   func(*DicInputs) (any, error)
}
