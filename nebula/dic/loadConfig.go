//go:build !js

package dic

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/bot/secludedbot"
	"github.com/cjxpj/nebula/debugLog"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"
	dic_server "github.com/cjxpj/nebula/server"
	"github.com/cjxpj/nebula/utils"
	ini "gopkg.in/ini.v1"
)

func Start() string {

	// 启动时清理超过保留天数的旧日志文件
	dic_server.ClearOldServerLogs()

	// 启动时清空词库编译缓存（进程内加速，重启后重建）
	run.ClearDicCache()

	file := utils.NewFile()

	loadConfig()

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
	infoDic, err := dic_dto.NewDicFile("private/system/start.n")
	if err != nil {
		utils.ErrorStop("启动词库不存在")
	}
	infoDic.SetGlobal_v(GV)

	res := dic_server.Start(dto.ServerConfig.Primary().Http.Addr)
	// 遍历res，收集最后一个非空返回值作为启动页
	var startupResult string
	for _, t := range res {
		var dicRes string
		if t.Event != "" {
			dicRes = dic_api.Api.DicRunEvent(infoDic, t.Event, t.Trigger)
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

	file.SetPath(dto.CONFIG_SYSTEM_PATH)
	if !file.FileExists() {
		if data, err := appfiles.GetFile("dic/system/system.ini"); err == nil {
			file.WriteFileByte(data)
		} else {
			fmt.Println("embed err:", err)
		}
	}

	httpData, err := file.LoadIni()
	if err != nil {
		utils.ErrorStop("系统配置不存在")
	}

	HTTP_Config := httpData.Section("HTTP")

	// 全局共享配置：调试 / 临时读写清理周期（只读 [HTTP] 段）
	dto.ServerConfig.Debug = HTTP_Config.Key("调试").MustBool(false)
	dto.ServerConfig.TempCleanupInterval = HTTP_Config.Key("临时读写清理周期").MustInt(60)
	// 启动时同步全局调试开关，控制词库缓存等调试信息打印
	debugLog.SetDebug(dto.ServerConfig.Debug)

	// 多开 HTTP 服务器：遍历 [HTTP]、[HTTP2]、[HTTP3]... 段，每台服务器独立配置跨域/路由词库/映射目录
	routers := make([]*dto.ServerHTTP, 0)
	for _, sec := range httpData.Sections() {
		name := sec.Name()
		if name != "HTTP" && !strings.HasPrefix(name, "HTTP") {
			continue
		}
		routers = append(routers, newServerRouter(sec))
	}
	if len(routers) == 0 {
		routers = append(routers, newServerRouter(HTTP_Config))
	}
	dto.ServerConfig.Routers = routers

	opUi := httpData.Section("管理面板")
	if ok, _ := opUi.Key("启用").Bool(); ok {
		dto.ServerConfig.OPUI = &dto.OPUI{
			Addr:   "/" + opUi.Key("访问路径").String(),
			Secret: opUi.Key("密钥").String(),
			Cors:   opUi.Key("跨域").MustBool(false),
		}
	}

	// 内置云工具服务端
	cloudToolCfg := httpData.Section("云工具服务端")
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

	file.SetPath(dto.CONFIG_PATH)
	if !file.FileExists() {
		if data, err := appfiles.GetFile("dic/system/config.ini"); err == nil {
			file.WriteFileByte(data)
		} else {
			fmt.Println("embed err:", err)
		}
	}

	botData, err := file.LoadIni()
	if err != nil {
		utils.ErrorStop("对接配置不存在")
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

	// 恢复路径为 config.ini，防止后续 SaveIni 写入错误文件
	file.SetPath(dto.CONFIG_PATH)

	Ngrok_Config := httpData.Section("Ngrok")
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

	WebSocket_Config := httpData.Section("WebSocket")
	dto.LoadConfig_websocket(WebSocket_Config)

	// 加载所有QQ机器人实例（支持多开）
	for _, sec := range botData.Sections() {
		secName := sec.Name()
		if secName == "QQ" || strings.HasPrefix(secName, "QQ") {
			dto.LoadConfig_qq(botData.Section(secName), secName)
		}
	}

	NapCat_Config := botData.Section("NapCat")
	dto.LoadConfig_napcat(NapCat_Config)

	YunHu_Config := botData.Section("云湖")
	dto.LoadConfig_yunhu(YunHu_Config)

	FeiShu_Config := botData.Section("飞书")
	dto.LoadConfig_feishu(FeiShu_Config)

	Secluded_Config := botData.Section("Secluded")
	dto.LoadConfig_secluded(Secluded_Config)
	if dto.ServerConfig.SecludedBot != nil && dto.ServerConfig.SecludedBot.Open {
		secludedbot.Start(dto.ServerConfig.SecludedBot.Addr, dto.ServerConfig.SecludedBot.Token)
	}

	// 启动时逐台连接 BeerWebFrp（每个服务器独立穿透）
	for _, router := range dto.ServerConfig.Routers {
		if router != nil && router.FrpOpen {
			dic_server.StartServerFrp(router)
		}
	}

	// 启动时检查 FTP 是否启用，若启用则自动启动
	FTP_Config := httpData.Section("FTP")
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
			file.SaveIni(httpData)
		}

		dic_server.StartFtp(port, debug, FTP_Config.Key("用户名").String(), FTP_Config.Key("密码").String(), FTP_Config.Key("TLS").MustBool(false), FTP_Config.Key("PASV端口起始").MustInt(32000), FTP_Config.Key("PASV端口结束").MustInt(32005))
	}

	// 启动时检查 SFTP 是否启用，若启用则自动启动
	SFTP_Config := httpData.Section("SFTP")
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
			file.SaveIni(httpData)
		}

		dic_server.StartSftp(port, debug, SFTP_Config.Key("用户名").String(), SFTP_Config.Key("密码").String())
	}

	// 启动时恢复云工具调试开关，并用上次登录持久化的账号与 token 自动连接
	dic_server.StartCloudTool()

	// 启动内置云工具服务端
	dic_server.StartCloudToolServer()

}

