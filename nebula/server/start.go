package dic_server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
	"golang.ngrok.com/ngrok"
	"golang.ngrok.com/ngrok/config"
)

// StartEvent 启动事件，Event 为空时按普通触发词处理
type StartEvent struct {
	Event   string
	Trigger string
}

// 启动服务器：不内置监听 HTTP 端口，端口监听与 WebUI 挂载交由启动词库
// start.n 中的「$创建服务器$ + $服务器.启动$」负责；Ngrok 隧道单独启动。
func Start() []StartEvent {
	res := make([]StartEvent, 0)

	if dto.ServerConfig.Ngrok != nil {
		authToken := dto.ServerConfig.Ngrok.Token
		ngrokUrl := dto.ServerConfig.Ngrok.Addr

		if u, err := StartNgrok(authToken, ngrokUrl); err == nil {
			res = append(res, StartEvent{Event: "Ngrok", Trigger: "启动 " + u})
		} else {
			debugLog.Errorf("Ngrok配置失败>%v", err)
		}
	}

	res = append(res, StartEvent{Trigger: "Main"})
	res = append(res, StartEvent{Event: "系统", Trigger: "首页"})
	return res
}

// StartNgrok 启动 Ngrok 隧道（支持运行时调用）
func StartNgrok(authToken, ngrokUrl string) (string, error) {
	if dto.ServerConfig.NgrokListener != nil {
		return "", fmt.Errorf("Ngrok 已在运行中")
	}

	ctx, cancel := context.WithCancel(context.Background())
	dto.ServerConfig.NgrokCancel = cancel

	ngrokUrlHttp := config.HTTPEndpoint()
	if ngrokUrl != "" {
		ngrokUrlHttp = config.HTTPEndpoint(
			config.WithDomain(ngrokUrl),
		)
	}

	listener, err := ngrok.Listen(ctx,
		ngrokUrlHttp,
		ngrok.WithAuthtoken(authToken),
	)
	if err != nil {
		cancel()
		dto.ServerConfig.NgrokCancel = nil
		return "", err
	}

	dto.ServerConfig.NgrokListener = listener

	// 转发目标：优先使用配置里指定的服务器，未指定则用核心服务器
	handler := ngrokTargetHandler("")
	if dto.ServerConfig.Ngrok != nil {
		handler = ngrokTargetHandler(dto.ServerConfig.Ngrok.ServerAddr)
	}

	go func() {
		if err := http.Serve(listener, handler); err != nil {
			utils.Error("Ngrok启动失败>" + err.Error())
		}
		dto.ServerConfig.NgrokListener = nil
		dto.ServerConfig.NgrokCancel = nil
	}()

	return listener.URL(), nil
}

// ngrokTargetHandler 根据服务器监听地址返回对应的 HTTP Handler；
// 未匹配到（或地址为空）时回退到核心服务器。
func ngrokTargetHandler(serverAddr string) http.Handler {
	if serverAddr != "" {
		if e := dto.FuncServers.Get(serverAddr); e != nil && e.Handler != nil {
			return e.Handler
		}
	}
	if addr := dto.FuncServers.CoreAddr(); addr != "" {
		if e := dto.FuncServers.Get(addr); e != nil && e.Handler != nil {
			return e.Handler
		}
	}
	return http.DefaultServeMux
}

// StopNgrok 停止 Ngrok 隧道（支持运行时调用）
func StopNgrok() {
	if dto.ServerConfig.NgrokCancel != nil {
		dto.ServerConfig.NgrokCancel()
		dto.ServerConfig.NgrokCancel = nil
	}
	if dto.ServerConfig.NgrokListener != nil {
		dto.ServerConfig.NgrokListener.Close()
		dto.ServerConfig.NgrokListener = nil
	}
}
