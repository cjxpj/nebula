//go:generate goversioninfo -64
//go:build windows

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/dic"
	"github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	dic_server "github.com/cjxpj/nebula/server"
	"github.com/cjxpj/nebula/utils"
)

func init() {
	if len(os.Args) > 1 && os.Args[1] == "--autostart" {
		exePath, _ := os.Executable()
		exeDir := filepath.Dir(exePath)
		err := os.Chdir(exeDir) // 强制切换为 exe 所在目录
		if err != nil {
			fmt.Println("自动启动失败:", err)
		}
	}
}

func main() {
	// fmt.Println("Nebula 启动中...")

	// 初始化执行环境
	dto.GV.Set("_PythonPath_", "python")

	// 检测文件是否存在ffmpeg.exe
	ffmpegPath := utils.FindFfmpegExe("private/extensions/ffmpeg")
	if ffmpegPath != "" {
		dto.GV.Set("_Ffmpeg_", filepath.Join(utils.GetAppDir(), ffmpegPath))
	} else {
		dto.GV.Set("_Ffmpeg_", "ffmpeg")
	}

	// 检测文件是否存在silk_v3.exe
	if utils.NewFileQueue("private/extensions/silk_v3").DirExists() {
		dto.GV.Set("_SilkPath_", filepath.Join(utils.GetAppDir(), "private", "extensions", "silk_v3"))
	}

	// 检测文件是否存在php.exe
	if utils.NewFileQueue("private/extensions/php/php.exe").FileExists() {
		dto.GV.Set("_PhpPath_", filepath.Join(utils.GetAppDir(), "private", "extensions", "php", "php.exe"))
	} else {
		dto.GV.Set("_PhpPath_", "php")
	}

	// 检测文件是否存在python.exe
	if utils.NewFileQueue("private/extensions/python/python.exe").FileExists() {
		dto.GV.Set("_PythonPath_", filepath.Join(utils.GetAppDir(), "private", "extensions", "python", "python.exe"))
	}

	// 用主上下文控制整个进程生命周期
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer ShutdownPhp() // 确保退出时同步 kill PHP 进程

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

	// 设备电量（Windows 笔记本）
	funcs.Register("设备电量", "0", func(d *dto.DicInputs) (any, error) {
		return getBatteryStatus(), nil
	})

	funcs.Register("回收站", "1", func(d *dto.DicInputs) (any, error) {
		fq := utils.NewFileQueue(d.Inputs.String(1))
		// 检查文件夹是否存在
		p, err := os.Stat(fq.FileName)
		if err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, fmt.Errorf("检查文件夹失败: %v", err)
		}
		if !p.IsDir() {
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
			getData, postData, fileData, cleanup, err := parseRequestToMap(r)
			if cleanup != nil {
				defer cleanup()
			}
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
	case "-help":
		fmt.Println("-help               		（显示帮助）")
		fmt.Println("-v                  		（显示版本）")
		fmt.Println("-autostart          		（开机自启）")
		fmt.Println("-noautostart        		（取消开机自启）")
	case "-v":
		fmt.Print(appfiles.Version)
		return
	case "-autostart":
		err := dic_server.SetAutoStart()
		if err != nil {
			fmt.Println("设置开机启动失败:", err)
		} else {
			fmt.Println("已设置为开机启动")
		}
		return
	case "-noautostart":
		err := dic_server.CancelAutoStart()
		if err != nil {
			fmt.Println("取消开机启动失败:", err)
		} else {
			fmt.Println("已取消开机启动")
		}
		return
	default:
		fmt.Println("未知命令")
		return
	}
}

// ---------- 设备电量 ----------

type systemPowerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	Reserved1           byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

func getBatteryStatus() string {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GetSystemPowerStatus")

	var sps systemPowerStatus
	ret, _, _ := proc.Call(uintptr(unsafe.Pointer(&sps)))
	if ret == 0 {
		return `{"level":-1,"charging":false,"error":"call failed"}`
	}

	// 255 表示未知
	level := sps.BatteryLifePercent
	if level == 255 {
		return `{"level":-1,"charging":false,"error":"unknown"}`
	}

	// ACLineStatus: 0=电池供电, 1=外接电源, 255=未知
	charging := sps.ACLineStatus == 1

	return fmt.Sprintf(`{"level":%d,"charging":%v}`, level, charging)
}
