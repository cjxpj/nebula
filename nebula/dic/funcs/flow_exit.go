//go:build !js

package funcs

import "os"

// exitProcess 结束程序：$STOP$ 执行后直接退出整个进程。
func exitProcess() {
	os.Exit(0)
}
