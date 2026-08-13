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
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	dic_server "github.com/cjxpj/nebula/server"
	"github.com/cjxpj/nebula/utils"
)

func Start() string {

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

	// start := time.Now()
	FileData, err := file.ReadFromFile()
	if err != nil {
		utils.ErrorStop("启动词库不存在")
	}

	GV := dto.NewVal()
	GV.Set("版本", appfiles.Version)
	infoDic := dic_dto.NewDic("private/system/start.n", FileData)
	infoDic.SetGlobal_v(GV)

	res := dic_server.Start(dto.ServerConfig.Router.Http.Addr)
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
			fmt.Printf("%v\n", dicRes)
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
	infoServerPath := HTTP_Config.Key("server").String()
	corsOk, _ := HTTP_Config.Key("跨域").Bool()
	tlsOk, _ := HTTP_Config.Key("TLS").Bool()
	dto.ServerConfig.Router = &dto.ServerHTTP{
		Http: &http.Server{
			Addr:    infoServerPath,
			Handler: http.HandlerFunc(webRun),
		},
		Cors:                corsOk,
		CorsOrigins:         HTTP_Config.Key("跨域白名单").String(),
		TempCleanupInterval: HTTP_Config.Key("临时读写清理周期").MustInt(60),
		TLS:                 tlsOk,
		CertFile:            HTTP_Config.Key("TLS证书文件").String(),
		KeyFile:             HTTP_Config.Key("TLS密钥文件").String(),
	}

	opUi := httpData.Section("管理面板")
	if ok, _ := opUi.Key("启用").Bool(); ok {
		path := opUi.Key("访问路径").String()
		if path == "nebula" {
			fmt.Println("管理面板的密码忘记可以去配置文件看或者自己改，请不要泄漏导致服务器被攻击！")
			path = fmt.Sprintf("%s/%s", path, utils.RandomString("大小字母", 12))
			opUi.Key("访问路径").SetValue(path)
			file.SaveIni(httpData)
		}
		dto.ServerConfig.OPUI = &dto.OPUI{
			Addr:   "/" + path,
			Secret: opUi.Key("密钥").String(),
			Cors:   opUi.Key("跨域").MustBool(false),
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

	// 恢复路径为 config.ini，防止后续 SaveIni 写入错误文件
	file.SetPath(dto.CONFIG_PATH)

	Ngrok_Config := httpData.Section("Ngrok")
	if ok, _ := Ngrok_Config.Key("启用").Bool(); ok {
		ngrokUrl := Ngrok_Config.Key("访问链接").String()
		authToken := Ngrok_Config.Key("密钥").String()
		dto.ServerConfig.Ngrok = &dto.NgrokConfig{
			Addr:  ngrokUrl,
			Token: authToken,
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

	// 启动时检查 FRP 是否启用，若启用则自动连接
	FRP_Config := httpData.Section("FRP")
	if ok, _ := FRP_Config.Key("启用").Bool(); ok {
		serverAddr := strings.TrimSpace(FRP_Config.Key("服务端地址").String())
		token := strings.TrimSpace(FRP_Config.Key("令牌").String())
		if debug := FRP_Config.Key("调试").MustBool(false); debug {
			dic_server.SetFrpDebug(true)
		}
		dic_server.ConnectFrp(serverAddr, token)
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

}
