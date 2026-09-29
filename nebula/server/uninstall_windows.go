//go:build windows

package dic_server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// doUninstall 执行 Windows 端卸载：清理自启项后把删除工作交给批处理。
// 运行中的 exe 无法直接删除，先重命名释放路径，批处理等待进程退出（文件解锁）后再删除。
// 只删除程序本体（与勾选时的数据目录），start.n 等其余文件保留。
func doUninstall(p uninstallPlan) {
	// 先清理开机自启，避免卸载后注册表仍指向已删除的程序
	_ = CancelAutoStart()

	// Windows 允许重命名正在运行的程序，重命名后批处理只需等待 .old 解锁
	target := p.exePath
	oldExe := p.exePath + ".old"
	if err := os.Rename(p.exePath, oldExe); err == nil {
		target = oldExe
	}

	script := fmt.Sprintf(`@echo off
chcp 65001 >nul
:wait
timeout /t 1 /nobreak >nul
del /f "%s" >nul 2>&1
if exist "%s" goto wait
`, target, target)
	if p.removeData && p.dataDir != "" {
		script += fmt.Sprintf("rd /s /q \"%s\" >nul 2>&1\n", p.dataDir)
	}
	script += "del \"%~f0\" & exit\n"

	batFile := filepath.Join(os.TempDir(), "nebula_uninstall.bat")
	if err := os.WriteFile(batFile, []byte(script), 0644); err != nil {
		// 脚本写入失败时回滚改名，保证程序仍可正常启动
		if target != p.exePath {
			_ = os.Rename(target, p.exePath)
		}
		fmt.Println("卸载失败：创建卸载脚本出错:", err)
		return
	}

	cmd := exec.Command("cmd.exe", "/c", batFile)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008} // DETACHED_PROCESS
	_ = cmd.Start()

	os.Exit(0)
}