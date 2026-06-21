//go:build darwin

package dic_server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	darwinServiceName = "com.nebula.app"
	darwinServiceDir  = "Library/LaunchAgents"
)

// SetAutoStart 创建 launchd plist 实现开机自启
func SetAutoStart() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取可执行文件路径失败: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取用户目录失败: %w", err)
	}

	serviceDir := filepath.Join(homeDir, darwinServiceDir)
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		return fmt.Errorf("创建 LaunchAgents 目录失败: %w", err)
	}

	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
	</array>
	<key>WorkingDirectory</key>
	<string>%s</string>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>%s/Library/Logs/%s.log</string>
	<key>StandardErrorPath</key>
	<string>%s/Library/Logs/%s.err.log</string>
	<key>ProcessType</key>
	<string>Interactive</string>
</dict>
</plist>
`, darwinServiceName, exePath, filepath.Dir(exePath),
		homeDir, darwinServiceName,
		homeDir, darwinServiceName)

	plistPath := filepath.Join(serviceDir, darwinServiceName+".plist")
	if err := os.WriteFile(plistPath, []byte(plistContent), 0644); err != nil {
		return fmt.Errorf("写入 plist 文件失败: %w", err)
	}

	// 加载 plist 到 launchd
	if err := exec.Command("launchctl", "load", plistPath).Run(); err != nil {
		return fmt.Errorf("launchctl load 失败: %w", err)
	}

	return nil
}

// CancelAutoStart 移除 launchd plist
func CancelAutoStart() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取用户目录失败: %w", err)
	}

	plistPath := filepath.Join(homeDir, darwinServiceDir, darwinServiceName+".plist")

	// 先 unload 再删除文件
	if _, err := os.Stat(plistPath); err == nil {
		exec.Command("launchctl", "unload", plistPath).Run()
		if err := os.Remove(plistPath); err != nil {
			return fmt.Errorf("删除 plist 文件失败: %w", err)
		}
	}

	return nil
}

// GetAutoStart 检查 launchd plist 是否存在
func GetAutoStart() (bool, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return false, fmt.Errorf("获取用户目录失败: %w", err)
	}

	plistPath := filepath.Join(homeDir, darwinServiceDir, darwinServiceName+".plist")
	if _, err := os.Stat(plistPath); err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else {
		return false, fmt.Errorf("检查 plist 文件失败: %w", err)
	}
}
