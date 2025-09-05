package main

import (
	"fmt"
	"os"

	"github.com/cjxpj/nebula/utils"
)

func installFFmpeg(destDir string) error {
	// Windows 64 位 FFmpeg 静态构建下载地址
	url := "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip"

	zipPath := utils.NewFileQueue("ffmpeg_download.zip")
	defer zipPath.DeleteFile() // 确保最后清理

	fmt.Println("正在分段下载 FFmpeg ...")
	if err := zipPath.DownloadWithDynamicThreads(url, 8, true); err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}

	fmt.Println("下载完成，正在解压...")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	if !zipPath.UnZip(destDir) {
		return fmt.Errorf("解压失败")
	}

	fmt.Println("✅ FFmpeg 安装成功，路径：" + destDir)
	return nil
}
