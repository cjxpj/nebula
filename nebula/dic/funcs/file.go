package funcs

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// FileDeleteConfirm 删除确认回调：词库执行「删除文件 / 删除文件夹」前调用，
// 返回 true 才真正执行删除；未注册回调（如非交互式运行）时默认允许，保持原有行为。
var FileDeleteConfirm func(action, target string) bool

// SetFileDeleteConfirm 注册删除确认回调。
func SetFileDeleteConfirm(fn func(action, target string) bool) {
	FileDeleteConfirm = fn
}

// confirmFileDelete 询问是否允许执行删除操作。
func confirmFileDelete(action, target string) bool {
	if FileDeleteConfirm == nil {
		return true
	}
	return FileDeleteConfirm(action, target)
}

// checkPathValue 校验给定路径解析后是否位于工作目录内，越界返回错误。
func checkPathValue(raw string) error {
	if raw == "" {
		return nil
	}
	if !utils.IsWithinWorkDir(utils.NewFileQueue(raw).FileName) {
		return fmt.Errorf("路径越界（超出工作目录）：%s", raw)
	}
	return nil
}

// checkFuncPath 校验第 idx 个参数解析后的路径是否位于工作目录内，越界返回错误。
func checkFuncPath(d *dto.DicInputs, idx int) error {
	return checkPathValue(d.Inputs.String(idx))
}

// 删除文件（越界拒绝，执行前人工确认）
func deleteFile(d *dto.DicInputs) (any, error) {
	if err := checkFuncPath(d, 1); err != nil {
		return "", err
	}
	target := d.Inputs.String(1)
	if !confirmFileDelete("删除文件", target) {
		return "", fmt.Errorf("已取消删除文件：%s", target)
	}
	utils.NewFileQueue(target).DeleteFile()
	return "", nil
}

// 删除文件夹（越界拒绝，执行前人工确认）
func deleteDir(d *dto.DicInputs) (any, error) {
	if err := checkFuncPath(d, 1); err != nil {
		return "", err
	}
	target := d.Inputs.String(1)
	if !confirmFileDelete("删除文件夹", target) {
		return "", fmt.Errorf("已取消删除文件夹：%s", target)
	}
	utils.NewFileQueue(target).DeleteFolder()
	return "", nil
}

// 文件后缀
func fileSuffix(d *dto.DicInputs) (any, error) {
	return filepath.Ext(d.Inputs.String(1)), nil
}

