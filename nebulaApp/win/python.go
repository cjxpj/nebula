package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// RunPythonCode 执行一段 Python 脚本（通过 stdin 传入，无临时文件）
func runPythonCode(code string) (string, error) {
	appDir := utils.GetAppDir()
	pythonDir := filepath.Join(appDir, "private", "extensions", "python")

	// 确保目录存在（脚本内 import 本地模块时可能需要）
	_ = os.MkdirAll(pythonDir, 0755)

	pythonExec := dto.GV.GetStr("_PythonPath_")
	if pythonExec == "" {
		return "", fmt.Errorf("未设置 Python 执行路径 (_PythonPath_)")
	}

	cmd := exec.Command(pythonExec, "-")
	cmd.Dir = pythonDir
	cmd.Stdin = strings.NewReader(code)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("Python 执行失败: %v\n输出:\n%s", err, output), nil
	}

	return string(output), nil
}
