package dto

// 函数框
type FuncBox struct {
	Trigger string
	Content []string
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
