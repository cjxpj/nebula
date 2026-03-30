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

// 路由
func webRun(w http.ResponseWriter, r *http.Request) {
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

	if s.QQBot != nil && s.QQBot.Open && r.URL.Path == s.QQBot.Addr {
		qqbot.BotMessage(w, r)
		return
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
