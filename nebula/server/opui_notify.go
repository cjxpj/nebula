package dic_server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var (
	serverStartTime = time.Now()
	loginEventsMu   sync.Mutex
	loginEvents     = make([]LoginEvent, 0, 20)
)

type LoginEvent struct {
	Time   string `json:"time"`
	Type   string `json:"type"`   // "admin_login" / "admin_login_fail" / "bot_online" / "bot_offline"
	Detail string `json:"detail"` // 描述信息
	IP     string `json:"ip"`
}

func addLoginEvent(eventType, detail, ip string) {
	loginEventsMu.Lock()
	e := LoginEvent{
		Time:   time.Now().Format("01-02 15:04:05"),
		Type:   eventType,
		Detail: detail,
		IP:     ip,
	}
	loginEvents = append(loginEvents, e)
	if len(loginEvents) > 50 {
		loginEvents = loginEvents[len(loginEvents)-50:]
	}
	loginEventsMu.Unlock()

	// 广播通知给所有 OPUI WebSocket 客户端（排除当前登录者自己）
	notifyData, _ := json.Marshal(map[string]string{
		"type":       "login_event",
		"event_type": eventType,
		"detail":     detail,
		"ip":         ip,
		"time":       e.Time,
	})
	broadcastOpuiNotifyExcept(notifyData, ip)
}

// SecurityInfo 安全检测返回数据
type SecurityInfo struct {
	ServerStart string       `json:"server_start"`
	Uptime      string       `json:"uptime"`
	LoginEvents []LoginEvent `json:"login_events"`
	OnlineList  []OnlineItem `json:"online_list"`
}

type OnlineItem struct {
	Name   string `json:"name"`
	Type   string `json:"type"` // "bot" / "service"
	Online bool   `json:"online"`
	Detail string `json:"detail"`
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "刚刚启动"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%d小时%d分钟", h, m)
	}
	return fmt.Sprintf("%d分钟", m)
}

// ---------- OPUI WebSocket 通知广播 ----------

type opuiClientInfo struct {
	IP           string
	Connected    time.Time
	SubServerLog bool        // 是否在查看实时终端页面（决定是否向其推送 server_log）
	WriteMu      *sync.Mutex // 保护该连接 conn.WriteMessage 的并发写入（广播/心跳/响应共用）
}

const opuiWriteWait = 10 * time.Second

var (
	opuiNotifyUpgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	opuiNotifyClients   = make(map[*websocket.Conn]*opuiClientInfo)
	opuiNotifyClientsMu sync.Mutex
)

func addOpuiNotifyClient(conn *websocket.Conn, ip string, writeMu *sync.Mutex) {
	opuiNotifyClientsMu.Lock()
	opuiNotifyClients[conn] = &opuiClientInfo{IP: ip, Connected: time.Now(), WriteMu: writeMu}
	opuiNotifyClientsMu.Unlock()
	broadcastOnlineUpdate()
}

func removeOpuiNotifyClient(conn *websocket.Conn) {
	opuiNotifyClientsMu.Lock()
	delete(opuiNotifyClients, conn)
	opuiNotifyClientsMu.Unlock()
	broadcastOnlineUpdate()
}

func broadcastOnlineUpdate() {
	data, _ := json.Marshal(map[string]string{"type": "online_update"})
	broadcastOpuiNotify(data)
}

func broadcastOpuiNotify(msg []byte) {
	opuiNotifyClientsMu.Lock()
	defer opuiNotifyClientsMu.Unlock()
	for conn, info := range opuiNotifyClients {
		info.WriteMu.Lock()
		conn.SetWriteDeadline(time.Now().Add(opuiWriteWait))
		err := conn.WriteMessage(websocket.TextMessage, msg)
		conn.SetWriteDeadline(time.Time{}) // 写完立即清除，避免残留 deadline 阻断后续写入
		info.WriteMu.Unlock()
		if err != nil {
			delete(opuiNotifyClients, conn)
		}
	}
}

func broadcastOpuiNotifyExcept(msg []byte, excludeIP string) {
	opuiNotifyClientsMu.Lock()
	defer opuiNotifyClientsMu.Unlock()
	for conn, info := range opuiNotifyClients {
		if info.IP == excludeIP {
			continue
		}
		info.WriteMu.Lock()
		conn.SetWriteDeadline(time.Now().Add(opuiWriteWait))
		err := conn.WriteMessage(websocket.TextMessage, msg)
		conn.SetWriteDeadline(time.Time{})
		info.WriteMu.Unlock()
		if err != nil {
			delete(opuiNotifyClients, conn)
		}
	}
}

// setServerLogSub 设置指定连接的实时终端订阅状态（客户端进入/离开终端页面时调用）
func setServerLogSub(conn *websocket.Conn, sub bool) {
	opuiNotifyClientsMu.Lock()
	if info, ok := opuiNotifyClients[conn]; ok {
		info.SubServerLog = sub
	}
	opuiNotifyClientsMu.Unlock()
}

// hasServerLogSubscriber 是否存在正在查看实时终端的客户端
func hasServerLogSubscriber() bool {
	opuiNotifyClientsMu.Lock()
	defer opuiNotifyClientsMu.Unlock()
	for _, info := range opuiNotifyClients {
		if info.SubServerLog {
			return true
		}
	}
	return false
}

// broadcastServerLog 仅向订阅了实时终端的客户端推送日志
func broadcastServerLog(msg []byte) {
	opuiNotifyClientsMu.Lock()
	defer opuiNotifyClientsMu.Unlock()
	for conn, info := range opuiNotifyClients {
		if !info.SubServerLog {
			continue
		}
		info.WriteMu.Lock()
		conn.SetWriteDeadline(time.Now().Add(opuiWriteWait))
		err := conn.WriteMessage(websocket.TextMessage, msg)
		conn.SetWriteDeadline(time.Time{})
		info.WriteMu.Unlock()
		if err != nil {
			delete(opuiNotifyClients, conn)
		}
	}
}

// ---------- 词库调试：删除操作人工确认 ----------
// 词库调试运行期间，若脚本要删除文件 / 文件夹，后端向前端推送确认请求并阻塞等待用户答复，
// 未答复（拒绝 / 超时 / 断线）一律按拒绝处理，确保删除操作必须经人工放行。
