// Package mobile 手机端专属状态，供 JNI 层和词库函数共享访问。
package mobile

import "sync"

var (
	mu              sync.RWMutex
	deviceInfo      string
	batteryLevel    int = -1
	batteryCharging bool
)

// SetDeviceInfo 由 JNI setDeviceInfo 调用，保存设备信息 JSON。
func SetDeviceInfo(info string) {
	mu.Lock()
	deviceInfo = info
	mu.Unlock()
}

// GetDeviceInfo 供词库函数 / Go 层读取设备信息。
func GetDeviceInfo() string {
	mu.RLock()
	defer mu.RUnlock()
	return deviceInfo
}

// UpdateBattery 由 JNI updateBatteryStatus 调用。
func UpdateBattery(level int, charging bool) {
	mu.Lock()
	batteryLevel = level
	batteryCharging = charging
	mu.Unlock()
}

// GetBatteryLevel 返回电量百分比 (0-100)，-1 表示未知。
func GetBatteryLevel() int {
	mu.RLock()
	defer mu.RUnlock()
	return batteryLevel
}

// IsBatteryCharging 是否正在充电。
func IsBatteryCharging() bool {
	mu.RLock()
	defer mu.RUnlock()
	return batteryCharging
}
