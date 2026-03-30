//go:build windows

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/dic"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"

	"golang.org/x/sys/windows/registry"
)

func init() {
	if len(os.Args) > 1 && os.Args[1] == "--autostart" {
		exePath, _ := os.Executable()
		exeDir := filepath.Dir(exePath)
		// 打印./位置
		// fmt.Println("启动位置:", exeDir)
		err := os.Chdir(exeDir) // 强制切换为 exe 所在目录
		if err != nil {
			fmt.Println("自动启动失败:", err)
		}
	}
}

// 设置开机启动
func setAutoStart(appName string) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	k, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Run`,
		registry.SET_VALUE,
	)
	if err != nil {
		return err
	}
	defer k.Close()

	// 注册表里写入带参数的启动命令
	return k.SetStringValue(appName, fmt.Sprintf(`"%s" --autostart`, exePath))
}

// 取消开机启动
func cancelAutoStart(appName string) error {
	k, err := registry.OpenKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Run`,
		registry.SET_VALUE,
	)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.DeleteValue(appName)
}

func main() {
	// fmt.Println("Nebula 启动中...")

	// 初始化执行环境
	dto.GV.Set("_PythonPath_", "python")

	// 检测文件是否存在ffmpeg.exe
	if utils.NewFileQueue("private/ffmpeg/ffmpeg-7.1.1-essentials_build/bin/ffmpeg.exe").FileExists() {
		dto.GV.Set("_Ffmpeg_", filepath.Join(utils.GetAppDir(), "private", "ffmpeg", "ffmpeg-7.1.1-essentials_build", "bin", "ffmpeg.exe"))
	} else {
		dto.GV.Set("_Ffmpeg_", "ffmpeg")
	}

	// 检测文件是否存在silk_v3.exe
	if utils.NewFileQueue("private/ffmpeg/silk_v3").DirExists() {
		dto.GV.Set("_SilkPath_", filepath.Join(utils.GetAppDir(), "private", "ffmpeg", "silk_v3"))
	}

	// 检测文件是否存在php.exe
	if utils.NewFileQueue("private/php/php.exe").FileExists() {
		dto.GV.Set("_PhpPath_", filepath.Join(utils.GetAppDir(), "private", "php", "php.exe"))
	} else {
		dto.GV.Set("_PhpPath_", "php")
	}

	// 用主上下文控制整个进程生命周期
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 监听系统信号，退出时取消 ctx
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("收到退出信号，准备关闭...")
		cancel()
	}()

	funcs.Register("打开浏览器", "1", func(d *dto.DicInputs) (any, error) {
		err := openBrowser(d.Inputs.String(1))
		return "", err
	})

	funcs.Register("回收站", "1", func(d *dto.DicInputs) (any, error) {
		fq := utils.NewFileQueue(d.Inputs.String(1))
		// 检查文件夹是否存在
		if p, err := os.Stat(fq.FileName); os.IsNotExist(err) || !p.IsDir() {
			return false, nil
		}

		// 使用Windows API将文件移动到回收站
		// 首先获取绝对路径
		absPath, err := filepath.Abs(fq.FileName)
		if err != nil {
			return false, nil
		}

		// 调用Windows API移动到回收站
		// 这里使用简单的方法：通过cmd命令行调用recycle命令
		cmd := exec.Command("cmd", "/c", "powershell", "Remove-Item", "-Path", absPath, "-Recurse", "-Force", "-ErrorAction", "SilentlyContinue")
		if err := cmd.Run(); err != nil {
			return false, nil
		}

		return true, nil
	})

	funcs.Register("PHP", "1|2|3", func(d *dto.DicInputs) (any, error) {

		phpCode := d.Inputs.String(1)

		// 如果有传入 *http.Request 作为第二个参数
		if r, ok := d.Inputs.Get(2).(*http.Request); ok {
			getData, postData, fileData, err := parseRequestToMap(r)
			if err != nil {
				return "请求解析失败: " + err.Error(), nil
			}
			var result string
			if w, ok := d.Inputs.Get(3).(http.ResponseWriter); ok {
				result, err = runTempPHP(ctx, phpCode, &getData, &postData, &fileData, w)
				if err != nil {
					return "执行失败: " + err.Error(), nil
				}
			} else {
				result, err = runTempPHP(ctx, phpCode, &getData, &postData, &fileData, nil)
				if err != nil {
					return "执行失败: " + err.Error(), nil
				}
			}
			return result, nil
		}

		// 无请求，直接执行
		result, err := runTempPHP(ctx, phpCode, nil, nil, nil, nil)
		return result, err
	})

	// 注入自义定函数
	funcs.Register("Python", "1", func(d *dto.DicInputs) (any, error) {
		output, err := runPythonCode(d.Inputs.String(1))
		return output, err
	})

	// 注入自定义函数
	funcs.Register("音频转Silk", "2", func(d *dto.DicInputs) (any, error) {

		inputMP3 := utils.NewFileQueue(d.Inputs.String(1))
		if !inputMP3.FileExists() {
			return "false", nil
		}
		outputSilk := utils.NewFileQueue(d.Inputs.String(2))

		err := mp3ToSilk(inputMP3.FileName, outputSilk.FileName)
		if err != nil {
			return "false", err
		}
		return "true", nil
	})

	args := os.Args
	argsLen := len(args)

	if argsLen == 1 {
		// 启动
		dic.Start()
		<-ctx.Done()
		fmt.Println("主程序退出")
		return
	}

	switch args[1] {
	case "--autostart":
		// 启动
		dic.Start()
		<-ctx.Done()
		fmt.Println("主程序退出")
		return
	case "-napcat_bot":
		if argsLen < 3 {
			fmt.Println("用法: NebulaApp.exe -napcat_bot [QQ号]")
			return
		}
		qq := args[2]

		if !utils.NewFileQueue("private/NapCat.Shell/launcher.bat").FileExists() {
			installDir := filepath.Join("private", "NapCat.Shell")
			if err := installNapCatBot(installDir, qq); err != nil {
				fmt.Println("napcat_bot 安装失败:", err)
				return
			}
			if !utils.NewFileQueue(filepath.Join("private", "NapCat.Shell", "config", fmt.Sprintf("onebot11_%s.json", qq))).FileExists() {
				initNapCatBotConfig(installDir, qq)
			}
		}

		batPath := filepath.Join(utils.GetAppDir(), "private", "NapCat.Shell", "launcher.bat")
		absPath, _ := filepath.Abs(batPath)
		dir := filepath.Dir(absPath)

		// 1. 新建独立窗口运行；2. 不继承本进程控制台；3. 立即返回
		cmd := exec.Command("cmd", "/c", "start", "", absPath, qq)
		cmd.Dir = dir
		cmd.Start()

		// 启动
		dic.Start()
		<-ctx.Done()
		fmt.Println("主程序退出")
		return

	case "-help":
		fmt.Println("-help               		（显示帮助）")
		fmt.Println("-v                  		（显示版本）")
		fmt.Println("-autostart          		（开机自启）")
		fmt.Println("-noautostart        		（取消开机自启）")
		fmt.Println("-install php        		（安装 PHP）")
		fmt.Println("-install ffmpeg     		（安装 ffmpeg）")
		fmt.Println("-napcat_bot [QQ号] 		（安装运行 napcat非官方机器人）")
		fmt.Println("run [文件] [触发词] （运行文件）")
	case "-v":
		fmt.Print(appfiles.Version)
		return
	case "-autostart":
		err := setAutoStart("NebulaApp")
		if err != nil {
			fmt.Println("设置开机启动失败:", err)
		} else {
			fmt.Println("已设置为开机启动")
		}
		return
	case "-noautostart":
		err := cancelAutoStart("NebulaApp")
		if err != nil {
			fmt.Println("取消开机启动失败:", err)
		} else {
			fmt.Println("已取消开机启动")
		}
		return
	case "-install":
		if argsLen < 3 {
			fmt.Println("用法: -install php")
			return
		}
		install_name := args[2]
		switch install_name {
		case "php",
			"silk_v3":
			install(install_name)
		case "ffmpeg":
			install(install_name)
			fmt.Println("准备安装附带silk_v3工具")
			install("silk_v3")
		case "napcat_bot":
			install(install_name)
		default:
			fmt.Println("未知安装目标:", args[2])
		}
		return
	case "run":
		if argsLen < 3 || argsLen > 4 {
			fmt.Println("用法: NebulaApp.exe run [文件路径] [触发词]")
			return
		}

		cmdInput := args[2]
		triggerWord := "Main"
		if argsLen == 4 {
			triggerWord = args[3]
		}
		// 计时
		start := time.Now()
		d, err := dic_dto.NewDicPro(cmdInput)
		if err != nil {
			log.Fatal(err)
			return
		}
		results := dic_api.Api.DicRunPro(d, triggerWord)
		fmt.Print(results)
		elapsed := time.Since(start)
		fmt.Println("执行时间:", elapsed)
		return
	default:
		fmt.Println("未知命令")
		return
	}
}
