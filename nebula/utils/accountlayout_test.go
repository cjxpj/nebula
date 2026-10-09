package utils

import (
	"path/filepath"
	"testing"
)

// TestPrivateRootRel 验证私有目录解析：账号目录内的词库（任意子目录）归到该账号的
// 私有目录，账号目录之外/内存词库回退为应用数据目录下的私有目录名。
func TestPrivateRootRel(t *testing.T) {
	root := t.TempDir()
	SetAppDir(root)
	t.Cleanup(func() {
		SetAppDir("")
		SetAccountLayout(DefaultAccountLayout())
	})
	SetAccountLayout(AccountLayout{PrivateDir: "私有"})

	cases := []struct {
		name string
		p    string
		want string
	}{
		{"网站词库", filepath.Join(root, "1001", "网站词库", "主页.n"), "1001/私有"},
		{"机器人词库", filepath.Join(root, "1001", "机器人词库", "qq", "dic", "a.n"), "1001/私有"},
		{"站点目录", filepath.Join(root, "1001", "网站我的站", "index.n"), "1001/私有"},
		{"另一个账号", filepath.Join(root, "2002", "网站词库", "主页.n"), "2002/私有"},
		{"相对路径按应用目录解析", "1001/网站词库/主页.n", "1001/私有"},
		{"应用目录根下文件", filepath.Join(root, "test.n"), "私有"},
		{"空路径", "", "私有"},
		{"账号目录之外", filepath.Join(filepath.Dir(root), "outside.n"), "私有"},
	}
	for _, c := range cases {
		if got := PrivateRootRel(c.p); got != c.want {
			t.Errorf("%s: PrivateRootRel(%q) = %q, want %q", c.name, c.p, got, c.want)
		}
	}
}

// TestAccountRootOfSitePrefix 验证账号根目录反推：固定子目录与「网站」前缀目录
// （自定义网站，如「网站我的站」）都归属账号目录；未启用前缀时自定义网站目录不被识别。
func TestAccountRootOfSitePrefix(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(func() { SetAccountLayout(DefaultAccountLayout()) })

	cases := []struct {
		name string
		p    string
		want string
		ok   bool
	}{
		{"机器人词库", filepath.Join(root, "1001", "机器人词库", "qq", "dic", "a.n"), filepath.Join(root, "1001"), true},
		{"网站词库", filepath.Join(root, "1001", "网站词库", "主页.n"), filepath.Join(root, "1001"), true},
		{"储存", filepath.Join(root, "1001", "储存", "logs", "1.log"), filepath.Join(root, "1001"), true},
		{"账号目录", filepath.Join(root, "1001", "网站我的站", "index.n"), filepath.Join(root, "1001"), true},
		{"自定义网站子路径", filepath.Join(root, "1001", "网站我的站", "sub", "a.n"), filepath.Join(root, "1001"), true},
		{"非账号路径", filepath.Join(filepath.Dir(root), "outside.n"), "", false},
	}

	// 启用网站目录前缀：自定义网站目录与固定子目录同样反推到账号目录
	SetAccountLayout(AccountLayout{SitePrefix: "网站"})
	for _, c := range cases {
		got, ok := AccountRootOf(c.p)
		if ok != c.ok || got != c.want {
			t.Errorf("%s: AccountRootOf(%q) = (%q, %v)，期望 (%q, %v)", c.name, c.p, got, ok, c.want, c.ok)
		}
	}

	// 未启用前缀：自定义网站目录不再被识别（保持独立运行行为）
	SetAccountLayout(DefaultAccountLayout())
	if got, ok := AccountRootOf(cases[3].p); ok {
		t.Errorf("未启用网站前缀时不应识别自定义网站目录，得到 %q", got)
	}
	// 固定子目录仍识别
	if got, ok := AccountRootOf(cases[0].p); !ok || got != cases[0].want {
		t.Errorf("固定子目录应始终被识别，得到 (%q, %v)", got, ok)
	}
}
