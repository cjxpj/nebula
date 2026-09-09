//go:build !js && !dll

package dic

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/bot/secludedbot"
	"github.com/cjxpj/nebula/debugLog"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/extloader"
	"github.com/cjxpj/nebula/run"
	dic_server "github.com/cjxpj/nebula/server"
	"github.com/cjxpj/nebula/utils"
)

func Start() string {
	// 启动阶段：日志与编译缓存不落盘，避免启动时自动创建 database/log、private/.dic_cache 目录
	utils.SetStartupMode(true)
	defer utils.SetStartupMode(false)

	// 桌面端以程序(exe)所在目录为初始工作目录，便于 start.n 用相对路径设置数据目录。
	if utils.GetAppDir() == "" {
		if exe, err := os.Executable(); err == nil {
			_ = os.Chdir(filepath.Dir(exe))
		}
	}

	startPath := startDicPath()
	routerPath := routerDicPath()
	terminalPath := terminalDicPath()
	ensureStartDic(startPath)

	// 注册一次性配置加载：首次调用「服务器.设置核心服务器」时触发
	dto.SetConfigLoader(func() {
		// 数据目录确定后再清理旧日志，避免在错误目录下创建/扫描 log 目录
		dic_server.ClearOldServerLogs()

		loadConfig()

		// 词库编译缓存默认关闭；关闭时清空磁盘缓存目录，避免旧缓存累积
		if !dto.ServerConfig.DicCache {
			run.ClearDicCache()
		}

		// 启动时加载扩展动态库（private/plugins 目录下的 .dll/.so）并注册其字典函数，供 $函数名$ 调用
		extloader.InitAll()

		// 数据目录确定后，再检测并注入 ffmpeg/python/php/silk 等扩展可执行文件路径
		if SetupExtensionPaths != nil {
			SetupExtensionPaths()
		}
	})

	GV := dto.NewVal()
	GV.Set("版本", appfiles.Version)
	// 终端词库路径注入系统全局变量，供 Go 侧读取（start.n 通过 $线程变量$ 自行设置）
	dto.SetThreadVarRaw("_终端词库路径_", terminalPath)
	infoDic, err := dic_dto.NewDicFile(startPath)
	if err != nil {
		utils.ErrorStop("启动词库不存在")
	}
	infoDic.SetGlobal_v(GV)

	// 运行启动词库：头部（$设置工作目录$）先确定数据目录，正文创建服务器并设置核心服务器，
	// 首次「服务器.设置核心服务器」会触发一次性配置加载。
	if mainRes := dic_api.Api.DicRun(infoDic, "Main"); mainRes != "" {
		fmt.Printf("%v\n", debugLog.EscapeControlChars(mainRes))
	}

	// 配置加载完成后，再编译路由词库并处理特殊事件（[系统]手机首页 / [Ngrok]启动 / [BeerWebFrp]启动）
	ensureRouterDic(routerPath)

	routerGV := dto.NewVal()
	routerGV.Set("版本", appfiles.Version)
	routerDic, routerErr := dic_dto.NewDicFile(routerPath)
	if routerErr != nil {
		utils.ErrorStop("路由词库不存在")
	}
	routerDic.SetGlobal_v(routerGV)

	res := dic_server.Start()
	// 遍历res，收集最后一个非空返回值作为启动页
	var startupResult string
	for _, t := range res {
		if t.Event == "" {
			// Main 已在上方执行，跳过
			continue
		}
		dicRes := dic_api.Api.DicRunEvent(routerDic, t.Event, t.Trigger)
		if dicRes != "" {
			fmt.Printf("%v\n", debugLog.EscapeControlChars(dicRes))
			startupResult = dicRes
		}
	}
	return startupResult
	// fmt.Println("启动成功，耗时：", time.Since(start))
}

// SetupExtensionPaths 由平台 main 注入：在确定数据目录后设置 ffmpeg/python/php/silk 等扩展路径。
var SetupExtensionPaths func()

// startDicPath 返回启动词库路径。
// 桌面端使用程序(exe)目录下的 start.n；移动端使用应用主目录（Android 为 Documents/Nebula，鸿蒙为注入沙箱目录）下的 start.n。
func startDicPath() string {
	if utils.GetAppDir() != "" {
		return "start.n"
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "start.n")
	}
	return "start.n"
}

// ensureStartDic 确保启动词库存在，缺失时从内嵌模板写入。
func ensureStartDic(startPath string) {
	f := utils.NewFileQueue(startPath)
	if f.FileExists() {
		return
	}
	if data, err := appfiles.GetFile("dic/system/start.n"); err == nil {
		f.WriteFileByte(data)
	} else {
		fmt.Println("embed err:", err)
	}
}

// routerDicPath 返回路由词库路径（数据目录下 private/system/router.n）。
func routerDicPath() string {
	return "private/system/router.n"
}

// terminalDicPath 返回终端词库路径（数据目录下 private/system/terminal.n）。
func terminalDicPath() string {
	return "private/system/terminal.n"
}

