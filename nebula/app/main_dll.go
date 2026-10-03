//go:generate goversioninfo
//go:build dll

// main_dll.go 是 c-shared（-tags dll）构建的 C ABI 壳子。
// 全部业务实现位于同目录的 dll_impl.go（纯 Go，不 import "C"）。
//
// 约定：
//   - 所有返回值均为 C 字符串，调用方需用 NebulaFreeString 释放（legacy FreeString 等价）；
//   - 除 legacy RunN/RunCompiled 外，其余导出统一返回 JSON 信封：
//     {"ok":true,"data":...} / {"ok":false,"error":"..."}；
//   - 事件通过 NebulaPollEvent 轮询（返回空串表示暂无事件）。
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

// ================= 初始化 / 版本 / 调试 =================

// NebulaInit 初始化引擎：收紧沙箱、设置日志目录、启用编译缓存、注册事件回调。
// cfgJSON: {"botsRoot":"...","logDir":"...","debug":true}
//
//export NebulaInit
func NebulaInit(cfgJSON *C.char) *C.char {
	return C.CString(dllInit(C.GoString(cfgJSON)))
}

// NebulaVersion 返回引擎版本（JSON 信封）。
//
//export NebulaVersion
func NebulaVersion() *C.char {
	return C.CString(dllVersionString())
}

// NebulaSetDebug 设置调试开关（on=0 关闭，非 0 打开）。
//
//export NebulaSetDebug
func NebulaSetDebug(on C.int) *C.char {
	return C.CString(dllSetDebug(int(on)))
}

// ================= 机器人生命周期 =================

// NebulaBotStart 让机器人上线（幂等）。
//
//export NebulaBotStart
func NebulaBotStart(id C.longlong, appID, secret, name, filePath *C.char) *C.char {
	return C.CString(dllBotStart(int64(id), C.GoString(appID), C.GoString(secret), C.GoString(name), C.GoString(filePath)))
}

// NebulaBotStop 让机器人下线。
//
//export NebulaBotStop
func NebulaBotStop(id C.longlong) *C.char {
	return C.CString(dllBotStop(int64(id)))
}

// NebulaBotStopAll 停止全部机器人。
//
//export NebulaBotStopAll
func NebulaBotStopAll() *C.char {
	return C.CString(dllBotStopAll())
}

// NebulaBotOnline 返回机器人是否已与网关完成鉴权（true/false）。
//
//export NebulaBotOnline
func NebulaBotOnline(id C.longlong) *C.char {
	return C.CString(dllBotOnline(int64(id)))
}

// NebulaBotRunning 返回机器人是否已下发上线任务（true/false）。
//
//export NebulaBotRunning
func NebulaBotRunning(id C.longlong) *C.char {
	return C.CString(dllBotRunning(int64(id)))
}

// NebulaBotRemove 下线机器人并释放其线程变量（删除机器人时调用）。
//
//export NebulaBotRemove
func NebulaBotRemove(id C.longlong, filePath *C.char) *C.char {
	return C.CString(dllBotRemove(int64(id), C.GoString(filePath)))
}

// ================= 消息收发 =================

// NebulaRecall 撤回消息。
//
//export NebulaRecall
func NebulaRecall(id C.longlong, appID, secret, scene, target, msgID *C.char) *C.char {
	return C.CString(dllRecall(int64(id), C.GoString(appID), C.GoString(secret), C.GoString(scene), C.GoString(target), C.GoString(msgID)))
}

// NebulaReply 回复消息；msgID 为空即主动发送。
//
//export NebulaReply
func NebulaReply(id C.longlong, appID, secret, scene, target, msgID, content *C.char) *C.char {
	return C.CString(dllReply(int64(id), C.GoString(appID), C.GoString(secret), C.GoString(scene), C.GoString(target), C.GoString(msgID), C.GoString(content)))
}

// ================= 词库执行 =================

// NebulaDicRun 执行一次词库匹配。
//
//export NebulaDicRun
func NebulaDicRun(path, content, trigger *C.char) *C.char {
	return C.CString(dllDicRun(C.GoString(path), C.GoString(content), C.GoString(trigger)))
}

// NebulaWebDicRun 执行一次网页词库。
//
//export NebulaWebDicRun
func NebulaWebDicRun(path, content *C.char) *C.char {
	return C.CString(dllWebDicRun(C.GoString(path), C.GoString(content)))
}

// NebulaDicReadInfo 静态读取词库内 [函数]词库信息 的名称/描述/价格。
//
//export NebulaDicReadInfo
func NebulaDicReadInfo(path, content *C.char) *C.char {
	return C.CString(dllDicReadInfo(C.GoString(path), C.GoString(content)))
}

// NebulaSandboxScan 扫描词库内容，返回其中调用的高危函数名（字符串数组）。
//
//export NebulaSandboxScan
func NebulaSandboxScan(content *C.char) *C.char {
	return C.CString(dllSandboxScan(C.GoString(content)))
}

// ================= 云词库公开访问 =================

// NebulaCloudServe 执行一次云词库页面请求（.n/.wn），返回响应状态、头部与正文。
//
//export NebulaCloudServe
func NebulaCloudServe(reqJSON *C.char) *C.char {
	return C.CString(dllCloudServe(C.GoString(reqJSON)))
}

// ================= 事件轮询 =================

// NebulaPollEvent 取出一个事件（空串表示暂无事件）。返回的原始 JSON 字符串同样需用
// NebulaFreeString 释放。
//
//export NebulaPollEvent
func NebulaPollEvent() *C.char {
	return C.CString(dllPollEvent())
}

// ================= 内存释放 =================

// NebulaFreeString 释放本 DLL 返回的任意 C 字符串。
//
//export NebulaFreeString
func NebulaFreeString(s *C.char) {
	if s != nil {
		C.free(unsafe.Pointer(s))
	}
}

// ================= legacy 导出（保持向后兼容） =================

// RunN 执行一次词库匹配（legacy）。
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

// RunCompiled 执行已编译的词库产物（legacy）。
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

// FreeString 释放 RunN 等接口返回的 C 字符串内存（legacy 别名）。
//
//export FreeString
func FreeString(s *C.char) {
	if s != nil {
		C.free(unsafe.Pointer(s))
	}
}

func main() {}
