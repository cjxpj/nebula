package utils

import (
	"path/filepath"
	"strings"
	"sync"
)

// AccountLayout 描述宿主平台的「账号目录布局」：账号根目录下的各固定子目录名，
// 以及机器人账号路径内的词库目录名。
//
// 这些名字全部由宿主自定义，并通过引擎 Init 一并下发（见 SetAccountLayout）；
// 未下发（或字段为空）时回退到 DefaultAccountLayout，保证引擎独立运行
// （非宿主 DLL 模式，如自带 server）行为不变。
type AccountLayout struct {
	MyDicDir    string `json:"myDicDir"`    // 机器人词库目录（按机器人分子目录）
	MyStoreDir  string `json:"myStoreDir"`  // 通用存储目录
	MyCloudDir  string `json:"myCloudDir"`  // 网站词库目录
	DatabaseDir string `json:"databaseDir"` // 账号数据库目录
	BotDicDir   string `json:"botDicDir"`   // 机器人账号路径内的词库目录
	PrivateDir  string `json:"privateDir"`  // 私有目录（词库引入/资源的唯一来源）
	// SitePrefix 网站目录命名前缀（同属账号根目录）：账号根目录下以此前缀开头的目录
	// 与固定子目录并列（如宿主平台的「网站我的站」），其内词库同属该账号。
	// 为空表示宿主未启用网站目录，保持独立运行行为不变。
	SitePrefix string `json:"sitePrefix"`
}

// DefaultAccountLayout 返回引擎内置的账号目录布局默认值。
// PrivateDir 默认 private：引擎独立运行（非宿主 DLL 模式）时的应用数据目录。
func DefaultAccountLayout() AccountLayout {
	return AccountLayout{
		MyDicDir:    "机器人词库",
		MyStoreDir:  "储存",
		MyCloudDir:  "网站词库",
		DatabaseDir: "database",
		BotDicDir:   "dic",
		PrivateDir:  "private",
	}
}

var (
	accountLayoutMu sync.RWMutex
	accountLayout   = DefaultAccountLayout()
)

// SetAccountLayout 由宿主下发账号目录布局；空字段回退到默认值。
// 仅应在引擎 Init 阶段调用一次。
func SetAccountLayout(a AccountLayout) {
	def := DefaultAccountLayout()
	if a.MyDicDir == "" {
		a.MyDicDir = def.MyDicDir
	}
	if a.MyStoreDir == "" {
		a.MyStoreDir = def.MyStoreDir
	}
	if a.MyCloudDir == "" {
		a.MyCloudDir = def.MyCloudDir
	}
	if a.DatabaseDir == "" {
		a.DatabaseDir = def.DatabaseDir
	}
	if a.BotDicDir == "" {
		a.BotDicDir = def.BotDicDir
	}
	if a.PrivateDir == "" {
		a.PrivateDir = def.PrivateDir
	}
	accountLayoutMu.Lock()
	accountLayout = a
	accountLayoutMu.Unlock()
}

// CurrentAccountLayout 返回当前账号目录布局。
func CurrentAccountLayout() AccountLayout {
	accountLayoutMu.RLock()
	defer accountLayoutMu.RUnlock()
	return accountLayout
}

// DicDirName 返回机器人账号路径内的词库目录名（默认 "dic"）。
func DicDirName() string {
	return CurrentAccountLayout().BotDicDir
}

// PrivateDirName 返回私有目录名（默认 "private"）：词库的引入与编译期资源
// 只能从该目录（相对应用数据目录、或相对所属账号目录）读取。
func PrivateDirName() string {
	name := CurrentAccountLayout().PrivateDir
	if name == "" {
		name = DefaultAccountLayout().PrivateDir
	}
	return name
}

// PrivateRootRel 返回某词库（p 为词库文件路径）的私有目录「相对应用数据目录」的路径，
// 统一用正斜杠分隔，供拼接引入/资源目标路径与越界校验共用：
//   - 词库位于账号目录内时取「<账号目录>/<私有目录>」相对应用数据目录的路径，
//     即每个账号各自独立、互不可见的私有目录；
//   - 其余情况（引擎独立运行、词库在账号目录之外、内存词库等）回退为私有目录名本身，
//     即应用数据目录下的 private/，保证引擎独立运行行为不变。
//
// 账号目录的判定见 accountRelOf：宿主约定账号目录为词库根目录（沙箱/应用数据目录）
// 的直接子目录（<BotsRoot>/<uid>），账号下所有子目录（机器人词库、网站目录等）都归到
// 同一私有目录，不受固定子目录名限制。
func PrivateRootRel(p string) string {
	name := PrivateDirName()
	if rel, ok := accountRelOf(p); ok {
		return rel + "/" + name
	}
	return filepath.ToSlash(name)
}

// accountRelOf 判断词库路径 p 是否位于某个账号目录内，是则返回该账号目录
// 相对应用数据目录的路径（正斜杠分隔）。相对路径按应用数据目录解析。
func accountRelOf(p string) (string, bool) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", false
	}
	base := GetAppDir()
	if base == "" {
		base = WorkDir()
	}
	if base == "" {
		return "", false
	}
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", false
	}
	absP := p
	if !filepath.IsAbs(absP) {
		absP = filepath.Join(absBase, absP)
	}
	// 从词库所在目录开始逐级上溯：父目录恰为应用数据目录者即账号目录。
	for dir := filepath.Dir(absP); ; dir = filepath.Dir(dir) {
		parent := filepath.Dir(dir)
		if parent == absBase {
			rel, err := filepath.Rel(absBase, dir)
			if err != nil {
				return "", false
			}
			rel = filepath.ToSlash(rel)
			if rel == "" || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
				return "", false
			}
			return rel, true
		}
		if parent == dir {
			return "", false
		}
	}
}

// AccountRootSubDirs 返回账号根目录下的固定子目录名集合。
func (a AccountLayout) AccountRootSubDirs() []string {
	return []string{a.MyDicDir, a.MyStoreDir, a.MyCloudDir, a.PrivateDir}
}

// isAccountSubDir 判断目录名 base 是否为账号根目录下的子目录名：
// 命中固定子目录名，或（宿主启用网站目录前缀时）以该前缀开头——网站目录与固定子目录
// 并列在账号根目录下，其内词库同属该账号，必须一并以同样方式反推账号。
func (a AccountLayout) isAccountSubDir(base string) bool {
	for _, name := range a.AccountRootSubDirs() {
		if name != "" && base == name {
			return true
		}
	}
	return a.SitePrefix != "" && strings.HasPrefix(base, a.SitePrefix)
}

// AccountRootOf 从路径 p 向上查找账号根目录：命中任一账号子目录名
// （固定子目录名，或以网站目录前缀开头的网站目录）时返回其父目录（即账号目录），
// 否则返回 false。
// 例：.../users/1/机器人词库/123/dic → .../users/1；.../users/1/网站词库 → .../users/1；
// .../users/1/网站我的站/index.n → .../users/1。
func AccountRootOf(p string) (string, bool) {
	if strings.TrimSpace(p) == "" {
		return "", false
	}
	layout := CurrentAccountLayout()
	dir := filepath.Clean(p)
	for {
		if layout.isAccountSubDir(filepath.Base(dir)) {
			return filepath.Dir(dir), true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
