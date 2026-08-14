//go:build ohos && (arm64 || amd64)

package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"

	"github.com/cjxpj/nebula/dic"
	"github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/mobile"
	"github.com/cjxpj/nebula/utils"
)

func init() {
	// 鸿蒙端可用的手机端词库函数（Shizuku/DEX 等安卓专属能力不注册）
	funcs.Registers(
		dto.RegisterDicFunc{Name: "设备信息", L: "0", Fn: funcs.DicDeviceInfo},
		dto.RegisterDicFunc{Name: "设备电量", L: "0", Fn: funcs.DicBattery},
		dto.RegisterDicFunc{Name: "发送通知", L: "2", Fn: funcs.DicSendNotification},
	)

	// 鸿蒙端无法像 Android 那样通过 JNI 回调 ArkTS，
	// 改为把通知放入队列，由 ArkTS 侧通过 NebulaPollNotification 轮询拉取。
	mobile.SendNotificationFunc = func(title, content string) error {
		mobile.PushNotification(title, content)
		return nil
	}
}

// NebulaSetDataDir 接收 ArkTS 侧注入的应用沙箱数据目录。
// 必须在 NebulaRun 之前调用，否则配置文件写入相对路径会失败导致退出。
//
//export NebulaSetDataDir
func NebulaSetDataDir(dir *C.char) {
	utils.SetAppDir(C.GoString(dir))
}

// NebulaRun 启动 Go HTTP 服务（非阻塞，内部起 goroutine）。
//
//export NebulaRun
func NebulaRun() {
	startupResult := dic.Start()
	if startupResult != "" {
		mobile.SetStartupUrl(startupResult)
	}
}

// NebulaGetOpuiUrl 返回 WebView 启动页路径。返回的 C 字符串由调用方 free。
//
//export NebulaGetOpuiUrl
func NebulaGetOpuiUrl() *C.char {
	url := mobile.GetStartupUrl()
	if url == "" && dto.ServerConfig.OPUI != nil {
		url = dto.ServerConfig.OPUI.Addr
	}
	return C.CString(url)
}

// NebulaSetDeviceInfo 接收 ArkTS 侧采集的设备信息 JSON。
//
//export NebulaSetDeviceInfo
func NebulaSetDeviceInfo(info *C.char) {
	mobile.SetDeviceInfo(C.GoString(info))
}

// NebulaUpdateBattery 接收 ArkTS 侧电量回调（charging: 非 0 表示充电中）。
//
//export NebulaUpdateBattery
func NebulaUpdateBattery(level C.int, charging C.int) {
	mobile.UpdateBattery(int(level), charging != 0)
}

// NebulaPollNotification 拉取队首待发送通知。
// 无通知时返回空字符串；否则返回 {"title":..,"content":..} JSON。调用方负责 free。
//
//export NebulaPollNotification
func NebulaPollNotification() *C.char {
	title, content, ok := mobile.PopNotification()
	if !ok {
		return C.CString("")
	}
	b, _ := json.Marshal(map[string]string{"title": title, "content": content})
	return C.CString(string(b))
}

func main() {}