// 设置工作目录：切换进程当前工作目录，后续相对路径基于该目录解析。
func setWorkDir(d *dto.DicInputs) (any, error) {
	dir := d.Inputs.String(1)
	if dir == "" {
		return "", nil
	}
	// 移动端数据主目录已由 GetAppDir 确定，此处把「设置工作目录」映射为切换到主目录下的子目录
	// （如 Android：/storage/emulated/0/Documents/Nebula -> .../Nebula/NebulaData）
	if utils.GetAppDir() != "" {
		newDir := filepath.Join(utils.GetAppDir(), dir)
		if !utils.IsWithinWorkDir(newDir) {
			return "", fmt.Errorf("路径越界（超出工作目录）：%s", dir)
		}
		if err := os.MkdirAll(newDir, 0755); err != nil {
			return "", err
		}
		utils.SetAppDir(newDir)
		return "", nil
	}
	// 桌面端：切换进程当前工作目录；目标目录不存在时先创建，
	// 避免首次启动因目录缺失导致 chdir 失败、后续文件写入落到错误位置。
	// 目标必须位于当前工作目录之下，否则可借切换工作目录逃逸出受限范围。
	if !utils.IsWithinWorkDir(dir) {
		return "", fmt.Errorf("路径越界（超出工作目录）：%s", dir)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	if err := os.Chdir(dir); err != nil {
		return "", err
	}
	return "", nil
}

// 存在文件
func fileExist(d *dto.DicInputs) (any, error) {
	if err := checkFuncPath(d, 1); err != nil {
		return "", err
	}
	file := utils.NewFileQueue(d.Inputs.String(1)).FileExists()
	return strconv.FormatBool(file), nil
}

// 存在文件夹
func dirExist(d *dto.DicInputs) (any, error) {
	if err := checkFuncPath(d, 1); err != nil {
		return "", err
	}
	dir := utils.NewFileQueue(d.Inputs.String(1)).DirExists()
	return strconv.FormatBool(dir), nil
}

// 存在文件或文件夹
func fileOrDirExist(d *dto.DicInputs) (any, error) {
	if err := checkFuncPath(d, 1); err != nil {
		return "", err
	}
	file := utils.NewFileQueue(d.Inputs.String(1)).FileOrDirExists()
	return strconv.FormatBool(file), nil
}

// 写文件
func writeStringFile(d *dto.DicInputs) (any, error) {
	if err := checkFuncPath(d, 1); err != nil {
		return "", err
	}
	utils.NewFileQueue(d.Inputs.String(1)).WriteToFile(d.Inputs.String(2))
	return "", nil
}

// 读文件
func readStringFile(d *dto.DicInputs) (any, error) {
	if err := checkFuncPath(d, 1); err != nil {
		return "", err
	}
	data, err := utils.NewFileQueue(d.Inputs.String(1)).ReadFile()
	if err != nil {
		return d.Inputs.String(2), nil
	}
	return data, nil
}

// 读文件随机一行
func readStringFileRandomLine(d *dto.DicInputs) (any, error) {
	if err := checkFuncPath(d, 1); err != nil {
		return "", err
	}
	data, err := utils.NewFileQueue(d.Inputs.String(1)).ReadFileRandomLine()
	if err != nil {
		return d.Inputs.String(2), nil
	}
	return data, nil
}

// 读文件行
func readStringFileLines(d *dto.DicInputs) (any, error) {
	if err := checkFuncPath(d, 1); err != nil {
		return "", err
	}
	one := max(d.Inputs.Int(2), 1)
	two := max(d.Inputs.Int(3), 1)
	data, err := utils.NewFileQueue(d.Inputs.String(1)).ReadLines(one, two)
	if err != nil {
		return d.Inputs.String(4), nil
	}
	// []string 转字符串
	resS, err := json.Marshal(data)
	if err != nil {
		return d.Inputs.String(4), nil
	}
	return string(resS), nil
}

// 读文件行数
func readStringFileLinesCount(d *dto.DicInputs) (any, error) {
	if err := checkFuncPath(d, 1); err != nil {
		return "", err
	}
	count, err := utils.NewFileQueue(d.Inputs.String(1)).GetLineCount()
	if err != nil {
		return d.Inputs.String(2), nil
	}
	return strconv.Itoa(count), nil
}

func writeKeyStringFile(d *dto.DicInputs) (any, error) {
	path := "database/" + d.Inputs.String(1)
	if err := checkPathValue(path); err != nil {
		return "", err
	}
	if strings.EqualFold(filepath.Ext(path), ".json") {
		return writeJsonKeyFile(d, path)
	}
	utils.NewFileQueue(path).WriteFileKey(d.Inputs.String(2), d.Inputs.String(3))
	return "", nil
}

func readKeyStringFile(d *dto.DicInputs) (any, error) {
	path := "database/" + d.Inputs.String(1)
	if err := checkPathValue(path); err != nil {
		return "", err
	}
	if strings.EqualFold(filepath.Ext(path), ".json") {
		return readJsonKeyFile(d, path)
	}
	file := utils.NewFileQueue(path)
	if d.Inputs.LenOk(1) {
		data, err := file.ReadFileKeyList()
		if err != nil {
			return "[]", nil
		}

		resS, err := json.Marshal(data)
		if err != nil {
			return "[]", nil
		}
		return string(resS), nil
	}
	data, err := file.ReadFileKey(d.Inputs.String(2))
	if err != nil {
		return d.Inputs.String(3), nil
	}
	return data, nil
}

// splitJsonPath 将 . 分隔的键路径拆分为路径段，过滤空段
func splitJsonPath(key string) []string {
	parts := strings.Split(key, ".")
	path := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			path = append(path, p)
		}
	}
	return path
}

// writeJsonKeyFile 写入 JSON 键值文件，键以 . 分隔多层路径，值作为字符串写入
func writeJsonKeyFile(d *dto.DicInputs, path string) (any, error) {
	file := utils.NewFileQueue(path)

	// 读取现有 JSON，不存在或解析失败时初始化为空对象
	var obj any = map[string]any{}
	if data, err := file.ReadFile(); err == nil && strings.TrimSpace(data) != "" {
		if err := json.Unmarshal([]byte(data), &obj); err != nil {
			obj = map[string]any{}
		}
	}

	keys := splitJsonPath(d.Inputs.String(2))
	obj = JsonSetValue(obj, keys, d.Inputs.String(3), true)

	b, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	file.WriteToFile(string(b))
	return "", nil
}

// readJsonKeyFile 读取 JSON 键值文件，键以 . 分隔多层路径
func readJsonKeyFile(d *dto.DicInputs, path string) (any, error) {
	file := utils.NewFileQueue(path)

	// 无 key（仅文件路径）：返回整个 JSON 内容
	if d.Inputs.LenOk(1) {
		data, err := file.ReadFile()
		if err != nil {
			return "", nil
		}
		return data, nil
	}

	data, err := file.ReadFile()
	if err != nil {
		return d.Inputs.String(3), nil
	}

	var obj any
	if err := json.Unmarshal([]byte(data), &obj); err != nil {
		return d.Inputs.String(3), nil
	}

	cur := obj
	for _, k := range splitJsonPath(d.Inputs.String(2)) {
		switch v := cur.(type) {
		case map[string]any:
			if val, ok := v[k]; ok {
				cur = val
			} else {
				return d.Inputs.String(3), nil
			}
		case []any:
			idx, err := strconv.Atoi(k)
			if err != nil || idx < 0 || idx >= len(v) {
				return d.Inputs.String(3), nil
			}
			cur = v[idx]
		default:
			return d.Inputs.String(3), nil
		}
	}

	return utils.AnyToString(cur), nil
}

