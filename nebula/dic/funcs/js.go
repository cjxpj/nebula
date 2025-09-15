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

func runJs(d *dto.DicInputs) (any, error) {
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

	if input := d.Inputs.String(2); input != "" {
		args := strings.Split(input, ",")
		for i, arg := range args {
			key := fmt.Sprintf("参数%d", i)
			vm.Set(key, d.V.Text(arg))
		}
	}

	res, err := vm.RunString(d.Inputs.String(1))
	if err != nil {
		return "", err
	}
	resStr := res.String()
	return resStr, nil
}
