package dic_server

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/cjxpj/nebula/utils"
)

// uninstallPlan 卸载计划：记录需要删除的目标。
// 程序本体（exe）始终删除；start.n 等其他文件一律保留，便于重装后继续使用；
// 用户数据目录仅在勾选「同时卸载用户数据」时删除。
type uninstallPlan struct {
	exePath    string // 程序本体（可执行文件）
	dataDir    string // 用户数据目录（NebulaData）；为空表示未定位到，不删除
	removeData bool   // 是否同时删除用户数据
}

// startUninstall 校验卸载可行性并异步执行卸载。
// 可行时先同步返回，再延迟片刻执行删除与退出，确保接口响应先送达前端。
func startUninstall(removeData bool) error {
	if runtime.GOOS == "android" || runtime.GOOS == "ohos" {
		return fmt.Errorf("移动端不支持面板卸载，请在系统设置中卸载应用")
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取程序路径失败: %v", err)
	}
	if exePath, err = filepath.Abs(exePath); err != nil {
		return fmt.Errorf("解析程序路径失败: %v", err)
	}
	if _, err := os.Stat(exePath); err != nil {
		return fmt.Errorf("程序文件不存在: %v", err)
	}

	plan := uninstallPlan{
		exePath:    exePath,
		dataDir:    resolveUninstallDataDir(exePath),
		removeData: removeData,
	}

	go func() {
		time.Sleep(1200 * time.Millisecond)
		doUninstall(plan)
	}()
	return nil
}

// resolveUninstallDataDir 定位用户数据目录（NebulaData）。
// 桌面端数据目录即进程当前工作目录（启动词库会切换到 NebulaData），
// 仅当目录名为 NebulaData 时返回，避免误删用户自定义目录。
func resolveUninstallDataDir(exePath string) string {
	var dir string
	if d := utils.GetAppDir(); d != "" {
		dir = d
	} else if wd, err := os.Getwd(); err == nil {
		dir = wd
	}
	if dir == "" {
		return ""
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}

	exeDir := filepath.Dir(exePath)
	// 工作目录仍是程序目录（未切换到数据目录）时，退回程序目录下的 NebulaData
	if dir == exeDir {
		dir = filepath.Join(exeDir, "NebulaData")
	}
	// 安全校验：只允许删除名为 NebulaData 的目录
	if filepath.Base(dir) != "NebulaData" || dir == exeDir {
		return ""
	}
	return dir
}