package main

import (
	"fmt"
	"os"

	"github.com/cjxpj/nebula/utils"
)

func installFFmpeg(destDir string, output *[]string) error {
	// Windows 64 位 FFmpeg 静态构建下载地址（含镜像回退）
	urls := []string{
		"https://cjxpj.com/download/ffmpeg-release-essentials.zip",
		"https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip",
	}

	zipPath := utils.NewFileQueue("ffmpeg_download.zip")
	defer zipPath.DeleteFile() // 确保最后清理

	if output != nil {
		*output = append(*output, "正在分段下载 FFmpeg ...")
	}
	if err := zipPath.DownloadWithMirrors(urls, 8, true, nil); err != nil {
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
