package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cjxpj/nebula/utils"
)

func install(a string, output *[]string, params map[string]string) error {
	switch a {
	case "php":
		if utils.NewFileQueue("private/php/php.exe").FileExists() {
			msg := "PHP 已安装"
			if output != nil {
				*output = append(*output, msg)
			}
			return nil
		}
		installDir := filepath.Join("private", "php")
		_ = os.MkdirAll(installDir, 0755)
		if err := installPHP(installDir, output); err != nil {
			msg := "PHP 安装失败: " + err.Error()
			if output != nil {
				*output = append(*output, msg)
			}
			return err
		}
		msg := "PHP 安装成功，路径: " + installDir
		if output != nil {
			*output = append(*output, msg)
		}
		return nil
	case "ffmpeg":
		if utils.NewFileQueue("private/ffmpeg/ffmpeg.exe").FileExists() {
			msg := "FFmpeg 已安装"
			if output != nil {
				*output = append(*output, msg)
			}
			return nil
		}
		installDir := filepath.Join("private", "ffmpeg")
		_ = os.MkdirAll(installDir, 0755)
		if err := installFFmpeg(installDir, output); err != nil {
			msg := "FFmpeg 安装失败: " + err.Error()
			if output != nil {
				*output = append(*output, msg)
			}
			return err
		}
		msg := "FFmpeg 安装成功，路径: " + installDir
		if output != nil {
			*output = append(*output, msg)
		}
		return nil
	case "silk_v3":
		if utils.NewFileQueue("private/ffmpeg/silk_v3/silk_v3_encoder.exe").FileExists() {
			msg := "silk_v3 已安装"
			if output != nil {
				*output = append(*output, msg)
			}
			return nil
		}
		installDir := filepath.Join("private", "ffmpeg")
		_ = os.MkdirAll(installDir, 0755)
		// 安装silk_v3
		if err := installSilkV3(installDir, output); err != nil {
			msg := "silk_v3 安装失败: " + err.Error()
			if output != nil {
				*output = append(*output, msg)
			}
			return err
		}
		msg := "silk_v3 安装成功，路径: " + installDir
		if output != nil {
			*output = append(*output, msg)
		}
		return nil
	case "napcat_bot":
		if utils.NewFileQueue("private/NapCat.Shell/launcher.bat").FileExists() {
			msg := "napcat_bot 已安装"
			if output != nil {
				*output = append(*output, msg)
			}
			return nil
		}
		installDir := filepath.Join("private", "NapCat.Shell")
		qq := ""
		if params != nil {
			if qqVal, ok := params["qq"]; ok {
				qq = qqVal
			}
		}
		if qq == "" {
			msg := "缺少QQ账号参数"
			if output != nil {
				*output = append(*output, msg)
			}
			return fmt.Errorf("missing qq")
		}
		if err := installNapCatBot(installDir, qq, output); err != nil {
			msg := "napcat_bot 安装失败: " + err.Error()
			if output != nil {
				*output = append(*output, msg)
			}
			return err
		}
		msg := "napcat_bot 安装成功"
		if output != nil {
			*output = append(*output, msg)
		}
		return nil
	}
	return nil
}
