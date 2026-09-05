//go:build !js && !dll

package dic

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strings"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/bot/secludedbot"
	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/extloader"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"
	dic_server "github.com/cjxpj/nebula/server"
	"github.com/cjxpj/nebula/utils"
)

func Start() string {

	// 启动时清理超过保留天数的旧日志文件
	dic_server.ClearOldServerLogs()

	file := utils.NewFile()

	loadConfig()

	// 词库编译缓存默认关闭；关闭时清空磁盘缓存目录，避免旧缓存累积
	if !dto.ServerConfig.DicCache {
		run.ClearDicCache()
	}

	// 启动时加载扩展动态库（private/plugins 目录下的 .dll/.so）并注册其字典函数，供 $函数名$ 调用
	extloader.InitAll()

	file.SetPath("private/system/start.n")
	if !file.FileExists() {
		if data, err := appfiles.GetFile("dic/system/start.n"); err == nil {
			file.WriteFileByte(data)
		} else {
			fmt.Println("embed err:", err)
		}
	}

	GV := dto.NewVal()
	GV.Set("版本", appfiles.Version)
	// 注入词库路径，便于启动词库内函数报错时显示来源（顶层词库默认没有 _词库路径_）
	GV.Set("_词库路径_", "private/system/start.n")
	infoDic, err := dic_dto.NewDicFile("private/system/start.n")
	if err != nil {
		utils.ErrorStop("启动词库不存在")
	}
	infoDic.SetGlobal_v(GV)

	// 路由词库承载事件触发（[系统]首页 / [Ngrok]启动 / [BeerWebFrp]启动）
	routerGV := dto.NewVal()
	routerGV.Set("版本", appfiles.Version)
	routerGV.Set("_词库路径_", "private/system/router.n")
	routerDic, routerErr := dic_dto.NewDicFile("private/system/router.n")
	if routerErr != nil {
		utils.ErrorStop("路由词库不存在")
	}
	routerDic.SetGlobal_v(routerGV)

	res := dic_server.Start()
	// 遍历res，收集最后一个非空返回值作为启动页
	var startupResult string
	for _, t := range res {
		var dicRes string
		if t.Event != "" {
			dicRes = dic_api.Api.DicRunEvent(routerDic, t.Event, t.Trigger)
		} else {
			dicRes = dic_api.Api.DicRun(infoDic, t.Trigger)
		}
		if dicRes != "" {
			fmt.Printf("%v\n", debugLog.EscapeControlChars(dicRes))
			startupResult = dicRes
		}
	}
	return startupResult
	// fmt.Println("启动成功，耗时：", time.Since(start))
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
			Addr:   "/" + opUi.Key("访问路径").String(),
			Secret: opUi.Key("密钥").String(),
			Cors:   opUi.Key("跨域").MustBool(false),
		}
	}

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
	file.SetPath("private/system/router.n")
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
	file.SetPath("private/system/terminal.n")
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