// ensureRouterDic 确保路由词库存在，缺失时从内嵌模板写入。
func ensureRouterDic(routerPath string) {
	f := utils.NewFileQueue(routerPath)
	if f.FileExists() {
		return
	}
	if data, err := appfiles.GetFile("dic/system/router.n"); err == nil {
		f.WriteFileByte(data)
	} else {
		fmt.Println("embed err:", err)
	}
}

func loadConfig() {

	// 加载 IP 黑名单
	dic_server.LoadIPBlacklist()

	file := utils.NewFile()

	file.SetPath("private/ttf/font.ttf")
	if !file.FileExists() {
		if data, err := appfiles.GetFile("font.ttf.gz"); err == nil {
			// gzip 解压字体
			gr, err := gzip.NewReader(bytes.NewReader(data))
			if err != nil {
				fmt.Println("gzip decompress font err:", err)
			} else {
				decompressed, err := io.ReadAll(gr)
				gr.Close()
				if err != nil {
					fmt.Println("gzip read font err:", err)
				} else {
					file.WriteFileByte(decompressed)
				}
			}
		} else {
			fmt.Println("embed err:", err)
		}
	}

	// 迁移旧的 system.ini / config.ini 到合并后的 config.yaml（幂等，仅首次执行）
	dto.MigrateIniToYaml()

	file.SetPath(dto.CONFIG_PATH)
	if !file.FileExists() {
		if data, err := appfiles.GetFile("dic/system/config.yaml"); err == nil {
			file.WriteFileByte(data)
		} else {
			fmt.Println("embed err:", err)
		}
	}

	cfg, err := dto.LoadConfigFile()
	if err != nil {
		utils.ErrorStop("配置文件不存在")
	}

	HTTP_Config := cfg.Section("HTTP")

	// 全局共享配置：调试 / 临时读写清理周期 / 词库编译缓存（只读 [HTTP] 段）
	dto.ServerConfig.Debug = HTTP_Config.Key("调试").MustBool(false)
	dto.ServerConfig.TempCleanupInterval = HTTP_Config.Key("临时读写清理周期").MustInt(60)
	dto.ServerConfig.DicCache = HTTP_Config.Key("词库编译缓存").MustBool(false)
	// 启动时同步全局调试开关，控制词库缓存等调试信息打印
	debugLog.SetDebug(dto.ServerConfig.Debug)

	opUi := cfg.Section("管理面板")
	if ok, _ := opUi.Key("启用").Bool(); ok {
		dto.ServerConfig.OPUI = &dto.OPUI{
			Addr: "/" + opUi.Key("访问路径").String(),
			Cors: opUi.Key("跨域").MustBool(false),
		}
	}

	// 仅在设置核心服务器（首次配置加载）时，检测管理面板是否已初始化登录密码；
	// 未初始化则自动生成随机初始密码并写入线程变量，交由启动词库打印，供首次登录使用。
	if pwd, ok := dic_server.EnsureOpuiInitialPassword(); ok {
		dto.SetThreadVarRaw("_管理面板初始密码_", pwd)
	}

	// 生成快捷登录码（每次启动刷新），明文保存在内存供 WebUI 地址拼接 ?key= 快捷登录。
	dic_server.EnsureOpuiQuickToken()

	// 内置云工具服务端
	cloudToolCfg := cfg.Section("云工具服务端")
	if ok, _ := cloudToolCfg.Key("启用").Bool(); ok {
		allowRegister := cloudToolCfg.Key("任意账号注册").MustBool(true)
		addr := strings.TrimSpace(cloudToolCfg.Key("访问路径").String())
		if addr == "" {
			addr = "cloudtool"
		}
		if !strings.HasPrefix(addr, "/") {
			addr = "/" + addr
		}
		dicDir := strings.TrimSpace(cloudToolCfg.Key("词库目录").String())
		if dicDir == "" {
			dicDir = "cloudtool"
		}
		dto.ServerConfig.CloudTool = &dto.CloudTool{
			Open:          true,
			Addr:          addr,
			AllowRegister: allowRegister,
			Whitelist:     dic_server.CloudToolSplitWhitelist(cloudToolCfg.Key("白名单").String()),
			DicDir:        dicDir,
			// 断开自动注销时长（秒），默认 30
			LogoutSec: cloudToolCfg.Key("断开注销时长").MustInt(30),
			Debug:     cloudToolCfg.Key("调试").MustBool(false),
		}
	}

	// 路由词库
	file.SetPath(routerDicPath())
	if !file.FileExists() {
		if data, err := appfiles.GetFile("dic/system/router.n"); err == nil {
			file.WriteFileByte(data)
		} else {
			fmt.Println("embed err:", err)
		}

		// 主页文件
		file.SetPath("public")
		if !file.DirExists() {
			// 默认主页
			file.SetPath("public/index.wn")
			if s, err := appfiles.GetFileString("dic/public/index.wn"); err == nil {
				file.WriteToFile(s)
			} else {
				fmt.Println("embed err:", err)
			}
			// 默认图标
			file.SetPath("public/favicon.ico")
			if data, err := appfiles.GetFile("dic/public/favicon.ico"); err == nil {
				file.WriteFileByte(data)
			} else {
				fmt.Println("embed err:", err)
			}
			// 默认样板文件
			file.SetPath("public/api.n")
			if s, err := appfiles.GetFileString("dic/public/api.n"); err == nil {
				file.WriteToFile(s)
			} else {
				fmt.Println("embed err:", err)
			}
			// 404文件
			file.SetPath("public/404.wn")
			if data, err := appfiles.GetFile("dic/public/404.wn"); err == nil {
				file.WriteFileByte(data)
			} else {
				fmt.Println("embed err:", err)
			}
		}
	}

	// 专门监听终端触发的词库
	file.SetPath(terminalDicPath())
	if !file.FileExists() {
		if data, err := appfiles.GetFile("dic/system/terminal.n"); err == nil {
			file.WriteFileByte(data)
		} else {
			fmt.Println("embed err:", err)
		}
	}

	Ngrok_Config := cfg.Section("Ngrok")
	if ok, _ := Ngrok_Config.Key("启用").Bool(); ok {
		ngrokUrl := Ngrok_Config.Key("访问链接").String()
		authToken := Ngrok_Config.Key("密钥").String()
		serverAddr := Ngrok_Config.Key("服务器").String()
		dto.ServerConfig.Ngrok = &dto.NgrokConfig{
			Addr:       ngrokUrl,
			Token:      authToken,
			ServerAddr: serverAddr,
		}
	}

	WebSocket_Config := cfg.Section("WebSocket")
	dto.LoadConfig_websocket(WebSocket_Config)

	// 加载所有QQ机器人实例（支持多开）
	for _, secName := range cfg.Sections() {
		if secName == "QQ" || strings.HasPrefix(secName, "QQ") {
			dto.LoadConfig_qq(cfg.Section(secName), secName)
		}
	}

	NapCat_Config := cfg.Section("NapCat")
	dto.LoadConfig_napcat(NapCat_Config)

	YunHu_Config := cfg.Section("云湖")
	dto.LoadConfig_yunhu(YunHu_Config)

	FeiShu_Config := cfg.Section("飞书")
	dto.LoadConfig_feishu(FeiShu_Config)

	Secluded_Config := cfg.Section("Secluded")
	dto.LoadConfig_secluded(Secluded_Config)
	if dto.ServerConfig.SecludedBot != nil && dto.ServerConfig.SecludedBot.Open {
		secludedbot.Start(dto.ServerConfig.SecludedBot.Addr, dto.ServerConfig.SecludedBot.Token)
	}

	// 启动时检查 FTP 是否启用，若启用则自动启动
	FTP_Config := cfg.Section("FTP")
	if ok, _ := FTP_Config.Key("启用").Bool(); ok {
		port := FTP_Config.Key("端口").MustInt(21)
		debug := FTP_Config.Key("调试").MustBool(false)

		// 初始化默认账户和随机密码
		needSave := false
		if FTP_Config.Key("用户名").String() == "" {
			FTP_Config.Key("用户名").SetValue("admin")
			needSave = true
		}
		if FTP_Config.Key("密码").String() == "" {
			pwd := utils.RandomString("大小字母", 8)
			FTP_Config.Key("密码").SetValue(pwd)
			needSave = true
		}
		if needSave {
			cfg.Save()
		}

		dic_server.StartFtp(port, debug, FTP_Config.Key("用户名").String(), FTP_Config.Key("密码").String(), FTP_Config.Key("TLS").MustBool(false), FTP_Config.Key("PASV端口起始").MustInt(32000), FTP_Config.Key("PASV端口结束").MustInt(32005))
	}

	// 启动时检查 SFTP 是否启用，若启用则自动启动
	SFTP_Config := cfg.Section("SFTP")
	if ok, _ := SFTP_Config.Key("启用").Bool(); ok {
		port := SFTP_Config.Key("端口").MustInt(22)
		debug := SFTP_Config.Key("调试").MustBool(false)

		// 初始化默认账户和随机密码
		needSave := false
		if SFTP_Config.Key("用户名").String() == "" {
			SFTP_Config.Key("用户名").SetValue("root")
			needSave = true
		}
		if SFTP_Config.Key("密码").String() == "" {
			pwd := utils.RandomString("大小字母", 8)
			SFTP_Config.Key("密码").SetValue(pwd)
			needSave = true
		}
		if needSave {
			cfg.Save()
		}

		dic_server.StartSftp(port, debug, SFTP_Config.Key("用户名").String(), SFTP_Config.Key("密码").String())
	}

	// 启动时恢复云工具调试开关，并用上次登录持久化的账号与 token 自动连接
	dic_server.StartCloudTool()

	// 启动内置云工具服务端
	dic_server.StartCloudToolServer()

}
