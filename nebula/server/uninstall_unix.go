//go:build !windows

package dic_server

import (
	"fmt"
	"os"
)

// doUninstall 执行 Linux/macOS 端卸载：运行中的可执行文件可直接删除（inode 保留至进程退出）。
// 只删除程序本体（与勾选时的数据目录），start.n 等其余文件保留。
func doUninstall(p uninstallPlan) {
	// 先清理自启项（Linux 为 systemd user service）
	_ = CancelAutoStart()

	// 切出数据目录，避免删除当前工作目录
	_ = os.Chdir(os.TempDir())

	if err := os.Remove(p.exePath); err != nil {
		fmt.Println("卸载失败：删除程序文件出错:", err)
	}
	if p.removeData && p.dataDir != "" {
		if err := os.RemoveAll(p.dataDir); err != nil {
			fmt.Println("卸载失败：删除用户数据目录出错:", err)
		}
	}

	os.Exit(0)
}