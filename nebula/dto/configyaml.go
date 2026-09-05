package dto

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/cjxpj/nebula/utils"
	"gopkg.in/ini.v1"
	"gopkg.in/yaml.v3"
)

// CONFIG_PATH 合并后的单一配置文件路径：系统配置（原 system.ini）与对接配置（原 config.ini）合并到此 YAML。
const CONFIG_PATH = "private/system/config.yaml"

// configMu 保护 config.yaml 的并发读写，独立于 utils 的全局文件锁，不同文件路径之间互不阻塞。
var configMu sync.RWMutex

// ConfigFile 表示合并后的 YAML 配置，顶层为「节」→「键 → 值」的扁平结构，与旧 INI 的 [节] → 键=值 一一对应。
type ConfigFile struct {
	path string
	data map[string]any // section 名 -> map[string]any（键 -> 值）
}

// ConfigSection 对应配置中的一个节（等价于 INI 的 [节]）。
type ConfigSection struct {
	name string
	keys map[string]any
}

// ConfigKey 对应节内的一个键（等价于 INI 的 键 = 值）。
type ConfigKey struct {
	section *ConfigSection
	name    string
}

// configFilePath 返回合并配置文件的绝对路径（基于应用数据主目录）。
func configFilePath() string {
	return filepath.Join(utils.GetAppDir(), CONFIG_PATH)
}

// LoadConfigFile 加载合并配置文件；文件不存在时返回空配置（由调用方决定是否写入默认内容）。
func LoadConfigFile() (*ConfigFile, error) {
	f := &ConfigFile{path: configFilePath(), data: map[string]any{}}
	configMu.RLock()
	raw, err := os.ReadFile(f.path)
	configMu.RUnlock()
	if err != nil {
		if os.IsNotExist(err) {
			return f, nil
		}
		return nil, err
	}
	if len(raw) == 0 {
		return f, nil
	}

	var root map[string]any
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	if root == nil {
		root = map[string]any{}
	}
	f.data = root
	return f, nil
}

// Section 返回指定名称的节，不存在时创建空节。
func (c *ConfigFile) Section(name string) *ConfigSection {
	m, ok := c.data[name].(map[string]any)
	if !ok {
		m = map[string]any{}
		c.data[name] = m
	}
	return &ConfigSection{name: name, keys: m}
}

// Sections 返回全部节名（QQ 系列按编号升序，其余按字典序）。
func (c *ConfigFile) Sections() []string {
	names := make([]string, 0, len(c.data))
	for k := range c.data {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool {
		return sectionLess(names[i], names[j])
	})
	return names
}

// sectionLess 节名比较：QQ 系列（QQ、QQ2、QQ3…）按编号升序，其余按字典序。
func sectionLess(a, b string) bool {
	ai, aOK := qqSectionNum(a)
	bi, bOK := qqSectionNum(b)
	if aOK && bOK {
		return ai < bi
	}
	return a < b
}

// qqSectionNum 解析 QQ 实例编号：QQ -> 1，QQ2 -> 2；非 QQ 系列返回 0,false。
func qqSectionNum(name string) (int, bool) {
	if name == "QQ" {
		return 1, true
	}
	if strings.HasPrefix(name, "QQ") {
		if n, err := strconv.Atoi(name[2:]); err == nil {
			return n, true
		}
	}
	return 0, false
}

// DeleteSection 删除指定名称的节。
func (c *ConfigFile) DeleteSection(name string) {
	delete(c.data, name)
}

// Save 将配置写回 YAML 文件，目录不存在时自动创建。
func (c *ConfigFile) Save() error {
	p := c.path
	if p == "" {
		p = configFilePath()
	}

	out, err := yaml.Marshal(c.data)
	if err != nil {
		return err
	}

	configMu.Lock()
	defer configMu.Unlock()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	return os.WriteFile(p, out, 0644)
}

// Name 返回节名。
func (s *ConfigSection) Name() string {
	return s.name
}

// Key 返回节内指定名称的键。
func (s *ConfigSection) Key(name string) *ConfigKey {
	return &ConfigKey{section: s, name: name}
}

func (k *ConfigKey) get() (any, bool) {
	v, ok := k.section.keys[k.name]
	return v, ok
}

// String 返回键的字符串值，键不存在或值为空时返回空串。
func (k *ConfigKey) String() string {
	v, ok := k.get()
	if !ok || v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// SetValue 以字符串形式设置键值（与旧 INI 语义一致）。
func (k *ConfigKey) SetValue(v string) {
	k.section.keys[k.name] = v
}

// MustBool 返回键的布尔值，解析失败或不存在时返回默认值。
func (k *ConfigKey) MustBool(def bool) bool {
	v, ok := k.get()
	if !ok || v == nil {
		return def
	}
	b, err := strconv.ParseBool(strings.TrimSpace(fmt.Sprintf("%v", v)))
	if err != nil {
		return def
	}
	return b
}

// Bool 返回键的布尔值与解析错误。
func (k *ConfigKey) Bool() (bool, error) {
	v, ok := k.get()
	if !ok || v == nil {
		return false, fmt.Errorf("配置键 %q 不存在", k.name)
	}
	return strconv.ParseBool(strings.TrimSpace(fmt.Sprintf("%v", v)))
}

// MustInt 返回键的整数值，解析失败或不存在时返回默认值。
func (k *ConfigKey) MustInt(def int) int {
	v, ok := k.get()
	if !ok || v == nil {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(fmt.Sprintf("%v", v)))
	if err != nil {
		return def
	}
	return n
}

// Int 返回键的整数值与解析错误。
func (k *ConfigKey) Int() (int, error) {
	v, ok := k.get()
	if !ok || v == nil {
		return 0, fmt.Errorf("配置键 %q 不存在", k.name)
	}
	return strconv.Atoi(strings.TrimSpace(fmt.Sprintf("%v", v)))
}

// MigrateIniToYaml 将旧的 system.ini / config.ini 合并迁移为 config.yaml。
// 仅当 config.yaml 不存在时执行，避免覆盖已迁移的配置；迁移失败静默跳过，由调用方写入默认配置兜底。
func MigrateIniToYaml() {
	if _, err := os.Stat(configFilePath()); err == nil {
		return
	}

	merged := map[string]any{}
	for _, iniPath := range []string{"private/system/system.ini", "private/system/config.ini"} {
		f, err := ini.Load(filepath.Join(utils.GetAppDir(), iniPath))
		if err != nil {
			continue
		}
		for _, sec := range f.Sections() {
			name := sec.Name()
			if name == "" || name == ini.DefaultSection {
				continue
			}
			keys := sec.Keys()
			if len(keys) == 0 {
				continue
			}
			m, ok := merged[name].(map[string]any)
			if !ok {
				m = map[string]any{}
				merged[name] = m
			}
			for _, k := range keys {
				m[k.Name()] = k.String()
			}
		}
	}
	if len(merged) == 0 {
		return
	}

	(&ConfigFile{path: configFilePath(), data: merged}).Save()
}
