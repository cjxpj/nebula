package funcs

import (
	"fmt"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/mobile"
)

// DexCallback 由 main_so.go 的 init() 注入，实现 Go → Java JNI 回调执行 DEX。
// 仅在 Android .so 编译时非 nil。
var DexCallback func(dexPath, className, methodName, argsJson string) (string, error)

// ---------- 设备信息 ----------

func DicDeviceInfo(d *dto.DicInputs) (any, error) {
	return mobile.GetDeviceInfo(), nil
}

// ---------- 电量 ----------

func DicBattery(d *dto.DicInputs) (any, error) {
	return fmt.Sprintf(`{"level":%d,"charging":%v}`, mobile.GetBatteryLevel(), mobile.IsBatteryCharging()), nil
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
