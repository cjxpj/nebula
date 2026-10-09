package botdic_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	botdic "github.com/cjxpj/nebula/bot/botdic"
	_ "github.com/cjxpj/nebula/dic" // 触发引擎初始化：注入 dic_api.Api
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"
	"github.com/cjxpj/nebula/utils"
)

// countGob 统计目录下的 .gob 编译缓存文件数量。
func countGob(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".gob" {
			n++
		}
	}
	return n
}

// waitCache 等待异步写盘出现缓存文件（对照用）。
func waitCache(t *testing.T, dir string) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if countGob(dir) > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("等待编译缓存写入超时：%s", dir)
}

// TestExternalDicNoDisk 验证「外部词库」（商城/云词库）执行不落盘：
//   - 内容取自 Real（公共目录的真实文件），虚拟路径不真实存在；
//   - 账号上下文仍按 Virtual 推断（词库能正常执行）；
//   - 完全不写磁盘编译缓存（对照：账号目录下的普通词库会写）。
func TestExternalDicNoDisk(t *testing.T) {
	root := t.TempDir()
	utils.SetAppDir(root)
	t.Cleanup(func() { utils.SetAppDir("") })

	dto.ServerConfig.DicCache = true
	t.Cleanup(func() { dto.ServerConfig.DicCache = false })

	cacheDir := t.TempDir()
	run.SetDicCacheDirFunc(func(string) string { return cacheDir })
	t.Cleanup(func() { run.SetDicCacheDirFunc(nil) })

	account := filepath.Join(root, "1001", "机器人词库", "7")
	dicDir := filepath.Join(account, "dic")
	if err := os.MkdirAll(dicDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// 对照：账号目录下的普通词库会写编译缓存
	own := filepath.Join(dicDir, "own.n")
	if err := os.WriteFile(own, []byte("\nMain\n自有词库内容"), 0o644); err != nil {
		t.Fatal(err)
	}
	var got string
	botdic.Run{FilePath: account, Trigger: "Main", Deliver: func(msg string, _ *dto.DicVal) { got = msg }}.Exec()
	if got != "自有词库内容" {
		t.Fatalf("普通词库未执行，得到 %q", got)
	}
	waitCache(t, cacheDir)

	// 清空对照缓存，移除普通词库，改为注册「外部词库」
	if err := os.RemoveAll(cacheDir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(own)

	real := filepath.Join(t.TempDir(), "p_1.n")
	if err := os.WriteFile(real, []byte("\nMain\n外部词库命中"), 0o644); err != nil {
		t.Fatal(err)
	}
	virtual := filepath.Join(dicDir, "pack_1.n")
	botdic.SetExtraDics(account, []botdic.ExtraDic{{Virtual: virtual, Real: real}})
	t.Cleanup(func() { botdic.SetExtraDics(account, nil) })

	// 外部词库出现在待执行列表里（虚拟路径）
	if files := botdic.ListFiles(account, false); len(files) != 1 || files[0] != virtual {
		t.Fatalf("ListFiles=%v，期望仅含外部词库虚拟路径 %q", files, virtual)
	}

	got = ""
	botdic.Run{FilePath: account, Trigger: "Main", Deliver: func(msg string, _ *dto.DicVal) { got = msg }}.Exec()
	if got != "外部词库命中" {
		t.Fatalf("外部词库未执行，得到 %q", got)
	}
	if _, err := os.Stat(virtual); err == nil {
		t.Fatalf("外部词库的虚拟路径不应真实落盘：%s", virtual)
	}
	if n := countGob(cacheDir); n != 0 {
		t.Fatalf("外部词库不应写编译缓存，实际缓存文件数=%d", n)
	}
}
