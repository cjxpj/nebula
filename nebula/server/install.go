package dic_server

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"

	"github.com/cjxpj/nebula/utils"
)

func installPHP(destDir string, output *[]string) error {
	url := "https://windows.php.net/downloads/releases/php-7.4.33-Win32-vc15-x64.zip"

	zipPath := utils.NewFileQueue("php_download.zip")
	defer zipPath.DeleteFile() // 确保下载文件最终被删除

	*output = append(*output, "正在分段下载 PHP ...")
	if err := zipPath.DownloadWithDynamicThreads(url, 8, true); err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}

	*output = append(*output, "下载完成，正在解压...")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	if !zipPath.UnZip(destDir) {
		return fmt.Errorf("解压失败")
	}

	*output = append(*output, "✅ PHP 安装成功，路径："+destDir)
	return nil
}

func installFFmpeg(destDir string, output *[]string) error {
	url := "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip"

	zipPath := utils.NewFileQueue("ffmpeg_download.zip")
	defer zipPath.DeleteFile() // 确保最后清理

	*output = append(*output, "正在分段下载 FFmpeg ...")
	if err := zipPath.DownloadWithDynamicThreads(url, 8, true); err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}

	*output = append(*output, "下载完成，正在解压...")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	if !zipPath.UnZip(destDir) {
		return fmt.Errorf("解压失败")
	}

	*output = append(*output, "✅ FFmpeg 安装成功，路径：" + destDir)
	return nil
}

func installSilkV3(destDir string, output *[]string) error {
	url := "https://cjxpj.com/download/silk_v3.zip"

	zipPath := utils.NewFileQueue("silk_v3_download.zip")

	*output = append(*output, "正在分段下载 silk_v3 ...")
	if err := zipPath.DownloadWithDynamicThreads(url, 4, true); err != nil {
		return fmt.Errorf("下载失败: %v", err)
	}

	*output = append(*output, "下载完成，正在解压...")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	if !zipPath.UnZip(destDir) {
		return fmt.Errorf("解压失败")
	}
	zipPath.DeleteFile()

	*output = append(*output, "✅ silk_v3 安装成功，路径："+destDir)
	return nil
}

func installNapCatBot(destDir string, qq string, output *[]string) error {
	url := "https://gh-proxy.org/https://github.com/NapNeko/NapCatQQ/releases/download/v4.9.81/NapCat.Shell.zip"

	zipPath := utils.NewFileQueue("napcat_download.zip")
	defer zipPath.DeleteFile() // 确保下载文件最终被删除

	if output != nil {
		*output = append(*output, "正在分段下载 NapCat ...")
	}
	if err := zipPath.DownloadWithDynamicThreads(url, 8, true); err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}

	if output != nil {
		*output = append(*output, "下载完成，正在解压...")
	}

	if !zipPath.UnZip(destDir) {
		return fmt.Errorf("解压失败")
	}

	if output != nil {
		*output = append(*output, "✅ NapCat 安装成功，路径：" + utils.NewFileQueue(destDir).FileName)
	}

	if err := initNapCatBotConfig(destDir, qq, output); err != nil {
		return fmt.Errorf("初始化配置失败: %w", err)
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
	net := cfg["network"].(map[string]any)
	net["httpServers"].([]any)[0].(map[string]any)["token"] = httpServerToken
	if output != nil {
		*output = append(*output, "正在生成HTTP客户端Token ...")
	}
	net["httpClients"].([]any)[0].(map[string]any)["token"] = randToken(16)

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

	destPath := utils.NewFileQueue(filepath.Join(destDir, "config", fmt.Sprintf("onebot11_%s.json", qq)))
	if output != nil {
		*output = append(*output, "配置文件路径：" + destPath.FileName)
	}
	destPath.WriteFileByte(outConfig)

	if output != nil {
		*output = append(*output, "✅ 配置文件写入成功")
		*output = append(*output, "每个账号的HTTP服务器Token都是独立的，切换账号时候需要重新配置。")
		*output = append(*output, "HTTP服务器Token：" + httpServerToken)
		*output = append(*output, "请前往填写配置：NebulaData/private/system/config.n")
		*output = append(*output, "✅ 填写完配置后关掉此窗口，重新运行程序即可")
	}
	return nil
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