// splitDomains 将换行/逗号分隔的域名拆分为去空白、去空项后的列表
func splitDomains(s string) []string {
	out := make([]string, 0)
	for _, line := range strings.Split(s, "\n") {
		for _, d := range strings.Split(line, ",") {
			if d = strings.TrimSpace(d); d != "" {
				out = append(out, d)
			}
		}
	}
	return out
}

// newServerRouter 根据 ini 段构建一个 HTTP 服务器（含独立 handler）
func newServerRouter(sec *ini.Section) *dto.ServerHTTP {
	tlsOk, _ := sec.Key("TLS").Bool()

	webRoot := sec.Key("映射目录").String()
	if webRoot == "" {
		webRoot = dto.DefaultWebRoot
	}
	routerFile := sec.Key("路由词库").String()
	if routerFile == "" {
		routerFile = dto.DefaultRouterFile
	}
	cors, _ := sec.Key("跨域").Bool()

	// BeerWebFrp 穿透（每个服务器独立），地址统一规范为 ws/wss
	frpAddr := strings.TrimSpace(sec.Key("Frp服务端地址").String())
	if after, ok := strings.CutPrefix(frpAddr, "https://"); ok {
		frpAddr = "wss://" + after
	} else if after, ok := strings.CutPrefix(frpAddr, "http://"); ok {
		frpAddr = "ws://" + after
	}

	router := &dto.ServerHTTP{
		Http: &http.Server{
			Addr: sec.Key("server").String(),
		},
		Enabled:      sec.Key("启用").MustBool(true),
		Domains:      splitDomains(sec.Key("绑定域名").String()),
		WebRoot:      webRoot,
		RouterFile:   routerFile,
		Cors:         cors,
		CorsOrigins:  sec.Key("跨域白名单").String(),
		TLS:          tlsOk,
		CertFile:     sec.Key("TLS证书文件").String(),
		KeyFile:      sec.Key("TLS密钥文件").String(),
		TLSMode:      sec.Key("TLS方式").MustString("file"),
		TLSDomains:   sec.Key("TLS域名").String(),
		TLSEmail:     sec.Key("TLS邮箱").String(),
		FrpOpen:      sec.Key("Frp启用").MustBool(false),
		FrpServerAddr: frpAddr,
		FrpToken:     sec.Key("Frp令牌").String(),
		FrpDebug:     sec.Key("Frp调试").MustBool(false),
	}
	if dto.WebHandlerFactory != nil {
		router.Http.Handler = dto.WebHandlerFactory(router)
	}
	return router
}
