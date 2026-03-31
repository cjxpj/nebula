package dto

import (
	"mime"
	"net/http"
	"time"

	"github.com/cjxpj/nebula/appfiles"
	feishubot_msg "github.com/cjxpj/nebula/bot/feishubot/msg"
	napcatbot_dto "github.com/cjxpj/nebula/bot/napcatbot/dto"
	qqbot_msg "github.com/cjxpj/nebula/bot/qqbot/msg"
	yunhubot_dto "github.com/cjxpj/nebula/bot/yunhubot/dto"
	"github.com/cjxpj/nebula/utils"
	"github.com/gorilla/websocket"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/patrickmn/go-cache"
	"gopkg.in/ini.v1"
)

func init() {
	mime.AddExtensionType(".amr", "audio/amr")
	mime.AddExtensionType(".silk", "audio/silk")
}

const CONFIG_SYSTEM_PATH = "private/system/system.ini"
const CONFIG_PATH = "private/system/config.ini"

func LoadConfig_napcat(NapCat_Config *ini.Section) {
	if ok, _ := NapCat_Config.Key("启用").Bool(); ok {
		secret := NapCat_Config.Key("密钥").String()
		dicPath := NapCat_Config.Key("词库").String()
		ServerConfig.NapCatBot = &napcatbot_dto.RouterNapCatBot{
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
}

func LoadConfig_qq(QQBot_Config *ini.Section) {
	if ok, _ := QQBot_Config.Key("启用").Bool(); ok {
		appId := QQBot_Config.Key("APPID").String()
		secret := QQBot_Config.Key("密钥").String()
		dicPath := QQBot_Config.Key("词库").String()
		ServerConfig.QQBot = &qqbot_msg.RouterQQBot{
			// 缓存 50 秒，3 分钟内没有访问就删除
			LastMsg:  cache.New(50*time.Second, 3*time.Minute),
			Open:     true,
			Addr:     "/" + QQBot_Config.Key("访问路径").String(),
			FilePath: dicPath,
			API:      qqbot_msg.NewQQBot(appId, secret),
		}
		BotDic := utils.NewFileQueue(dicPath)
		if !BotDic.DirExists() {
			BotDic.SetPath(dicPath + "/dic/dic.n")
			BotDic.WriteFileByte(appfiles.GetFile("dic/QQBot.n"))
			// 主人文件
			BotDic.SetPath(dicPath + "/admin.txt")
			BotDic.WriteFileByte([]byte(""))
		}
	}
}

func LoadConfig_yunhu(YunHu_Config *ini.Section) {
	if ok, _ := YunHu_Config.Key("启用").Bool(); ok {
		secret := YunHu_Config.Key("密钥").String()
		dicPath := YunHu_Config.Key("词库").String()
		ServerConfig.YunHuBot = &yunhubot_dto.RouterYunHuBot{
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
}

func LoadConfig_feishu(FeiShu_Config *ini.Section) {
	if ok, _ := FeiShu_Config.Key("启用").Bool(); ok {
		appId := FeiShu_Config.Key("APPID").String()
		secret := FeiShu_Config.Key("密钥").String()
		dicPath := FeiShu_Config.Key("词库").String()
		ServerConfig.FeiShuBot = &feishubot_msg.RouterFeishubot{
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

func LoadConfig_websocket(WebSocket_Config *ini.Section) {
	if ok, _ := WebSocket_Config.Key("启用").Bool(); ok {
		corsOk, _ := WebSocket_Config.Key("跨域").Bool()
		wsPath := "/" + WebSocket_Config.Key("访问路径").String()
		wsConn := &websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// 跨域连接
				return corsOk
			},
		}
		ServerConfig.Ws = &ServerRouterWebSocket{
			Open: true,
			Addr: wsPath,
			Conn: wsConn,
		}
		// WS词库
		wsfile := utils.NewFileQueue("private/websocket")
		if !wsfile.DirExists() {
			// 服务器
			wsfile.SetPath("private/websocket/server.n")
			wsfile.WriteToFile(appfiles.GetFileString("dic/websocket/server.n"))
			// 客户端
			wsfile.SetPath("private/websocket/app.n")
			wsfile.WriteToFile(appfiles.GetFileString("dic/websocket/app.n"))
		}
	}
}
