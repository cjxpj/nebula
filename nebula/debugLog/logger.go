package debugLog

// Logger 日志需要实现的接口定义
type Logger interface {
	Debug(v ...any)
	Info(v ...any)
	Warn(v ...any)
	Error(v ...any)

	Debugf(format string, v ...any)
	Infof(format string, v ...any)
	Warnf(format string, v ...any)
	Errorf(format string, v ...any)
	// Sync logger Sync calls to flush buffer
	Sync() error
}
