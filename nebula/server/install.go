package dic_server

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/cjxpj/nebula/utils"
)

func installPHP(destDir string, output *[]string, progressFn func(float64)) error {
	urls := []string{
		"https://cjxpj.com/download/php-7.4.33-Win32-vc15-x64.zip",
		"https://windows.php.net/downloads/releases/archives/php-7.4.33-Win32-vc15-x64.zip",
	}

	zipPath := utils.NewFileQueue("php_download.zip")
	defer zipPath.DeleteFile() // 确保下载文件最终被删除

	if output != nil {
		*output = append(*output, "正在分段下载 PHP ...")
	}
	if err := zipPath.DownloadWithMirrors(urls, 0, true, progressFn); err != nil { // 0 = 自动线程数
		return fmt.Errorf("下载失败: %w", err)
	}

	if output != nil {
		*output = append(*output, "下载完成，正在解压...")
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	if !zipPath.UnZip(destDir) {
		return fmt.Errorf("解压失败")
	}

	if output != nil {
		*output = append(*output, "✅ PHP 安装成功，路径："+destDir)
	}
	return nil
}

func installFFmpeg(destDir string, output *[]string, progressFn func(float64)) error {
	urls := []string{
		"https://cjxpj.com/download/ffmpeg-release-essentials.zip",
		"https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip",
	}

	zipPath := utils.NewFileQueue("ffmpeg_download.zip")
	defer zipPath.DeleteFile() // 确保最后清理

	if output != nil {
		*output = append(*output, "正在分段下载 FFmpeg ...")
	}
	if err := zipPath.DownloadWithMirrors(urls, 0, true, progressFn); err != nil { // 0 = 自动线程数
		return fmt.Errorf("下载失败: %w", err)
	}

	if output != nil {
		*output = append(*output, "下载完成，正在解压...")
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	if !zipPath.UnZip(destDir) {
		return fmt.Errorf("解压失败")
	}

	if output != nil {
		*output = append(*output, "✅ FFmpeg 安装成功，路径："+destDir)
	}
	return nil
}

func installSilkV3(destDir string, output *[]string, progressFn func(float64)) error {
	urls := []string{
		"https://cjxpj.com/download/silk_v3.zip",
		"https://mirror.cjxpj.com/silk_v3.zip",
	}

	zipPath := utils.NewFileQueue("silk_v3_download.zip")
	defer zipPath.DeleteFile() // 确保下载文件最终被删除

	if output != nil {
		*output = append(*output, "正在分段下载 silk_v3 ...")
	}
	if err := zipPath.DownloadWithMirrors(urls, 0, true, progressFn); err != nil { // 0 = 自动线程数
		return fmt.Errorf("下载失败: %w", err)
	}

	if output != nil {
		*output = append(*output, "下载完成，正在解压...")
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	if !zipPath.UnZip(destDir) {
		return fmt.Errorf("解压失败")
	}

	if output != nil {
		*output = append(*output, "✅ silk_v3 安装成功，路径："+filepath.Join(destDir, "silk_v3"))
	}
	return nil
}

func installNapCatBot(destDir string, qq string, output *[]string, progressFn func(float64)) error {
	urls := []string{
		"https://gh-proxy.org/github.com/NapNeko/NapCatQQ/releases/download/v4.9.81/NapCat.Shell.zip",
		"https://cjxpj.com/download/NapCat.Shell.zip",
	}

	zipPath := utils.NewFileQueue("napcat_download.zip")
	defer zipPath.DeleteFile() // 确保下载文件最终被删除

	if output != nil {
		*output = append(*output, "正在分段下载 NapCat ...")
	}
	if err := zipPath.DownloadWithMirrors(urls, 0, true, progressFn); err != nil { // 0 = 自动线程数
		return fmt.Errorf("下载失败: %w", err)
	}

	if output != nil {
		*output = append(*output, "下载完成，正在解压...")
	}

	if !zipPath.UnZip(destDir) {
		return fmt.Errorf("解压失败")
	}

	if output != nil {
		*output = append(*output, "✅ NapCat 安装成功，路径："+destDir)
	}

	if err := initNapCatBotConfig(destDir, qq, output); err != nil {
		return fmt.Errorf("初始化配置失败: %w", err)
	}

	return nil
}

// installGo 下载并解压 Go 工具链（词库编译环境），用于把词库编译为独立可执行文件。
// destDir 为扩展根目录（private/extensions），Go zip 自带 go/ 顶层目录，
// 解压后工具链位于 destDir/go（go.exe 位于 destDir/go/bin/go.exe）。
func installGo(destDir string, output *[]string, progressFn func(float64)) error {
	urls := []string{
		"https://golang.google.cn/dl/go1.27.0.windows-amd64.zip",
		"https://go.dev/dl/go1.27.0.windows-amd64.zip",
	}

	zipPath := utils.NewFileQueue("go_download.zip")
	defer zipPath.DeleteFile() // 确保下载文件最终被删除

	if output != nil {
		*output = append(*output, "正在分段下载 Go 工具链 ...")
	}
	if err := zipPath.DownloadWithMirrors(urls, 0, true, progressFn); err != nil { // 0 = 自动线程数
		return fmt.Errorf("下载失败: %w", err)
	}

	if output != nil {
		*output = append(*output, "下载完成，正在解压...")
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	if !zipPath.UnZip(destDir) {
		return fmt.Errorf("解压失败")
	}

	if output != nil {
		*output = append(*output, "✅ Go 工具链安装成功，路径："+filepath.Join(destDir, "go"))
	}
	return nil
}

func installPython(destDir string, output *[]string, progressFn func(float64)) error {
	urls := []string{
		"https://registry.npmmirror.com/-/binary/python/3.12.8/python-3.12.8-embed-amd64.zip",
		"https://cjxpj.com/download/python-3.12.8-embed-amd64.zip",
	}

	zipPath := utils.NewFileQueue("python_download.zip")
	defer zipPath.DeleteFile() // 确保下载文件最终被删除

	if output != nil {
		*output = append(*output, "正在分段下载 Python ...")
	}
	if err := zipPath.DownloadWithMirrors(urls, 0, true, progressFn); err != nil { // 0 = 自动线程数
		return fmt.Errorf("下载失败: %w", err)
	}

	if output != nil {
		*output = append(*output, "下载完成，正在解压...")
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	if !zipPath.UnZip(destDir) {
		return fmt.Errorf("解压失败")
	}

	if output != nil {
		*output = append(*output, "✅ Python 安装成功，路径："+destDir)
	}
	return nil
}

func initNapCatBotConfig(destDir string, qq string, output *[]string) error {
	if output != nil {
		*output = append(*output, "正在初始化配置文件 ...")
	}
	cfg := map[string]any{
		"network": map[string]any{
			"httpServers": []any{
				map[string]any{
					"enable":            true,
					"name":              "msg",
					"host":              "127.0.0.1",
					"port":              3000,
					"enableCors":        true,
					"enableWebsocket":   false,
					"messagePostFormat": "array",
					"token":             "", // 待填充
					"debug":             false,
				},
			},
			"httpSseServers": []any{},
			"httpClients": []any{
				map[string]any{
					"enable":            true,
					"name":              "nebula",
					"url":               "http://127.0.0.1:8080/napcat",
					"reportSelfMessage": false,
					"messagePostFormat": "array",
					"token":             "", // 待填充
					"debug":             false,
				},
			},
			"websocketServers": []any{},
			"websocketClients": []any{},
			"plugins":          []any{},
		},
		"musicSignUrl":        "",
		"enableLocalFile2Url": false,
		"parseMultMsg":        false,
	}

	if output != nil {
		*output = append(*output, "正在生成HTTP服务器Token ...")
	}

	httpServerToken := randToken(16)
	// 2. 生成并覆盖两个 token
	net, ok := cfg["network"].(map[string]any)
	if !ok {
		return fmt.Errorf("配置文件 network 字段格式错误: 期望 map[string]any")
	}
	httpServers, ok := net["httpServers"].([]any)
	if !ok || len(httpServers) == 0 {
		return fmt.Errorf("配置文件 httpServers 字段格式错误: 期望非空 []any")
	}
	httpServer, ok := httpServers[0].(map[string]any)
	if !ok {
		return fmt.Errorf("配置文件 httpServers[0] 字段格式错误: 期望 map[string]any")
	}
	httpServer["token"] = httpServerToken
	if output != nil {
		*output = append(*output, "正在生成HTTP客户端Token ...")
	}
	httpClients, ok := net["httpClients"].([]any)
	if !ok || len(httpClients) == 0 {
		return fmt.Errorf("配置文件 httpClients 字段格式错误: 期望非空 []any")
	}
	httpClient, ok := httpClients[0].(map[string]any)
	if !ok {
		return fmt.Errorf("配置文件 httpClients[0] 字段格式错误: 期望 map[string]any")
	}
	httpClient["token"] = randToken(16)

	if output != nil {
		*output = append(*output, "正在生成配置文件 ...")
	}

	outConfig, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if output != nil {
		*output = append(*output, "正在写入配置文件 ...")
	}

	destPath := filepath.Join(destDir, "config", fmt.Sprintf("onebot11_%s.json", qq))
	if output != nil {
		*output = append(*output, "配置文件路径："+destPath)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	if err := os.WriteFile(destPath, outConfig, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	if output != nil {
		*output = append(*output, "✅ 配置文件写入成功")
		*output = append(*output, "每个账号的HTTP服务器Token都是独立的，切换账号时候需要重新配置。")
		*output = append(*output, "HTTP服务器Token："+httpServerToken)
		*output = append(*output, "请前往填写配置：NebulaData/private/system/config.n")
		*output = append(*output, "✅ 填写完配置后关掉此窗口，重新运行程序即可")
	}
	return nil
}

// fileExists 直接按给定完整路径判断文件是否存在（路径已含应用目录，避免 NewFileQueue 二次拼接）。
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// isPythonInstalled 判断 Python 是否可用。
// Windows 检测内置 python.exe；其他平台优先检测内置 python3，否则检测系统 python3。
func isPythonInstalled(destDir string) bool {
	if runtime.GOOS == "windows" {
		return fileExists(filepath.Join(destDir, "python.exe"))
	}
	if fileExists(filepath.Join(destDir, "python3")) {
		return true
	}
	_, err := exec.LookPath("python3")
	return err == nil
}

const letterBytes = "23456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz@]{}"

func randToken(n int) string {
	b := make([]byte, n)
	l := big.NewInt(int64(len(letterBytes)))
	for i := range b {
		idx, _ := rand.Int(rand.Reader, l)
		b[i] = letterBytes[idx.Int64()]
	}
	return string(b)
}
