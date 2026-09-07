//go:generate goversioninfo -64
//go:build windows

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/dic"
	"github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	dic_server "github.com/cjxpj/nebula/server"
	"github.com/cjxpj/nebula/extloader"
	"github.com/cjxpj/nebula/utils"
	"github.com/hymkor/trash-go"
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

// detectExtExec 检测扩展目录下的可执行文件，存在则返回其绝对路径，否则返回系统命令名。
func detectExtExec(relPath, fallback string) string {
	if utils.NewFileQueue(relPath).FileExists() {
		return filepath.Join(utils.GetAppDir(), filepath.FromSlash(relPath))
	}
	return fallback
}

func main() {
	// fmt.Println("Nebula 启动中...")

	// 扩展可执行文件路径需在启动词库确定数据目录后检测，故注入回调由 dic.Start 调用
	dic.SetupExtensionPaths = func() {
		// 检测扩展目录中的可执行文件，优先使用内置扩展，否则回退到系统命令名
		dto.SetThreadVarRaw("_Ffmpeg_", "ffmpeg")
		if ffmpegPath := utils.FindFfmpegExe(filepath.Join(utils.GetAppDir(), "private", "extensions", "ffmpeg")); ffmpegPath != "" {
			dto.SetThreadVarRaw("_Ffmpeg_", ffmpegPath)
		}

		if utils.NewFileQueue("private/extensions/silk_v3").DirExists() {
			dto.SetThreadVarRaw("_SilkPath_", filepath.Join(utils.GetAppDir(), "private", "extensions", "silk_v3"))
		}

		dto.SetThreadVarRaw("_Php_", detectExtExec("private/extensions/php/php.exe", "php"))
		dto.SetThreadVarRaw("_Python_", detectExtExec("private/extensions/python/python.exe", "python"))
	}

	// 用主上下文控制整个进程生命周期
	ctx, cancel := context.WithCancel(context.Background())
	defer extloader.CloseAll() // 退出时统一关闭扩展动态库
	defer ShutdownPhp()        // 确保退出时同步 kill PHP 进程
	defer ShutdownPython()     // 确保退出时同步 kill Python 进程
	defer cancel()             // 最后注册最先执行：先取消上下文，再关闭 PHP/Python

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
		// 检查路径是否存在
		if _, err := os.Stat(fq.FileName); err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, fmt.Errorf("检查路径失败: %v", err)
		}

		// 获取绝对路径
		absPath, err := filepath.Abs(fq.FileName)
		if err != nil {
			return false, fmt.Errorf("获取绝对路径失败: %v", err)
		}

		// 使用 Windows Shell API 将文件/文件夹移动到回收站
		if err := trash.Throw(absPath); err != nil {
			return false, fmt.Errorf("移动到回收站失败: %v", err)
		}

		return true, nil
	})

	funcs.Register("PHP", "1|2|3", func(d *dto.DicInputs) (any, error) {
		phpCode := d.Inputs.String(1)

		// 可选参数：*http.Request（第2个）与 http.ResponseWriter（第3个）
		var req *http.Request
		if r, ok := d.Inputs.Get(2).(*http.Request); ok {
			req = r
		}
		var w http.ResponseWriter
		if rw, ok := d.Inputs.Get(3).(http.ResponseWriter); ok {
			w = rw
		}

		// 无请求，直接执行
		if req == nil {
			return runTempPHP(ctx, phpCode, nil, nil, nil, nil)
		}

		getData, postData, fileData, cleanup, err := parseRequestToMap(req)
		if cleanup != nil {
			defer cleanup()
		}
		if err != nil {
			return "请求解析失败: " + err.Error(), nil
		}

		result, err := runTempPHP(ctx, phpCode, &getData, &postData, &fileData, w)
		if err != nil {
			return "执行失败: " + err.Error(), nil
		}
		return result, nil
	})

	// 注入自定义函数
	funcs.Register("Python", "1|2", func(d *dto.DicInputs) (any, error) {
		output, err := runPythonCode(d, d.Inputs.String(1), d.Inputs.Bool(2))
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
		if dto.FuncServers.Len() == 0 {
			fmt.Println("主程序退出")
			return
		}
		<-ctx.Done()
		fmt.Println("主程序退出")
		return
	}

	switch args[1] {
	case "--autostart":
		// 启动
		dic.Start()
		if dto.FuncServers.Len() == 0 {
			fmt.Println("主程序退出")
			return
		}
		<-ctx.Done()
		fmt.Println("主程序退出")
		return
	case "-help":
		fmt.Println("-help               		（显示帮助）")
		fmt.Println("-v                  		（显示版本）")
		fmt.Println("-autostart          		（开机自启）")
		fmt.Println("-noautostart        		（取消开机自启）")
		fmt.Println("-run <文件>         		（执行指定词库文件）")
		fmt.Println("-check <文件>       		（预编译检测，输出警告与报错）")
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
	case "-run":
		if argsLen < 3 {
			fmt.Println("用法：-run <词库文件路径>")
			return
		}
		res, err := dic.RunFile(args[2])
		if err != nil {
			fmt.Println("执行失败:", err)
			return
		}
		if res != "" {
			fmt.Println(res)
		}
		return
	case "-check":
		if argsLen < 3 {
			fmt.Println("用法：-check <词库文件路径>")
			return
		}
		warns, errs, err := dic.CheckFile(args[2])
		if err != nil {
			fmt.Println("检测失败:", err)
			return
		}
		if len(errs) == 0 && len(warns) == 0 {
			fmt.Println("检测通过：无警告，无报错")
			return
		}
		for _, e := range errs {
			fmt.Printf("[错误] 第 %d 行：%s\n", e.Line, e.Text)
		}
		for _, w := range warns {
			fmt.Printf("[警告] 第 %d 行：%s\n", w.Line, w.Text)
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
