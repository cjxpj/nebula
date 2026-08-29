package dic_server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
	"golang.ngrok.com/ngrok"
	"golang.ngrok.com/ngrok/config"
	"golang.org/x/crypto/acme/autocert"
)

// HTTP 服务器热重启相关状态
var (
	httpSrvMu sync.Mutex
	httpSrv   *http.Server // 当前运行的 HTTP/HTTPS 服务器
	acmeSrv   *http.Server // acme 模式下的 80 端口验证服务
)

// resolveTLSPath 将证书相对路径解析为应用数据目录下的绝对路径
func resolveTLSPath(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(utils.GetAppDir(), filepath.FromSlash(p))
}

// RestartHTTPServer 启动或热重启 HTTP 服务器。返回错误表示新配置无法启动（旧服务器保持不变）。
func RestartHTTPServer() error {
	httpSrvMu.Lock()
	defer httpSrvMu.Unlock()

	router := dto.ServerConfig.Router
	if router == nil || router.Http == nil {
		return fmt.Errorf("HTTP 服务器未初始化")
	}

	handler := router.Http.Handler
	addr := router.Http.Addr

	// 校验 HTTPS 配置完整性，避免启动后因证书缺失而监听失败
	if router.TLS {
		if router.TLSMode == "acme" {
			if strings.TrimSpace(router.TLSDomains) == "" {
				return fmt.Errorf("Let's Encrypt 模式需要填写域名")
			}
			// 80 端口专用于 ACME HTTP-01 证书验证，主服务不能占用，否则两者监听冲突
			if _, port, err := net.SplitHostPort(addr); err == nil && port == "80" {
				return fmt.Errorf("Let's Encrypt 模式下监听地址不能使用 80 端口（已用于证书验证），请改为 443")
			}
		} else if router.CertFile == "" || router.KeyFile == "" {
			return fmt.Errorf("HTTPS 证书文件或密钥文件未配置")
		}
	}

	// 预校验证书，捕获最常见的启动错误（证书缺失/不匹配）
	var (
		newAcmeSrv *http.Server
		newAcmeMgr *autocert.Manager
		certFile   string
		keyFile    string
	)
	if router.TLS && router.TLSMode == "acme" {
		dir := filepath.Join(utils.GetAppDir(), "private", "https", "acme")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("创建 ACME 目录失败: %v", err)
		}
		domains := make([]string, 0)
		for _, d := range strings.Split(router.TLSDomains, ",") {
			d = strings.TrimSpace(d)
			// 容错：去掉常见的协议前缀与路径部分
			d = strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://")
			if i := strings.IndexByte(d, '/'); i >= 0 {
				d = d[:i]
			}
			if d == "" {
				continue
			}
			// HTTP-01 / TLS-ALPN-01 均不支持泛域名，提前给出明确错误
			if strings.Contains(d, "*") {
				return fmt.Errorf("Let's Encrypt 不支持泛域名（%s），请填写具体域名", d)
			}
			domains = append(domains, d)
		}
		if len(domains) == 0 {
			return fmt.Errorf("Let's Encrypt 域名列表为空")
		}
		m := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(domains...),
			Cache:      autocert.DirCache(dir),
			Email:      router.TLSEmail,
		}
		newAcmeMgr = m
		newAcmeSrv = &http.Server{Addr: ":80", Handler: m.HTTPHandler(nil)}
	} else if router.TLS {
		certFile = resolveTLSPath(router.CertFile)
		keyFile = resolveTLSPath(router.KeyFile)
		if _, err := tls.LoadX509KeyPair(certFile, keyFile); err != nil {
			return fmt.Errorf("证书加载失败: %v", err)
		}
	}

	old := httpSrv
	oldAcme := acmeSrv

	// 先关闭旧监听器释放端口（Close 不关闭已 hijack 的 WebSocket 连接，
	// 因此 save_server 等经 WS 进来的请求响应不受影响）
	if old != nil {
		_ = old.Close()
	}
	if oldAcme != nil && oldAcme != newAcmeSrv {
		_ = oldAcme.Close()
	}

	// 启动新服务器
	srv := &http.Server{Addr: addr, Handler: handler}
	if newAcmeMgr != nil {
		srv.TLSConfig = newAcmeMgr.TLSConfig()
	}
	go func(s *http.Server) {
		var err error
		switch {
		case newAcmeMgr != nil:
			err = s.ListenAndServeTLS("", "")
		case certFile != "":
			err = s.ListenAndServeTLS(certFile, keyFile)
		default:
			err = s.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			debugLog.Errorf("HTTP 服务器启动失败: %v", err)
		}
	}(srv)

	// 启动 acme 80 端口验证服务
	if newAcmeSrv != nil {
		go func() {
			if err := newAcmeSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				debugLog.Errorf("ACME HTTP-01 验证服务启动失败（80 端口可能被占用）: %v", err)
			}
		}()
	}

	// 更新运行时引用
	httpSrv = srv
	acmeSrv = newAcmeSrv
	router.Http = srv

	return nil
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

// StartEvent 启动事件，Event 为空时按普通触发词处理
type StartEvent struct {
	Event   string
	Trigger string
}

// 启动服务器
func Start(infoServerPath string) []StartEvent {
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

	// 启动 HTTP 服务器（使用可热重启的统一入口，首次启动失败视为致命错误）
	if err := RestartHTTPServer(); err != nil {
		utils.Error("HTTP启动失败>" + err.Error())
		debugLog.Errorf("HTTP 服务器启动失败: %v", err)
		os.Exit(1)
	}

	if _, port, err := net.SplitHostPort(dto.ServerConfig.Router.Http.Addr); err == nil {
		scheme := "http"
		if dto.ServerConfig.Router.TLS && (dto.ServerConfig.Router.CertFile != "" || dto.ServerConfig.Router.TLSMode == "acme") {
			scheme = "https"
		}
		url := fmt.Sprintf("%s://%s:%s", scheme, "localhost", port)
		if dto.ServerConfig.OPUI != nil {
			opuiUrl := url + dto.ServerConfig.OPUI.Addr
			if dto.ServerConfig.OPUI.Secret != "" {
				opuiUrl += "?key=" + dto.ServerConfig.OPUI.Secret
			}
			fmt.Printf("WebUi: %v\n", opuiUrl)
		}
	}

	res = append(res, StartEvent{Trigger: "Main"})
	res = append(res, StartEvent{Event: "系统", Trigger: "首页"})
	return res
}
