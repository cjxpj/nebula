package funcs

import (
	"fmt"
	"sort"
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
		if v.Fn == nil {
			continue
		}
		if err := Register(v.Name, v.L, v.Fn); err != nil {
			return err
		}
	}
	return nil
}

// FuncInfo 单个已注册函数的补全信息（供 OPUI 前端代码补全）
type FuncInfo struct {
	Name  string `json:"name"`
	NoArg bool   `json:"no_arg"` // 无参数（L 为 "0"）
}

// ListFuncs 返回全部已注册函数名称与是否无参，按名称排序。
// FuncList 在应用启动时由 funcs.Setup 与 dic 包动态注入，故返回值为实时最新列表。
func ListFuncs() []FuncInfo {
	infos := make([]FuncInfo, 0)
	FuncList.Range(func(key, value any) bool {
		name, ok := key.(string)
		if !ok {
			return true
		}
		df, ok := value.(dto.DicFunc)
		if !ok {
			return true
		}
		infos = append(infos, FuncInfo{Name: name, NoArg: df.L == "0"})
		return true
	})
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })
	return infos
}

var json = jsoniter.Config{
	EscapeHTML: false, // 禁用 HTML 转义
}.Froze()

// wrapObj 将依赖对象（第一参数）的自由函数包装为对象方法。
func wrapObj(obj any, fn func(*dto.DicInputs) (any, error), l string) dto.DicFunc {
	return dto.DicFunc{
		L: l,
		Fn: func(d *dto.DicInputs) (any, error) {
			inputs := utils.NewDicInputs()
			list := make([]any, 0, d.Inputs.Len()+2)
			list = append(list, "")
			list = append(list, obj)
			for i := 1; i <= d.Inputs.Len(); i++ {
				list = append(list, d.Inputs.Get(i))
			}
			inputs.Set(list)
			return fn(dto.NewDicInputsWithOutput(d.Dic, d.V, &inputs, d.Output))
		},
	}
}
