//go:build !windows

package dic_server

import (
	"fmt"
	"os"
	"os/exec"
)

func replaceAndRestart(exePath, newExe string) error {
	// 重命名当前文件，释放原路径用于新文件写入（运行中的进程仍持有 inode）
	oldExe := exePath + ".old"
	os.Rename(exePath, oldExe)

	// 替换为新文件
	if err := os.Rename(newExe, exePath); err != nil {
		// 回滚
		os.Rename(oldExe, exePath)
		return fmt.Errorf("替换文件失败: %v", err)
	}

	// 确保可执行权限
	os.Chmod(exePath, 0755)

	// 重启
	cmd := exec.Command(exePath, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("重启失败: %v", err)
	}
	os.Exit(0)
	return nil
}
