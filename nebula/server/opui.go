package dic_server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
	"github.com/gorilla/websocket"
)

type HttpOpUiData struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func OpUI(w http.ResponseWriter, r *http.Request, getpath string) {
	// WebSocket 升级：统一通信（API 请求 + 事件推送），挂在访问路径本身（如 /nebula）
	if websocket.IsWebSocketUpgrade(r) {
		if getpath != "" && getpath != "/" {
			http.NotFound(w, r)
			return
		}
		conn, err := opuiNotifyUpgrader.Upgrade(w, r, nil)
		if err != nil {
			debugLog.Errorf("[OPUI] ws upgrade failed: %v", err)
			return
		}

		// 心跳机制：防止中间代理/防火墙断开空闲连接
		const (
			pongWait   = 60 * time.Second
			pingPeriod = (pongWait * 9) / 10
			writeWait  = opuiWriteWait
		)
		conn.SetReadDeadline(time.Now().Add(pongWait))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(pongWait))
			return nil
		})
		// 保护该连接 conn.WriteMessage 的并发写入（心跳 ping、API 响应、广播共用；
		// gorilla/websocket 同一连接禁止并发写，否则会 panic 或数据错乱）
		var writeMu sync.Mutex
		pingDone := make(chan struct{})
		go func() {
			ticker := time.NewTicker(pingPeriod)
			defer ticker.Stop()
			for {
				select {
				case <-pingDone:
					return
				case <-ticker.C:
					writeMu.Lock()
					conn.SetWriteDeadline(time.Now().Add(writeWait))
					err := conn.WriteMessage(websocket.PingMessage, nil)
					conn.SetWriteDeadline(time.Time{}) // 写完立即清除，否则残留的 deadline 过期后会阻断所有业务写入
					writeMu.Unlock()
					if err != nil {
						return
					}
				}
			}
		}()

		addOpuiNotifyClient(conn, utils.GetClientIP(r), &writeMu)
		// 限制并发处理数，防止 goroutine 爆炸 + OpUI 共享状态竞争
		sem := make(chan struct{}, 10)
		go func() {
			// 连接内认证：密钥不再走 URL 参数，改为连接后首条 check_opui_key 消息验证
			authenticatedKey := ""
			defer func() {
				if r := recover(); r != nil {
					debugLog.Errorf("[OPUI] ws read loop panic: %v", r)
				}
				close(pingDone)
				removeOpuiNotifyClient(conn)
				conn.Close()
			}()
			for {
				_, msg, readErr := conn.ReadMessage()
				if readErr != nil {
					break
				}
				var wsMsg struct {
					ID   string          `json:"id"`
					Type string          `json:"type"`
					Data json.RawMessage `json:"data"`
				}
				if err := json.Unmarshal(msg, &wsMsg); err != nil || wsMsg.Type == "" {
					continue
				}

				// ping/pong 应用层心跳：客户端通过 ping 检测连接是否存活，服务端快速回复 pong
				if wsMsg.Type == "ping" {
					resp, _ := json.Marshal(map[string]any{
						"id": wsMsg.ID, "type": "pong",
					})
					writeMu.Lock()
					conn.WriteMessage(websocket.TextMessage, resp)
					writeMu.Unlock()
					continue
				}

				// check_opui_key 在连接内处理：验证密码或快捷登录码并标记认证状态
				if wsMsg.Type == "check_opui_key" {
					var authReq struct {
						Key string `json:"key"`
					}
					valid := false
					if json.Unmarshal(wsMsg.Data, &authReq) == nil && authReq.Key != "" {
						if ok, err := opuiVerifyKey(authReq.Key); err == nil && ok {
							authenticatedKey = authReq.Key
							valid = true
							clientIP := utils.GetClientIP(r)
							addLoginEvent("admin_login", "OPUI 管理员登录成功", clientIP)
						} else {
							clientIP := utils.GetClientIP(r)
							addLoginEvent("admin_login_fail", "OPUI 登录失败: 密码错误", clientIP)
						}
					}
					resp, _ := json.Marshal(map[string]any{
						"id": wsMsg.ID, "type": wsMsg.Type, "data": json.RawMessage(fmt.Sprintf(`{"valid":%t}`, valid)),
					})
					writeMu.Lock()
					conn.WriteMessage(websocket.TextMessage, resp)
					writeMu.Unlock()
					continue
				}

				// 实时终端订阅：客户端进入/离开实时终端页面时切换订阅状态，
				// 后端据此决定是否向其推送 server_log，避免无关客户端也收到终端信息
				if wsMsg.Type == "sub_server_log" || wsMsg.Type == "unsub_server_log" {
					setServerLogSub(conn, wsMsg.Type == "sub_server_log")
					continue
				}

				// 词库调试删除确认：前端弹窗得到答复后回传，唤醒正在阻塞等待的调试运行协程
				if wsMsg.Type == "dic_delete_confirm_result" {
					var res struct {
						ID      string `json:"id"`
						Confirm bool   `json:"confirm"`
					}
					if json.Unmarshal(wsMsg.Data, &res) == nil && res.ID != "" {
						if v, ok := dicDeletePending.Load(res.ID); ok {
							select {
							case v.(chan bool) <- res.Confirm:
							default:
							}
						}
					}
					continue
				}

				// AI 工具调用审批：前端内联卡片得到答复后回传，唤醒正在阻塞等待的对话协程
				if wsMsg.Type == "ai_tool_approval_result" {
					var res struct {
						ID    string `json:"id"`
						Allow bool   `json:"allow"`
					}
					if json.Unmarshal(wsMsg.Data, &res) == nil && res.ID != "" {
						resolveAIToolApproval(res.ID, res.Allow)
					}
					continue
				}

				// 构造虚拟 HTTP 请求
				fakeReq, _ := http.NewRequest("POST", "/", bytes.NewReader(msg))
				fakeReq.Header.Set("Content-Type", "application/json")
				fakeReq.RemoteAddr = r.RemoteAddr
				if authenticatedKey != "" {
					fakeReq.Header.Set("X-OPUI-Key", authenticatedKey)
				}

				// 异步处理：每条消息独立 goroutine，读循环不被阻塞
				go func(msgID, msgType string, req *http.Request) {
					// 在 goroutine 内获取信号量，避免阻塞读循环
					sem <- struct{}{}
					defer func() { <-sem }()

					// 捕获 panic，防止单个消息处理崩溃导致整个进程退出
					defer func() {
						if r := recover(); r != nil {
							debugLog.Errorf("[OPUI] ws handler panic (type=%s, id=%s): %v", msgType, msgID, r)
							resp, _ := json.Marshal(map[string]any{
								"id":   msgID,
								"type": msgType,
								"data": json.RawMessage(`{"status":"error","error":"internal server error"}`),
							})
							writeMu.Lock()
							conn.WriteMessage(websocket.TextMessage, resp)
							writeMu.Unlock()
						}
					}()

					cw := &wsResponseWriter{header: make(http.Header)}
					opuiHandleApi(cw, req)

					respData := cw.buf.Bytes()
					if len(respData) == 0 {
						respData = []byte(`{"status":"ok"}`)
					}
					resp, _ := json.Marshal(map[string]any{
						"id": msgID, "type": msgType, "data": json.RawMessage(respData),
					})
					writeMu.Lock()
					conn.WriteMessage(websocket.TextMessage, resp)
					writeMu.Unlock()
				}(wsMsg.ID, wsMsg.Type, fakeReq)
			}
		}()
		return
	}

	// HTTP：本地托管 OPUI 前端静态资源
	if getpath == "" {
		// 保留原始查询参数（如 ?key= 快捷登录码），避免重定向到带斜杠地址时丢失
		target := dto.ServerConfig.OPUI.Addr + "/"
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusFound)
		return
	}
	if getpath == "/" {
		getpath = "/index.html"
	}

	fullPath := "dic/public/opui" + getpath

	ext := filepath.Ext(fullPath)
	if ct := mime.TypeByExtension(ext); ct != "" {
		w.Header().Set("Content-Type", ct)
	}

	data, err := appfiles.GetFile(fullPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if _, err := w.Write(data); err != nil {
		utils.Error("服务器输出 Error: " + err.Error())
	}
}

// opuiHandleApi 处理 OPUI API 请求（仅由 WebSocket 内部调用）
func opuiHandleApi(w http.ResponseWriter, r *http.Request) {
	var h *HttpOpUiData
	if err := json.NewDecoder(r.Body).Decode(&h); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if !opuiCheckKey(r, h.Type) {
		http.Error(w, `{"status":"error","error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// 模块化分派：各业务模块在 init() 中把自己负责的 API 类型注册进 opuiApiHandlers，
	// 命中即处理，opui.go 不再维护巨型 switch。
	if fn, ok := opuiApiHandlers[h.Type]; ok {
		fn(w, r, h)
		return
	}

	http.Error(w, `{"status":"error","error":"invalid type"}`, http.StatusBadRequest)
}
