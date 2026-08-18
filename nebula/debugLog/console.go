package debugLog

import (
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

var _ Logger = (*consoleLogger)(nil)

// consoleLogger 命令行日志实现
type consoleLogger struct{}

// Debug 日志
func (consoleLogger) Debug(v ...any) {
	output("Debug", fmt.Sprint(v...))
}

// Info 日志
func (consoleLogger) Info(v ...any) {
	output("Info", fmt.Sprint(v...))
}

// Warn 日志
func (consoleLogger) Warn(v ...any) {
	output("Warning", fmt.Sprint(v...))
}

// Error
func (consoleLogger) Error(v ...any) {
	output("Error", fmt.Sprint(v...))
}

// Debugf Debug Format 日志
func (consoleLogger) Debugf(format string, v ...any) {
	output("Debug", fmt.Sprintf(format, v...))
}

// Infof Info Format 日志
func (consoleLogger) Infof(format string, v ...any) {
	output("Info", fmt.Sprintf(format, v...))
}

// Warnf Warning Format 日志
func (consoleLogger) Warnf(format string, v ...any) {
	output("Warning", fmt.Sprintf(format, v...))
}

// Errorf Error Format 日志
func (consoleLogger) Errorf(format string, v ...any) {
	output("Error", fmt.Sprintf(format, v...))
}

// Sync 控制台 logger 不需要 sync
func (consoleLogger) Sync() error {
	return nil
}

func output(level string, v ...any) {
	pc, file, line, _ := runtime.Caller(3)
	file = filepath.Base(file)
	funcName := strings.TrimPrefix(filepath.Ext(runtime.FuncForPC(pc).Name()), ".")

	date := time.Now().Format("2006-01-02 15:04:05")
	msg := EscapeControlChars(fmt.Sprint(v...))

	fmt.Printf("[%s] %s %s:%d:%s %s\n", level, date, file, line, funcName, msg)
}

// EscapeControlChars 将消息中的控制字符转义为可见文本，保证日志单行输出。
// 先转义反斜杠，再转义换行等控制字符，避免原本就是 "\n" 字面量被二次混淆。
func EscapeControlChars(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}

// unescapeControlCharsRe 匹配被 EscapeControlChars 转义出的可见序列：\n、\r、\t、\\
var unescapeControlCharsRe = regexp.MustCompile(`\\(\\|n|r|t)`)

// UnescapeControlChars 还原 EscapeControlChars 转义的控制字符（\n、\r、\t、\\）。
// 用于在真实终端回显时恢复多行等原始显示效果，与前端 unescapeControlChars 保持一致。
func UnescapeControlChars(s string) string {
	return unescapeControlCharsRe.ReplaceAllStringFunc(s, func(m string) string {
		switch m {
		case `\\`:
			return `\`
		case `\n`:
			return "\n"
		case `\r`:
			return "\r"
		case `\t`:
			return "\t"
		}
		return m
	})
}
