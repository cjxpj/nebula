package funcs

import (
	"fmt"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/mobile"
)

// DexCallback 由 main_so.go 的 init() 注入，实现 Go → Java JNI 回调执行 DEX。
// 仅在 Android .so 编译时非 nil。
var DexCallback func(dexPath, className, methodName, argsJson string) (string, error)

// ShizukuCheckCallback 由 main_so.go 的 init() 注入，查询 Shizuku 服务状态。
var ShizukuCheckCallback func() (string, error)

// ShizukuExecCallback 由 main_so.go 的 init() 注入，使用 Shizuku 提权执行命令。
var ShizukuExecCallback func(command string) (string, error)

// ---------- 设备信息 ----------

func DicDeviceInfo(d *dto.DicInputs) (any, error) {
	return mobile.GetDeviceInfo(), nil
}

// ---------- 电量 ----------

func DicBattery(d *dto.DicInputs) (any, error) {
	return fmt.Sprintf(`{"level":%d,"charging":%v}`, mobile.GetBatteryLevel(), mobile.IsBatteryCharging()), nil
}

// ---------- 发送通知 ----------

// DicSendNotification 词库函数：$发送通知 标题 内容$
// 通过 JNI 回调 Android 系统通知栏发送通知。
func DicSendNotification(d *dto.DicInputs) (any, error) {
	if mobile.SendNotificationFunc == nil {
		return nil, fmt.Errorf("发送通知 仅支持 Android 端")
	}
	return nil, mobile.SendNotificationFunc(
		d.Inputs.String(1),
		d.Inputs.String(2),
	)
}

// ---------- 执行 DEX ----------

// DicExecuteDex 词库函数：$执行DEX dex路径 类名 方法名 [参数JSON]$
func DicExecuteDex(d *dto.DicInputs) (any, error) {
	if DexCallback == nil {
		return nil, fmt.Errorf("执行DEX 仅支持 Android 端")
	}
	argsJson := ""
	if d.Inputs.Len() >= 4 {
		argsJson = d.Inputs.String(4)
	}
	return DexCallback(
		d.Inputs.String(1),
		d.Inputs.String(2),
		d.Inputs.String(3),
		argsJson,
	)
}

// ---------- Shizuku ----------

// DicShizukuCheck 词库函数：$Shizuku检查$
// 返回 Shizuku 服务状态 JSON：{"available":bool,"granted":bool,"version":int}
func DicShizukuCheck(d *dto.DicInputs) (any, error) {
	if ShizukuCheckCallback == nil {
		return nil, fmt.Errorf("Shizuku检查 仅支持 Android 端")
	}
	return ShizukuCheckCallback()
}

// DicShizukuExec 词库函数：$Shizuku执行 命令$
// 使用 Shizuku 提权执行 Shell 命令（以 ADB shell 权限运行），返回 stdout+stderr
func DicShizukuExec(d *dto.DicInputs) (any, error) {
	if ShizukuExecCallback == nil {
		return nil, fmt.Errorf("Shizuku执行 仅支持 Android 端")
	}
	return ShizukuExecCallback(d.Inputs.String(1))
}
