//go:generate goversioninfo
//go:build dll

package main

/*
#include <stdlib.h>
*/
import "C"
import (
	"unsafe"

	_ "github.com/cjxpj/nebula/dic" // 触发引擎初始化：注入 dic_api.Api 并注册内置函数
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"
)

// RunN 执行一次词库匹配。
//
//	text    词库内容（.n 脚本文本）
//	trigger 触发词
//	path    词库路径（用于错误定位与编译缓存），可为空字符串
//
// 返回执行结果字符串，调用方需用 FreeString 释放。
//
//export RunN
func RunN(text, trigger, path *C.char) *C.char {
	goText := C.GoString(text)
	goTrigger := C.GoString(trigger)
	goPath := C.GoString(path)

	d := dic_dto.NewDic(goPath, goText)
	result := dic_api.Api.DicRun(d, goTrigger)

	return C.CString(result)
}

// RunCompiled 执行已编译的词库产物（打包进可执行文件时使用，避免运行时重新编译与读取外部依赖）。
//
//	data    编译产物（run.MarshalBuildValue 输出的 gob 字节）
//	dataLen data 的字节长度
//	trigger 触发词
//
// 返回执行结果字符串，调用方需用 FreeString 释放。
//
//export RunCompiled
func RunCompiled(data unsafe.Pointer, dataLen C.int, trigger *C.char) *C.char {
	goTrigger := C.GoString(trigger)
	buf := C.GoBytes(data, dataLen)

	bv, err := run.UnmarshalBuildValue(buf)
	if err != nil {
		return C.CString("编译产物加载失败: " + err.Error())
	}

	d := &dic_dto.Dic{
		Data:   bv,
		Val:    dto.NewDicVal(),
		MyFunc: bv.MyFunc,
	}
	result := dic_api.Api.DicRun(d, goTrigger)

	return C.CString(result)
}

// FreeString 释放 RunN 等接口返回的 C 字符串内存。
//
//export FreeString
func FreeString(s *C.char) {
	if s != nil {
		C.free(unsafe.Pointer(s))
	}
}

func main() {}
