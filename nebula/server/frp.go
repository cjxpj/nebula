package dic_server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cjxpj/nebula/debugLog"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/gorilla/websocket"
)

// BeerWebFrp 协议消息类型
type frpWSMessage struct {
	ID      string            `json:"id"`
	Type    string            `json:"type"` // "http"(默认)、"ws"、"ws_frame"、"ws_close"
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Body    []byte            `json:"body"`
}

type frpWSResponse struct {
	ID         string            `json:"id"`
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body,omitempty"`
}

// frpWSResponseStart 分块流式响应的起始消息（状态码+响应头）
type frpWSResponseStart struct {
	Type       string            `json:"type"`
	ID         string            `json:"id"`
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers,omitempty"`
}

// frpWSChunk 分块流式响应的数据块
type frpWSChunk struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Chunk []byte `json:"chunk"`
	Final bool   `json:"final"`
}

// frpCon 单条 FRP WebSocket 连接，写队列模式实现异步发送
type frpCon struct {
	conn   *websocket.Conn
	wch    chan writeJob // 写队列
	ctx    context.Context
	cancel context.CancelFunc
	// localAddr 该隧道转发到的本地 HTTP 地址（每个服务器独立）
	localAddr string
	// debug 调试日志开关（每个服务器独立）
	debug bool
}

// writeJob 一次写任务
type writeJob struct {
	data []byte
	done chan error // nil = 异步（HTTP响应），非nil = 同步（WS帧/状态等结果）
}

