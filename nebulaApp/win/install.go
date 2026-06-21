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
		if utils.NewFileQueue("private/extensions/php/php.exe").FileExists() {
			msg := "PHP 已安装"
			if output != nil {
				*output = append(*output, msg)
			}
			return nil
		}
		installDir := filepath.Join("private", "extensions", "php")
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
		if utils.FindFfmpegExe("private/extensions/ffmpeg") != "" {
			msg := "FFmpeg 已安装"
			if output != nil {
				*output = append(*output, msg)
			}
			return nil
		}
		installDir := filepath.Join("private", "extensions", "ffmpeg")
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
		if utils.NewFileQueue("private/extensions/silk_v3/silk_v3_encoder.exe").FileExists() {
			msg := "silk_v3 已安装"
			if output != nil {
				*output = append(*output, msg)
			}
			return nil
		}
		installDir := filepath.Join("private", "extensions")
		_ = os.MkdirAll(installDir, 0755)
		// 安装silk_v3
		if err := installSilkV3(installDir, output); err != nil {
			msg := "silk_v3 安装失败: " + err.Error()
			if output != nil {
				*output = append(*output, msg)
			}
			return err
		}
		msg := "silk_v3 安装成功，路径: " + filepath.Join(installDir, "silk_v3")
		if output != nil {
			*output = append(*output, msg)
		}
		return nil
	case "napcat_bot":
		if utils.NewFileQueue("private/extensions/NapCat.Shell/launcher.bat").FileExists() {
			msg := "napcat_bot 已安装"
			if output != nil {
				*output = append(*output, msg)
			}
			return nil
		}
		installDir := filepath.Join("private", "extensions", "NapCat.Shell")
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
	case "python":
		if utils.NewFileQueue("private/extensions/python/python.exe").FileExists() {
			msg := "Python 已安装"
			if output != nil {
				*output = append(*output, msg)
			}
			return nil
		}
		installDir := filepath.Join("private", "extensions", "python")
		_ = os.MkdirAll(installDir, 0755)
		if err := installPython(installDir, output); err != nil {
			msg := "Python 安装失败: " + err.Error()
			if output != nil {
				*output = append(*output, msg)
			}
			return err
		}
		msg := "Python 安装成功，路径: " + installDir
		if output != nil {
			*output = append(*output, msg)
		}
		return nil
	}
	return nil
}

func uninstall(component string, output *[]string) error {
	appDir := utils.GetAppDir()
	var rmDir string
	switch component {
	case "php":
		rmDir = filepath.Join(appDir, "private", "extensions", "php")
	case "ffmpeg":
		rmDir = filepath.Join(appDir, "private", "extensions", "ffmpeg")
	case "silk_v3":
		rmDir = filepath.Join(appDir, "private", "extensions", "silk_v3")
	case "napcat_bot":
		rmDir = filepath.Join(appDir, "private", "extensions", "NapCat.Shell")
	case "python":
		rmDir = filepath.Join(appDir, "private", "extensions", "python")
	default:
		return fmt.Errorf("未知组件: %s", component)
	}
	if err := os.RemoveAll(rmDir); err != nil {
		return fmt.Errorf("卸载失败: %w", err)
	}
	msg := component + " 已卸载"
	if output != nil {
		*output = append(*output, msg)
	}
	return nil
}
