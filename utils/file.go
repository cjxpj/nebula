package utils

import (
	"archive/zip"
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Error 将文本写入文件
func Error(text, path string) {
	currentTime := time.Now().Format("20060102/15")
	file := NewFileQueue(path + "/database/log/" + currentTime + ".txt")
	currentTime2 := time.Now().Format("04m05s")
	file.AppendToFile("No:" + currentTime2 + ">" + text + "\n")
}

// ErrorStop 写入错误信息文件并停止程序
func ErrorStop(text, path string) {
	Error(text, path)
	os.Exit(0)
}

// Log 将文本写入到 log.txt 文件
func Log(text, path string) {
	currentTime := time.Now().Format("20060102/15")
	file := NewFileQueue(path + "/database/log/" + currentTime + ".txt")
	currentTime2 := time.Now().Format("04m05s")
	file.AppendToFile("Yes:" + currentTime2 + ">" + text + "\n")
}

// LogStop 将文本写入到 log.txt 文件并停止程序
func LogStop(text, path string) {
	Log(text, path)
	os.Exit(0)
}

// GetAppDir 获取应用目录
func GetAppDir() string {
	if runtime.GOOS == "android" {
		// /Android/data/com.cjxpj.juice/files
		// return "/storage/emulated/0/Android/data/com.cjxpj.juice/juiceData"
		// return "/data/user/0/com.cjxpj.juice/juiceData"
		return "/storage/emulated/0/Documents/juiceData"
	}
	return "juiceData"
}

// 随机数
func RandNum(min, max int) int {
	if min == max {
		return min
	}
	if min > max {
		min, max = max, min
	}
	// 生成一个安全的随机数
	randomNumber, err := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
	if err != nil {
		return min - 1
	}
	// 将随机数转换为min和max范围内的数字
	randomNumberInt := int(randomNumber.Int64()) + min
	return randomNumberInt
}

// 获取唯一编号
func GetUid(uid string) string {
	var result []byte
	str := base64.StdEncoding.EncodeToString([]byte(uid))
	for i := 0; i < len(str); i++ {
		if i > 0 && i%8 == 0 {
			result = append(result, '/')
		}
		result = append(result, str[i])
	}
	return string(result) + "D"
}

// NewFileQueue 创建一个新的文件队列实例
func NewFileQueue(FileName string) *FileQueue {
	file := &FileQueue{}
	file.SetPath(FileName)
	return file
}

// NewFileQueue 创建一个新的文件队列实例
func NewFile() *FileQueue {
	file := &FileQueue{}
	return file
}

// 重新设置文件路径
func (fq *FileQueue) SetPath(FileName string) *FileQueue {
	appDir := GetAppDir()
	setfilePath := filepath.Join(appDir, FileName)
	if FileName == "/" {
		setfilePath = appDir
	}
	if filepath.IsAbs(FileName) {
		setfilePath = FileName
	}
	fq.FileName = setfilePath
	return fq
}

// FileExists 检查文件是否存在
func (fq *FileQueue) FileExists() bool {
	_, err := os.Stat(fq.FileName)
	return !os.IsNotExist(err)
}

// DirExists 检查文件夹是否存在
func (fq *FileQueue) DirExists() bool {
	_, err := os.Stat(fq.FileName)
	return !os.IsNotExist(err)
}

// GetFileSize 返回文件的大小
func (fq *FileQueue) GetFileSize() (int64, error) {
	// 确保文件存在
	fileInfo, err := os.Stat(fq.FileName)
	if os.IsNotExist(err) {
		return 0, fmt.Errorf("文件 '%s' 不存在", fq.FileName)
	}
	if err != nil {
		return 0, err
	}

	// 如果是目录，则返回错误，因为我们只想要文件大小
	if fileInfo.IsDir() {
		return 0, fmt.Errorf("'%s' 是一个目录，不是文件", fq.FileName)
	}

	// 返回文件大小
	return fileInfo.Size(), nil
}

// GetDirSize 返回目录的大小
func (fq *FileQueue) GetDirSize() (int64, error) {
	var size int64

	// 确保路径存在
	if _, err := os.Stat(fq.FileName); os.IsNotExist(err) {
		return 0, fmt.Errorf("目录 '%s' 不存在", fq.FileName)
	}

	// 使用 filepath.Walk 遍历目录
	err := filepath.Walk(fq.FileName, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Printf("遍历 '%s' 时出错: %v\n", path, err)
			return err
		}
		if !info.IsDir() {
			// 累加文件大小
			size += info.Size()
		}
		return nil
	})

	if err != nil {
		return 0, err
	}

	return size, nil
}

