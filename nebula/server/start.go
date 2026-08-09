package dic_server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
	"golang.ngrok.com/ngrok"
	"golang.ngrok.com/ngrok/config"
)

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

	go func() {
		if err := http.Serve(listener, dto.ServerConfig.Router.Http.Handler); err != nil {
			utils.Error("Ngrok启动失败>" + err.Error())
		}
		dto.ServerConfig.NgrokListener = nil
		dto.ServerConfig.NgrokCancel = nil
	}()

	return listener.URL(), nil
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

// 启动服务器
func Start(infoServerPath string) []string {
	res := make([]string, 0)
	if dto.ServerConfig.Ngrok != nil {
		authToken := dto.ServerConfig.Ngrok.Token
		ngrokUrl := dto.ServerConfig.Ngrok.Addr

		if u, err := StartNgrok(authToken, ngrokUrl); err == nil {
			res = append(res, fmt.Sprintf("Ngrok启动成功 %s", u))
		} else {
			debugLog.Errorf("Ngrok配置失败>%v", err)
		}
	}

	// 使用 Goroutine 启动服务器
	go func() {
		router := dto.ServerConfig.Router
		if router.TLS && router.CertFile != "" && router.KeyFile != "" {
			if err := router.Http.ListenAndServeTLS(router.CertFile, router.KeyFile); err != nil && err != http.ErrServerClosed {
				utils.Error("HTTPS启动失败>" + err.Error())
				debugLog.Errorf("HTTPS 服务器致命错误，即将退出: %v", err)
				os.Exit(1)
			}
		} else {
			if err := router.Http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				utils.Error("HTTP启动失败>" + err.Error())
				debugLog.Errorf("HTTP 服务器致命错误，即将退出: %v", err)
				os.Exit(1)
			}
		}
	}()

	if _, port, err := net.SplitHostPort(dto.ServerConfig.Router.Http.Addr); err == nil {
		scheme := "http"
		if dto.ServerConfig.Router.TLS && dto.ServerConfig.Router.CertFile != "" && dto.ServerConfig.Router.KeyFile != "" {
			scheme = "https"
		}
		url := fmt.Sprintf("%s://%s:%s%s", scheme, "localhost", port, dto.ServerConfig.OPUI.Addr)
		if dto.ServerConfig.OPUI != nil && dto.ServerConfig.OPUI.Secret != "" {
			url += "?key=" + dto.ServerConfig.OPUI.Secret
		}
		fmt.Printf("WebUi: %v\n", url)
	}

	res = append(res, "Main")
	res = append(res, "启动首页")
	return res
}
