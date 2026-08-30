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

// init 注入服务器处理器工厂，供 loadConfig 与 save_servers 统一构建每服务器独立 handler
func init() {
	dto.WebHandlerFactory = newWebHandler
}

// newWebHandler 构建单个 HTTP 服务器的处理器（每服务器独立，携带该服务器配置）
func newWebHandler(router *dto.ServerHTTP) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webRun(w, r, router)
	})
}

// 路由
func webRun(w http.ResponseWriter, r *http.Request, router *dto.ServerHTTP) {

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

	// 内置云工具服务端
	cloudTool := s.CloudTool
	if cloudTool != nil && cloudTool.Open {
		if r.URL.Path == cloudTool.Addr || strings.HasPrefix(r.URL.Path, cloudTool.Addr+"/") {
			dic_server.CloudToolServerHandler(w, r)
			return
		}
	}

	// 当前服务器跨域（每服务器单独配置）
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

	// 当前服务器已关闭（实时开关）：管理面板/机器人/云工具等内置入口仍可访问，仅关闭对外网站路由
	if router != nil && !router.Enabled {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("该服务器已关闭"))
		return
	}

	dicWebRouter(w, r, router)
}
