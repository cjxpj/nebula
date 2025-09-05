package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cjxpj/nebula/utils"
)

func install(a string) {
	switch a {
	case "php":
		if utils.NewFileQueue("private/php/php.exe").FileExists() {
			fmt.Println("PHP 已安装")
			return
		}
		installDir := filepath.Join("private", "php")
		_ = os.MkdirAll(installDir, 0755)
		if err := installPHP(installDir); err != nil {
			fmt.Println("PHP 安装失败:", err)
		} else {
			fmt.Println("PHP 安装成功，路径:", installDir)
		}
	case "ffmpeg":
		if utils.NewFileQueue("private/ffmpeg/ffmpeg.exe").FileExists() {
			fmt.Println("FFmpeg 已安装")
			return
		}
		installDir := filepath.Join("private", "ffmpeg")
		_ = os.MkdirAll(installDir, 0755)
		if err := installFFmpeg(installDir); err != nil {
			fmt.Println("FFmpeg 安装失败:", err)
		} else {
			fmt.Println("FFmpeg 安装成功，路径:", installDir)
		}
	case "silk_v3":
		if utils.NewFileQueue("private/ffmpeg/silk_v3/silk_v3_encoder.exe").FileExists() {
			fmt.Println("silk_v3 已安装")
			return
		}
		installDir := filepath.Join("private", "ffmpeg")
		_ = os.MkdirAll(installDir, 0755)
		// 安装silk_v3
		if err := installSilkV3(installDir); err != nil {
			fmt.Println("silk_v3 安装失败:", err)
		} else {
			fmt.Println("silk_v3 安装成功，路径:", installDir)
		}
	}
}
