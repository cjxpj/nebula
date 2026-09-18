//go:build linux

package dic_server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	linuxServiceName = "nebula"
	linuxServiceDir  = ".config/systemd/user"
)

// systemctlEnv 返回执行 systemctl/loginctl 所需的环境变量。
// 无桌面会话（SSH、后台进程、systemd 服务内）时 XDG_RUNTIME_DIR 可能缺失，
// 导致 --user 命令无法连接用户总线，这里显式补上 /run/user/<uid>。
func systemctlEnv() []string {
	env := os.Environ()
	if os.Getenv("XDG_RUNTIME_DIR") == "" {
		env = append(env, fmt.Sprintf("XDG_RUNTIME_DIR=/run/user/%d", os.Getuid()))
	}
	return env
}

// runUserCmd 以正确环境执行 systemctl/loginctl 命令，失败时带出 stderr 便于排查。
func runUserCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = systemctlEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return fmt.Errorf("%s %s 失败: %w", name, strings.Join(args, " "), err)
		}
		return fmt.Errorf("%s %s 失败: %s", name, strings.Join(args, " "), msg)
	}
	return nil
}

// systemdQuote 将路径安全写入 ExecStart/WorkingDirectory 等字段，避免含空格等特殊字符被拆分。
func systemdQuote(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

// enableLinger 开启当前用户的 systemd linger，使用户级服务在开机（而非仅登录）时也能启动。
// loginctl 不可用（非 systemd 环境）时忽略失败，退化为登录自启。
func enableLinger() {
	_ = runUserCmd("loginctl", "enable-linger", fmt.Sprintf("%d", os.Getuid()))
}

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

	// 先开启 linger，确保开机即可启动用户级服务，同时让无桌面会话下 systemctl --user 可用
	enableLinger()

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
`, systemdQuote(exePath), systemdQuote(filepath.Dir(exePath)))

	servicePath := filepath.Join(serviceDir, linuxServiceName+".service")
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("写入 service 文件失败: %w", err)
	}

	// 重新加载 systemd user daemon 并启用服务
	if err := runUserCmd("systemctl", "--user", "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload 失败: %w", err)
	}
	if err := runUserCmd("systemctl", "--user", "enable", linuxServiceName+".service"); err != nil {
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
	_ = runUserCmd("systemctl", "--user", "disable", linuxServiceName+".service")
	_ = runUserCmd("systemctl", "--user", "daemon-reload")

	if _, err := os.Stat(servicePath); err == nil {
		if err := os.Remove(servicePath); err != nil {
			return fmt.Errorf("删除 service 文件失败: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("检查 service 文件失败: %w", err)
	}

	return nil
}

// GetAutoStart 检查 systemd user service 是否已启用
func GetAutoStart() (bool, error) {
	cmd := exec.Command("systemctl", "--user", "is-enabled", linuxServiceName+".service")
	cmd.Env = systemctlEnv()
	out, err := cmd.Output()
	if err != nil {
		// systemctl is-enabled 返回非0 = 未启用
		return false, nil
	}
	return strings.HasPrefix(strings.TrimSpace(string(out)), "enabled"), nil
}
