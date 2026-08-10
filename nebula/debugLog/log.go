// Package log 是 SDK 的 logger 接口定义与内置的 logger。
package debugLog

// DefaultLogger 默认logger
var DefaultLogger = Logger(new(consoleLogger))

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
