//go:build windows

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/dic"
	"github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"

	"github.com/laurent22/go-trash"
	"golang.org/x/sys/windows/registry"
)

// func init() {
// 	exePath, _ := os.Executable()
// 	exeDir := filepath.Dir(exePath)
// 	// 打印./位置
// 	// fmt.Println("启动位置:", exeDir)
// 	err := os.Chdir(exeDir) // 强制切换为 exe 所在目录
// 	if err != nil {
// 		fmt.Println("切换目录失败:", err)
// 	}
// }

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
	return k.SetStringValue(appName, exePath)
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

// func main() {
// 	install("ffmpeg")
// 	fmt.Println("准备安装附带silk_v3工具")
// 	install("silk_v3")
// }

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

	funcs.Register("回收站", "1", func(d *dto.DicInputs) (any, error) {
		fq := utils.NewFileQueue(d.Inputs.String(1))
		// 检查文件夹是否存在
		if p, err := os.Stat(fq.FileName); os.IsNotExist(err) || !p.IsDir() {
			return false, nil
		}

		if trash.IsAvailable() {
			_, err := trash.MoveToTrash(fq.FileName)
			if err != nil {
				return false, nil
			}
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

	// debug.SetGCPercent(50) // 增长 50% 就触发 GC，更频繁

	// go func() {
	// 	mux := http.NewServeMux()

	// 	// 注册标准 pprof 路由
	// 	mux.HandleFunc("/debug/pprof/", httpPprof.Index)
	// 	mux.HandleFunc("/debug/pprof/cmdline", httpPprof.Cmdline)
	// 	mux.HandleFunc("/debug/pprof/profile", httpPprof.Profile)
	// 	mux.HandleFunc("/debug/pprof/symbol", httpPprof.Symbol)
	// 	mux.HandleFunc("/debug/pprof/trace", httpPprof.Trace)

	// 	// 添加你自己的 heap_text 路由
	// 	mux.HandleFunc("/debug/pprof/heap_text", func(w http.ResponseWriter, r *http.Request) {
	// 		runtimePprof.Lookup("heap").WriteTo(w, 1) // 1 = human-readable text format
	// 	})

	// 	// 启动服务
	// 	http.ListenAndServe(":6060", mux)
	// }()

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
	case "-help":
		fmt.Println("-help               （显示帮助）")
		fmt.Println("-v                  （显示版本）")
		fmt.Println("-autostart          （开机自启）")
		fmt.Println("-noautostart        （取消开机自启）")
		fmt.Println("-install php        （安装 PHP）")
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

		file, err := os.Open(cmdInput)
		if err != nil {
			fmt.Println("打开文件失败:", err)
			return
		}
		defer file.Close()

		var result strings.Builder
		buf := make([]byte, 1024)
		for {
			n, err := file.Read(buf)
			if n > 0 {
				result.Write(buf[:n])
			}
			if err != nil {
				if err == io.EOF {
					break
				}
				fmt.Println("读取文件失败:", err)
				return
			}
		}

		results := dic.NewDic(cmdInput, result.String()).Run(triggerWord)
		fmt.Print(results)
		return
	default:
		fmt.Println("未知命令")
		return
	}
}
