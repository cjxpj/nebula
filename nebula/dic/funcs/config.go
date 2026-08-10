package funcs

import (
	"fmt"
	"path/filepath"
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
// 节点格式为 节.键（如 Server.启用 或 Bot.Secret）。
// 键不存在或值为空时返回默认值，默认值可省略（默认为空串）。
func readConfig(d *dto.DicInputs) (any, error) {
	if d.Inputs.Len() < 2 {
		return nil, fmt.Errorf("参数不足：$读配置 文件 节点 [默认值]$")
	}

	fileName := d.Inputs.String(1)
	filePath := resolveConfigPath(fileName)
	section, key := parseNode(d.Inputs.String(2))

	defaultVal := ""
	if d.Inputs.Len() >= 3 {
		defaultVal = d.Inputs.String(3)
	}

	if isYamlFile(filePath) {
		return readYaml(filePath, section, key, defaultVal)
	}
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

func readYaml(filePath, section, key, defaultVal string) (any, error) {
	yamlData, err := loadYaml(filePath, "")
	if err != nil {
		return defaultVal, nil
	}

	sectionMap, ok := yamlData[section]
	if !ok {
		return defaultVal, nil
	}

	sectionMapTyped, ok := sectionMap.(map[string]any)
	if !ok {
		return defaultVal, nil
	}

	val, ok := sectionMapTyped[key]
	if !ok {
		return defaultVal, nil
	}

	strVal := fmt.Sprintf("%v", val)
	if strVal == "" {
		return defaultVal, nil
	}
	return strVal, nil
}

// 写配置 词库函数：$写配置 [文件] [节点] [值]$
// 支持 .ini、.yaml、.yml 配置文件，根据文件后缀自动选择解析方式。
// 节点格式为 节.键（如 Server.启用 或 Bot.Secret）。
// 值可省略，默认为空串。
func writeConfig(d *dto.DicInputs) (any, error) {
	if d.Inputs.Len() < 2 {
		return nil, fmt.Errorf("参数不足：$写配置 文件 节点 [值]$")
	}

	fileName := d.Inputs.String(1)
	filePath := resolveConfigPath(fileName)
	section, key := parseNode(d.Inputs.String(2))

	value := ""
	if d.Inputs.Len() >= 3 {
		value = d.Inputs.String(3)
	}

	if isYamlFile(filePath) {
		return writeYaml(filePath, section, key, value)
	}
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

func writeYaml(filePath, section, key, value string) (any, error) {
	yamlData, err := loadYaml(filePath, "")
	if err != nil {
		return nil, err
	}

	sectionMap, ok := yamlData[section]
	if !ok {
		sectionMap = make(map[string]any)
		yamlData[section] = sectionMap
	}

	sectionMapTyped, ok := sectionMap.(map[string]any)
	if !ok {
		sectionMapTyped = make(map[string]any)
		yamlData[section] = sectionMapTyped
	}

	sectionMapTyped[key] = value

	file := utils.NewFile()
	file.SetPath(filePath)
	if err := file.SaveYaml(yamlData); err != nil {
		return nil, fmt.Errorf("保存Yaml配置文件失败: %v", err)
	}
	return nil, nil
}

// parseNode 解析 "节.键" 格式的节点名
func parseNode(node string) (section, key string) {
	if before, after, ok := strings.Cut(node, "."); ok {
		return before, after
	}
	return "", node
}

// resolveConfigPath 解析文件名简写为实际路径，同时支持 ini 和 yaml
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
	default:
		return fileName
	}
}

// isYamlFile 判断文件路径是否为 yaml 格式
func isYamlFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}
