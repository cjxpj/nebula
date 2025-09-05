package funcs

import (
	"fmt"
	"sync"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"

	jsoniter "github.com/json-iterator/go"
)

type DicFunc struct {
	// 长度
	Len int
	// 原始参数
	InputData []string
	// 参数
	Inputs *utils.DicInputs
}

type DicFuncs struct {
	// 长度
	L string
	// 函数
	Fn func(d *dto.DicInputs) (any, error)
}

// =============注册函数================

// 自义定函数信息
type MyFuncInfo struct {
	Name string
	L    string
	Fn   func(*dto.DicInputs) (any, error)
}

// 自定义函数别名
type f = MyFuncInfo

// 全部函数
var FuncList sync.Map

// 获取函数
func GetFunc(name string) (DicFuncs, bool) {
	v, ok := FuncList.Load(name)
	if !ok {
		return DicFuncs{}, false
	}
	return v.(DicFuncs), true
}

// 注册函数
func Register(name, l string, fn func(d *dto.DicInputs) (any, error)) error {
	_, loaded := FuncList.LoadOrStore(name, DicFuncs{
		L:  l,
		Fn: fn,
	})
	if loaded {
		return fmt.Errorf("已存在函数 %s", name)
	}
	return nil
}

// 批量注册函数
func Registers(list ...MyFuncInfo) error {
	for _, v := range list {
		if err := Register(v.Name, v.L, v.Fn); err != nil {
			return err
		}
	}
	return nil
}

var json = jsoniter.Config{
	EscapeHTML: false, // 禁用 HTML 转义
}.Froze()
