package funcs

import (
	"errors"

	"github.com/cjxpj/nebula/dto"

	lua "github.com/yuin/gopher-lua"
)

func runLua(d *dto.DicInputs) (any, error) {
	// 创建一个新的 Lua 解释器
	L := lua.NewState()
	defer L.Close()

	// 执行 Lua 脚本
	if err := L.DoString(d.Inputs.String(1)); err != nil {
		return "", errors.New("Lua加载错误")
	}

	// 获取Lua的main函数
	fn := L.GetGlobal("main")
	if fn.Type() != lua.LTFunction {
		return "", errors.New("Lua函数main未定义")
	}

	// 调用main函数
	L.Push(fn)

	luaArgsLen := 0

	args := d.Inputs.StringAfterList(2)
	for _, arg := range args {
		luaArgsLen++
		luaArgs := lua.LString(arg)
		L.Push(luaArgs)
	}

	// 调用Lua函数，没有参数，有一个返回值
	if err := L.PCall(luaArgsLen, lua.MultRet, nil); err != nil {
		return "", err
	}
	// 判断栈里是否有返回值
	top := L.GetTop()
	if top == 0 {
		return "", nil // 没有返回值，直接返回空
	}
	// 从栈中弹出返回值L.Pop(1)
	results := L.Get(-1)
	// 从栈中弹出返回值
	L.Pop(1)
	res := results.String()
	return res, nil
}
