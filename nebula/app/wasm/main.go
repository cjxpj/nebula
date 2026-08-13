//go:build js && wasm

package main

import (
	"syscall/js"

	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
)

func main() {
	// 创建一个全局函数，让 JS 可以调用
	js.Global().Set("runDic", js.FuncOf(runDic))

	// 阻塞 main
	select {}
}

// runDic 会被 JS 调用，参数是文本 a
func runDic(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return "需要传入文本参数"
	}
	text := args[0].String()

	// 调用原 dic 处理逻辑
	dic := dic_dto.NewDic("main.n", text)
	defer dic.Close()

	result := dic_api.Api.DicRun(dic, "Main")

	return result // 返回给 JS
}
