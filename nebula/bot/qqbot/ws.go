package qqbot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	qqbot_msg "github.com/cjxpj/nebula/bot/qqbot/msg"
	"github.com/cjxpj/nebula/debugLog"
	"github.com/gorilla/websocket"
)

func init() {
	qqbot_msg.StartWsFunc = StartQQWs
	qqbot_msg.StopWsFunc = StopQQWs
}

func dbg(bot *qqbot_msg.RouterQQBot, format string, args ...any) {
	if bot == nil || !bot.Debug {
		return
	}
	prefix := "[QQBot"
	if bot.Remark != "" {
		prefix += "/" + bot.Remark
	}
	prefix += " WS] "
	debugLog.Infof(prefix+format, args...)
}

func mustMarshal(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

// 意图值 — 仅 GUILDS/PUBLIC_GUILD_MESSAGES/GUILD_MEMBERS 为默认权限，其余需审批
// 文档: https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/interface-framework/event-emit.html
const (
	intentGUILDS       = 1 << 0  // 频道基础（默认权限）
	intentGuildMembers = 1 << 1  // 频道成员（默认权限）
	intentGuildMsg     = 1 << 9  // 私域消息（需审批）
	intentDirectMsg    = 1 << 12 // 频道私聊（需审批）
	intentGroupC2C     = 1 << 25 // 群聊和C2C（需审批）
	intentAudit        = 1 << 27 // 消息审核（需审批）
	intentPublicGuild  = 1 << 30 // 公域消息（默认权限）

	intentPrivateMin = intentGUILDS | intentGuildMembers | intentGuildMsg | intentGroupC2C | intentDirectMsg | intentAudit
	intentPublicMsg  = intentGUILDS | intentGuildMembers | intentGroupC2C | intentPublicGuild | intentDirectMsg | intentAudit
)

// wsIntentProbes 订阅意图组合列表，逐个尝试直到成功
var wsIntentProbes = []struct {
	Name    string
	Intents int
}{
	{"公域", intentPublicMsg},
	{"私域", intentPrivateMin},
}

// ================= WS 连接管理 =================

func StartQQWs(bot *qqbot_msg.RouterQQBot) {
	bot.WsMutex.Lock()
	if bot.WsConn != nil {
		bot.WsMutex.Unlock()
		return
	}
	bot.WsSessionID = "" // 新连接清空旧 session
	bot.WsMutex.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	bot.WsCancel = cancel
	go wsLoop(ctx, bot)
	dbg(bot, "已启动 WebSocket 连接")
}

func StopQQWs(bot *qqbot_msg.RouterQQBot) {
	if bot.WsCancel != nil {
		bot.WsCancel()
		bot.WsCancel = nil
	}
	bot.WsMutex.Lock()
	defer bot.WsMutex.Unlock()
	if bot.WsConn != nil {
		bot.WsConn.Close()
		bot.WsConn = nil
	}
	bot.WsSessionID = ""
	dbg(bot, "已停止 WebSocket 连接")
}

// ================= WS 连接循环 =================

func wsLoop(ctx context.Context, bot *qqbot_msg.RouterQQBot) {
	// 构建探针列表（用户自定义意图优先）
	probes := wsIntentProbes
	if bot.WsIntents != 0 {
		probes = append([]struct {
			Name    string
			Intents int
		}{{"自定义", bot.WsIntents}}, probes...)
	}

	idx := 0
	retry := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		p := probes[idx]
		dbg(bot, "尝试探针: %s (intents=%d)", p.Name, p.Intents)
		err := wsConnect(ctx, bot, p.Intents)
		if err == nil {
			// 正常关闭（ctx cancelled）
			return
		}

		dbg(bot, "连接异常: %v", err)

		isInvalidSession := err == errInvalidSession

		if isInvalidSession {
			// op=9 Session 无效 → 换下一个探针重连
			if bot.WsIntents == 0 {
				idx++
				if idx >= len(probes) {
					idx = 0 // 全部试完，从头循环
				}
			}
			retry = 0
		} else {
			// 网络断开/服务端要求重连 → 保持当前探针，退避重试
			retry++
			if retry > 5 {
				retry = 5
			}
		}

		backoff := time.Duration(1<<retry) * time.Second
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

// errInvalidSession 标记 op=9 需要换探针
var errInvalidSession = fmt.Errorf("invalid session")

func wsConnect(ctx context.Context, bot *qqbot_msg.RouterQQBot, intents int) error {
	// 获取网关地址（优先从 API，失败则用默认）
	wsUrl := getWsGatewayUrl(bot)
	conn, _, err := websocket.DefaultDialer.Dial(wsUrl, http.Header{})
	if err != nil {
		return fmt.Errorf("dial %s: %w", wsUrl, err)
	}

	bot.WsMutex.Lock()
	bot.WsConn = conn
	bot.WsMutex.Unlock()

	defer func() {
		bot.WsMutex.Lock()
		if bot.WsConn == conn {
			bot.WsConn = nil
		}
		bot.WsMutex.Unlock()
		conn.Close()
	}()

	dbg(bot, "已连接到网关，等待 Hello...")

	// 步骤1: 接收 Hello (op=10)
	var heartbeatInterval int
	if err := wsReadOp(ctx, conn, bot, 10, func(raw json.RawMessage) error {
		var hello struct {
			HeartbeatInterval int `json:"heartbeat_interval"`
		}
		if err := json.Unmarshal(raw, &hello); err != nil {
			return fmt.Errorf("parse hello: %w", err)
		}
		heartbeatInterval = hello.HeartbeatInterval
		dbg(bot, "Hello 收到, 心跳间隔=%dms", heartbeatInterval)
		return nil
	}); err != nil {
		return fmt.Errorf("hello: %w", err)
	}

	// 步骤2: 鉴权 — 优先用 Resume 恢复，失败则 Identify
	bot.WsMutex.Lock()
	sessionID := bot.WsSessionID
	bot.WsMutex.Unlock()
	if sessionID != "" {
		if err := wsResume(ctx, conn, bot); err == nil {
			dbg(bot, "Session 恢复成功")
			return wsEventLoop(ctx, conn, bot, time.Duration(heartbeatInterval)*time.Millisecond)
		}
		dbg(bot, "Resume 失败，改为重新 Identify")
		bot.WsMutex.Lock()
		bot.WsSessionID = ""
		bot.WsMutex.Unlock()
	}

	// 步骤3: Identify 上线鉴权 — 只发送，不等待，由 wsEventLoop 处理 READY/op=9
	dbg(bot, "上线鉴权 intents=%d", intents)
	if err := wsIdentify(ctx, conn, bot, intents); err != nil {
		return fmt.Errorf("identify: %w", err)
	}
	return wsEventLoop(ctx, conn, bot, time.Duration(heartbeatInterval)*time.Millisecond)
}

// wsEventLoop 事件消息循环
func wsEventLoop(ctx context.Context, conn *websocket.Conn, bot *qqbot_msg.RouterQQBot, hbInterval time.Duration) error {
	dbg(bot, "事件循环已启动，等待消息...")
	hbSend := hbInterval / 2
	hbRead := hbInterval
	errCh := make(chan error, 1)

	// 派生 context：wsEventLoop 返回时自动取消，防止心跳 goroutine 泄漏
	loopCtx, loopCancel := context.WithCancel(ctx)
	defer loopCancel()

	// 收到 READY/RESUMED 后才允许发心跳（对齐 botpy：鉴权成功后才开启心跳）
	readyCh := make(chan struct{})

	go func() {
		select {
		case <-loopCtx.Done():
			return
		case <-readyCh:
		}
		ticker := time.NewTicker(hbSend)
		defer ticker.Stop()
		for {
			select {
			case <-loopCtx.Done():
				return
			case <-ticker.C:
				bot.WsMutex.Lock()
				seq := bot.WsSeq
				bot.WsMutex.Unlock()
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteJSON(map[string]any{"op": 1, "d": seq}); err != nil {
					select {
					case errCh <- err:
					default:
					}
					return
				}
				dbg(bot, ">>> 心跳, seq=%d", seq)
			}
		}
	}()

	var hbRunning bool

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			return fmt.Errorf("write: %w", err)
		default:
		}

		// 服务端原始心跳间隔内没收任何数据 → 触发重连
		conn.SetReadDeadline(time.Now().Add(hbRead))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
		dbg(bot, "<<< %s", string(raw))

		var payload struct {
			Op int             `json:"op"`
			D  json.RawMessage `json:"d"`
			S  int             `json:"s"`
			T  string          `json:"t"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			dbg(bot, "解析消息失败: %v, raw=%s", err, string(raw))
			continue
		}

		if payload.S > 0 {
			bot.WsMutex.Lock()
			bot.WsSeq = payload.S
			bot.WsMutex.Unlock()
		}

		switch payload.Op {
		case 0: // Dispatch / Ready / Resumed
			if payload.T == "READY" || payload.T == "RESUMED" {
				// 解析 session_id
				var event struct {
					SessionID string `json:"session_id"`
				}
				bot.WsMutex.Lock()
				if err := json.Unmarshal(payload.D, &event); err == nil && event.SessionID != "" {
					bot.WsSessionID = event.SessionID
				}
				dbg(bot, "%s, session=%s", payload.T, bot.WsSessionID)
				bot.WsMutex.Unlock()
				// 鉴权成功，启动心跳（对齐 botpy）
				if !hbRunning {
					hbRunning = true
					close(readyCh)
				}
			} else {
				wsDispatch(bot, payload.T, payload.D)
			}

		case 7: // Reconnect
			dbg(bot, "服务端要求重连")
			return fmt.Errorf("server requested reconnect")

		case 9: // Invalid Session — 返回错误触发探针重试
			dbg(bot, "Session 无效(d=%s)，触发重连", string(payload.D))
			return errInvalidSession

		case 10: // Hello
			var hello struct {
				HeartbeatInterval int `json:"heartbeat_interval"`
			}
			if err := json.Unmarshal(payload.D, &hello); err == nil {
				hbInterval = time.Duration(hello.HeartbeatInterval) * time.Millisecond
			}
			dbg(bot, "再次 Hello, 心跳间隔=%dms", hbInterval/time.Millisecond)

		case 11: // Heartbeat ACK
			dbg(bot, "心跳 ACK")
		}
	}
}

// wsIdentify 发送 Identify(op=2)，不等待响应，由 wsEventLoop 处理后续 READY/op=9
func wsIdentify(_ context.Context, conn *websocket.Conn, bot *qqbot_msg.RouterQQBot, intents int) error {
	if bot == nil || bot.API == nil {
		return fmt.Errorf("bot or bot.API is nil")
	}
	if err := bot.API.EnsureToken(); err != nil {
		return fmt.Errorf("获取 token: %w", err)
	}
	token := fmt.Sprintf("QQBot %s", bot.API.Key.AccessToken)

	identify := map[string]any{
		"op": 2,
		"d": map[string]any{
			"token":   token,
			"intents": intents,
			"shard":   []int{0, 1},
			"properties": map[string]any{
				"$os": "windows",
			},
		},
	}
	if err := conn.WriteJSON(identify); err != nil {
		return fmt.Errorf("send identify: %w", err)
	}
	dbg(bot, ">>> 上线鉴权(订阅intents=%d) %s", intents, mustMarshal(identify))
	return nil
}

// wsResume 发送 Resume(op=6)，不等待响应，由 wsEventLoop 处理后续 RESUME/op=9
func wsResume(_ context.Context, conn *websocket.Conn, bot *qqbot_msg.RouterQQBot) error {
	if bot == nil || bot.API == nil {
		return fmt.Errorf("bot or bot.API is nil")
	}
	if err := bot.API.EnsureToken(); err != nil {
		return err
	}
	token := fmt.Sprintf("QQBot %s", bot.API.Key.AccessToken)

	bot.WsMutex.Lock()
	sessionID := bot.WsSessionID
	seq := bot.WsSeq
	bot.WsMutex.Unlock()

	resume := map[string]any{
		"op": 6,
		"d": map[string]any{
			"token":      token,
			"session_id": sessionID,
			"seq":        seq,
		},
	}
	if err := conn.WriteJSON(resume); err != nil {
		return err
	}
	dbg(bot, ">>> Resume, seq=%d", seq)
	return nil
}

func wsReadOp(ctx context.Context, conn *websocket.Conn, bot *qqbot_msg.RouterQQBot, expectedOp int, parse func(raw json.RawMessage) error) error {
	var payload struct {
		Op int             `json:"op"`
		D  json.RawMessage `json:"d"`
		S  int             `json:"s"`
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
		dbg(bot, "<<< %s", string(raw))
		if err := json.Unmarshal(raw, &payload); err != nil {
			continue
		}
		if payload.S > 0 {
			bot.WsMutex.Lock()
			bot.WsSeq = payload.S
			bot.WsMutex.Unlock()
		}
		if payload.Op == 7 {
			return fmt.Errorf("server requested reconnect")
		}
		if payload.Op == 9 {
			return errInvalidSession
		}
		if payload.Op == expectedOp {
			return parse(payload.D)
		}
	}
}

// getWsGatewayUrl 获取 WebSocket 网关地址
func getWsGatewayUrl(bot *qqbot_msg.RouterQQBot) string {
	// 尝试从 API 获取（无需鉴权）
	req, err := http.NewRequest("GET", "https://api.sgroup.qq.com/gateway", nil)
	if err == nil {
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				var res struct {
					URL string `json:"url"`
				}
				if json.NewDecoder(resp.Body).Decode(&res) == nil && res.URL != "" {
					dbg(bot, "网关地址: %s", res.URL)
					return res.URL
				}
			}
		}
	}
	return "wss://api.sgroup.qq.com/websocket"
}

// ================= WS 消息分发 =================

func wsDispatch(bot *qqbot_msg.RouterQQBot, t string, d json.RawMessage) {
	payload := &qqbot_msg.Payload{
		Op:   0,
		Data: d,
		Type: t,
	}

	dbg(bot, "========== 收到消息 ==========")
	dbg(bot, "事件: %s, d=%s", t, string(d))

	switch t {
	// === 频道消息 ===
	case "MESSAGE_CREATE":
		qqBOTChannelRun(payload, bot)
	case "AT_MESSAGE_CREATE":
		qqBOTChannelRun(payload, bot)
	case "DIRECT_MESSAGE_CREATE":
		qqBOTChannelPrivateRun(payload, bot)

	// === 群聊 & C2C ===
	case "GROUP_AT_MESSAGE_CREATE":
		qqBOTGroupATRun(payload, bot)
	case "GROUP_MESSAGE_CREATE":
		qqBOTGroupRun(payload, bot)
	case "C2C_MESSAGE_CREATE":
		qqBOTGroupPrivateRun(payload, bot)
	case "GROUP_MEMBER_ADD", "GROUP_MEMBER_REMOVE",
		"GROUP_ADD_ROBOT", "GROUP_DEL_ROBOT":
		qqBOTGroupEventRun(payload, bot)
	case "GROUP_MSG_REJECT", "GROUP_MSG_RECEIVE",
		"FRIEND_ADD", "FRIEND_DEL",
		"C2C_MSG_REJECT", "C2C_MSG_RECEIVE":
		dbg(bot, "系统通知: %s", t)

	// === 审核 ===
	case "MESSAGE_AUDIT_PASS", "MESSAGE_AUDIT_REJECT":
		dbg(bot, "审核通知: %s", t)

	// === 频道/子频道变更 ===
	case "GUILD_CREATE", "GUILD_UPDATE", "GUILD_DELETE",
		"CHANNEL_CREATE", "CHANNEL_UPDATE", "CHANNEL_DELETE",
		"GUILD_MEMBER_ADD", "GUILD_MEMBER_UPDATE", "GUILD_MEMBER_REMOVE":
		dbg(bot, "频道变更: %s", t)

	default:
		dbg(bot, "未知类型，走兜底处理: %s", t)
		qqBOTGroupRun(payload, bot)
	}

	dbg(bot, "================================")
}
