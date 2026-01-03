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
	InputData []any
	// 参数
	Inputs *utils.DicInputs
}

// =============注册函数================

// 全部函数
var FuncList sync.Map

// 获取函数
func GetFunc(name string) (dto.DicFunc, bool) {
	v, ok := FuncList.Load(name)
	if !ok {
		return dto.DicFunc{}, false
	}
	return v.(dto.DicFunc), true
}

// 注册函数
func Register(name, l string, fn func(d *dto.DicInputs) (any, error)) error {
	_, loaded := FuncList.LoadOrStore(name, dto.DicFunc{
		L:  l,
		Fn: fn,
	})
	if loaded {
		return fmt.Errorf("已存在函数 %s", name)
	}
	return nil
}

// 批量注册函数
func Registers(list ...dto.RegisterDicFunc) error {
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
