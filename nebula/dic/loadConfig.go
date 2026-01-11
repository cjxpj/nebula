package dic

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cjxpj/nebula/appfiles"
	feishubot_msg "github.com/cjxpj/nebula/bot/feishubot/msg"
	napcatbot_dto "github.com/cjxpj/nebula/bot/napcatbot/dto"
	qqbot_msg "github.com/cjxpj/nebula/bot/qqbot/msg"
	yunhubot_dto "github.com/cjxpj/nebula/bot/yunhubot/dto"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	dic_server "github.com/cjxpj/nebula/server"
	"github.com/cjxpj/nebula/utils"
	lark "github.com/larksuite/oapi-sdk-go/v3"

	"github.com/gorilla/websocket"
	"github.com/patrickmn/go-cache"
)

func Start() {

	file := utils.NewFile()
	file.SetPath("README.md").WriteFileByte(appfiles.GetFile("dic.md"))

	loadConfig()

	file.SetPath("private/system/start.n")
	if !file.FileExists() {
		file.WriteFileByte(appfiles.GetFile("dic/system/start.n"))
	}

	FileData, err := file.ReadFromFile()
	if err != nil {
		utils.ErrorStop("启动词库不存在")
	}

	GV := dto.NewVal()
	GV.Set("版本", appfiles.Version)
	infoDic := dic_dto.NewDic("private/system/start.n", FileData)
	infoDic.SetGlobal_v(GV)

	res := dic_server.Start(dto.ServerConfig.Router.Http.Addr)
	// 遍历res
	for _, t := range res {
		if dicRes := dic_api.Api.DicRun(infoDic, t); dicRes != "" {
			fmt.Println(dicRes)
		}
	}
}

