// Package log 是 SDK 的 logger 接口定义与内置的 logger。
package debugLog

import "sync/atomic"

// DefaultLogger 默认logger
var DefaultLogger = Logger(new(consoleLogger))

// debugEnabled 全局调试开关，控制 Debug/Debugf 级别日志是否输出。
// 由前端服务器配置（config.yaml [HTTP] 调试）在启动与保存时同步。
var debugEnabled atomic.Bool

// SetDebug 设置全局调试开关，true 时输出 Debug/Debugf 日志。
func SetDebug(enabled bool) {
	debugEnabled.Store(enabled)
}

// DebugEnabled 返回当前全局调试开关状态。
func DebugEnabled() bool {
	return debugEnabled.Load()
}

// Debug log.Debug
func Debug(v ...any) {
	DefaultLogger.Debug(v...)
}

// Info log.Info
func Info(v ...any) {
	DefaultLogger.Info(v...)
}

// Warn log.Warn
func Warn(v ...any) {
	DefaultLogger.Warn(v...)
}

// Error log.Error
func Error(v ...any) {
	DefaultLogger.Error(v...)
}

// Debugf log.Debugf
func Debugf(format string, v ...any) {
	DefaultLogger.Debugf(format, v...)
}

// Infof log.Infof
func Infof(format string, v ...any) {
	DefaultLogger.Infof(format, v...)
}

// Warnf log.Warnf
func Warnf(format string, v ...any) {
	DefaultLogger.Warnf(format, v...)
}

// Errorf log.Errorf
func Errorf(format string, v ...any) {
	DefaultLogger.Errorf(format, v...)
}

// Sync logger Sync calls to flush buffer
func Sync() {
	_ = DefaultLogger.Sync()
}
