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

func installPython(destDir string, output *[]string) error {
	urls := []string{
		"https://registry.npmmirror.com/-/binary/python/3.12.8/python-3.12.8-embed-amd64.zip",
		"https://cjxpj.com/download/python-3.12.8-embed-amd64.zip",
	}

	zipPath := utils.NewFileQueue("python_download.zip")
	defer zipPath.DeleteFile() // 确保下载文件最终被删除

	*output = append(*output, "正在分段下载 Python ...")
	if err := zipPath.DownloadWithMirrors(urls, 8, true, nil); err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}

	*output = append(*output, "下载完成，正在解压...")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	if !zipPath.UnZip(destDir) {
		return fmt.Errorf("解压失败")
	}

	*output = append(*output, "✅ Python 安装成功，路径："+destDir)
	return nil
}

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