// WriteToFile 向文件写入数据
func (fq *FileQueue) WriteToFile(data string) {
	fileMutex.Lock()
	defer fileMutex.Unlock()

	// 检查文件夹是否存在，不存在则创建
	dir := filepath.Dir(fq.FileName)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			Error("创建文件夹失败", "system")
			return
		}
	}

	// 创建文件，如果不存在
	file, err := os.OpenFile(fq.FileName, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		Error("创建文件失败", "system")
		return
	}
	defer file.Close()

	_, err = file.WriteString(data)
	if err != nil {
		Error("写入数据失败", "system")
	}
}

func (fq *FileQueue) Download(url string) bool {
	fileMutex.Lock()
	defer fileMutex.Unlock()

	// 确保目标文件夹存在
	dir := filepath.Dir(fq.FileName)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			Error("创建文件夹失败", "system")
			return false
		}
	}

	// 拼接目标文件路径
	filePath := filepath.Join(dir, filepath.Base(url))

	// 创建目标文件
	out, err := os.Create(filePath)
	if err != nil {
		Error("创建文件失败", "system")
		return false
	}
	defer out.Close()

	// 发送 HTTP GET 请求
	resp, err := http.Get(url)
	if err != nil {
		Error("访问失败", "system")
		return false
	}
	defer resp.Body.Close()

	// 将 HTTP 响应的主体写入目标文件
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		Error("写入数据失败", "system")
		return false
	}

	return true
}

// ZipFolder 将文件夹压缩成 ZIP 文件
func (fq *FileQueue) ZipFolder(destZip string) bool {
	// 创建 ZIP 文件
	zipFile, err := os.Create(destZip)
	if err != nil {
		Error("创建 ZIP 文件失败: "+err.Error(), "system")
		return false
	}
	defer zipFile.Close()

	// 创建 ZIP 写入器
	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// 处理文件夹及其文件
	err = filepath.WalkDir(fq.FileName, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("遍历文件夹失败: %w", err)
		}

		// 跳过根文件夹
		if path == fq.FileName {
			return nil
		}

		// 获取相对路径
		relPath, err := filepath.Rel(fq.FileName, path)
		if err != nil {
			return fmt.Errorf("获取相对路径失败: %w", err)
		}

		// 如果是目录，则创建目录项
		if d.IsDir() {
			return nil
		}

		// 创建 ZIP 文件项
		zipFile, err := zipWriter.Create(relPath)
		if err != nil {
			return fmt.Errorf("创建 ZIP 文件项失败: %w", err)
		}

		// 打开源文件
		srcFile, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("打开源文件失败: %w", err)
		}
		defer srcFile.Close()

		// 复制文件内容到 ZIP 文件项
		_, err = io.Copy(zipFile, srcFile)
		if err != nil {
			return fmt.Errorf("复制文件内容失败: %w", err)
		}

		return nil
	})

	if err != nil {
		Error("压缩文件夹失败: "+err.Error(), "system")
		return false
	}

	return true
}

// 解压zip
func (fq *FileQueue) UnZip(dest string) bool {
	// 打开ZIP文件
	r, err := zip.OpenReader(fq.FileName)
	if err != nil {
		Error("打开ZIP文件失败", "system")
		return false
	}
	defer r.Close()

	// 遍历ZIP文件中的每个文件/文件夹
	for _, f := range r.File {
		// 构建解压文件的完整路径
		fPath := filepath.Join(dest, f.Name)

		// 如果是目录，则创建目录
		if f.FileInfo().IsDir() {
			os.MkdirAll(fPath, os.ModePerm)
			continue
		}

		// 确保文件父目录存在
		if err = os.MkdirAll(filepath.Dir(fPath), os.ModePerm); err != nil {
			Error("创建目录失败", "system")
			return false
		}

		// 打开压缩文件中的文件
		inFile, err := f.Open()
		if err != nil {
			Error("打开压缩文件中的文件失败", "system")
			return false
		}
		defer inFile.Close() // 延迟关闭输入文件

		// 创建解压后的文件
		outFile, err := os.OpenFile(fPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			Error("创建解压文件失败", "system")
			return false
		}
		defer outFile.Close() // 延迟关闭输出文件

		// 复制文件内容
		_, err = io.Copy(outFile, inFile)
		if err != nil {
			Error("复制文件内容失败", "system")
			return false
		}
	}
	return true
}

// DeleteFile 删除文件，并返回是否成功删除
func (fq *FileQueue) DeleteFile() bool {
	fileMutex.Lock()
	defer fileMutex.Unlock()

	// 检查文件是否存在
	_, err := os.Stat(fq.FileName)
	if os.IsNotExist(err) {
		return false
	}

	// 删除文件
	if err := os.Remove(fq.FileName); err != nil {
		return false
	}

	return true
}

