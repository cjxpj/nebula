package funcs

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/cjxpj/nebula/dto"
)

// 重启
func restart(d *dto.DicInputs) (any, error) {
	// 获取当前程序的路径
	exe, err := exec.LookPath(os.Args[0])
	if err != nil {
		return "", fmt.Errorf("无法找到程序路径: %v", err)
	}

	// 使用 os/exec 调用自身程序
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 启动新进程
	err = cmd.Start()
	if err != nil {
		return "", fmt.Errorf("重启失败: %v", err)
	}

	// 根据操作系统退出当前进程
	defer os.Exit(0)

	return "", nil
}
