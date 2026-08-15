//go:build !js

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

// setCORS 设置跨域响应头。若为 OPTIONS 预检请求则处理后返回 true。
func setCORS(w http.ResponseWriter, r *http.Request, origin string) bool {
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-OPUI-Key")
	if origin != "*" {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	if r.Method == http.MethodOptions {
		if reqMethod := r.Header.Get("Access-Control-Request-Method"); reqMethod != "" {
			w.Header().Set("Access-Control-Allow-Methods", reqMethod)
		}
		if reqHeaders := r.Header.Get("Access-Control-Request-Headers"); reqHeaders != "" {
			w.Header().Set("Access-Control-Allow-Headers", reqHeaders)
		}
		w.Header().Set("Access-Control-Max-Age", "86400")
		w.WriteHeader(http.StatusOK)
		return true
	}
	return false
}

// 路由
func webRun(w http.ResponseWriter, r *http.Request) {

	// IP黑名单 + 防火墙词库拦截
	if dic_server.CheckFirewall(w, r) {
		return
	}

	s := dto.ServerConfig

	opui := s.OPUI
	if opui != nil {
		if getpath, ok := strings.CutPrefix(r.URL.Path, opui.Addr); ok {
			if opui.Cors && setCORS(w, r, "*") {
				return
			}
			dic_server.OpUI(w, r, getpath)
			return
		}
	}

	// 全局跨域
	router := s.Router
	if router != nil && router.Cors {
		origin := router.CorsOrigins
		if origin == "" {
			origin = "*"
		}
		if setCORS(w, r, origin) {
			return
		}
	}

	feishu := s.FeiShuBot
	if feishu != nil && feishu.Open && r.URL.Path == feishu.Addr {
		feishubot.BotMessage(w, r)
		return
	}

	if s.QQBots != nil {
		for _, bot := range s.QQBots {
			if bot != nil && bot.Open && !bot.Ws && r.URL.Path == bot.Addr {
				qqbot.BotMessage(w, r, bot)
				return
			}
		}
	}

	napcat := s.NapCatBot
	if napcat != nil && napcat.Open && r.URL.Path == napcat.Addr {
		napcatbot.BotMessage(w, r)
		return
	}

	yunhu := s.YunHuBot
	if yunhu != nil && yunhu.Open && r.URL.Path == yunhu.Addr {
		yunhubot.BotMessage(w, r)
		return
	}
	dicWebRouter(w, r)
}
