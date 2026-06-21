package dic

import (
	"net/http"
	"strings"

	"github.com/cjxpj/nebula/bot/feishubot"
	"github.com/cjxpj/nebula/bot/napcatbot"
	"github.com/cjxpj/nebula/bot/qqbot"
	"github.com/cjxpj/nebula/bot/yunhubot"
	"github.com/cjxpj/nebula/dto"
	dic_server "github.com/cjxpj/nebula/server"
)

// var websocket_connect *websocket.Conn

// 跨域中间件检测
func checkCors(w http.ResponseWriter, r *http.Request) bool {
	if dto.ServerConfig.Router != nil && dto.ServerConfig.Router.Cors {
		origin := dto.ServerConfig.Router.CorsOrigins
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-OPUI-Key")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return true
		}
	}
	return false
}

// 路由
func webRun(w http.ResponseWriter, r *http.Request) {
	// 检测跨域配置
	if checkCors(w, r) {
		return
	}

	s := dto.ServerConfig

	if s.OPUI != nil {
		if getpath, ok := strings.CutPrefix(r.URL.Path, s.OPUI.Addr); ok {
			dic_server.OpUI(w, r, getpath)
			return
		}
	}

	if s.FeiShuBot != nil && s.FeiShuBot.Open && r.URL.Path == s.FeiShuBot.Addr {
		feishubot.BotMessage(w, r)
		return
	}

	if s.QQBots != nil {
		for _, bot := range s.QQBots {
			if bot != nil && bot.Open && r.URL.Path == bot.Addr {
				qqbot.BotMessage(w, r, bot)
				return
			}
		}
	}

	if s.NapCatBot != nil && s.NapCatBot.Open && r.URL.Path == s.NapCatBot.Addr {
		napcatbot.BotMessage(w, r)
		return
	}

	if s.YunHuBot != nil && s.YunHuBot.Open && r.URL.Path == s.YunHuBot.Addr {
		yunhubot.BotMessage(w, r)
		return
	}
	dicWebRouter(w, r)
}