// DeleteFolder 删除文件夹及其所有内容，并返回是否成功删除
func (fq *FileQueue) DeleteFolder() bool {
	fileMutex.Lock()
	defer fileMutex.Unlock()

	// 检查文件夹是否存在
	_, err := os.Stat(fq.FileName)
	if os.IsNotExist(err) {
		return false
	}

	// 删除文件夹及其所有内容
	if err := os.RemoveAll(fq.FileName); err != nil {
		return false
	}

	return true
}

// AppendToFile 向文件追加数据
func (fq *FileQueue) AppendToFile(data string) {
	fileMutex.Lock()
	defer fileMutex.Unlock()

	// 检查文件夹是否存在，不存在则创建
	dir := filepath.Dir(fq.FileName)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			Error("创建文件夹失败", "system")
			return
		}
	}

	// 打开文件以追加模式
	file, err := os.OpenFile(fq.FileName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		Error("打开文件失败", "system")
		return
	}
	defer file.Close()

	// 创建缓存器
	writer := bufio.NewWriter(file)

	// 写入数据到缓存
	if _, err := writer.WriteString(data); err != nil {
		Error("追加数据缓存失败", "system")
		return
	}

	// 刷新缓存，将数据写入文件
	if err := writer.Flush(); err != nil {
		Error("追加数据失败", "system")
	}

	// 写入数据到文件末尾
	// _, err = file.WriteString(data)
	// if err != nil {
	// 	Error("追加数据失败")
	// }
}

// ReadFile 完整从文件读取数据
func (fq *FileQueue) ReadFile() (string, error) {
	fileMutex.RLock()
	defer fileMutex.RUnlock()

	file, err := os.Open(fq.FileName)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var result strings.Builder
	buf := make([]byte, 1024)
	for {
		n, err := file.Read(buf)
		if n > 0 {
			result.Write(buf[:n])
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
	}
	return result.String(), nil
}

// GetFileList 函数用于获取指定文件夹中的文件列表
func (fq *FileQueue) GetFileList(t string) ([]string, error) {
	fileList := []string{}

	entries, err := os.ReadDir(fq.FileName)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		// 根据 t 的值来决定是否添加到 fileList 中
		switch t {
		case "file", "文件":
			if !entry.IsDir() {
				fileList = append(fileList, entry.Name())
			}
		case "dir", "文件夹":
			if entry.IsDir() {
				fileList = append(fileList, entry.Name())
			}
		default: // 默认处理 "all"
			fileList = append(fileList, entry.Name())
		}
	}

	// err := filepath.Walk(fq.FileName, func(path string, info os.FileInfo, err error) error {
	// 	if err != nil {
	// 		return err
	// 	}

	// 	// 根据 t 的值来决定是否添加到 fileList 中
	// 	switch t {
	// 	case "file", "文件":
	// 		if !info.IsDir() && filepath.Dir(path) == fq.FileName {
	// 			fileList = append(fileList, filepath.Base(path))
	// 		}
	// 	case "dir", "文件夹":
	// 		if info.IsDir() && path != fq.FileName {
	// 			fileList = append(fileList, filepath.Base(path))
	// 		}
	// 	default: // 默认处理 "all"
	// 		if filepath.Dir(path) == fq.FileName {
	// 			fileList = append(fileList, filepath.Base(path))
	// 		}
	// 	}

	// 	return nil
	// })

	return fileList, nil
}

// ReadFileExt 获取文件后缀
func (fq *FileQueue) ReadFileExt() string {
	return filepath.Ext(fq.FileName)
}

// ReadFromFile 从文件读取数据
func (fq *FileQueue) ReadFromFile() (string, error) {
	fileMutex.RLock()
	defer fileMutex.RUnlock()

	file, err := os.Open(fq.FileName)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var result strings.Builder
	buf := make([]byte, 1024)
	for {
		n, err := file.Read(buf)
		if n > 0 {
			result.Write(buf[:n])
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
	}
	// 将\r\n替换为\n
	return strings.ReplaceAll(result.String(), "\r\n", "\n"), nil
}

// 复制粘贴
// Copy 复制文件或文件夹
func (fq *FileQueue) Copy(newName string) bool {
	fileMutex.Lock() // 使用写锁，确保线程安全
	defer fileMutex.Unlock()

	// 新文件名
	newfile := NewFileQueue(newName)
	newPath := newfile.FileName

	if newPath == fq.FileName {
		return false
	}

	// 获取文件信息
	fileInfo, err := os.Stat(fq.FileName)
	if err != nil {
		return false
	}

	// 根据文件类型进行不同的处理
	if fileInfo.IsDir() {
		// 处理文件夹
		err = copyDir(fq.FileName, newPath)
	} else {
		// 处理文件
		err = copyFile(fq.FileName, newPath)
	}

	return err == nil
}

// copyFile 复制文件
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return err
	}

	return dstFile.Sync()
}

