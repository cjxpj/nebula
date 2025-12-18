package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"path/filepath"

	"github.com/cjxpj/nebula/utils"
)

// 初始化配置文件
func initNapCatBotConfig(destDir string, qq string) error {
	fmt.Println("正在初始化配置文件 ...")
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

	fmt.Println("正在生成HTTP服务器Token ...")

	httpServerToken := randToken(16)
	// 2. 生成并覆盖两个 token
	net := cfg["network"].(map[string]any)
	net["httpServers"].([]any)[0].(map[string]any)["token"] = httpServerToken
	fmt.Println("正在生成HTTP客户端Token ...")
	net["httpClients"].([]any)[0].(map[string]any)["token"] = randToken(16)

	fmt.Println("正在生成配置文件 ...")

	outConfig, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	fmt.Println("正在写入配置文件 ...")

	destPath := utils.NewFileQueue(filepath.Join(destDir, "config", fmt.Sprintf("onebot11_%s.json", qq)))
	fmt.Println("配置文件路径：", destPath.FileName)
	destPath.WriteFileByte(outConfig)

	fmt.Println("✅ 配置文件写入成功")

	fmt.Println("每个账号的HTTP服务器Token都是独立的，切换账号时候需要重新配置。")
	fmt.Println("HTTP服务器Token：", httpServerToken)
	fmt.Println("请前往填写配置：NebulaData/private/system/config.n")
	fmt.Println("✅ 填写完配置后关掉此窗口，重新运行程序即可")
	fmt.Println("⚠如果机器人程序不存在，就关掉全部窗口通过CMD启动")
	fmt.Println("指令：nebula -napcat_bot [QQ]")
	return nil
}

func installNapCatBot(destDir string, qq string) error {
	url := "https://gh-proxy.org/https://github.com/NapNeko/NapCatQQ/releases/download/v4.9.81/NapCat.Shell.zip"

	zipPath := utils.NewFileQueue("napcat_download.zip")
	defer zipPath.DeleteFile() // 确保下载文件最终被删除

	fmt.Println("正在分段下载 NapCat ...")
	if err := zipPath.DownloadWithDynamicThreads(url, 8, true); err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}

	fmt.Println("下载完成，正在解压...")

	if !zipPath.UnZip(destDir) {
		return fmt.Errorf("解压失败")
	}

	fmt.Println("✅ NapCat 安装成功，路径：", utils.NewFileQueue(destDir).FileName)

	if err := initNapCatBotConfig(destDir, qq); err != nil {
		return fmt.Errorf("初始化配置失败: %w", err)
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
