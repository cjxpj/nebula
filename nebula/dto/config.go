package dto

import (
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/cjxpj/nebula/appfiles"
	feishubot_msg "github.com/cjxpj/nebula/bot/feishubot/msg"
	napcatbot_dto "github.com/cjxpj/nebula/bot/napcatbot/dto"
	qqbot_msg "github.com/cjxpj/nebula/bot/qqbot/msg"
	secludedbot_dto "github.com/cjxpj/nebula/bot/secludedbot/dto"
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
		BotDic := utils.NewFileQueue(filepath.Join(dicPath, "dic"))
		if !BotDic.DirExists() {
			BotDic.SetPath(dicPath + "/dic/dic.n")
			if data, err := appfiles.GetFile("dic/NapCatBot.n"); err == nil {
				BotDic.WriteFileByte(data)
			} else {
				fmt.Println("embed err:", err)
			}
			// 群白名单
			BotDic.SetPath(dicPath + "/groups.txt")
			BotDic.WriteFileByte([]byte("all"))
			// 主人文件
			BotDic.SetPath(dicPath + "/admin.txt")
			BotDic.WriteFileByte([]byte(""))
		}
	}
}

func LoadConfig_qq(QQBot_Config *ini.Section, sectionName string) {
	if ServerConfig.QQBots == nil {
		ServerConfig.QQBots = make(map[string]*qqbot_msg.RouterQQBot)
	}
	if ok, _ := QQBot_Config.Key("启用").Bool(); ok {
		appId := QQBot_Config.Key("APPID").String()
		secret := QQBot_Config.Key("密钥").String()
		dicPath := QQBot_Config.Key("词库").String()
		atCompat := QQBot_Config.Key("全量艾特兼容").MustBool(true)
		filterSlash := QQBot_Config.Key("过滤开头斜杠").MustBool(true)
		debug, _ := QQBot_Config.Key("调试打印").Bool()
		ws, _ := QQBot_Config.Key("WebSocket").Bool()
		wsIntents := QQBot_Config.Key("监听码").MustInt(0)

		// 停止旧实例的 WS 连接
		if oldBot := ServerConfig.QQBots[sectionName]; oldBot != nil {
			if qqbot_msg.StopWsFunc != nil {
				qqbot_msg.StopWsFunc(oldBot)
			}
		}

		api := qqbot_msg.NewQQBot(appId, secret)
		api.Debug = debug
		ServerConfig.QQBots[sectionName] = &qqbot_msg.RouterQQBot{
			// 缓存 50 秒，3 分钟内没有访问就删除
			LastMsg:     cache.New(50*time.Second, 3*time.Minute),
			Open:        true,
			Addr:        "/" + QQBot_Config.Key("访问路径").String(),
			FilePath:    dicPath,
			API:         api,
			AtCompat:    atCompat,
			FilterSlash: filterSlash,
			Debug:       debug,
			Remark:      QQBot_Config.Key("备注").String(),
			Ws:          ws,
			WsIntents:   wsIntents,
		}

		// 启动 WS 连接
		if ws && qqbot_msg.StartWsFunc != nil {
			qqbot_msg.StartWsFunc(ServerConfig.QQBots[sectionName])
		}

		BotDic := utils.NewFileQueue(dicPath)
		if !BotDic.DirExists() {
			BotDic.SetPath(dicPath + "/dic/dic.n")
			if data, err := appfiles.GetFile("dic/QQBot.n"); err == nil {
				BotDic.WriteFileByte(data)
			} else {
				fmt.Println("embed err:", err)
			}
			// 主人文件
			BotDic.SetPath(dicPath + "/admin.txt")
			BotDic.WriteFileByte([]byte(""))
		}
	} else if bot, exists := ServerConfig.QQBots[sectionName]; exists {
		if qqbot_msg.StopWsFunc != nil {
			qqbot_msg.StopWsFunc(bot)
		}
		bot.Open = false
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
			if data, err := appfiles.GetFile("dic/YunHuBot.n"); err == nil {
				BotDic.WriteFileByte(data)
			} else {
				fmt.Println("embed err:", err)
			}
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
			if data, err := appfiles.GetFile("dic/NapCatBot.n"); err == nil {
				BotDic.WriteFileByte(data)
			} else {
				fmt.Println("embed err:", err)
			}
			// 主人文件
			BotDic.SetPath(dicPath + "/admin.txt")
			BotDic.WriteFileByte([]byte(""))
		}
	}
}

func LoadConfig_websocket(WebSocket_Config *ini.Section) {
	if ok, _ := WebSocket_Config.Key("启用").Bool(); ok {
		// 跨域默认 true（允许），显式设为 false 才限制同源
		corsOk := WebSocket_Config.Key("跨域").MustBool(true)
		wsPath := "/" + WebSocket_Config.Key("访问路径").String()
		wsConn := &websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		if !corsOk {
			wsConn.CheckOrigin = nil // 回退到 gorilla/websocket 默认同源检查
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
			if s, err := appfiles.GetFileString("dic/websocket/server.n"); err == nil {
				wsfile.WriteToFile(s)
			} else {
				fmt.Println("embed err:", err)
			}
			// 客户端
			wsfile.SetPath("private/websocket/app.n")
			if s, err := appfiles.GetFileString("dic/websocket/app.n"); err == nil {
				wsfile.WriteToFile(s)
			} else {
				fmt.Println("embed err:", err)
			}
		}
	}
}

func LoadConfig_secluded(Secluded_Config *ini.Section) {
	if ok, _ := Secluded_Config.Key("启用").Bool(); ok {
		addr := Secluded_Config.Key("对接地址").String()
		token := Secluded_Config.Key("令牌").String()
		dicPath := Secluded_Config.Key("词库").String()
		debug, _ := Secluded_Config.Key("调试打印").Bool()
		ServerConfig.SecludedBot = &secludedbot_dto.RouterSecludedBot{
			Open:     true,
			Addr:     addr,
			Token:    token,
			FilePath: dicPath,
			Debug:    debug,
		}
		BotDic := utils.NewFileQueue(dicPath)
		if !BotDic.DirExists() {
			os.MkdirAll(BotDic.FileName, 0755)
			BotDic.SetPath(filepath.Join(dicPath, "dic", "dic.n"))
			if data, err := appfiles.GetFile("dic/SecludedBot.n"); err == nil {
				BotDic.WriteFileByte(data)
			} else {
				fmt.Println("embed err:", err)
			}
		}
		// 初始化主人列表
		adminPath := utils.NewFileQueue(filepath.Join(dicPath, "admin.txt"))
		if !adminPath.FileExists() {
			adminPath.WriteFileByte([]byte(""))
		}
		// 初始化群白名单（默认all允许所有群）
		groupPath := utils.NewFileQueue(filepath.Join(dicPath, "groups.txt"))
		if !groupPath.FileExists() {
			groupPath.WriteFileByte([]byte("all"))
		}
	} else if ServerConfig.SecludedBot != nil {
		ServerConfig.SecludedBot.Open = false
	}
}
