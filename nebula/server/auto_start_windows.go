//go:build windows

package dic_server

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const autoStartAppName = "NebulaApp"

// SetAutoStart 写入 Windows 注册表实现开机自启
func SetAutoStart() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	k, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Run`,
		registry.SET_VALUE,
	)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(autoStartAppName, fmt.Sprintf(`"%s" --autostart`, exePath))
}

// CancelAutoStart 从注册表删除开机自启项
func CancelAutoStart() error {
	k, err := registry.OpenKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Run`,
		registry.SET_VALUE,
	)
	if err != nil {
		return err
	}
	defer k.Close()
	err = k.DeleteValue(autoStartAppName)
	// 如果值本身不存在，视为已取消，不报错
	if err != nil && !errors.Is(err, syscall.Errno(2)) {
		return err
	}
	return nil
}

// GetAutoStart 检查注册表中是否存在开机自启项
func GetAutoStart() (bool, error) {
	k, err := registry.OpenKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Run`,
		registry.QUERY_VALUE,
	)
	if err != nil {
		return false, fmt.Errorf("打开注册表失败: %w", err)
	}
	defer k.Close()
	_, _, err = k.GetStringValue(autoStartAppName)
	if err != nil {
		// 值不存在 = 未设置自启
		return false, nil
	}
	return true, nil
}