// 文件夹列表
func dirList(d *dto.DicInputs) (any, error) {
	if err := checkFuncPath(d, 1); err != nil {
		return "", err
	}
	path := d.Inputs.String(1)
	list, err := utils.NewFileQueue(path).GetDirList()
	if err != nil {
		return "{}", nil
	}

	resS, err := json.Marshal(list)
	if err != nil {
		return "{}", nil
	}
	return string(resS), nil
}

// 随机文件夹
func randomDirName(d *dto.DicInputs) (any, error) {
	if err := checkFuncPath(d, 1); err != nil {
		return "", err
	}
	path := d.Inputs.String(1)
	list, err := utils.NewFileQueue(path).GetDirList()
	if err != nil {
		return "", nil
	}
	if len(list) == 0 {
		return "", nil
	}
	return list[utils.RandNum(0, len(list)-1)], nil
}

// 随机文件
func randomFileName(d *dto.DicInputs) (any, error) {
	if err := checkFuncPath(d, 1); err != nil {
		return "", err
	}
	path := d.Inputs.String(1)
	list, err := utils.NewFileQueue(path).GetFileList()
	if err != nil {
		return "", nil
	}
	if len(list) == 0 {
		return "", nil
	}
	return list[utils.RandNum(0, len(list)-1)], nil
}

// 文件列表
func fileList(d *dto.DicInputs) (any, error) {
	if err := checkFuncPath(d, 1); err != nil {
		return "", err
	}
	path := d.Inputs.String(1)
	list, err := utils.NewFileQueue(path).GetFileList()
	if err != nil {
		return "{}", nil
	}

	resS, err := json.Marshal(list)
	if err != nil {
		return "{}", nil
	}
	return string(resS), nil
}

func (f *DicFunc) DirSize() string {
	var path string
	if f.Len == 0 || f.Len == 1 {
		if f.Len == 1 {
			path += f.Inputs.String(1)
		}
		file := utils.NewFileQueue(path)
		fileSize, err := file.GetDirSize()
		if err != nil {
			return "0"
		}
		str := strconv.FormatInt(fileSize, 10)
		return str
	}
	return ""
}

func (f *DicFunc) FileSize() string {
	var path string
	if f.Len == 0 || f.Len == 1 {
		if f.Len == 1 {
			path += f.Inputs.String(1)
		}
		file := utils.NewFileQueue(path)
		fileSize, err := file.GetFileSize()
		if err != nil {
			return "0"
		}
		str := strconv.FormatInt(fileSize, 10)
		return str
	}
	return ""
}

func (f *DicFunc) FileRename() string {
	if f.Len == 2 {
		path := f.Inputs.String(1)
		path2 := f.Inputs.String(2)
		file := utils.NewFileQueue(path)
		ok := file.Rename(path2)
		if ok {
			return "true"
		}
		return "false"
	}
	return ""
}

func (f *DicFunc) FileCopy() string {
	if f.Len == 2 {
		if f.Inputs.String(1) == f.Inputs.String(2) {
			return "false"
		}
		path := f.Inputs.String(1)
		path2 := f.Inputs.String(2)
		file := utils.NewFileQueue(path)
		ok := file.Copy(path2)
		if ok {
			return "true"
		}
		return "false"
	}
	return ""
}

func dirSize(d *dto.DicInputs) (any, error) {
	if err := checkFuncPath(d, 1); err != nil {
		return "", err
	}
	path := d.Inputs.String(1)
	file := utils.NewFileQueue(path)
	fileSize, err := file.GetDirSize()
	if err != nil {
		return "0", nil
	}
	return strconv.FormatInt(fileSize, 10), nil
}

func fileSize(d *dto.DicInputs) (any, error) {
	if err := checkFuncPath(d, 1); err != nil {
		return "", err
	}
	path := d.Inputs.String(1)
	file := utils.NewFileQueue(path)
	fileSize, err := file.GetFileSize()
	if err != nil {
		return "0", nil
	}
	return strconv.FormatInt(fileSize, 10), nil
}

func fileRename(d *dto.DicInputs) (any, error) {
	if err := checkFuncPath(d, 1); err != nil {
		return "", err
	}
	if err := checkFuncPath(d, 2); err != nil {
		return "", err
	}
	path := d.Inputs.String(1)
	path2 := d.Inputs.String(2)
	file := utils.NewFileQueue(path)
	ok := file.Rename(path2)
	if ok {
		return "true", nil
	}
	return "false", nil
}

func fileCopy(d *dto.DicInputs) (any, error) {
	if d.Inputs.String(1) == d.Inputs.String(2) {
		return "false", nil
	}
	if err := checkFuncPath(d, 1); err != nil {
		return "", err
	}
	if err := checkFuncPath(d, 2); err != nil {
		return "", err
	}
	path := d.Inputs.String(1)
	path2 := d.Inputs.String(2)
	file := utils.NewFileQueue(path)
	ok := file.Copy(path2)
	if ok {
		return "true", nil
	}
	return "false", nil
}
