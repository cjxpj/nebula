package dic

import (
	"net"
	"net/http"
	"strings"

	"github.com/cjxpj/nebula/bot/feishubot"
	"github.com/cjxpj/nebula/bot/napcatbot"
	"github.com/cjxpj/nebula/bot/qqbot"
	"github.com/cjxpj/nebula/bot/yunhubot"
	"github.com/cjxpj/nebula/dto"
	dic_server "github.com/cjxpj/nebula/server"
)

func getClientIP(r *http.Request) string {
	// 尝试从X-Forwarded-For头获取IP
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// X-Forwarded-For可能包含多个IP，通常第一个是最原始的客户端IP
		parts := strings.Split(forwarded, ", ")
		if len(parts) > 0 && net.ParseIP(parts[0]) != nil {
			return parts[0]
		}
	}

	// 尝试从X-Real-IP头获取IP
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" && net.ParseIP(realIP) != nil {
		return realIP
	}

	// 如果X-Forwarded-For和X-Real-IP都不存在或无效，则回退到RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	return ip
}

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
