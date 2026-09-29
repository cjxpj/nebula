package dic_server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cjxpj/nebula/utils"
)

// TestResolveUninstallDataDir 覆盖卸载数据目录的定位与安全校验：
// 只有名为 NebulaData 的目录才允许被删除，避免误删用户自定义目录或程序目录。
func TestResolveUninstallDataDir(t *testing.T) {
	exeDir := t.TempDir()
	exePath := filepath.Join(exeDir, "nebula.exe")
	if err := os.WriteFile(exePath, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(exeDir, "NebulaData")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}

	utils.SetAppDir("")
	defer utils.SetAppDir("")

	// 工作目录已切到 NebulaData：返回该目录
	t.Chdir(dataDir)
	if got := resolveUninstallDataDir(exePath); got != dataDir {
		t.Fatalf("工作目录为 NebulaData 时应返回该目录，实际 %q", got)
	}

	// 工作目录仍是程序目录：退回程序目录下的 NebulaData
	t.Chdir(exeDir)
	if got := resolveUninstallDataDir(exePath); got != dataDir {
		t.Fatalf("工作目录为程序目录时应退回 NebulaData，实际 %q", got)
	}

	// 自定义目录名：安全校验拒绝返回（不删除）
	customDir := filepath.Join(exeDir, "MyData")
	if err := os.MkdirAll(customDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(customDir)
	if got := resolveUninstallDataDir(exePath); got != "" {
		t.Fatalf("非 NebulaData 目录应拒绝，实际 %q", got)
	}
}