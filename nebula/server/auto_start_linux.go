//go:build linux

package dic_server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	linuxServiceName = "nebula"
	linuxServiceDir  = ".config/systemd/user"
)

// SetAutoStart 创建 systemd user service 实现开机自启
func SetAutoStart() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取可执行文件路径失败: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取用户目录失败: %w", err)
	}

	serviceDir := filepath.Join(homeDir, linuxServiceDir)
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		return fmt.Errorf("创建 systemd user 目录失败: %w", err)
	}

	serviceContent := fmt.Sprintf(`[Unit]
Description=Nebula 服务
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s
WorkingDirectory=%s
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=default.target
`, exePath, filepath.Dir(exePath))

	servicePath := filepath.Join(serviceDir, linuxServiceName+".service")
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("写入 service 文件失败: %w", err)
	}

	// 重新加载 systemd user daemon 并启用服务
	if err := runCmd("systemctl", "--user", "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload 失败: %w", err)
	}
	if err := runCmd("systemctl", "--user", "enable", linuxServiceName+".service"); err != nil {
		return fmt.Errorf("systemctl enable 失败: %w", err)
	}

	return nil
}

// CancelAutoStart 移除 systemd user service
func CancelAutoStart() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取用户目录失败: %w", err)
	}

	servicePath := filepath.Join(homeDir, linuxServiceDir, linuxServiceName+".service")

	// 先 disable 再删除
	runCmd("systemctl", "--user", "disable", linuxServiceName+".service")
	runCmd("systemctl", "--user", "daemon-reload")

	if _, err := os.Stat(servicePath); err == nil {
		if err := os.Remove(servicePath); err != nil {
			return fmt.Errorf("删除 service 文件失败: %w", err)
		}
	}

	return nil
}

// GetAutoStart 检查 systemd user service 是否存在且已启用
func GetAutoStart() (bool, error) {
	cmd := exec.Command("systemctl", "--user", "is-enabled", linuxServiceName+".service")
	output, err := cmd.Output()
	if err != nil {
		// systemctl is-enabled 返回非0 = 未启用
		return false, nil
	}
	outStr := string(output)
	return outStr == "enabled\n", nil
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}
