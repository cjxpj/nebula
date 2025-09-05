package funcs

import (
	"fmt"
	"strings"
	"time"

	"github.com/cjxpj/nebula/dto"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/buffer"
	"github.com/dop251/goja_nodejs/console"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/process"
	"github.com/dop251/goja_nodejs/require"
	"github.com/dop251/goja_nodejs/url"
)

// type Console struct{}

// func (*Console) Log(msg ...interface{}) {
// 	fmt.Println(msg...)
// }

func (f *DicFunc) RunJs(V *dto.Val) string {
	if f.Len == 1 || f.Len == 2 {
		V.Set("报错", "")
		// globalPath := filepath.Join(utils.GetAppDir(), DataPath)
		// fmt.Println(globalPath)
		// registry := require.NewRegistryWithLoader(func(path string) ([]byte, error) {
		// 	// 尝试从全局路径加载模块
		// 	fullPath := filepath.Join(globalPath, path)
		// 	pack := filepath.Join(fullPath, "package.json")
		// 	// 读取json解析
		// 	j, err := os.ReadFile(pack)
		// 	if err != nil {
		// 		fmt.Println("加载模块不存在")
		// 		return nil, err
		// 	}
		// 	var mainJ map[string]interface{}
		// 	Jerr := json.Unmarshal(j, &mainJ)
		// 	// 读取js文件
		// 	if Jerr != nil {
		// 		return nil, Jerr
		// 	}
		// 	main := mainJ["main"].(string)
		// 	// 去掉前面./
		// 	main = strings.TrimPrefix(main, "./")
		// 	fullPath = filepath.Join(fullPath, main)
		// 	fmt.Println("加载模块:"+path, "路径:"+fullPath)
		// 	d, err := os.ReadFile(fullPath)
		// 	if err != nil {
		// 		fmt.Println("加载模块失败")
		// 		return nil, err
		// 	}
		// 	return d, nil
		// })
		registry := new(require.Registry)
		loop := eventloop.NewEventLoop()
		vm := goja.New()
		registry.Enable(vm)
		console.Enable(vm)
		url.Enable(vm)
		buffer.Enable(vm)
		process.Enable(vm)
		// utilObj := util.New(vm)
		// vm.Set("util", utilObj)
		// 将 setTimeout 和 setInterval 注册到 JavaScript 环境中
		vm.Set("setTimeout", func(call goja.FunctionCall) goja.Value {
			fn := call.Argument(0)
			delay := call.Argument(1).ToInteger()

			// 确保第一个参数是函数
			if fnFn, ok := goja.AssertFunction(fn); ok {
				loop.SetTimeout(func(vm *goja.Runtime) {
					fnFn(goja.Undefined())
				}, time.Duration(delay)*time.Millisecond)
			}
			return goja.Undefined()
		})

		vm.Set("setInterval", func(call goja.FunctionCall) goja.Value {
			fn := call.Argument(0)
			delay := call.Argument(1).ToInteger()

			// 确保第一个参数是函数
			if fnFn, ok := goja.AssertFunction(fn); ok {
				loop.SetInterval(func(vm *goja.Runtime) {
					fnFn(goja.Undefined())
				}, time.Duration(delay)*time.Millisecond)
			}
			return goja.Undefined()
		})

		// 启动事件循环
		loop.Start()

		// 获取当前文件的目录路径
		// dirname := filepath.Dir(os.Args[0])

		// 将 __dirname 注入到 JavaScript 环境中
		// vm.Set("__dirname", dirname)

		// vm.Set("console", &Console{})

		if f.Len == 2 {
			input := f.InputData[2]
			args := strings.Split(input, ",")

			for i, arg := range args {
				key := fmt.Sprintf("参数%d", i)
				vm.Set(key, V.Text(arg))
			}
		}

		res, err := vm.RunString(f.Inputs.String(1))
		if err != nil {
			V.Set("报错", err.Error())
			return ""
		}
		resStr := res.String()
		return resStr
	}
	return ""
}
