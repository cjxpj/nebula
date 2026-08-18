package funcs

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
	ini "gopkg.in/ini.v1"
)

// loadIni 加载 ini 配置文件，文件不存在时返回空 ini
func loadIni(filePath, _ string) (*ini.File, error) {
	file := utils.NewFile()
	file.SetPath(filePath)
	iniFile, err := file.LoadIni()
	if err != nil {
		return ini.Empty(), nil
	}
	return iniFile, nil
}

// loadYaml 加载 yaml 配置文件，文件不存在时返回空 map
func loadYaml(filePath, _ string) (map[string]any, error) {
	file := utils.NewFile()
	file.SetPath(filePath)
	yamlData, err := file.LoadYaml()
	if err != nil {
		return make(map[string]any), nil
	}
	return yamlData, nil
}

// 读配置 词库函数：$读配置 [文件] [节点] [默认值]$
// 支持 .ini、.yaml、.yml 配置文件，根据文件后缀自动选择解析方式。
// 文件名可省略扩展名，自动匹配已存在的 .ini/.yml/.yaml。
// 节点格式：YAML 为嵌套路径（如 A.B.C 表示 A 下的 B 下的 C），INI 为 节.键（如 A.B.C 表示 [A.B] 节的 C 键）。
// 键不存在或值为空时返回默认值，默认值可省略（默认为空串）。
func readConfig(d *dto.DicInputs) (any, error) {
	if d.Inputs.Len() < 2 {
		return nil, fmt.Errorf("参数不足：$读配置 文件 节点 [默认值]$")
	}

	fileName := d.Inputs.String(1)
	filePath := resolveConfigPath(fileName)
	node := d.Inputs.String(2)

	defaultVal := ""
	if d.Inputs.Len() >= 3 {
		defaultVal = d.Inputs.String(3)
	}

	if isYamlFile(filePath) {
		return readYaml(filePath, parseYamlPath(node), defaultVal)
	}
	section, key := parseIniNode(node)
	return readIni(filePath, section, key, defaultVal)
}

func readIni(filePath, section, key, defaultVal string) (any, error) {
	iniFile, err := loadIni(filePath, "")
	if err != nil {
		return defaultVal, nil
	}

	val := iniFile.Section(section).Key(key).String()
	if val == "" {
		return defaultVal, nil
	}
	return val, nil
}

// readYaml 读取 yaml 配置，path 为嵌套路径（如 A.B.C 表示 A 下的 B 下的 C）
func readYaml(filePath string, path []string, defaultVal string) (any, error) {
	yamlData, err := loadYaml(filePath, "")
	if err != nil {
		return defaultVal, nil
	}

	var current any = yamlData
	for i, seg := range path {
		child, ok := current.(map[string]any)
		if !ok {
			return defaultVal, nil
		}
		val, ok := child[seg]
		if !ok {
			return defaultVal, nil
		}
		if i == len(path)-1 {
			strVal := fmt.Sprintf("%v", val)
			if strVal == "" {
				return defaultVal, nil
			}
			return strVal, nil
		}
		current = val
	}
	return defaultVal, nil
}

// 写配置 词库函数：$写配置 [文件] [节点] [值]$
// 支持 .ini、.yaml、.yml 配置文件，根据文件后缀自动选择解析方式。
// 文件名可省略扩展名，自动匹配已存在的 .ini/.yml/.yaml，都不存在默认按 .ini 创建。
// 节点格式：YAML 为嵌套路径（如 A.B.C 表示 A 下的 B 下的 C），INI 为 节.键（如 A.B.C 表示 [A.B] 节的 C 键）。
// 值可省略，默认为空串。
func writeConfig(d *dto.DicInputs) (any, error) {
	if d.Inputs.Len() < 2 {
		return nil, fmt.Errorf("参数不足：$写配置 文件 节点 [值]$")
	}

	fileName := d.Inputs.String(1)
	filePath := resolveConfigPath(fileName)
	node := d.Inputs.String(2)

	value := ""
	if d.Inputs.Len() >= 3 {
		value = d.Inputs.String(3)
	}

	if isYamlFile(filePath) {
		return writeYaml(filePath, parseYamlPath(node), value)
	}
	section, key := parseIniNode(node)
	return writeIni(filePath, section, key, value)
}

func writeIni(filePath, section, key, value string) (any, error) {
	iniFile, err := loadIni(filePath, "")
	if err != nil {
		return nil, err
	}

	iniFile.Section(section).Key(key).SetValue(value)

	file := utils.NewFile()
	file.SetPath(filePath)
	if err := file.SaveIni(iniFile); err != nil {
		return nil, fmt.Errorf("保存配置文件失败: %v", err)
	}
	return nil, nil
}

