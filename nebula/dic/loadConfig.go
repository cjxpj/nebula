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

	infoDic := loadConfig()

	if res := dic_server.Start(dto.ServerConfig.Http.Addr); res != "" {
		if res := dic_api.Api.DicRun(infoDic, res); res != "" {
			fmt.Println(res)
		}
	}
}

func loadConfig() *dic_dto.Dic {

	file := utils.NewFile()

	file.SetPath("private/ttf/font.ttf")
	if !file.FileExists() {
		file.WriteFileByte(appfiles.GetFile("font.ttf"))
	}

	file.SetPath("private/system/config.n")
	if !file.FileExists() {
		file.WriteFileByte(appfiles.GetFile("dic/config.n"))
	}

	FileData, err := file.ReadFromFile()
	if err != nil {
		utils.ErrorStop("启动配置不存在")
	}

	GV := dto.NewVal()
	GV.Set("版本", appfiles.Version)
	infoDic := dic_dto.NewDic("private/system/config.n", FileData)
	infoDic.SetGlobal_v(GV)

	// fmt.Println("Nebula触发")
	if res := dic_api.Api.DicRun(infoDic, "启动"); res != "" {
		fmt.Println(res)
	}

	// 路由词库
	file.SetPath("private/system/router.n")
	if !file.FileExists() {
		file.WriteFileByte(appfiles.GetFile("dic/router.n"))

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

	if d := dic_api.Api.DicRun(infoDic, "WebSocket"); d == "是" {
		wsPath := "/" + infoDic.Val.P.GetStr("访问路径")
		wsConn := &websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // 允许所有跨域连接
			},
		}
		dto.ServerConfig.Ws = &dto.ServerRouterWebSocket{
			Open: true,
			Addr: wsPath,
			Conn: wsConn,
		}
	}

	if d := dic_api.Api.DicRun(infoDic, "QQBot"); d == "是" {
		appId := infoDic.Val.P.GetStr("APPID")
		secret := infoDic.Val.P.GetStr("密钥")
		dicPath := infoDic.Val.P.GetStr("词库")
		dto.ServerConfig.QQBot = &qqbot_msg.RouterQQBot{
			// 缓存 50 秒，3 分钟内没有访问就删除
			LastMsg:  cache.New(50*time.Second, 3*time.Minute),
			Open:     true,
			Addr:     "/" + infoDic.Val.P.GetStr("访问路径"),
			FilePath: dicPath,
			API:      qqbot_msg.NewQQBot(appId, secret),
		}
		BotDic := utils.NewFileQueue(dicPath)
		if !BotDic.FileExists() {
			BotDic.WriteFileByte(appfiles.GetFile("dic/QQBot.n"))
		}
	}

	if d := dic_api.Api.DicRun(infoDic, "NapCatBot"); d == "是" {
		secret := infoDic.Val.P.GetStr("密钥")
		dicPath := infoDic.Val.P.GetStr("词库")
		dto.ServerConfig.NapCatBot = &napcatbot_dto.RouterNapCatBot{
			Open:     true,
			APIAddr:  infoDic.Val.P.GetStr("发送消息接口"),
			Addr:     "/" + infoDic.Val.P.GetStr("访问路径"),
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

	if d := dic_api.Api.DicRun(infoDic, "YunHuBot"); d == "是" {
		secret := infoDic.Val.P.GetStr("密钥")
		dicPath := infoDic.Val.P.GetStr("词库")
		dto.ServerConfig.YunHuBot = &yunhubot_dto.RouterYunHuBot{
			Open:     true,
			Addr:     "/" + infoDic.Val.P.GetStr("访问路径"),
			Secret:   secret,
			FilePath: dicPath,
		}
		BotDic := utils.NewFileQueue(dicPath)
		if !BotDic.FileExists() {
			BotDic.WriteFileByte(appfiles.GetFile("dic/YunHuBot.n"))
		}
	}

	if d := dic_api.Api.DicRun(infoDic, "FeiShuBot"); d == "是" {
		appId := infoDic.Val.P.GetStr("APPID")
		secret := infoDic.Val.P.GetStr("密钥")
		dicPath := infoDic.Val.P.GetStr("词库")
		dto.ServerConfig.FeiShuBot = &feishubot_msg.RouterFeishubot{
			Open:     true,
			Addr:     "/" + infoDic.Val.P.GetStr("访问路径"),
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

	if infoNgrokServerPath := dic_api.Api.DicRun(infoDic, "Ngrok"); infoNgrokServerPath == "是" {
		authToken := infoDic.Val.P.GetStr("密钥")
		ngrokUrl := infoDic.Val.P.GetStr("访问链接")
		dto.ServerConfig.Ngrok = &dto.NgrokConfig{
			Addr:  ngrokUrl,
			Token: authToken,
		}
	}

	infoServerPath := dic_api.Api.DicRun(infoDic, "监听访问路径")
	dto.ServerConfig.Http = &http.Server{
		Addr:    infoServerPath,
		Handler: http.HandlerFunc(webRun),
	}

	return infoDic

}
