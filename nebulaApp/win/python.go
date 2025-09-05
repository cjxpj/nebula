package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// RunPythonCode 执行一段 Python 脚本，返回输出或错误
func runPythonCode(code string) (string, error) {
	// 获取当前可执行文件所在目录
	appDir := utils.GetAppDir()
	pythonDir := filepath.Join(appDir, "private", "python")

	// 确保目录存在
	if _, err := os.Stat(pythonDir); os.IsNotExist(err) {
		if err := os.MkdirAll(pythonDir, 0755); err != nil {
			return "", fmt.Errorf("创建目录失败: %v", err)
		}
	}

	// 创建临时 Python 文件
	tmpFile, err := os.CreateTemp("", "tempscript-*.py")
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// 写入脚本内容
	if _, err := tmpFile.WriteString(code); err != nil {
		return "", fmt.Errorf("写入脚本失败: %v", err)
	}
	tmpFile.Close()

	// 获取 Python 执行路径
	pythonExec := dto.GV.GetStr("_PythonPath_")
	if pythonExec == "" {
		return "", fmt.Errorf("未设置 Python 执行路径 (_PythonPath_)")
	}

	// 执行命令
	cmd := exec.Command(pythonExec, tmpFile.Name())
	cmd.Dir = pythonDir // 保证 import 本地模块时在正确目录

	// 获取输出
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("Python 执行失败: %v\n输出:\n%s", err, output), nil
	}

	return string(output), nil
}