// BeerWebFrp WebSocket 连接管理
var (
	frpHTTPClient = &http.Client{
		// 超时需小于 BeerWebFrp 服务端 proxyRequestTimeout(90s)，避免服务端先超时导致 502
		Timeout: 85 * time.Second,
		// 不自动跟随重定向：3xx 及 Location 必须原样回传隧道服务端，
		// 由服务端改写 Location（如 "/" -> "/token/"）以适配隧道域名
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// frpRuntimes 每个服务器的 BeerWebFrp 隧道取消上下文（key 为监听地址字符串）
	frpRuntimesMu sync.Mutex
	frpRuntimes   = make(map[string]context.CancelFunc)

	// BeerWebFrp WS 代理流映射表
	frpWsStreams   = make(map[string]*frpWsStream) // streamID -> 本地 WS 流
	frpWsStreamsMu sync.Mutex
)

// frpWsStream 一条本地 WebSocket 代理流。
// gorilla/websocket 不允许多个 goroutine 并发调用 WriteMessage，
// 因此用 mu 串行化对本地 WS 连接的所有写入。
type frpWsStream struct {
	conn *websocket.Conn
	fc   *frpCon // 所属的 FRP 连接（用于按连接清理流）
	mu   sync.Mutex
}

// printFrpLink 打印 BeerWebFrp 隧道访问链接。
// 与 Ngrok 一致，将链接作为特殊触发 [BeerWebFrp] 交给路由词库 router.n
// 匹配并格式化输出（默认输出 “BeerWebFrp：<链接>”），支持用户自定义路由词库。
func printFrpLink(link string) {
	if link == "" {
		return
	}
	if dic, err := dic_dto.RunDic("private/system/router.n"); err == nil && dic != nil {
		defer dic.Close()
		if out := dic_api.Api.DicRunEvent(dic, "BeerWebFrp", "启动 "+link); out != "" {
			fmt.Printf("%v\n", out)
			return
		}
	}
	// 词库不可用时直接打印原始链接
	fmt.Printf("BeerWebFrp启动成功 %s\n", link)
}

// frpLocalAddr 将监听地址转换为可回连的本地回环地址（0.0.0.0 / [::] / 空主机 → 127.0.0.1）
func frpLocalAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

// StartServerFrp 启动单个服务器的 BeerWebFrp 隧道（若已存在则先停止旧的）
func StartServerFrp(serverAddr string, frpOpen bool, frpServerAddr, frpToken string, frpDebug bool) {
	if serverAddr == "" {
		return
	}
	StopServerFrp(serverAddr)

	addr := strings.TrimSpace(frpServerAddr)
	token := strings.TrimSpace(frpToken)

	if frpDebug {
		debugLog.Infof("[FRP] 启动隧道, server=%s, addr=%s, token长度=%d (空则匿名)", serverAddr, addr, len(token))
	}

	if !frpOpen || addr == "" {
		if frpDebug {
			debugLog.Infof("[FRP] 跳过启动: open=%v, addr为空=%v", frpOpen, addr == "")
		}
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	frpRuntimesMu.Lock()
	frpRuntimes[serverAddr] = cancel
	frpRuntimesMu.Unlock()

	go runFrpConn(ctx, addr, token, frpLocalAddr(serverAddr), frpDebug)
}

// StopServerFrp 停止单个服务器的 BeerWebFrp 隧道
func StopServerFrp(serverAddr string) {
	if serverAddr == "" {
		return
	}
	frpRuntimesMu.Lock()
	cancel := frpRuntimes[serverAddr]
	delete(frpRuntimes, serverAddr)
	frpRuntimesMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// runFrpConn 管理单条 FRP WebSocket 连接的生命周期，断线自动重连
func runFrpConn(ctx context.Context, serverAddr, token, localAddr string, debug bool) {
	const readTimeout = 300 * time.Second // 读超时需大于服务端 pongWait，防止慢带宽下大文件写入时读超时断开

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		wsURL := strings.TrimRight(serverAddr, "/") + "/api/ws"
		dialer := websocket.Dialer{
			EnableCompression: true,
			HandshakeTimeout:  10 * time.Second,
		}
		conn, httpResp, err := dialer.Dial(wsURL, nil)
		if err != nil {
			if debug {
				if httpResp != nil {
					debugLog.Infof("[FRP] 连接失败: %v (HTTP %d), 5秒后重连", err, httpResp.StatusCode)
				} else {
					debugLog.Infof("[FRP] 连接失败: %v, 5秒后重连 (请确认 BeerWebFrp 服务端已启动)", err)
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}

		// 握手：token 为空时走匿名连接（服务端创建临时隧道）
		handshake := map[string]any{}
		if token == "" {
			handshake["anonymous"] = true
		} else {
			handshake["token"] = token
		}
		if err := conn.WriteJSON(handshake); err != nil {
			if debug {
				debugLog.Infof("[FRP] 握手发送失败: %v", err)
			}
			conn.Close()
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}

		var hs struct {
			Message string `json:"message"`
			Error   string `json:"error"`
			Link    string `json:"link"`
		}
		if err := conn.ReadJSON(&hs); err != nil {
			if debug {
				debugLog.Infof("[FRP] 握手响应读取失败: %v", err)
			}
			conn.Close()
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}
		if hs.Error != "" {
			if debug {
				debugLog.Infof("[FRP] 握手失败: %s", hs.Error)
			}
			conn.Close()
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}
		if debug {
			debugLog.Infof("[FRP] 握手成功, link=%s", hs.Link)
		}
		// 打印隧道访问链接（经启动词库 start.n 格式化输出，如 “BeerWebFrp：<链接>”）
		printFrpLink(hs.Link)

		// 构建连接对象并启动异步写入器
		fc := &frpCon{
			conn:      conn,
			wch:       make(chan writeJob, 64),
			localAddr: localAddr,
			debug:     debug,
		}
		fc.ctx, fc.cancel = context.WithCancel(ctx)

		go fc.writer()

		if debug {
			debugLog.Infof("[FRP] 连接已注册，进入代理模式")
		}

		// 设置保活
		conn.SetReadDeadline(time.Now().Add(readTimeout))
		conn.SetPingHandler(func(appData string) error {
			conn.SetReadDeadline(time.Now().Add(readTimeout))
			return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(10*time.Second))
		})
		// Pong 回调：客户端发 Ping 后服务端回复 Pong 时触发，重置读超时防止空闲断连
		conn.SetPongHandler(func(appData string) error {
			conn.SetReadDeadline(time.Now().Add(readTimeout))
			return nil
		})

		pingDone := make(chan struct{})
		go func() {
			ticker := time.NewTicker(50 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); err != nil {
						return
					}
				case <-pingDone:
					return
				}
			}
		}()

		// 读取代理请求
		for {
			var msg frpWSMessage
			if err := conn.ReadJSON(&msg); err != nil {
				if debug {
					debugLog.Infof("[FRP] 读取消息失败(连接断开): %v", err)
				}
				break
			}
			if msg.ID == "" {
				continue
			}

			// 每次成功读取后重置读超时，配合 Ping 保活防止空闲断连
			conn.SetReadDeadline(time.Now().Add(readTimeout))

			switch msg.Type {
			case "ws":
				if debug {
					debugLog.Infof("[FRP] 收到WS代理请求: %s (id=%s)", msg.Path, msg.ID)
				}
				go handleFrpWsProxy(fc, &msg)
			case "ws_frame":
				if debug {
					debugLog.Infof("[FRP] 收到WS数据帧 (id=%s)", msg.ID)
				}
				go handleFrpWsFrame(fc, &msg)
			case "ws_close":
				if debug {
					debugLog.Infof("[FRP] 收到WS关闭通知 (id=%s)", msg.ID)
				}
				go handleFrpWsClose(fc, &msg)
			default:
				if debug {
					debugLog.Infof("[FRP] 收到HTTP代理请求: %s %s (id=%s)", msg.Method, msg.Path, msg.ID)
				}
				go handleFrpProxyRequest(fc, &msg)
			}
		}

		// 清理：取消 writer，停止 Ping，断开残留的本地 WS 代理流（避免重连后旧流残留）
		fc.cancel()
		close(pingDone)
		closeFrpWsStreams(fc)

		if debug {
			debugLog.Infof("[FRP] 连接断开, 5秒后重连")
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

// writer 异步写 goroutine，从队列取数据按带宽自适应发送
func (fc *frpCon) writer() {
	defer fc.conn.Close()
	debug := fc.debug
	for {
		select {
		case job := <-fc.wch:
			writeTimeout := 60 * time.Second
			dataLen := len(job.data)
			if dataLen > 0 {
				// 按实际数据量计算：每 25KB 增加 1s，支持极低带宽（如 17KB/s 传 2.7MB）
				writeTimeout = max(time.Duration(60+dataLen/25600)*time.Second, 60*time.Second)
				writeTimeout = min(writeTimeout, 10*time.Minute)
			}
			fc.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			err := fc.conn.WriteMessage(websocket.TextMessage, job.data)
			if job.done != nil {
				job.done <- err
			}
			if err != nil {
				if debug {
					debugLog.Infof("[FRP] writer 写出错: %v，排空队列退出", err)
				}
				for {
					select {
					case job := <-fc.wch:
						if job.done != nil {
							job.done <- err
						}
					default:
						return
					}
				}
			}
		case <-fc.ctx.Done():
			for {
				select {
				case job := <-fc.wch:
					if job.done != nil {
						job.done <- fc.ctx.Err()
					}
				default:
					return
				}
			}
		}
	}
}

// proxyTimeoutHTML 当本地请求超时时返回的提示页面
const proxyTimeoutHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>504 响应超时</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI','Microsoft YaHei',sans-serif;background:#0f172a;color:#e2e8f0;display:flex;align-items:center;justify-content:center;min-height:100vh}
.card{background:#1e293b;border:1px solid #334155;border-radius:20px;padding:48px 40px;max-width:560px;width:90%;text-align:center;box-shadow:0 25px 60px rgba(0,0,0,.4);animation:fadeUp .5s ease-out}
.icon-wrap{width:80px;height:80px;margin:0 auto 20px;background:linear-gradient(135deg,#f59e0b,#ef4444);border-radius:50%;display:flex;align-items:center;justify-content:center;font-size:36px;animation:pulse 2s ease-in-out infinite}
.error-code{font-size:28px;font-weight:800;background:linear-gradient(135deg,#f59e0b,#ef4444);background-clip:text;-webkit-background-clip:text;-webkit-text-fill-color:transparent;margin-bottom:8px}
.error-title{font-size:22px;font-weight:700;color:#f1f5f9;margin-bottom:12px}
.divider{width:60px;height:3px;background:linear-gradient(90deg,#f59e0b,#ef4444);border-radius:2px;margin:0 auto 24px}
.error-sub{font-size:14px;color:#94a3b8;line-height:1.8;margin-bottom:20px}
.error-sub span{color:#f59e0b;font-weight:600}
.bw-info{background:#0f172a;border:1px solid #334155;border-radius:12px;padding:16px 20px;margin-bottom:20px;display:flex;align-items:center;gap:12px}
.bw-icon{font-size:24px;flex-shrink:0}
.bw-text{font-size:13px;color:#94a3b8;line-height:1.7;text-align:left}
.bw-text strong{color:#f1f5f9}
.hint{font-size:13px;color:#64748b;line-height:1.8}
@keyframes fadeUp{0%{opacity:0;transform:translateY(20px)}100%{opacity:1;transform:translateY(0)}}
@keyframes pulse{0%,100%{opacity:1;transform:scale(1)}50%{opacity:.6;transform:scale(1.08)}}
</style>
</head>
<body>
<div class="card">
  <div class="icon-wrap">⏳</div>
  <div class="error-code">504</div>
  <div class="error-title">响应超时</div>
  <div class="divider"></div>
  <p class="error-sub">
    本地服务处理请求超时，<span>未能及时返回响应</span>。
  </p>
  <div class="bw-info">
    <div class="bw-icon">📶</div>
    <div class="bw-text">
      <strong>请检查本地服务状态</strong><br>
      隧道客户端已收到请求，但本地后端服务未能在规定时间内响应。请确认后端服务运行正常，或适当增加超时时间。
    </div>
  </div>
  <p class="hint">请稍后刷新重试</p>
</div>
</body>
</html>`

// handleFrpProxyRequest 处理来自 BeerWebFrp 服务端的代理请求
// 将请求转发到本地 HTTP 服务器，并返回响应。大响应（>1MB）使用分块流式传输
func handleFrpProxyRequest(fc *frpCon, msg *frpWSMessage) {
	debug := fc.debug
	var (
		respStatusCode int
		respHeaders    map[string]string
		respBody       []byte
		sent           bool // 是否已由流式模式处理
	)

	defer func() {
		if sent {
			return
		}
		// 发送错误响应（单条消息）
		resp := frpWSResponse{
			ID:         msg.ID,
			StatusCode: respStatusCode,
			Headers:    respHeaders,
		}
		if respBody != nil {
			resp.Body = respBody
		}
		data, _ := json.Marshal(resp)
		select {
		case fc.wch <- writeJob{data: data}:
		default:
			if debug {
				debugLog.Infof("[FRP] 写队列满，丢弃响应 (id=%s)", msg.ID)
			}
		}
	}()

	respStatusCode = 502
	respHeaders = map[string]string{"Content-Type": "text/html; charset=utf-8"}
	respBody = []byte("backend server not available")

	if fc == nil || fc.localAddr == "" {
		if debug {
			debugLog.Infof("[FRP] 代理请求: 本地地址为空，返回502 (id=%s)", msg.ID)
		}
		return
	}

	targetURL := "http://" + fc.localAddr + msg.Path

	if debug {
		debugLog.Infof("[FRP] 转发请求: %s %s -> %s (body长度=%d, headers数=%d)", msg.Method, msg.Path, targetURL, len(msg.Body), len(msg.Headers))
	}

	var bodyReader io.Reader
	if len(msg.Body) > 0 {
		bodyReader = bytes.NewReader(msg.Body)
	}
	// 注：服务端 Body 已是 []byte，JSON 反序列化时自动 base64 解码

	req, err := http.NewRequest(msg.Method, targetURL, bodyReader)
	if err != nil {
		if debug {
			debugLog.Infof("[FRP] 代理请求: 构造请求失败 (id=%s): %v", msg.ID, err)
		}
		respBody = []byte("proxy build request error: " + err.Error())
		return
	}

	// 转发原始请求头（含 X-Real-Ip、X-Forwarded-For）
	if msg.Headers != nil {
		for k, v := range msg.Headers {
			req.Header.Set(k, v)
		}
	}

	// 发起本地 HTTP 请求
	httpResp, err := frpHTTPClient.Do(req)
	if err != nil {
		if debug {
			debugLog.Infof("[FRP] 代理请求: 本地请求失败 (id=%s): %v", msg.ID, err)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			respStatusCode = 504
			respBody = []byte(proxyTimeoutHTML)
			return
		}
		respBody = []byte("proxy request error: " + err.Error())
		return
	}
	defer httpResp.Body.Close()

	// 收集响应头（过滤局域网敏感信息）
	stripHeaders := map[string]bool{
		"Server": true, "X-Powered-By": true,
		"X-Forwarded-For": true, "X-Forwarded-Proto": true,
		"X-Forwarded-Host": true, "X-Forwarded-Port": true,
		"X-Real-Ip": true, "X-Real-Port": true,
		"X-Runtime": true, "Via": true,
	}
	respHeaders = make(map[string]string)
	for k, v := range httpResp.Header {
		if stripHeaders[k] {
			continue
		}
		respHeaders[k] = strings.Join(v, ", ")
	}

	// 读取响应体
	respBody, err = io.ReadAll(httpResp.Body)
	if err != nil {
		if debug {
			debugLog.Infof("[FRP] 代理请求: 读取响应体失败 (id=%s): %v", msg.ID, err)
		}
		respStatusCode = 502
		respBody = []byte("proxy read body error: " + err.Error())
		return
	}

	respStatusCode = httpResp.StatusCode

	if debug {
		debugLog.Infof("[FRP] 代理请求完成: id=%s, status=%d, 响应体长度=%d", msg.ID, httpResp.StatusCode, len(respBody))
	}

	// 小响应（≤1MB）：单条消息
	const chunkThreshold = 1 << 20
	if len(respBody) <= chunkThreshold {
		return // defer 会发送
	}

	// 大响应（>1MB）：分块流式传输
	sent = true

	// 1. 发送 http_response_start
	start := frpWSResponseStart{
		Type:       "http_response_start",
		ID:         msg.ID,
		StatusCode: respStatusCode,
		Headers:    respHeaders,
	}
	startData, _ := json.Marshal(start)
	select {
	case fc.wch <- writeJob{data: startData}:
	default:
		// 起始消息都发不出去，回退为单条响应（由 defer 兜底），避免服务端挂起等待
		sent = false
		if debug {
			debugLog.Infof("[FRP] 写队列满，回退单条响应 (id=%s)", msg.ID)
		}
		return
	}

	// 2. 分块发送 body（64KB/块）
	const chunkSize = 64 << 10
	for offset := 0; offset < len(respBody); offset += chunkSize {
		end := offset + chunkSize
		end = min(end, len(respBody))
		chunk := frpWSChunk{
			Type:  "http_chunk",
			ID:    msg.ID,
			Chunk: respBody[offset:end],
			Final: end == len(respBody),
		}
		chunkData, _ := json.Marshal(chunk)
		select {
		case fc.wch <- writeJob{data: chunkData}:
		default:
			// 队列满：补发一个 final 空块结束服务端流式管道，避免服务端挂起直到超时
			if debug {
				debugLog.Infof("[FRP] 写队列满，发送结束块 (id=%s)", msg.ID)
			}
			if finalData, err := json.Marshal(frpWSChunk{Type: "http_chunk", ID: msg.ID, Final: true}); err == nil {
				select {
				case fc.wch <- writeJob{data: finalData}:
				default:
				}
			}
			return
		}
	}

	// 清理 defer 不需要的数据
	respBody = nil
}

// handleFrpWsProxy 处理 BeerWebFrp 的 WebSocket 代理请求
// 收到 type:"ws" 消息后，连接本地 WebSocket 服务并双向转发数据帧
func handleFrpWsProxy(fc *frpCon, msg *frpWSMessage) {
	debug := fc.debug
	streamID := msg.ID

	if fc == nil || fc.localAddr == "" {
		sendFrpWsStatus(fc, streamID, -1)
		return
	}

	wsURL := "ws://" + fc.localAddr + msg.Path

	if debug {
		debugLog.Infof("[FRP] WS代理: 连接本地WS %s (id=%s)", wsURL, streamID)
	}

	// 转换请求头为 http.Header 格式
	var wsHeaders http.Header
	if len(msg.Headers) > 0 {
		wsHeaders = make(http.Header, len(msg.Headers))
		for k, v := range msg.Headers {
			wsHeaders.Set(k, v)
		}
	}
	localConn, _, err := websocket.DefaultDialer.Dial(wsURL, wsHeaders)
	if err != nil {
		if debug {
			debugLog.Infof("[FRP] WS代理: 本地WS连接失败 (id=%s): %v", streamID, err)
		}
		sendFrpWsStatus(fc, streamID, -1)
		return
	}

	// 注册流（记录所属连接，便于按连接清理）
	st := &frpWsStream{conn: localConn, fc: fc}
	frpWsStreamsMu.Lock()
	frpWsStreams[streamID] = st
	frpWsStreamsMu.Unlock()

	if debug {
		debugLog.Infof("[FRP] WS代理: 本地WS连接成功 (id=%s)", streamID)
	}

	// 本地 → 服务端：读取本地 WS 消息并回传
	go func() {
		defer func() {
			// 先关闭连接（短暂加锁），再清理 map，避免 st.mu → frpWsStreamsMu
			// 与 handleFrpWsClose 的 frpWsStreamsMu → st.mu 形成死锁。
			st.mu.Lock()
			localConn.Close()
			st.mu.Unlock()
			frpWsStreamsMu.Lock()
			delete(frpWsStreams, streamID)
			frpWsStreamsMu.Unlock()
		}()
		for {
			_, frame, err := localConn.ReadMessage()
			if err != nil {
				if debug {
					debugLog.Infof("[FRP] WS代理: 本地WS读取关闭 (id=%s): %v", streamID, err)
				}
				sendFrpWsStatus(fc, streamID, -1)
				return
			}
			sendFrpWsFrame(fc, streamID, frame)
		}
	}()
}

// handleFrpWsFrame 处理来自 BeerWebFrp 的 ws_frame 消息
// Base64 解码 body 后写入本地 WS 连接
func handleFrpWsFrame(fc *frpCon, msg *frpWSMessage) {
	debug := fc.debug
	streamID := msg.ID

	frpWsStreamsMu.Lock()
	st, ok := frpWsStreams[streamID]
	frpWsStreamsMu.Unlock()

	if !ok {
		if debug {
			debugLog.Infof("[FRP] WS代理: 收到ws_frame但流不存在 (id=%s)", streamID)
		}
		return
	}

	// 加锁串行化写入，避免多帧并发写导致 gorilla 连接数据竞争
	st.mu.Lock()
	err := st.conn.WriteMessage(websocket.BinaryMessage, msg.Body)
	st.mu.Unlock()
	if err != nil && debug {
		debugLog.Infof("[FRP] WS代理: 写入本地WS失败 (id=%s): %v", streamID, err)
	}
}

// handleFrpWsClose 处理来自 BeerWebFrp 的 ws_close 消息
// 关闭本地 WS 连接并清理流
func handleFrpWsClose(fc *frpCon, msg *frpWSMessage) {
	debug := fc.debug
	streamID := msg.ID

	frpWsStreamsMu.Lock()
	st, ok := frpWsStreams[streamID]
	if ok {
		delete(frpWsStreams, streamID)
	}
	frpWsStreamsMu.Unlock()

	if ok && st != nil {
		if debug {
			debugLog.Infof("[FRP] WS代理: 关闭本地WS流 (id=%s)", streamID)
		}
		st.mu.Lock()
		st.conn.Close()
		st.mu.Unlock()
	}
}

// sendFrpWsFrame 向服务端发送 WS 数据帧响应（同步等待写入结果）
func sendFrpWsFrame(fc *frpCon, streamID string, frame []byte) {
	debug := fc.debug
	data, _ := json.Marshal(frpWSResponse{
		ID:         streamID,
		StatusCode: 0,
		Headers:    map[string]string{},
		Body:       frame,
	})
	done := make(chan error, 1)
	select {
	case fc.wch <- writeJob{data: data, done: done}:
		select {
		case err := <-done:
			if err != nil && debug {
				debugLog.Infof("[FRP] WS代理: 发送数据帧失败 (id=%s): %v", streamID, err)
			}
		case <-time.After(10 * time.Second):
			// 连接已断开且写队列无人消费，放弃等待，避免 goroutine 泄漏
		}
	default:
		if debug {
			debugLog.Infof("[FRP] WS代理: 发送数据帧失败（写队列满） (id=%s)", streamID)
		}
	}
}

// sendFrpWsStatus 向服务端发送 WS 状态通知（同步等待写入结果）
func sendFrpWsStatus(fc *frpCon, streamID string, statusCode int) {
	debug := fc.debug
	if debug {
		debugLog.Infof("[FRP] WS代理: 发送状态通知 streamID=%s, statusCode=%d", streamID, statusCode)
	}
	data, _ := json.Marshal(frpWSResponse{
		ID:         streamID,
		StatusCode: statusCode,
	})
	done := make(chan error, 1)
	select {
	case fc.wch <- writeJob{data: data, done: done}:
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			// 连接已断开且写队列无人消费，放弃等待，避免 goroutine 泄漏
		}
	default:
	}
}

// closeFrpWsStreams 断开指定 FRP 连接的本地 WS 代理流（fc 为 nil 时断开全部）
func closeFrpWsStreams(fc *frpCon) {
	debug := false
	if fc != nil {
		debug = fc.debug
	}
	frpWsStreamsMu.Lock()
	defer frpWsStreamsMu.Unlock()
	n := 0
	for id, st := range frpWsStreams {
		if fc != nil && st.fc != fc {
			continue
		}
		st.mu.Lock()
		st.conn.Close()
		st.mu.Unlock()
		delete(frpWsStreams, id)
		n++
	}
	if debug && n > 0 {
		debugLog.Infof("[FRP] WS代理: 清理%d个本地WS流", n)
	}
}

// closeAllFrpWsStreams 断开所有本地 WS 代理连接
func closeAllFrpWsStreams() {
	closeFrpWsStreams(nil)
}