func loadConfig() {

	file := utils.NewFile()

	file.SetPath("private/ttf/font.ttf")
	if !file.FileExists() {
		file.WriteFileByte(appfiles.GetFile("font.ttf"))
	}

	file.SetPath(dto.DIC_CONFIG_PATH)
	if !file.FileExists() {
		file.WriteFileByte(appfiles.GetFile("dic/system/config.ini"))
	}

	httpData, err := file.LoadIni()
	if err != nil {
		utils.ErrorStop("系统配置不存在")
	}

	HTTP_Config := httpData.Section("HTTP")
	infoServerPath := HTTP_Config.Key("server").String()
	corsOk, _ := HTTP_Config.Key("跨域").Bool()
	dto.ServerConfig.Router = &dto.ServerHTTP{
		Cors: corsOk,
		Http: &http.Server{
			Addr:    infoServerPath,
			Handler: http.HandlerFunc(webRun),
		},
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
			Addr: "/" + path,
		}
	}

	file.SetPath(dto.BOT_CONFIG_PATH)
	if !file.FileExists() {
		file.WriteFileByte(appfiles.GetFile("dic/system/bot_config.ini"))
	}

	botData, err := file.LoadIni()
	if err != nil {
		utils.ErrorStop("对接配置不存在")
	}

	// 路由词库
	file.SetPath("private/system/router.n")
	if !file.FileExists() {
		file.WriteFileByte(appfiles.GetFile("dic/system/router.n"))

		// WS词库
		file.SetPath("private/websocket")
		if !file.DirExists() {
			// 服务器
			file.SetPath("private/websocket/server.n")
			if !file.FileExists() {
				file.WriteToFile(appfiles.GetFileString("dic/websocket/server.n"))
			}
			// 客户端
			file.SetPath("private/websocket/app.n")
			if !file.FileExists() {
				file.WriteToFile(appfiles.GetFileString("dic/websocket/app.n"))
			}
		}

		// 主页文件
		file.SetPath("public")
		if !file.DirExists() {
			file.SetPath("public/index.wn")
			if !file.FileExists() {
				file.WriteToFile(appfiles.GetFileString("dic/public/index.wn"))
			}

			// 默认样板文件
			file.SetPath("public/api.n")
			if !file.FileExists() {
				file.WriteToFile(appfiles.GetFileString("dic/public/api.n"))
			}
			// 404文件
			file.SetPath("public/404.wn")
			if !file.FileExists() {
				file.WriteFileByte(appfiles.GetFile("dic/public/404.wn"))
			}
		}

	}

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
	if ok, _ := WebSocket_Config.Key("启用").Bool(); ok {
		corsOk, _ := WebSocket_Config.Key("跨域").Bool()
		wsPath := "/" + WebSocket_Config.Key("访问路径").String()
		wsConn := &websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// 跨域连接
				return corsOk
			},
		}
		dto.ServerConfig.Ws = &dto.ServerRouterWebSocket{
			Open: true,
			Addr: wsPath,
			Conn: wsConn,
		}
	}

	QQBot_Config := botData.Section("QQ")
	if ok, _ := QQBot_Config.Key("启用").Bool(); ok {
		appId := QQBot_Config.Key("APPID").String()
		secret := QQBot_Config.Key("密钥").String()
		dicPath := QQBot_Config.Key("词库").String()
		dto.ServerConfig.QQBot = &qqbot_msg.RouterQQBot{
			// 缓存 50 秒，3 分钟内没有访问就删除
			LastMsg:  cache.New(50*time.Second, 3*time.Minute),
			Open:     true,
			Addr:     "/" + QQBot_Config.Key("访问路径").String(),
			FilePath: dicPath,
			API:      qqbot_msg.NewQQBot(appId, secret),
		}
		BotDic := utils.NewFileQueue(dicPath)
		if !BotDic.FileExists() {
			BotDic.WriteFileByte(appfiles.GetFile("dic/QQBot.n"))
		}
	}

	NapCat_Config := botData.Section("NapCat")
	if ok, _ := NapCat_Config.Key("启用").Bool(); ok {
		secret := NapCat_Config.Key("密钥").String()
		dicPath := NapCat_Config.Key("词库").String()
		dto.ServerConfig.NapCatBot = &napcatbot_dto.RouterNapCatBot{
			Open:     true,
			APIAddr:  NapCat_Config.Key("发送消息接口").String(),
			Addr:     "/" + NapCat_Config.Key("访问路径").String(),
			Secret:   secret,
			FilePath: dicPath,
		}
		BotDic := utils.NewFileQueue(dicPath)
		if !BotDic.DirExists() {
			BotDic.SetPath(dicPath + "/dic/dic.n")
			BotDic.WriteFileByte(appfiles.GetFile("dic/NapCatBot.n"))
			// 群白名单
			BotDic.SetPath(dicPath + "/groups.txt")
			BotDic.WriteFileByte([]byte("all"))
			// 主人文件
			BotDic.SetPath(dicPath + "/admin.txt")
			BotDic.WriteFileByte([]byte(""))
		}
	}

	YunHu_Config := botData.Section("云湖")
	if ok, _ := YunHu_Config.Key("启用").Bool(); ok {
		secret := YunHu_Config.Key("密钥").String()
		dicPath := YunHu_Config.Key("词库").String()
		dto.ServerConfig.YunHuBot = &yunhubot_dto.RouterYunHuBot{
			Open:     true,
			Addr:     "/" + YunHu_Config.Key("访问路径").String(),
			Secret:   secret,
			FilePath: dicPath,
		}
		BotDic := utils.NewFileQueue(dicPath)
		if !BotDic.FileExists() {
			BotDic.WriteFileByte(appfiles.GetFile("dic/YunHuBot.n"))
		}
	}

	FeiShu_Config := botData.Section("飞书")
	if ok, _ := FeiShu_Config.Key("启用").Bool(); ok {
		appId := FeiShu_Config.Key("APPID").String()
		secret := FeiShu_Config.Key("密钥").String()
		dicPath := FeiShu_Config.Key("词库").String()
		dto.ServerConfig.FeiShuBot = &feishubot_msg.RouterFeishubot{
			Open:     true,
			Addr:     "/" + FeiShu_Config.Key("访问路径").String(),
			API:      lark.NewClient(appId, secret),
			FilePath: dicPath,
		}
		BotDic := utils.NewFileQueue(dicPath)
		if !BotDic.DirExists() {
			BotDic.SetPath(dicPath + "/dic/dic.n")
			BotDic.WriteFileByte(appfiles.GetFile("dic/NapCatBot.n"))
			// 主人文件
			BotDic.SetPath(dicPath + "/admin.txt")
			BotDic.WriteFileByte([]byte(""))
		}
	}

}
