// Package mobile 手机端专属状态，供 JNI 层和词库函数共享访问。
package mobile

import "sync"

var (
	mu              sync.RWMutex
	deviceInfo      string
	batteryLevel    int = -1
	batteryCharging bool
	startupUrl      string // 词库自定义的启动页路径

	// SendNotificationFunc 由 main_so.go 注入，通过 JNI 回调 Android 发送系统通知
	SendNotificationFunc func(title, content string) error

	// UpdateDownloadProgressFunc 在线更新下载进度回调（通知栏进度条）
	UpdateDownloadProgressFunc func(progress, total int64) error

	// InstallApkFunc 在线更新下载完成后弹出 APK 安装
	InstallApkFunc func(apkPath string) error
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

// SetStartupUrl 由词库函数 $设置启动页$ 调用，自定义 WebView 启动加载的路径。
func SetStartupUrl(url string) {
	mu.Lock()
	startupUrl = url
	mu.Unlock()
}

// GetStartupUrl 供 getOpuiUrl JNI 调用，优先返回词库设置的启动页。
func GetStartupUrl() string {
	mu.RLock()
	defer mu.RUnlock()
	return startupUrl
}
