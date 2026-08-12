//go:build windows

package dic_server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func replaceAndRestart(exePath, newExe string) error {
	oldExe := exePath + ".old"
	batFile := filepath.Join(os.TempDir(), "nebula_update.bat")

	// 生成批处理脚本：
	// 1. 循环尝试删除 .old 文件（进程未退出则删除失败 → 重试）
	// 2. 删除成功后，将 .new 移动到原位置
	// 3. 启动新版本
	// 4. 批处理自删
	script := fmt.Sprintf(`@echo off
chcp 65001 >nul
:wait
timeout /t 1 /nobreak >nul
del /f "%s" >nul 2>&1
if exist "%s" goto wait
move /y "%s" "%s"
start "" "%s"
del "%%~f0" & exit
`, oldExe, oldExe, newExe, exePath, exePath)

	if err := os.WriteFile(batFile, []byte(script), 0644); err != nil {
		return fmt.Errorf("创建更新脚本失败: %v", err)
	}

	// 重命名当前 exe 为 .old（Windows 允许重命名正在运行的可执行文件）
	if err := os.Rename(exePath, oldExe); err != nil {
		// 清理已创建的 bat 文件
		os.Remove(batFile)
		return fmt.Errorf("准备更新失败: %v", err)
	}

	// 启动批处理（分离进程，不等待子进程）
	cmd := exec.Command("cmd.exe", "/c", batFile)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008} // DETACHED_PROCESS
	cmd.Start()

	os.Exit(0)
	return nil
}