// writeYaml 写入 yaml 配置，path 为嵌套路径，中间节点不存在时自动创建
func writeYaml(filePath string, path []string, value string) (any, error) {
	yamlData, err := loadYaml(filePath, "")
	if err != nil {
		return nil, err
	}
	if yamlData == nil {
		yamlData = make(map[string]any)
	}

	var current any = yamlData
	for i, seg := range path {
		if i == len(path)-1 {
			if m, ok := current.(map[string]any); ok {
				m[seg] = value
			}
			break
		}
		m, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("yaml配置节点路径 %q 处不是嵌套结构", seg)
		}
		next, ok := m[seg]
		if !ok {
			next = make(map[string]any)
			m[seg] = next
		}
		child, ok := next.(map[string]any)
		if !ok {
			child = make(map[string]any)
			m[seg] = child
		}
		current = child
	}

	file := utils.NewFile()
	file.SetPath(filePath)
	if err := file.SaveYaml(yamlData); err != nil {
		return nil, fmt.Errorf("保存Yaml配置文件失败: %v", err)
	}
	return nil, nil
}

// parseIniNode 解析 INI 节点名：A.B.C → 节 [A.B]，键 C（按最后一个点拆分，支持多层节）
func parseIniNode(node string) (section, key string) {
	if i := strings.LastIndex(node, "."); i >= 0 {
		return node[:i], node[i+1:]
	}
	return "", node
}

// parseYamlPath 将节点名按 "." 拆分为嵌套路径（如 A.B.C → [A B C]），跳过空段
func parseYamlPath(node string) []string {
	parts := strings.Split(node, ".")
	path := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			path = append(path, p)
		}
	}
	return path
}

// resolveConfigPath 解析文件名简写为实际路径，同时支持 ini 和 yaml
// 省略扩展名时自动匹配已存在的 .ini / .yml / .yaml，都不存在默认按 .ini 创建
func resolveConfigPath(fileName string) string {
	switch fileName {
	case "system":
		return dto.CONFIG_SYSTEM_PATH
	case "config":
		return dto.CONFIG_PATH
	case "system.yaml", "system.yml":
		return "private/system/system.yaml"
	case "config.yaml", "config.yml":
		return "private/system/config.yaml"
	}

	if filepath.Ext(fileName) == "" {
		for _, ext := range []string{".ini", ".yml", ".yaml"} {
			path := fileName + ext
			if configFileExists(path) {
				return path
			}
		}
		return fileName + ".ini"
	}
	return fileName
}

// configFileExists 检查配置文件（相对路径基于 NebulaData）是否存在
func configFileExists(path string) bool {
	file := utils.NewFile()
	file.SetPath(path)
	return file.FileExists()
}

// isYamlFile 判断文件路径是否为 yaml 格式
func isYamlFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

// 设置跨域 词库函数：$设置跨域 开关 [白名单]$
// 热开关全局 HTTP 服务器的跨域：立即更新内存配置并持久化到 system.ini，无需重启。
// 开关：true/false；白名单可省略，省略时保持原值。
func setServerCors(d *dto.DicInputs) (any, error) {
	if d.Inputs.Len() < 1 {
		return nil, fmt.Errorf("参数不足：$设置跨域 开关 [白名单]$")
	}
	if dto.ServerConfig.Router == nil {
		return nil, fmt.Errorf("设置跨域：HTTP 服务器配置尚未初始化")
	}

	cors := d.Inputs.Bool(1)
	dto.ServerConfig.Router.Cors = cors

	// 持久化，避免重启后丢失
	file := utils.NewFile()
	file.SetPath(dto.CONFIG_SYSTEM_PATH)
	iniFile, err := file.LoadIni()
	if err != nil {
		return nil, fmt.Errorf("设置跨域：读取系统配置失败: %v", err)
	}
	sec := iniFile.Section("HTTP")
	sec.Key("跨域").SetValue(strconv.FormatBool(cors))
	if d.Inputs.LenOk(2) {
		origin := d.Inputs.String(2)
		dto.ServerConfig.Router.CorsOrigins = origin
		sec.Key("跨域白名单").SetValue(origin)
	}
	if err := file.SaveIni(iniFile); err != nil {
		return nil, fmt.Errorf("设置跨域：保存系统配置失败: %v", err)
	}
	return nil, nil
}