// copyDir 递归复制文件夹
func copyDir(srcDir, dstDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}

	// 创建目标文件夹
	err = os.MkdirAll(dstDir, os.ModePerm)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())

		if entry.IsDir() {
			// 递归复制子文件夹
			err = copyDir(srcPath, dstPath)
			if err != nil {
				return err
			}
		} else {
			// 复制文件
			err = copyFile(srcPath, dstPath)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// 文件重命名
func (fq *FileQueue) Rename(newName string) bool {
	fileMutex.Lock() // 使用写锁，确保线程安全
	defer fileMutex.Unlock()

	// 新文件名
	newfile := NewFileQueue(newName)
	newPath := newfile.FileName

	if newPath == fq.FileName {
		return false
	}

	// 重命名文件
	err := os.Rename(fq.FileName, newPath)

	return err == nil
}

// ReadFileByte 从文件完整读取数据并返回字节切片
func (fq *FileQueue) ReadFileByte() ([]byte, error) {
	fileMutex.RLock()
	defer fileMutex.RUnlock()

	file, err := os.ReadFile(fq.FileName)
	if err != nil {
		return nil, err
	}

	return file, nil
}

// WriteFileByte 向文件写入数据
func (fq *FileQueue) WriteFileByte(data []byte) {
	fileMutex.Lock()
	defer fileMutex.Unlock()

	// 检查文件夹是否存在，不存在则创建
	dir := filepath.Dir(fq.FileName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		Error("创建文件夹失败", "system")
		return
	}

	// 写入数据到文件中
	if err := os.WriteFile(fq.FileName, data, 0644); err != nil {
		Error("写入数据失败", "system")
	}
}

// ReadFileKeyList 从文件中读取所有键值对并返回一个 []map[string]string
func (fq *FileQueue) ReadFileKeyList() ([]map[string]string, error) {
	data, err := fq.ReadFromFile()
	if err != nil {
		return nil, err
	}

	// 将数据拆分成行
	lines := strings.Split(data, "\n")

	// 跳过第一行的标志
	skipFirstLine := true

	var result []map[string]string

	// 解析每一行，并将键值对存储到 map 中
	for _, line := range lines {
		if skipFirstLine {
			skipFirstLine = false
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			entry := map[string]string{
				"key":  parts[0],
				"data": parts[1],
			}
			result = append(result, entry)
		}
	}

	return result, nil
}

// ReadFileKey 从文件中读取与给定键相关联的值
func (fq *FileQueue) ReadFileKey(key string) (string, error) {
	data, err := fq.ReadFromFile()
	if err != nil {
		return "", err
	}

	// 将数据拆分成行
	lines := strings.Split(data, "\n")

	// 跳过第一行的标志
	skipFirstLine := true

	// 查找具有指定键的行
	for _, line := range lines {
		if skipFirstLine {
			skipFirstLine = false
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 && parts[0] == key {
			return parts[1], nil
		}
	}

	return "", fmt.Errorf("未找到键 %s", key)
}

// WriteFileKey 将键值对写入文件
func (fq *FileQueue) WriteFileKey(key, value string) error {

	data, err := fq.ReadFromFile()
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// 获取当前时间并格式化为字符串
	currentTime := time.Now().Format("2006-01-02 15:04:05")

	// 将数据拆分成行
	lines := strings.Split(data, "\n")

	// 更新或追加键值对
	var found bool
	for i, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 && parts[0] == key {
			lines[i] = fmt.Sprintf("%s=%s", key, value)
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	}

	// 将第一行更新为当前时间
	lines[0] = fmt.Sprintf("更新时间: %s", currentTime)

	// 将行重新组合
	newData := strings.Join(lines, "\n")

	// 将更新的数据写入文件
	fq.WriteToFile(newData)

	return nil
}

// CreateFolderIfNotExists 检查并创建文件夹，返回是否成功创建
func CreateFolderIfNotExists(folderName string) bool {
	appDir := GetAppDir()
	dirPath := filepath.Join(appDir, folderName)

	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		err := os.MkdirAll(dirPath, 0755)
		if err != nil {
			ErrorStop("无法创建文件夹"+folderName, "system")
			return false
		}
		return true
	} else if err != nil {
		ErrorStop("无法检查文件夹"+folderName, "system")
	}
	return false
}
