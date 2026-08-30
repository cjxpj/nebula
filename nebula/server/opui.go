package dic_server

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/bot/qqbot"
	qqbot_msg "github.com/cjxpj/nebula/bot/qqbot/msg"
	"github.com/cjxpj/nebula/bot/secludedbot"
	"github.com/cjxpj/nebula/debugLog"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	dic_funcs "github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
	"github.com/gomarkdown/markdown"
	"github.com/gorilla/websocket"
)

type HttpOpUiData struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// HttpOpUiConfig_server 单个 HTTP 服务器配置（多开）
type HttpOpUiConfig_server struct {
	Server      string `json:"server"`          // 监听地址，如 0.0.0.0:8080
	Enabled     bool   `json:"enabled"`         // 是否启用（实时开关）
	TLS         bool   `json:"tls"`             // 是否启用 HTTPS
	CertFile    string `json:"cert_file"`       // 证书文件路径
	KeyFile     string `json:"key_file"`        // 密钥文件路径
	TLSMode     string `json:"tls_mode"`        // file/self/upload/system/acme
	TLSDomains  string `json:"tls_domains"`     // acme 域名列表（逗号分隔）
	TLSEmail    string `json:"tls_email"`       // acme 邮箱（可选）
	Domains     string `json:"domains"`         // 绑定域名（换行分隔多个）
	WebRoot     string `json:"web_root"`        // 映射目录（网站根目录）
	RouterFile  string `json:"router_file"`     // 路由词库文件（.n）
	Remark      string `json:"remark"`          // 备注（仅用于列表展示）
	Cors        bool   `json:"cors"`            // 跨域开关
	CorsOrigins string `json:"cors_origins"`    // 跨域白名单
	FrpOpen     bool   `json:"frp_open"`        // 是否启用 BeerWebFrp 穿透
	FrpAddr     string `json:"frp_server_addr"` // BeerWebFrp 服务端地址（ws/wss）
	FrpToken    string `json:"frp_token"`       // BeerWebFrp 隧道密钥
	FrpDebug    bool   `json:"frp_debug"`       // BeerWebFrp 调试日志
}

// HttpOpUiConfig_serverGlobal 全局共享配置（所有服务器共用）
type HttpOpUiConfig_serverGlobal struct {
	Debug               bool `json:"debug"` // 调试开关：控制打印词库缓存等调试信息
	TempCleanupInterval int  `json:"temp_cleanup_interval"`
}

// HttpOpUiConfig_servers 服务器列表 + 全局配置
type HttpOpUiConfig_servers struct {
	Servers []HttpOpUiConfig_server     `json:"servers"`
	Global  HttpOpUiConfig_serverGlobal `json:"global"`
	OS      string                      `json:"os"` // 服务器操作系统（windows/linux/darwin）
}

// splitServerDomains 将换行/逗号分隔的域名拆分为去空白、去空项后的列表
func splitServerDomains(s string) []string {
	out := make([]string, 0)
	for _, line := range strings.Split(s, "\n") {
		for _, d := range strings.Split(line, ",") {
			if d = strings.TrimSpace(d); d != "" {
				out = append(out, d)
			}
		}
	}
	return out
}

// serversNeedRestart 判断服务器配置变化是否需要热重启 HTTP 服务器（监听地址/证书/ACME 变化才需要）
func serversNeedRestart(old []*dto.ServerHTTP, new []HttpOpUiConfig_server) bool {
	if len(old) != len(new) {
		return true
	}
	for i, s := range new {
		o := old[i]
		if o == nil || o.Http == nil {
			return true
		}
		if o.Http.Addr != s.Server ||
			o.TLS != s.TLS ||
			o.TLSMode != s.TLSMode ||
			o.CertFile != s.CertFile ||
			o.KeyFile != s.KeyFile ||
			o.TLSDomains != s.TLSDomains ||
			o.TLSEmail != s.TLSEmail {
			return true
		}
	}
	return false
}

type HttpOpUiWebSocketItem struct {
	Addr     string `json:"addr"`
	Cors     bool   `json:"cors"`
	Open     bool   `json:"open"`
	Closable bool   `json:"closable"`
}

type HttpOpUiConfig_ngrok struct {
	Open       bool   `json:"open"`
	Token      string `json:"token"`
	Domain     string `json:"domain"`
	ServerAddr string `json:"server_addr"` // 要转发的本地 HTTP 服务器监听地址（空则主服务器）
}

type HttpOpUiConfig_opui struct {
	Open   bool   `json:"open"`
	Path   string `json:"path"`
	Secret string `json:"secret"`
	Cors   bool   `json:"cors"`
}

type HttpOpUiConfig_cloud_tool struct {
	Addr          string `json:"addr"`           // 星云云工具 WebSocket 连接地址（基础地址，账号会拼到路径 /:username）
	Connected     bool   `json:"connected"`      // 后端当前是否已连接云工具（get 返回）
	Reconnecting  bool   `json:"reconnecting"`   // 后端当前是否正在断线重连云工具（get 返回）
	Debug         bool   `json:"debug"`          // 是否开启云工具调试日志
	OfflineNotify bool   `json:"offline_notify"` // 云工具断开时是否向面板推送离线警告（来自云工具端，默认开启）
	MaxDevices    int    `json:"max_devices"`    // 当前账号最大在线设备数（来自云工具端，默认 1，0 表示无限制）
	Email         string `json:"email"`          // 当前账号绑定的邮箱（来自云工具端，空表示未绑定）
}

type HttpOpUiConfig_cloudtool_server struct {
	Open          bool   `json:"open"`           // 是否开启内置云工具服务端
	Addr          string `json:"addr"`           // 访问路径（不含前导 /）
	AllowRegister bool   `json:"allow_register"` // 是否允许任意账号注册
	DicDir        string `json:"dic_dir"`        // 云工具词库目录（相对 private/）
	LogoutSec     int    `json:"logout_sec"`     // 断开自动注销时长（秒），0 表示关闭
	Debug         bool   `json:"debug"`          // 调试打印
}

type HttpOpUiConfig_bg struct {
	Light HttpOpUiConfig_bgItem `json:"light"` // 亮色主题背景
	Dark  HttpOpUiConfig_bgItem `json:"dark"`  // 暗色主题背景
}

type HttpOpUiConfig_bgItem struct {
	Type  string `json:"type"`  // "url" | "local" | ""
	Data  string `json:"data"`  // URL 或 base64
	Color string `json:"color"` // 无背景图时的自定义背景色（如 #f5f5f5），留空表示默认
}

type HttpOpUiConfig_ftp struct {
	Open          bool   `json:"open"`
	Port          int    `json:"port"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	Debug         bool   `json:"debug"`
	Tls           bool   `json:"tls"`
	PasvPortStart int    `json:"pasv_port_start"`
	PasvPortEnd   int    `json:"pasv_port_end"`
}

type HttpOpUiConfig_sftp struct {
	Open     bool   `json:"open"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Debug    bool   `json:"debug"`
}

// ---------- 安全检测：登录事件追踪 ----------

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

	cloudToolMu            sync.Mutex
	cloudToolConn          *websocket.Conn
	cloudToolWriteMu       sync.Mutex
	cloudToolDebug         bool
	cloudToolSeq           uint64
	cloudToolPending       sync.Map // id(string) -> chan cloudToolMsg
	cloudToolUsername      string
	cloudToolToken         string
	cloudToolWantConn      bool        // 是否期望保持云工具连接（登录成功后 true，登出/主动断开 false）
	cloudToolReconnecting  atomic.Bool // 是否正在断线重连中，防止并发重连
	cloudToolOfflineNotify = true      // 云工具断开时是否推送离线警告（来自云工具端，默认开启）
	cloudToolMaxDevices    = 1         // 当前账号最大在线设备数（来自云工具端，默认 1，0 表示无限制）
	cloudToolEmail         = ""        // 当前账号绑定的邮箱（来自云工具端，空表示未绑定）

	cloudToolGen        uint64 // 连接代次，每次成功连接递增，用于断开时回收对应注入的云函数
	cloudToolInjectedMu sync.Mutex
	cloudToolInjected   = make(map[string]uint64) // 已注入的云函数名 -> 注入时的连接代次

	cloudToolFuncsMu  sync.RWMutex
	cloudToolFuncsMap = map[string]cloudFuncInfo{} // 云函数完整信息缓存（连接成功后由 list_funcs 写入，供前端复用）
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

// ---------- 云工具（NebulaCloudTool）后端代理 ----------
// 前端（OPUI）不再直连云工具，而是通过后端转发的这套 JSON 协议与 NebulaCloudTool 通信。

// cloudToolMsg 云工具 WebSocket 消息结构，与 NebulaCloudTool 端保持一致。
type cloudToolMsg struct {
	Type          string   `json:"type"`
	ID            string   `json:"id,omitempty"`
	Func          string   `json:"func,omitempty"`
	Data          string   `json:"data,omitempty"`
	Args          []string `json:"args,omitempty"`
	Msg           string   `json:"msg,omitempty"`
	OldPassword   string   `json:"old_password,omitempty"`
	NewPassword   string   `json:"new_password,omitempty"`
	NewUsername   string   `json:"new_username,omitempty"`
	OfflineNotify bool     `json:"offline_notify,omitempty"`
	Debug         string   `json:"debug,omitempty"`
	MaxDevices    int      `json:"max_devices,omitempty"`
	Email         string   `json:"email,omitempty"`
}

// broadcastCloudToolStatus 把云工具连接状态推送给所有面板客户端
func broadcastCloudToolStatus(connected bool) {
	data, _ := json.Marshal(map[string]any{
		"type": "cloud_tool_status",
		"data": map[string]any{
			"connected":    connected,
			"reconnecting": cloudToolReconnecting.Load(),
		},
	})
	broadcastOpuiNotify(data)
}

// broadcastCloudToolOffline 云工具异常断开时向所有面板客户端推送离线警告
func broadcastCloudToolOffline() {
	data, _ := json.Marshal(map[string]any{
		"type": "cloud_tool_offline",
		"data": map[string]any{"msg": "云工具连接已断开"},
	})
	broadcastOpuiNotify(data)
}

// cloudToolSelfAddr 返回本机内置云工具服务端的 WebSocket 地址（默认连接自己）。
// 地址 = ws://127.0.0.1:{HTTP端口}/{云工具服务端访问路径}。
func cloudToolSelfAddr() string {
	host := "127.0.0.1"
	port := "8080"
	if primary := dto.ServerConfig.Primary(); primary != nil && primary.Http != nil {
		if addr := primary.Http.Addr; addr != "" {
			if i := strings.LastIndex(addr, ":"); i != -1 {
				port = addr[i+1:]
			}
		}
	}
	path := "/cloudtool"
	if dto.ServerConfig.CloudTool != nil && dto.ServerConfig.CloudTool.Addr != "" {
		path = dto.ServerConfig.CloudTool.Addr
	}
	return "ws://" + host + ":" + port + path
}

// cloudToolAddr 读取 system.ini 中 [云工具] 连接地址，未配置时默认连接本机内置云工具服务端。
func cloudToolAddr() string {
	ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
	f, err := ff.LoadIni()
	if err != nil {
		return cloudToolSelfAddr()
	}
	addr := strings.TrimSpace(f.Section("云工具").Key("连接地址").String())
	if addr == "" {
		return cloudToolSelfAddr()
	}
	return addr
}

// cloudToolAuthTable 云工具登录凭据在全局数据库中的表名
const cloudToolAuthTable = "cloud_tool"

// cloudToolSaveAuth 把登录账号与 token 持久化到全局数据库，供后端重启后自动恢复连接
func cloudToolSaveAuth(username, token string) {
	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return
	}
	if err := dic_funcs.EnsureFsTable(db, cloudToolAuthTable); err != nil {
		return
	}
	now := time.Now().Unix()
	_, _ = db.Exec(`
		INSERT INTO "cloud_tool" (key, data, updated_at)
		VALUES ('username', ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			data = excluded.data,
			updated_at = excluded.updated_at
	`, username, now)
	_, _ = db.Exec(`
		INSERT INTO "cloud_tool" (key, data, updated_at)
		VALUES ('token', ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			data = excluded.data,
			updated_at = excluded.updated_at
	`, token, now)
}

// cloudToolClearAuth 清除持久化的账号与 token（登出或 token 失效时）
func cloudToolClearAuth() {
	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return
	}
	if err := dic_funcs.EnsureFsTable(db, cloudToolAuthTable); err != nil {
		return
	}
	_, _ = db.Exec(`DELETE FROM "cloud_tool" WHERE key IN ('username', 'token')`)
}

// cloudToolSavedAuth 读取持久化的账号与 token，供启动或断线重连时恢复连接
func cloudToolSavedAuth() (string, string) {
	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return "", ""
	}
	if err := dic_funcs.EnsureFsTable(db, cloudToolAuthTable); err != nil {
		return "", ""
	}

	var username, token string
	_ = db.QueryRow(`SELECT data FROM "cloud_tool" WHERE key='username'`).Scan(&username)
	_ = db.QueryRow(`SELECT data FROM "cloud_tool" WHERE key='token'`).Scan(&token)
	return strings.TrimSpace(username), strings.TrimSpace(token)
}

// cloudToolSetWantConn 设置是否期望保持云工具连接（主动断开时应设为 false，避免触发断线重连）
func cloudToolSetWantConn(want bool) {
	cloudToolMu.Lock()
	cloudToolWantConn = want
	cloudToolMu.Unlock()
}

// cloudToolBuildURL 将连接地址规范为完整 WebSocket 地址，账号作为路径 /:username。
// 若地址已包含路径前缀（如 /cloudtool），账号会拼到该前缀之后（/cloudtool/:username）。
func cloudToolBuildURL(addr, username string) string {
	if !strings.Contains(addr, "://") {
		addr = "ws://" + addr
	}
	// https/http 地址统一转为 wss/ws，保证 WebSocket 可连接
	if strings.HasPrefix(addr, "https://") {
		addr = "wss://" + strings.TrimPrefix(addr, "https://")
	} else if strings.HasPrefix(addr, "http://") {
		addr = "ws://" + strings.TrimPrefix(addr, "http://")
	}
	u, err := url.Parse(addr)
	if err != nil {
		return addr
	}
	base := strings.TrimRight(u.Path, "/")
	if base == "" {
		u.Path = "/" + url.PathEscape(username)
	} else {
		u.Path = base + "/" + url.PathEscape(username)
	}
	return u.String()
}

// cloudToolConnected 返回当前是否已连接云工具
func cloudToolConnected() bool {
	cloudToolMu.Lock()
	defer cloudToolMu.Unlock()
	return cloudToolConn != nil
}

// cloudToolGetOfflineNotify 返回云工具断开时是否推送离线警告（来自云工具端，默认开启）
func cloudToolGetOfflineNotify() bool {
	cloudToolMu.Lock()
	defer cloudToolMu.Unlock()
	return cloudToolOfflineNotify
}

// cloudToolGetMaxDevices 返回当前账号允许的最大在线设备数（来自云工具端，默认 1，0 表示无限制）
func cloudToolGetMaxDevices() int {
	cloudToolMu.Lock()
	defer cloudToolMu.Unlock()
	return cloudToolMaxDevices
}

// cloudToolGetEmail 返回当前账号绑定的邮箱（来自云工具端，空表示未绑定）
func cloudToolGetEmail() string {
	cloudToolMu.Lock()
	defer cloudToolMu.Unlock()
	return cloudToolEmail
}

// cloudToolDisconnect 断开与云工具的连接（不广播，由调用方决定是否广播）
func cloudToolDisconnect() {
	cloudToolMu.Lock()
	conn := cloudToolConn
	cloudToolConn = nil
	cloudToolUsername = ""
	cloudToolToken = ""
	cloudToolEmail = ""
	cloudToolMu.Unlock()
	if conn != nil {
		conn.Close()
	}
}

// cloudToolReadLoop 持续读取云工具响应，按 id 路由到等待中的请求；连接断开时清理状态并触发断线重连
func cloudToolReadLoop(conn *websocket.Conn, gen uint64) {
	// 客户端心跳：周期发送 ping 探测服务端存活，pong 到达即刷新读超时
	pingStop := make(chan struct{})
	defer close(pingStop)
	go func() {
		ticker := time.NewTicker(cloudPingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); err != nil {
					return
				}
			case <-pingStop:
				return
			}
		}
	}()

	defer func() {
		cloudToolMu.Lock()
		isCurrent := cloudToolConn == conn
		if isCurrent {
			cloudToolConn = nil
			cloudToolUsername = ""
			cloudToolToken = ""
			cloudToolEmail = ""
		}
		wantReconnect := isCurrent && cloudToolWantConn
		offlineNotify := cloudToolOfflineNotify
		cloudToolMu.Unlock()
		conn.Close()
		cloudToolPending.Range(func(key, value any) bool {
			if ch, ok := value.(chan cloudToolMsg); ok {
				ch <- cloudToolMsg{Type: "error", Msg: "云工具连接已断开"}
			}
			return true
		})
		broadcastCloudToolStatus(false)
		if wantReconnect && offlineNotify {
			broadcastCloudToolOffline()
		}
		clearInjectedCloudFuncs(gen) // 回收这条连接注入的云函数，断开后应不再存在
		if wantReconnect {
			go cloudToolReconnectLoop()
		}
	}()
	for {
		var msg cloudToolMsg
		if err := conn.ReadJSON(&msg); err != nil {
			// 连接断开（含服务端关闭）：交由断线重连循环持续重试恢复连接
			debugLog.Errorf("[云工具] 连接断开: %v", err)
			return
		}
		if cloudToolDebug {
			debugLog.Infof("[云工具] 收到: type=%s id=%s", msg.Type, msg.ID)
		}
		if msg.Type == "logout_ok" {
			// 服务端主动注销（显式登出或在线时间过短被注销）：停止重连并清除本地登录态，回到未登录状态
			cloudToolSetWantConn(false)
			cloudToolClearAuth()
			return
		}
		if msg.Type == "result" || msg.Type == "error" {
			if ch, ok := cloudToolPending.Load(msg.ID); ok {
				ch.(chan cloudToolMsg) <- msg
			}
		}
	}
}

// cloudToolReconnectLoop 断线后自动用持久化凭据重连，带指数退避，直到重连成功或主动停止
func cloudToolReconnectLoop() {
	if !cloudToolReconnecting.CompareAndSwap(false, true) {
		return // 已有重连协程在运行
	}
	// 进入重连状态，立即通知前端
	broadcastCloudToolStatus(false)
	defer func() {
		cloudToolReconnecting.Store(false)
		// 重连结束（成功或停止），同步最终连接状态给前端
		broadcastCloudToolStatus(cloudToolConnected())
	}()

	delay := time.Second
	for {
		cloudToolMu.Lock()
		want := cloudToolWantConn
		cloudToolMu.Unlock()
		if !want {
			return
		}

		username, token := cloudToolSavedAuth()
		if username == "" || token == "" {
			return
		}

		if _, _, _, _, err := cloudToolAuth(username, "", token); err == nil {
			return // 重连成功
		} else if cloudToolDebug {
			debugLog.Errorf("[云工具] 断线重连失败: %v", err)
		}

		if !cloudToolReconnectSleep(delay) {
			return // 等待期间被主动中断
		}
		if delay < 30*time.Second {
			delay *= 2
		}
	}
}

// cloudToolReconnectSleep 可中断的退避等待：期间若期望连接被置为 false 则立即返回 false。
func cloudToolReconnectSleep(d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		cloudToolMu.Lock()
		want := cloudToolWantConn
		cloudToolMu.Unlock()
		if !want {
			return false
		}
		time.Sleep(200 * time.Millisecond)
	}
	return true
}

// cloudToolAuth 建立到云工具的连接并完成认证（passwordHash 密码登录 / token 恢复登录）。
// 成功时返回 token（恢复登录原样返回传入 token）。
func cloudToolAuth(username, passwordHash, token string) (newToken string, offlineNotify bool, email string, debug string, err error) {
	cloudToolDisconnect()

	addr := cloudToolAddr()
	if addr == "" {
		return "", false, "", "", errors.New("云工具连接地址未配置")
	}

	conn, _, err := websocket.DefaultDialer.Dial(cloudToolBuildURL(addr, username), nil)
	if err != nil {
		return "", false, "", "", fmt.Errorf("连接云工具失败: %w", err)
	}

	var authMsg cloudToolMsg
	if token != "" {
		authMsg = cloudToolMsg{Type: "resume", Data: token}
	} else {
		authMsg = cloudToolMsg{Type: "auth", Data: passwordHash}
	}

	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := conn.WriteJSON(authMsg); err != nil {
		conn.Close()
		return "", false, "", "", fmt.Errorf("云工具认证发送失败: %w", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	var resp cloudToolMsg
	if err := conn.ReadJSON(&resp); err != nil {
		conn.Close()
		return "", false, "", "", fmt.Errorf("云工具认证读取失败: %w", err)
	}
	if resp.Type != "auth_ok" {
		conn.Close()
		if token != "" {
			// 恢复登录失败（token 过期/失效），清除持久化凭据
			cloudToolClearAuth()
		}
		return "", false, "", "", errors.New("云工具认证失败: " + resp.Msg)
	}
	// 连接后保持在线：以心跳保活探测服务端存活（收到 ping/pong 即刷新读超时），不做空闲超时断开
	conn.SetReadDeadline(time.Now().Add(cloudPongWait))
	conn.SetPingHandler(func(appData string) error {
		conn.SetReadDeadline(time.Now().Add(cloudPongWait))
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(10*time.Second))
	})
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(cloudPongWait))
	})
	_ = conn.SetWriteDeadline(time.Time{})

	notify := resp.OfflineNotify
	cloudToolMu.Lock()
	cloudToolConn = conn
	cloudToolUsername = username
	cloudToolToken = resp.Data
	cloudToolWantConn = true
	cloudToolOfflineNotify = notify
	cloudToolMaxDevices = resp.MaxDevices
	cloudToolEmail = resp.Email
	gen := atomic.AddUint64(&cloudToolGen, 1)
	cloudToolMu.Unlock()

	cloudToolSaveAuth(username, resp.Data)

	if cloudToolDebug {
		debugLog.Infof("[云工具] 已连接并认证: %s (%s)", addr, username)
	}
	go cloudToolReadLoop(conn, gen)
	broadcastCloudToolStatus(true)
	go injectAllCloudFuncs(gen)

	if token != "" {
		return token, notify, resp.Email, resp.Debug, nil
	}
	return resp.Data, notify, resp.Email, resp.Debug, nil
}

// cloudToolCall 发送一条带 id 的请求并等待对应 id 的响应
func cloudToolCall(msg cloudToolMsg, timeout time.Duration) (cloudToolMsg, error) {
	cloudToolMu.Lock()
	conn := cloudToolConn
	cloudToolMu.Unlock()
	if conn == nil {
		return cloudToolMsg{}, errors.New("云工具未连接")
	}

	id := fmt.Sprintf("%d", atomic.AddUint64(&cloudToolSeq, 1))
	msg.ID = id
	ch := make(chan cloudToolMsg, 1)
	cloudToolPending.Store(id, ch)
	defer cloudToolPending.Delete(id)

	cloudToolWriteMu.Lock()
	err := conn.WriteJSON(msg)
	cloudToolWriteMu.Unlock()
	if err != nil {
		return cloudToolMsg{}, fmt.Errorf("云工具发送失败: %w", err)
	}

	select {
	case res := <-ch:
		return res, nil
	case <-time.After(timeout):
		// 收不到响应视为连接堵塞：关闭当前连接触发断线重连，并返回错误中断后续执行
		conn.Close()
		return cloudToolMsg{}, errors.New("云工具等待返回超时")
	}
}

// cloudToolSend 发送一条无需响应的消息（如 logout）
func cloudToolSend(msg cloudToolMsg) error {
	cloudToolMu.Lock()
	conn := cloudToolConn
	cloudToolMu.Unlock()
	if conn == nil {
		return errors.New("云工具未连接")
	}
	cloudToolWriteMu.Lock()
	defer cloudToolWriteMu.Unlock()
	return conn.WriteJSON(msg)
}

// injectCloudFunc 把一个云工具云函数注入为 nebula 字典函数，供词库直接 $函数名$ 调用。
// rule 为该云函数声明的参数数量规则，注入后字典函数按同一规则校验并原样传递位置参数。
func injectCloudFunc(gen uint64, name, rule string) error {
	err := dic_funcs.Register(name, rule, func(d *dto.DicInputs) (any, error) {
		args := d.Inputs.StringAfterList(1) // 去掉第 0 位函数名占位，得到全部位置参数
		res, err := cloudToolCall(cloudToolMsg{Type: "exec", Func: name, Args: args}, 15*time.Second)
		if err != nil {
			return "", err
		}
		if res.Type == "error" {
			return "", errors.New(res.Msg)
		}
		return res.Data, nil
	})
	if err != nil {
		return err
	}
	cloudToolInjectedMu.Lock()
	cloudToolInjected[name] = gen
	cloudToolInjectedMu.Unlock()
	return nil
}

// cloudFuncInfo 云函数信息：参数规则、说明与每次扣除金额（与云工具 list_funcs 返回结构一致）。
type cloudFuncInfo struct {
	Rule  string `json:"rule"`
	Desc  string `json:"desc"`
	Price string `json:"price,omitempty"`
}

// fetchCloudFuncs 拉取云工具已注册的云函数完整信息并写入缓存，返回最新列表。
func fetchCloudFuncs() (map[string]cloudFuncInfo, bool) {
	res, err := cloudToolCall(cloudToolMsg{Type: "list_funcs"}, 15*time.Second)
	if err != nil {
		return nil, false
	}
	if res.Type != "result" {
		return nil, false
	}
	var infos map[string]cloudFuncInfo
	if err := json.Unmarshal([]byte(res.Data), &infos); err != nil {
		return nil, false
	}
	cloudToolFuncsMu.Lock()
	cloudToolFuncsMap = infos
	cloudToolFuncsMu.Unlock()
	return infos, true
}

// getCloudFuncs 返回缓存的云函数完整信息；缓存为空时返回 nil。
func getCloudFuncs() map[string]cloudFuncInfo {
	cloudToolFuncsMu.RLock()
	defer cloudToolFuncsMu.RUnlock()
	if len(cloudToolFuncsMap) == 0 {
		return nil
	}
	cp := make(map[string]cloudFuncInfo, len(cloudToolFuncsMap))
	for k, v := range cloudToolFuncsMap {
		cp[k] = v
	}
	return cp
}

// injectAllCloudFuncs 拉取云工具已注册的云函数并逐个注入为字典函数，同时缓存完整信息供前端复用。
// 每次连接成功（含断线重连）都会调用，重复注入已存在的函数会被忽略，新增函数会被补上。
func injectAllCloudFuncs(gen uint64) {
	infos, ok := fetchCloudFuncs()
	if !ok {
		return
	}
	for name, info := range infos {
		if name == "" {
			continue
		}
		_ = injectCloudFunc(gen, name, info.Rule) // 已存在会返回错误，忽略即可
	}
}

// clearInjectedCloudFuncs 注销指定代次连接注入的全部云函数，断开后这些函数应不再存在。
func clearInjectedCloudFuncs(gen uint64) {
	cloudToolInjectedMu.Lock()
	for name, g := range cloudToolInjected {
		if g == gen {
			dic_funcs.Unregister(name)
			delete(cloudToolInjected, name)
		}
	}
	cloudToolInjectedMu.Unlock()

	// 断开后清空云函数信息缓存，避免残留上一连接的数据
	cloudToolFuncsMu.Lock()
	cloudToolFuncsMap = map[string]cloudFuncInfo{}
	cloudToolFuncsMu.Unlock()
}

// StartCloudTool 启动时恢复云工具调试开关，并用上次登录持久化的账号与 token 自动恢复连接
func StartCloudTool() {
	// 注入状态查询回调，供字典函数 $云工具状态$ 判断是否已连接
	dto.CloudToolConnected = cloudToolConnected

	ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
	f, err := ff.LoadIni()
	if err != nil {
		return
	}
	cloudToolDebug = f.Section("云工具").Key("调试").MustBool(false)

	username, token := cloudToolSavedAuth()
	if username != "" && token != "" {
		cloudToolSetWantConn(true) // 期望保持连接，失败后交由断线重连循环持续重试
		go func() {
			if _, _, _, _, err := cloudToolAuth(username, "", token); err != nil {
				if cloudToolDebug {
					debugLog.Errorf("[云工具] 启动自动连接失败: %v", err)
				}
				go cloudToolReconnectLoop() // 失败后进入退避重试，直到云工具可用或凭据失效
			}
		}()
	}
}

// maxServerLogLines 面板回放日志时最多在内存中收集的最近行数（防止过大卡顿）
const maxServerLogLines = 20000

// serverLogPageSize 面板回放日志时每页默认返回的行数
const serverLogPageSize = 300

// serverLogDir 返回服务端日志目录（应用储存目录下的 database/log，绝对路径）
func serverLogDir() string {
	p := filepath.Join(utils.GetAppDir(), "database", "log")
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return p
}

// serverLogFq 返回当天对应的日志文件句柄（database/log/YYYYMMDD.txt，按天区分，不再建子目录）
func serverLogFq() *utils.FileQueue {
	now := time.Now()
	return utils.NewFileQueue(filepath.Join(serverLogDir(), now.Format("20060102")+".txt"))
}

// init 重定向标准输出，监听终端全部信息：写入日志文件并实时推送到 OPUI 面板
func init() {
	originalStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return
	}
	os.Stdout = w

	go func() {
		reader := bufio.NewReader(r)
		for {
			raw, readErr := reader.ReadString('\n')
			if len(raw) > 0 {
				// 回显到真实终端：还原 debugLog 转义的控制字符，让多行内容正常换行显示
				_, _ = originalStdout.WriteString(debugLog.UnescapeControlChars(raw))
				processServerLogLine(raw)
			}
			if readErr != nil {
				return
			}
		}
	}()
}

// processServerLogLine 处理一行终端输出：写入日志文件，仅当有客户端正在查看实时终端时才推送
func processServerLogLine(raw string) {
	line := strings.TrimRight(raw, "\r\n")
	if line == "" {
		return
	}
	level := parseLogLevel(line)

	// 追加写入当前小时的日志文件，作为面板历史回放的持久化来源
	serverLogFq().AppendToFile(line + "\n")

	// 无客户端查看实时终端时跳过序列化与推送，避免无谓的网络开销
	if !hasServerLogSubscriber() {
		return
	}
	data, _ := json.Marshal(map[string]string{
		"type":  "server_log",
		"level": level,
		"line":  line,
	})
	broadcastServerLog(data)
}

// listServerLogFiles 列出 database/log 下所有 .txt 日志文件，按路径升序（即时间升序）
func listServerLogFiles() []string {
	var files []string
	filepath.Walk(serverLogDir(), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), ".txt") {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files
}

// readFileTailLines 流式读取文件末尾最多 need 行（按时间升序返回）。
// 使用固定容量环形缓冲，内存占用始终有界，避免大日志文件一次性加载导致内存暴涨。
func readFileTailLines(path string, need int) []string {
	if need <= 0 {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	reader := bufio.NewReaderSize(f, 64*1024)
	ring := make([]string, need)
	head := 0   // 下一个写入位置
	filled := 0 // 环形缓冲中已保存的行数
	for {
		raw, readErr := reader.ReadString('\n')
		if len(raw) > 0 {
			line := strings.TrimRight(raw, "\r\n")
			if line != "" {
				ring[head] = line
				head = (head + 1) % need
				if filled < need {
					filled++
				}
			}
		}
		if readErr != nil {
			break
		}
	}

	// 从最旧到最新依次取出，保持时间升序
	start := head - filled
	if start < 0 {
		start += need
	}
	lines := make([]string, 0, filled)
	for i := 0; i < filled; i++ {
		lines = append(lines, ring[(start+i)%need])
	}
	return lines
}

// collectRecentLogLines 收集最近的日志行（时间升序），最多 maxLines 行
func collectRecentLogLines(maxLines int) []string {
	files := listServerLogFiles()
	var lines []string
	for i := len(files) - 1; i >= 0 && len(lines) < maxLines; i-- {
		need := maxLines - len(lines)
		fileLines := readFileTailLines(files[i], need)
		// 前置到结果前，保持整体时间升序
		lines = append(fileLines, lines...)
	}
	return lines
}

// readServerLogs 分页读取最近日志：跳过最新的 skip 行，返回其后 limit 行；hasMore 表示是否还有更早的日志
func readServerLogs(limit, skip int) ([]map[string]string, bool) {
	if limit <= 0 {
		limit = serverLogPageSize
	}
	if skip < 0 {
		skip = 0
	}
	lines := collectRecentLogLines(maxServerLogLines)
	total := len(lines)
	end := total - skip
	if end <= 0 {
		return []map[string]string{}, false
	}
	start := end - limit
	if start < 0 {
		start = 0
	}
	logs := make([]map[string]string, 0, end-start)
	for _, l := range lines[start:end] {
		logs = append(logs, map[string]string{"level": parseLogLevel(l), "line": l})
	}
	return logs, start > 0
}

// ClearServerLogs 清空当前（当天）日志文件，不影响历史日期的日志文件
func ClearServerLogs() {
	serverLogFq().DeleteFile()
}

// serverLogKeepDays 启动时保留的日志天数，超过该天数的旧日志文件会被清理
const serverLogKeepDays = 7

// ClearOldServerLogs 清理 database/log 下超过保留天数的旧日志文件（按 YYYYMMDD.txt 文件名判断日期）
func ClearOldServerLogs() {
	cutoff := time.Now().AddDate(0, 0, -serverLogKeepDays)
	for _, path := range listServerLogFiles() {
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if t, err := time.Parse("20060102", name); err == nil && t.Before(cutoff) {
			_ = os.Remove(path)
		}
	}
}

// runTerminalDic 运行专门监听终端触发的词库（private/system/terminal.n）。
// 前端实时终端输入作为触发词运行该词库，输出写入进程 stdout（已被 init 重定向到日志管道），实时回显到终端。
func runTerminalDic(input string) {
	dic, err := dic_dto.RunDic("private/system/terminal.n")
	if err != nil || dic == nil {
		fmt.Printf("[终端] 终端触发词库不可用: %v\n", err)
		return
	}
	defer dic.Close()

	if res := dic_api.Api.DicRun(dic, input); res != "" {
		fmt.Printf("%v\n", res)
	}
}

// parseLogLevel 从日志行首的 [Level] 提取级别，仅识别已知级别，其余默认 Info
func parseLogLevel(line string) string {
	if len(line) == 0 || line[0] != '[' {
		return "Info"
	}
	end := strings.IndexByte(line, ']')
	if end <= 1 {
		return "Info"
	}
	switch line[1:end] {
	case "Debug", "Info", "Warning", "Error":
		return line[1:end]
	default:
		return "Info"
	}
}

// GetOpuiOnlineClients 返回当前 OPUI 在线用户列表
func GetOpuiOnlineClients() []map[string]any {
	opuiNotifyClientsMu.Lock()
	defer opuiNotifyClientsMu.Unlock()
	list := make([]map[string]any, 0, len(opuiNotifyClients))
	for _, info := range opuiNotifyClients {
		list = append(list, map[string]any{
			"name":   info.IP,
			"type":   "opui",
			"online": true,
			"detail": "已连接 " + formatDuration(time.Since(info.Connected)),
		})
	}
	return list
}

// wsResponseWriter 实现 http.ResponseWriter，用于 WebSocket 消息处理时捕获输出
type wsResponseWriter struct {
	header http.Header
	buf    bytes.Buffer
	code   int
}

func (w *wsResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *wsResponseWriter) Write(b []byte) (int, error) {
	return w.buf.Write(b)
}

func (w *wsResponseWriter) WriteHeader(code int) {
	w.code = code
}

// ---------- IP 黑名单 ----------

const ipBlacklistFile = "private/system/ip_blacklist.json"

var (
	ipBlacklist   = make(map[string]bool)
	ipBlacklistMu sync.Mutex
)

func LoadIPBlacklist() {
	ff := utils.NewFileQueue(ipBlacklistFile)
	data, err := ff.ReadFromFile()
	if err != nil {
		return
	}
	var list []string
	if json.Unmarshal([]byte(data), &list) == nil {
		ipBlacklistMu.Lock()
		ipBlacklist = make(map[string]bool, len(list))
		for _, ip := range list {
			ipBlacklist[strings.TrimSpace(ip)] = true
		}
		ipBlacklistMu.Unlock()
	}
}

func saveIPBlacklist() {
	ipBlacklistMu.Lock()
	list := make([]string, 0, len(ipBlacklist))
	for ip := range ipBlacklist {
		list = append(list, ip)
	}
	ipBlacklistMu.Unlock()
	data, _ := json.Marshal(list)
	utils.NewFileQueue(ipBlacklistFile).WriteToFile(string(data))
}

// ---------- 防火墙（词库实现） ----------

// CheckFirewall IP黑名单 + 防火墙词库检查，返回 true 表示已拦截（已写入响应）
func CheckFirewall(w http.ResponseWriter, r *http.Request) bool {
	ip := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		ip = strings.Split(fwd, ",")[0]
	}
	if real := r.Header.Get("X-Real-IP"); real != "" {
		ip = real
	}
	ip = strings.TrimSpace(ip)
	if colon := strings.LastIndex(ip, ":"); colon > 0 && !strings.Contains(ip, ".") {
		// IPv6: [::1]:1234, 处理纯 IPv6
	} else if colon > 0 {
		ip = ip[:colon] // strip port
	}

	// IP 黑名单检查
	ipBlacklistMu.Lock()
	blocked := ipBlacklist[ip]
	ipBlacklistMu.Unlock()
	if blocked {
		http.Error(w, "403 Forbidden: IP is blacklisted", http.StatusForbidden)
		return true
	}

	// 防火墙词库检查
	ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
	cf, err := ff.LoadIni()
	if err != nil {
		return false
	}
	fwSec := cf.Section("防火墙")
	if fwSec == nil || !fwSec.Key("启用").MustBool(false) {
		return false
	}
	dicPath := fwSec.Key("词库").String()
	if dicPath == "" {
		return false
	}

	fileData, err := utils.NewFileQueue(dicPath).ReadFromFile()
	if err != nil {
		return false
	}

	dic := dic_dto.NewDic(dicPath, fileData)
	dic.Val.G.Set("IP", ip)
	dic.Val.G.Set("路径", r.URL.Path)
	dic.Val.G.Set("方法", r.Method)
	dic.Val.G.Set("UA", r.UserAgent())
	dic.Val.G.Set("请求头", fmt.Sprintf("%v", r.Header))

	result := dic_api.Api.DicRun(dic, "检查")
	if result != "" {
		// 对输出结果做最终变量插值，确保 %IP%、%路径% 等变量被正确替换
		result = utils.AnyToString(dic.Val.Text(result))
		if strings.HasPrefix(result, "放行") {
			return false
		}
		http.Error(w, result, http.StatusForbidden)
		return true
	}

	return false
}

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

	// frpRuntimes 每个服务器的 BeerWebFrp 隧道取消上下文（key 为服务器指针）
	frpRuntimesMu sync.Mutex
	frpRuntimes   = make(map[*dto.ServerHTTP]context.CancelFunc)

	// FTP 服务管理
	ftpListener    net.Listener
	ftpCancel      context.CancelFunc
	ftpUser        string // 配置的用户名
	ftpPass        string // 配置的密码
	ftpPasvPortMin int    // PASV 被动模式端口范围起始
	ftpPasvPortMax int    // PASV 被动模式端口范围结束

	// SFTP 服务管理
	sftpListener net.Listener
	sftpCancel   context.CancelFunc
	sftpUser     string // 配置的用户名
	sftpPass     string // 配置的密码

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
// 与 Ngrok 一致，将链接作为特殊触发 [BeerWebFrp] 交给启动词库 start.n
// 匹配并格式化输出（默认输出 “BeerWebFrp：<链接>”），支持用户自定义启动词库。
func printFrpLink(link string) {
	if link == "" {
		return
	}
	if dic, err := dic_dto.RunDic("private/system/start.n"); err == nil && dic != nil {
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
func StartServerFrp(server *dto.ServerHTTP) {
	if server == nil {
		return
	}
	StopServerFrp(server)

	addr := strings.TrimSpace(server.FrpServerAddr)
	token := strings.TrimSpace(server.FrpToken)

	if server.FrpDebug {
		debugLog.Infof("[FRP] 启动隧道, server=%s, addr=%s, token长度=%d (空则匿名)", server.Http.Addr, addr, len(token))
	}

	if !server.FrpOpen || addr == "" {
		if server.FrpDebug {
			debugLog.Infof("[FRP] 跳过启动: open=%v, addr为空=%v", server.FrpOpen, addr == "")
		}
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	frpRuntimesMu.Lock()
	frpRuntimes[server] = cancel
	frpRuntimesMu.Unlock()

	go runFrpConn(ctx, addr, token, frpLocalAddr(server.Http.Addr), server.FrpDebug)
}

// StopServerFrp 停止单个服务器的 BeerWebFrp 隧道
func StopServerFrp(server *dto.ServerHTTP) {
	if server == nil {
		return
	}
	frpRuntimesMu.Lock()
	cancel := frpRuntimes[server]
	delete(frpRuntimes, server)
	frpRuntimesMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// ReconnectAllFrp 停止所有服务器的 BeerWebFrp 隧道，再逐台启动已启用的服务器
func ReconnectAllFrp(servers []*dto.ServerHTTP) {
	frpRuntimesMu.Lock()
	for _, cancel := range frpRuntimes {
		if cancel != nil {
			cancel()
		}
	}
	frpRuntimes = make(map[*dto.ServerHTTP]context.CancelFunc)
	frpRuntimesMu.Unlock()

	closeAllFrpWsStreams()

	for _, s := range servers {
		StartServerFrp(s)
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

// StartFtp 启动 FTP 服务并打印局域网链接
func StartFtp(port int, debug bool, username, password string, tlsEnabled bool, pasvPortStart, pasvPortEnd int) {
	// 先停止旧的 FTP 服务
	StopFtp()

	prefix := "[FTP]"
	if tlsEnabled {
		prefix = "[FTPS]"
	}

	// 配置 TLS
	if tlsEnabled {
		tlsCfg, err := utils.GenerateSelfSignedTLS()
		if err != nil {
			debugLog.Errorf(prefix+" TLS 证书生成失败: %v", err)
		} else {
			ftpTlsConfig = tlsCfg
		}
	} else {
		ftpTlsConfig = nil
	}

	// 打印局域网链接
	lanIP := getLanIP()
	if lanIP != "127.0.0.1" {
		fmt.Printf("%s 局域网链接: ftp://%s:%d\n", prefix, lanIP, port)
	}

	ftpUser = username
	ftpPass = password

	// 设置 PASV 端口范围
	if pasvPortStart > 0 && pasvPortEnd > 0 && pasvPortStart <= pasvPortEnd {
		ftpPasvPortMin = pasvPortStart
		ftpPasvPortMax = pasvPortEnd
	} else {
		ftpPasvPortMin = 32000
		ftpPasvPortMax = 32005
	}

	if debug {
		debugLog.Infof(prefix+" FTP 服务已启动，端口: %d, PASV 端口范围: %d-%d", port, ftpPasvPortMin, ftpPasvPortMax)
		debugLog.Infof(prefix+" 根目录映射: %s", utils.FtpDir())
	}

	ctx, cancel := context.WithCancel(context.Background())
	ftpCancel = cancel

	// 异步启动 FTP 服务端
	go func() {
		if err := runFtpServer(ctx, port, debug); err != nil {
			debugLog.Errorf(prefix+" 服务异常: %v", err)
		}
	}()
}

// StopFtp 停止 FTP 服务
func StopFtp() {
	if ftpCancel != nil {
		ftpCancel()
		ftpCancel = nil
	}
	if ftpListener != nil {
		ftpListener.Close()
		ftpListener = nil
	}
}

// StartSftp 启动 SFTP 服务并打印局域网链接
func StartSftp(port int, debug bool, username, password string) {
	// 先停止旧的 SFTP 服务
	StopSftp()

	// 打印局域网链接
	lanIP := getLanIP()
	if lanIP != "127.0.0.1" {
		fmt.Printf("[SFTP] 局域网链接: sftp://%s:%d\n", lanIP, port)
	}

	sftpUser = username
	sftpPass = password

	if debug {
		debugLog.Infof("[SFTP] SFTP 服务已启动，端口: %d", port)
		debugLog.Infof("[SFTP] 根目录映射: %s", utils.FtpDir())
	}

	ctx, cancel := context.WithCancel(context.Background())
	sftpCancel = cancel

	// 异步启动 SFTP 服务端
	go func() {
		if err := runSftpServer(ctx, port, debug); err != nil {
			debugLog.Errorf("[SFTP] 服务异常: %v", err)
		}
	}()
}

// StopSftp 停止 SFTP 服务
func StopSftp() {
	if sftpCancel != nil {
		sftpCancel()
		sftpCancel = nil
	}
	if sftpListener != nil {
		sftpListener.Close()
		sftpListener = nil
	}
}

type HttpOpUiConfig_qq struct {
	Open        bool   `json:"open"`
	Dic         string `json:"dic"`
	Path        string `json:"path"`
	Appid       string `json:"appid"`
	Secret      string `json:"secret"`
	AtCompat    bool   `json:"at_compat"`
	FilterSlash bool   `json:"filter_slash"`
	Debug       bool   `json:"debug"`
	Ws          bool   `json:"ws"`
	WsIntents   int    `json:"ws_intents"`
	Remark      string `json:"remark"`
	BotName     string `json:"bot_name"`
	BotAvatar   string `json:"bot_avatar"`
	Robot       string `json:"robot"`
	Connected   bool   `json:"connected"`
}

type HttpOpUiConfig_qq_instance struct {
	Section string            `json:"section"`
	Config  HttpOpUiConfig_qq `json:"config"`
}

type HttpOpUiConfig_qq_list struct {
	Instances []HttpOpUiConfig_qq_instance `json:"instances"`
}

// qqBotInfoCache 是 botinfo.json 的最小字段，用于重启后兜底恢复机器人头像昵称。
type qqBotInfoCache struct {
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
}

// loadQQBotInfoCache 从机器人词库目录下的 botinfo.json 读取缓存的头像昵称。
func loadQQBotInfoCache(filePath string) *qqBotInfoCache {
	if filePath == "" {
		return nil
	}
	p := filepath.Join(filePath, "botinfo.json")
	if !filepath.IsAbs(p) {
		p = filepath.Join(utils.GetAppDir(), p)
	}
	var info qqBotInfoCache
	if data, err := os.ReadFile(p); err == nil && json.Unmarshal(data, &info) == nil {
		return &info
	}
	return nil
}

type HttpOpUiConfig_napcat struct {
	Open   bool   `json:"open"`
	Dic    string `json:"dic"`
	Path   string `json:"path"`
	Api    string `json:"api"`
	Secret string `json:"secret"`
}

type HttpOpUiConfig_yunhu struct {
	Open   bool   `json:"open"`
	Dic    string `json:"dic"`
	Path   string `json:"path"`
	Secret string `json:"secret"`
}

type HttpOpUiConfig_feishu struct {
	Open   bool   `json:"open"`
	Dic    string `json:"dic"`
	Path   string `json:"path"`
	Appid  string `json:"appid"`
	Secret string `json:"secret"`
}

type HttpOpUiConfig_secluded struct {
	Open    bool   `json:"open"`
	Dic     string `json:"dic"`
	Address string `json:"address"`
	Token   string `json:"token"`
	Debug   bool   `json:"debug"`
}

type HttpOpUiConfig_install struct {
	Component string            `json:"component"`
	Params    map[string]string `json:"params,omitempty"`
}

type HttpOpUiInstallResponse struct {
	Status string   `json:"status"`
	Output []string `json:"output,omitempty"`
	Error  string   `json:"error,omitempty"`
}

type HttpOpUiConfig_installStatus struct {
	Component string `json:"component"`
	TaskID    string `json:"task_id,omitempty"`
}

type HttpOpUiInstallStatusResponse struct {
	Installed bool     `json:"installed"`
	Output    []string `json:"output,omitempty"`
	Status    string   `json:"status,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// ==================== 异步安装任务管理 ====================

type InstallTask struct {
	ID        string   `json:"id"`
	Component string   `json:"component"`
	Status    string   `json:"status"` // "running", "completed", "failed", "cancelled"
	Output    []string `json:"output"`
	Error     string   `json:"error,omitempty"`
	Progress  float64  `json:"progress"` // 下载进度 0-100

	cancelled bool
	mu        sync.RWMutex
}

func (t *InstallTask) Cancel() {
	t.mu.Lock()
	t.cancelled = true
	t.mu.Unlock()
}

func (t *InstallTask) IsCancelled() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cancelled
}

func (t *InstallTask) addOutput(msg string) {
	t.mu.Lock()
	t.Output = append(t.Output, msg)
	t.mu.Unlock()
}

func (t *InstallTask) setProgress(p float64) {
	t.mu.Lock()
	if p > 100 {
		p = 100
	}
	t.Progress = p
	t.mu.Unlock()
}

func (t *InstallTask) snapshot() (status string, output []string, errMsg string, progress float64) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	status = t.Status
	output = make([]string, len(t.Output))
	copy(output, t.Output)
	errMsg = t.Error
	progress = t.Progress
	return
}

func (t *InstallTask) finish(err error) {
	t.mu.Lock()
	if t.cancelled {
		t.Status = "cancelled"
		t.Error = "用户取消"
	} else if err != nil {
		t.Status = "failed"
		t.Error = err.Error()
	} else {
		t.Status = "completed"
		t.Progress = 100
	}
	t.mu.Unlock()
	// 3 分钟后自动清理，给前端足够时间轮询到最终状态
	time.AfterFunc(3*time.Minute, func() {
		installTaskStore.Delete(t.ID)
	})
}

var installTaskStore sync.Map // map[string]*InstallTask

func generateTaskID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// findRunningTaskForComponent 检查指定组件是否已有正在运行的任务
func findRunningTaskForComponent(component string) *InstallTask {
	var found *InstallTask
	installTaskStore.Range(func(key, value any) bool {
		task, ok := value.(*InstallTask)
		if !ok {
			return true
		}
		task.mu.RLock()
		status := task.Status
		comp := task.Component
		task.mu.RUnlock()
		if comp == component && (status == "running") {
			found = task
			return false
		}
		return true
	})
	return found
}

func opuiCheckKey(r *http.Request, hType string) bool {
	if hType == "check_opui_key" || hType == "get_opui" || hType == "get_bg" {
		return true // 密钥校验和查询配置状态免鉴权
	}
	ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
	f, err := ff.LoadIni()
	if err != nil {
		return false
	}
	storedKey := f.Section("管理面板").Key("密钥").String()
	if storedKey == "" {
		return true // 未配置密钥，放行
	}
	reqKey := r.Header.Get("X-OPUI-Key")
	return reqKey == storedKey
}

// toDataURI 把图片字节转为 base64 data URI（带浏览器可识别的图片类型）
func toDataURI(data []byte) string {
	return "data:" + http.DetectContentType(data) + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// resolveImgSrc 把 ±img= 的值解析为浏览器可直接显示的图片地址：
// http(s)/data: 原样返回；本地文件路径读取后转 data URI；纯 base64 图片数据也转 data URI
func resolveImgSrc(src string) string {
	src = strings.TrimSpace(src)
	if src == "" {
		return src
	}
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") || strings.HasPrefix(src, "data:") {
		return src
	}
	// 尝试作为本地文件（相对路径基于应用目录）
	if data, err := utils.NewFileQueue(src).ReadFile(); err == nil {
		return toDataURI([]byte(data))
	}
	// 尝试作为 base64 图片数据（绘图等函数输出的纯 base64 字符串）
	if dec, err := base64.StdEncoding.DecodeString(src); err == nil && len(dec) > 4 {
		if ct := http.DetectContentType(dec); strings.HasPrefix(ct, "image/") {
			return toDataURI(dec)
		}
	}
	return src
}

// isImageData 判断字节数据是否为常见图片格式（按文件头魔数识别）
func isImageData(data []byte) bool {
	if len(data) < 8 {
		return false
	}
	ct := http.DetectContentType(data)
	return strings.HasPrefix(ct, "image/")
}

// imageMagics 常见图片格式的文件头魔数（用于在文本中扫描嵌入的图片字节）
var imageMagics = [][]byte{
	{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, // PNG
	{0xFF, 0xD8, 0xFF},                            // JPEG
	{'G', 'I', 'F', '8', '7', 'a'},                // GIF
	{'G', 'I', 'F', '8', '9', 'a'},                // GIF
	{0x00, 0x00, 0x01, 0x00},                      // ICO
	{'B', 'M'},                                    // BMP
}

// findImageStart 在字节流中查找第一个图片数据（文件头魔数）的起始位置，找不到返回 -1
func findImageStart(data []byte) int {
	for i := range len(data) {
		// WebP：RIFF + 4 字节长度 + WEBP
		if i+12 <= len(data) && data[i] == 'R' &&
			bytes.Equal(data[i:i+4], []byte("RIFF")) && bytes.Equal(data[i+8:i+12], []byte("WEBP")) {
			return i
		}
		for _, magic := range imageMagics {
			if i+len(magic) <= len(data) && bytes.Equal(data[i:i+len(magic)], magic) {
				// 短魔数（BMP 仅 2 字节）用内容嗅探二次确认，避免文本误报
				if len(magic) >= 4 || isImageData(data[i:]) {
					return i
				}
			}
		}
	}
	return -1
}

// findImageEnd 返回从 start 开始图片数据的结束位置（不含）。无法确定结束位置时返回 len(data)
func findImageEnd(data []byte, start int) int {
	rest := data[start:]
	switch {
	case bytes.HasPrefix(rest, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}):
		// PNG 以 IEND chunk（00 00 00 00 49 45 4E 44 AE 42 60 82）结束
		if idx := bytes.Index(rest, []byte{0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82}); idx >= 0 {
			return start + idx + 12
		}
	case bytes.HasPrefix(rest, []byte{0xFF, 0xD8, 0xFF}):
		// JPEG 以 FFD9 结束
		if idx := bytes.Index(rest, []byte{0xFF, 0xD9}); idx >= 0 {
			return start + idx + 2
		}
	case bytes.HasPrefix(rest, []byte("GIF87a")) || bytes.HasPrefix(rest, []byte("GIF89a")):
		// GIF 以 0x3B 结束
		if idx := bytes.IndexByte(rest, 0x3B); idx >= 0 {
			return start + idx + 1
		}
	case bytes.HasPrefix(rest, []byte("RIFF")) && len(rest) >= 12 && bytes.Equal(rest[8:12], []byte("WEBP")):
		// WebP：RIFF 头部 4~7 字节为整个文件长度（含 8 字节头）
		size := int(rest[4]) | int(rest[5])<<8 | int(rest[6])<<16 | int(rest[7])<<24
		if size >= 8 && len(rest) >= 8+size {
			return start + 8 + size
		}
	}
	return len(data)
}

// anyTypeName 返回变量值的类型名（中文，用于词库调试面板展示）
func anyTypeName(v any) string {
	switch v.(type) {
	case string:
		return "字符串"
	case bool:
		return "布尔"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return "数值"
	case time.Time:
		return "时间"
	case []byte:
		return "字节"
	case *dic_funcs.NDrawImg:
		return "画布"
	case nil:
		return "空"
	}
	// 兜底：按反射归类，避免常见类型（结构体、自定义切片/字典、指针等）显示为「未知」
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer:
		if rv.IsNil() {
			return "空"
		}
		return anyTypeName(rv.Elem().Interface())
	case reflect.Slice, reflect.Array:
		return "数组"
	case reflect.Map:
		return "字典"
	case reflect.Struct:
		return "对象"
	default:
		return "未知"
	}
}

// varDebugItem 将变量值转换为调试展示结构；类实例额外携带成员变量，供前端折叠展示
func varDebugItem(v any) map[string]any {
	return varDebugItemDepth(v, 0)
}

// varDebugItemDepth 递归构建变量调试结构，限制深度避免循环引用
func varDebugItemDepth(v any, depth int) map[string]any {
	// 类实例：展示为「类」类型，值用变量数量概括，成员变量放入 children 供前端折叠
	if cls, ok := v.(*dto.DicClass); ok {
		item := map[string]any{"t": "类", "v": "类实例"}
		if cls != nil && cls.LocalValue != nil {
			members := cls.LocalValue.GetAll()
			item["v"] = fmt.Sprintf("类实例（%d 个变量）", len(members))
			if depth < 3 && len(members) > 0 {
				children := make(map[string]any, len(members))
				for k, cv := range members {
					children[k] = varDebugItemDepth(cv, depth+1)
				}
				item["children"] = children
			}
		}
		return item
	}
	// 字符串：词库变量多为字符串存储，尝试智能识别类型（布尔/数值/JSON），便于调试展示
	if s, ok := v.(string); ok {
		return stringDebugItem(s, depth)
	}
	return map[string]any{
		"v": utils.AnyToString(v),
		"t": anyTypeName(v),
	}
}

// stringDebugItem 对字符串变量做类型识别：布尔/数值/JSON 对象/数组，其余按字符串展示
func stringDebugItem(s string, depth int) map[string]any {
	if s == "true" || s == "false" {
		return map[string]any{"v": s, "t": "布尔"}
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return map[string]any{"v": s, "t": "数值"}
	}
	if depth < 3 {
		if j := utils.IsJSONResult(s); j != nil {
			switch jv := j.(type) {
			case map[string]any:
				item := map[string]any{"v": s, "t": "对象"}
				if len(jv) > 0 {
					children := make(map[string]any, len(jv))
					for k, cv := range jv {
						children[k] = varDebugItemDepth(cv, depth+1)
					}
					item["children"] = children
				}
				return item
			case []any:
				item := map[string]any{"v": s, "t": "数组"}
				if len(jv) > 0 {
					children := make(map[string]any, len(jv))
					for i, cv := range jv {
						children[strconv.Itoa(i)] = varDebugItemDepth(cv, depth+1)
					}
					item["children"] = children
				}
				return item
			}
		}
	}
	return map[string]any{"v": s, "t": "字符串"}
}

// outputImgRe 匹配输出中的图片标记：±img=xxx± / <img src=...> / ![alt](url)
var outputImgRe = regexp.MustCompile(`±img=([^±]+)±|<img[^>]*\bsrc=["']([^"']+)["'][^>]*>|!\[[^\]]*\]\(([^)\s]+)\)`)

// parseOutputSegments 把词库输出中的图片标记解析为分段（文本/图片），
// 图片源解析为浏览器可直接显示的地址；同时自动识别直接输出的图片二进制数据
// （如 $画布.获取$ 返回的 PNG/JPEG 字节）
func parseOutputSegments(output string) []map[string]string {
	var segments []map[string]string

	// appendText 追加文本段，若该段本身是图片二进制或嵌入了图片字节则识别为图片
	appendText := func(text string) {
		if text == "" {
			return
		}
		// 整段本身就是图片二进制数据
		if isImageData([]byte(text)) {
			segments = append(segments, map[string]string{"type": "img", "src": toDataURI([]byte(text))})
			return
		}
		// 文本中嵌入图片字节（如 $画布.获取$ 输出带前缀文本）：按魔数拆分
		data := []byte(text)
		for {
			imgStart := findImageStart(data)
			if imgStart < 0 {
				if len(data) > 0 {
					segments = append(segments, map[string]string{"type": "text", "text": string(data)})
				}
				return
			}
			if imgStart > 0 {
				segments = append(segments, map[string]string{"type": "text", "text": string(data[:imgStart])})
			}
			imgEnd := findImageEnd(data, imgStart)
			segments = append(segments, map[string]string{"type": "img", "src": toDataURI(data[imgStart:imgEnd])})
			if imgEnd >= len(data) {
				return
			}
			data = data[imgEnd:]
		}
	}

	last := 0
	for _, m := range outputImgRe.FindAllStringSubmatchIndex(output, -1) {
		start, end := m[0], m[1]
		if start > last {
			appendText(output[last:start])
		}
		var src string
		switch {
		case m[2] >= 0: // ±img=xxx±
			src = output[m[2]:m[3]]
		case m[4] >= 0: // <img src="...">
			src = output[m[4]:m[5]]
		case m[6] >= 0: // ![alt](url)
			src = output[m[6]:m[7]]
		}
		segments = append(segments, map[string]string{"type": "img", "src": resolveImgSrc(src)})
		last = end
	}
	if last < len(output) {
		appendText(output[last:])
	}
	if len(segments) == 0 {
		appendText(output)
	}
	return segments
}

// loadDicDebugDefaults 读取 system.ini 中 [词库调试] 节的配置（运行配置的唯一存储位置）
func loadDicDebugDefaults() map[string]any {
	def := map[string]any{}
	file := utils.NewFile()
	file.SetPath("private/system/system.ini")
	if !file.FileExists() {
		return def
	}
	ini, err := file.LoadIni()
	if err != nil {
		return def
	}
	sec := ini.Section("词库调试")
	if v := sec.Key("默认词库").String(); v != "" {
		def["path"] = v
	}
	if v := sec.Key("打开的标签").String(); v != "" {
		var tabs []string
		if err := json.Unmarshal([]byte(v), &tabs); err == nil {
			def["tabs"] = tabs
		}
	}
	if v := sec.Key("触发文本").String(); v != "" {
		def["trigger"] = v
	}
	if b, err := sec.Key("保存运行").Bool(); err == nil {
		def["saveRun"] = b
	}
	if b, err := sec.Key("实时保存").Bool(); err == nil {
		def["autoSave"] = b
	}
	if n := sec.Key("超时").MustInt(0); n > 0 {
		def["timeout"] = n
	}
	if n := sec.Key("历史记录数量").MustInt(0); n > 0 {
		def["historyMax"] = n
	}
	if v := sec.Key("全局变量").String(); v != "" {
		// 值可含任意换行，因此整体按 JSON 数组存储；解析失败时兼容旧格式（每行一个 key=value）
		var g []string
		if err := json.Unmarshal([]byte(v), &g); err == nil {
			// JSON 数组解析成功；注意 "null" 字面量会解析成 nil 切片，视为空配置
			if g != nil {
				def["g"] = g
			}
		} else {
			// 旧格式：每行一个 key=value
			g = nil
			for line := range strings.SplitSeq(v, "\n") {
				if s := strings.TrimSpace(line); s != "" {
					g = append(g, s)
				}
			}
			if g != nil {
				def["g"] = g
			}
		}
	}
	return def
}

// checkDicPath 校验词库调试路径：仅允许应用目录内相对路径的 .n 文件，
// 拒绝绝对路径、包含 .. 的越权路径以及非词库文件
func checkDicPath(path string) bool {
	if path == "" || filepath.IsAbs(path) {
		return false
	}
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "\\") {
		return false
	}
	if strings.Contains(path, "..") {
		return false
	}
	if !strings.HasSuffix(strings.ToLower(path), ".n") {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

// checkFilePath 校验文件管理路径：仅允许应用目录内的相对路径，
// 拒绝绝对路径、包含 .. 的越权路径
func checkFilePath(path string) bool {
	if path == "" || filepath.IsAbs(path) {
		return false
	}
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "\\") {
		return false
	}
	if strings.Contains(path, "..") {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

// openSqliteOpui 打开应用目录下的 SQLite 数据库文件（modernc 纯 Go 驱动），
// 设置忙碌等待与单连接，避免与词库写入并发时出现 database is locked
func openSqliteOpui(full string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", full)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// opuiOpenSqlite 校验相对路径并打开应用目录下的 SQLite 数据库，失败时写出错误响应并返回 nil
func opuiOpenSqlite(w http.ResponseWriter, path string) *sql.DB {
	if !checkFilePath(path) {
		http.Error(w, `{"status":"error","error":"文件路径不合法"}`, http.StatusBadRequest)
		return nil
	}
	full := filepath.Join(utils.GetAppDir(), filepath.FromSlash(path))
	db, err := openSqliteOpui(full)
	if err != nil {
		http.Error(w, `{"status":"error","error":"数据库打开失败: `+err.Error()+`"}`, http.StatusBadRequest)
		return nil
	}
	return db
}

// sqliteCellValue 将 SQLite 扫描值转换为可 JSON 序列化的展示值：
// BLOB 可打印时按文本展示，否则转 hex；时间转为固定格式字符串
func sqliteCellValue(v any) any {
	switch t := v.(type) {
	case nil:
		return nil
	case []byte:
		if utf8.Valid(t) {
			return string(t)
		}
		return "0x" + hex.EncodeToString(t)
	case time.Time:
		return t.Format("2006-01-02 15:04:05")
	default:
		return v
	}
}

// isSimpleIdent 校验 SQLite 标识符（表名/列名）仅为字母数字下划线，
// 用于内联编辑时安全拼接 UPDATE 语句，避免 SQL 注入
func isSimpleIdent(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}

// stripLeadingSQLComments 去掉 SQL 开头的注释与空白，用于准确判断是否为查询语句，
// 避免带注释（如 "-- 说明\nSELECT ..."）的查询被误判为写语句而丢弃结果
func stripLeadingSQLComments(s string) string {
	s = strings.TrimSpace(s)
	for {
		if strings.HasPrefix(s, "--") {
			if i := strings.IndexByte(s, '\n'); i >= 0 {
				s = strings.TrimSpace(s[i+1:])
				continue
			}
			return ""
		}
		if strings.HasPrefix(s, "/*") {
			if i := strings.Index(s, "*/"); i >= 0 {
				s = strings.TrimSpace(s[i+2:])
				continue
			}
			return ""
		}
		return s
	}
}

// sqlAggRe 匹配聚合函数调用（要求函数名前为标识符边界，避免误伤 my_count() 等自定义函数）
var sqlAggRe = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_])(COUNT|SUM|AVG|MIN|MAX|TOTAL|GROUP_CONCAT)\s*\(`)

// isIdentByte 判断字节是否为 SQL 标识符字符（字母/数字/下划线）
func isIdentByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}

// findTopLevelKeyword 在跳过字符串、引号标识符、方括号标识符、注释与括号嵌套的前提下，
// 定位首个位于最外层的关键词（不区分大小写），返回其字节偏移，未找到返回 -1
func findTopLevelKeyword(s, kw string) int {
	depth := 0
	for i := 0; i < len(s); {
		c := s[i]
		switch c {
		case '\'', '"', '`':
			// 跳过字符串/引号标识符，处理双写转义
			q := c
			i++
			for i < len(s) {
				if s[i] == q {
					if q == '\'' && i+1 < len(s) && s[i+1] == '\'' {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
		case '[':
			if j := strings.IndexByte(s[i+1:], ']'); j >= 0 {
				i += j + 2
			} else {
				i++
			}
		case '(':
			depth++
			i++
		case ')':
			depth--
			i++
		case '-':
			if i+1 < len(s) && s[i+1] == '-' {
				for i < len(s) && s[i] != '\n' {
					i++
				}
			} else {
				i++
			}
		case '/':
			if i+1 < len(s) && s[i+1] == '*' {
				i += 2
				for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
					i++
				}
				i += 2
			} else {
				i++
			}
		default:
			if depth == 0 && i+len(kw) <= len(s) {
				beforeOK := i == 0 || !isIdentByte(s[i-1])
				afterOK := i+len(kw) >= len(s) || !isIdentByte(s[i+len(kw)])
				if beforeOK && afterOK && strings.EqualFold(s[i:i+len(kw)], kw) {
					return i
				}
			}
			i++
		}
	}
	return -1
}

// extractLeadingIdent 提取字符串开头的第一个标识符（支持裸标识符及引号/反引号/方括号包裹），
// 用于读取 FROM 子句中的表名
func extractLeadingIdent(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	switch s[0] {
	case '"', '`':
		q := s[0]
		for i := 1; i < len(s); i++ {
			if s[i] == q {
				if i+1 < len(s) && s[i+1] == q {
					i++
					continue
				}
				return s[1:i]
			}
		}
		return ""
	case '[':
		if i := strings.IndexByte(s[1:], ']'); i >= 0 {
			return s[1 : 1+i]
		}
		return ""
	default:
		i := 0
		for i < len(s) && isIdentByte(s[i]) {
			i++
		}
		return s[:i]
	}
}

// detectEditableTable 判断 SQL 是否为可安全附加 rowid 的「单表 SELECT」并返回表名。
// 多表连接、分组、去重、联合、聚合等无法精确定位行的查询返回空。
func detectEditableTable(sql string) (string, bool) {
	s := stripLeadingSQLComments(sql)
	upper := strings.ToUpper(s)
	if !strings.HasPrefix(upper, "SELECT") {
		return "", false
	}
	if sqlAggRe.MatchString(upper) ||
		strings.Contains(upper, " JOIN ") ||
		strings.Contains(upper, " GROUP BY") ||
		strings.Contains(upper, " HAVING") ||
		strings.Contains(upper, " UNION ") ||
		strings.Contains(upper, " DISTINCT") {
		return "", false
	}
	from := findTopLevelKeyword(s, "FROM")
	if from < 0 {
		return "", false
	}
	table := extractLeadingIdent(s[from+len("FROM"):])
	if table == "" || !isSimpleIdent(table) {
		return "", false
	}
	return table, true
}

// prependRowid 在 SELECT（及可选 DISTINCT/ALL 修饰符）之后插入 rowid 别名列，
// 供内联编辑按 rowid 精确定位行
func prependRowid(sql string) string {
	from := findTopLevelKeyword(sql, "FROM")
	head := strings.TrimSpace(sql[:from])
	tail := " " + strings.TrimSpace(sql[from:])
	pos := len("SELECT")
	rest := strings.TrimLeft(head[pos:], " \t")
	pos += len(head[pos:]) - len(rest)
	up := strings.ToUpper(rest)
	for _, kw := range []string{"DISTINCT", "ALL"} {
		if strings.HasPrefix(up, kw) && (len(rest) == len(kw) || !isIdentByte(rest[len(kw)])) {
			pos += len(kw)
			break
		}
	}
	return head[:pos] + ` rowid AS "_nbid",` + head[pos:] + tail
}

// isRowidTable 判断给定表名是否为可编辑的 rowid 表（排除视图与 WITHOUT ROWID 表）
func isRowidTable(db *sql.DB, name string) bool {
	var typ string
	var ddl *string
	if err := db.QueryRow(`SELECT type, sql FROM sqlite_master WHERE name = ?`, name).Scan(&typ, &ddl); err != nil {
		return false
	}
	if typ != "table" {
		return false
	}
	return ddl == nil || !strings.Contains(strings.ToLower(*ddl), "without rowid")
}

// copyPath 递归复制文件或目录，目标路径不存在时创建并保留原权限
func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return copyFile(src, dst, info.Mode().Perm())
	}
	if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := copyPath(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// copyFile 复制单个文件并保留权限
func copyFile(src, dst string, perm os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, perm)
}

// uploadTmpDir 返回分块上传的临时目录（按目标路径取 md5 避免冲突）
func uploadTmpDir(path string) string {
	sum := md5.Sum([]byte(path))
	return filepath.Join(os.TempDir(), "nebula_upload_"+fmt.Sprintf("%x", sum[:]))
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

				// check_opui_key 在连接内处理：验证密钥并标记认证状态
				if wsMsg.Type == "check_opui_key" {
					var authReq struct {
						Key string `json:"key"`
					}
					valid := false
					if json.Unmarshal(wsMsg.Data, &authReq) == nil && authReq.Key != "" {
						ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
						f, err := ff.LoadIni()
						if err == nil {
							storedKey := f.Section("管理面板").Key("密钥").String()
							if storedKey == "" || storedKey == authReq.Key {
								authenticatedKey = authReq.Key
								valid = true
								clientIP := utils.GetClientIP(r)
								addLoginEvent("admin_login", "OPUI 管理员登录成功", clientIP)
							} else {
								clientIP := utils.GetClientIP(r)
								addLoginEvent("admin_login_fail", "OPUI 登录失败: 密钥错误", clientIP)
							}
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
		http.Redirect(w, r, dto.ServerConfig.OPUI.Addr+"/", http.StatusFound)
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

	// 扩展部署：php/ffmpeg/silk_v3/napcat 仅支持 Windows，python 全平台支持
	switch h.Type {
	case "install_php", "install_ffmpeg", "install_silk_v3", "install_napcat_bot":
		if runtime.GOOS != "windows" {
			jsonResp, _ := json.Marshal(map[string]string{"status": "error", "error": "该组件仅支持 Windows 端"})
			w.Write(jsonResp)
			return
		}
	}

	switch h.Type {
	case "get_servers":
		ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		var j HttpOpUiConfig_servers
		j.OS = runtime.GOOS

		// 全局配置（[HTTP] 段共用：调试 / 临时读写清理周期）
		httpSec := f.Section("HTTP")
		j.Global.Debug = httpSec.Key("调试").MustBool(false)
		j.Global.TempCleanupInterval = httpSec.Key("临时读写清理周期").MustInt(60)

		// 遍历所有 HTTP 段（HTTP / HTTP2 / HTTP3...）
		j.Servers = make([]HttpOpUiConfig_server, 0)
		for _, sec := range f.Sections() {
			name := sec.Name()
			if name != "HTTP" && !strings.HasPrefix(name, "HTTP") {
				continue
			}
			webRoot := sec.Key("映射目录").String()
			if webRoot == "" {
				webRoot = dto.DefaultWebRoot
			}
			routerFile := sec.Key("路由词库").String()
			if routerFile == "" {
				routerFile = dto.DefaultRouterFile
			}
			j.Servers = append(j.Servers, HttpOpUiConfig_server{
				Server:      sec.Key("server").String(),
				Enabled:     sec.Key("启用").MustBool(true),
				TLS:         sec.Key("TLS").MustBool(false),
				CertFile:    sec.Key("TLS证书文件").String(),
				KeyFile:     sec.Key("TLS密钥文件").String(),
				TLSMode:     sec.Key("TLS方式").MustString("file"),
				TLSDomains:  sec.Key("TLS域名").String(),
				TLSEmail:    sec.Key("TLS邮箱").String(),
				Domains:     sec.Key("绑定域名").String(),
				WebRoot:     webRoot,
				RouterFile:  routerFile,
				Remark:      sec.Key("备注").String(),
				Cors:        sec.Key("跨域").MustBool(false),
				CorsOrigins: sec.Key("跨域白名单").String(),
				FrpOpen:     sec.Key("Frp启用").MustBool(false),
				FrpAddr:     sec.Key("Frp服务端地址").String(),
				FrpToken:    sec.Key("Frp令牌").String(),
				FrpDebug:    sec.Key("Frp调试").MustBool(false),
			})
		}
		if len(j.Servers) == 0 {
			j.Servers = append(j.Servers, HttpOpUiConfig_server{
				Server:     "",
				Enabled:    true,
				TLSMode:    "file",
				WebRoot:    dto.DefaultWebRoot,
				RouterFile: dto.DefaultRouterFile,
			})
		}
		if r, err := json.Marshal(j); err != nil {
			w.Write([]byte(`{"servers":[],"global":{"debug":false,"temp_cleanup_interval":60},"os":"` + runtime.GOOS + `"}`))
		} else {
			w.Write(r)
		}
		return

	case "save_servers":
		var j HttpOpUiConfig_servers
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if len(j.Servers) == 0 {
			w.Write([]byte(`{"status":"error","error":"至少需要一个服务器配置"}`))
			return
		}

		// 判断是否需要热重启 HTTP 服务器（无需重启进程）
		needRestart := serversNeedRestart(dto.ServerConfig.Routers, j.Servers)

		ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}

		// 删除所有现有 HTTP 段（HTTP / HTTP2 / HTTP3...）
		httpSections := make([]string, 0)
		for _, sec := range f.Sections() {
			name := sec.Name()
			if name == "HTTP" || strings.HasPrefix(name, "HTTP") {
				httpSections = append(httpSections, name)
			}
		}
		for _, name := range httpSections {
			f.DeleteSection(name)
		}

		// 重建各服务器段
		for i, s := range j.Servers {
			name := "HTTP"
			if i > 0 {
				name = fmt.Sprintf("HTTP%d", i+1)
			}
			sec := f.Section(name)
			sec.Key("server").SetValue(s.Server)
			sec.Key("启用").SetValue(strconv.FormatBool(s.Enabled))
			sec.Key("TLS").SetValue(strconv.FormatBool(s.TLS))
			sec.Key("TLS证书文件").SetValue(s.CertFile)
			sec.Key("TLS密钥文件").SetValue(s.KeyFile)
			sec.Key("TLS方式").SetValue(s.TLSMode)
			sec.Key("TLS域名").SetValue(s.TLSDomains)
			sec.Key("TLS邮箱").SetValue(s.TLSEmail)
			sec.Key("绑定域名").SetValue(s.Domains)
			sec.Key("映射目录").SetValue(s.WebRoot)
			sec.Key("路由词库").SetValue(s.RouterFile)
			sec.Key("备注").SetValue(s.Remark)
			sec.Key("跨域").SetValue(strconv.FormatBool(s.Cors))
			sec.Key("跨域白名单").SetValue(s.CorsOrigins)
			// BeerWebFrp 穿透（每个服务器独立），地址统一规范为 ws/wss
			frpAddr := strings.TrimSpace(s.FrpAddr)
			if after, ok := strings.CutPrefix(frpAddr, "https://"); ok {
				frpAddr = "wss://" + after
			} else if after, ok := strings.CutPrefix(frpAddr, "http://"); ok {
				frpAddr = "ws://" + after
			}
			sec.Key("Frp启用").SetValue(strconv.FormatBool(s.FrpOpen))
			sec.Key("Frp服务端地址").SetValue(frpAddr)
			sec.Key("Frp令牌").SetValue(s.FrpToken)
			sec.Key("Frp调试").SetValue(strconv.FormatBool(s.FrpDebug))
		}

		// 全局配置写入 [HTTP] 段
		httpSec := f.Section("HTTP")
		httpSec.Key("调试").SetValue(strconv.FormatBool(j.Global.Debug))
		httpSec.Key("临时读写清理周期").SetValue(strconv.Itoa(j.Global.TempCleanupInterval))

		if err := ff.SaveIni(f); err != nil {
			utils.ErrorStop("系统配置保存失败")
		}

		// 更新内存全局配置
		dto.ServerConfig.Debug = j.Global.Debug
		dto.ServerConfig.TempCleanupInterval = j.Global.TempCleanupInterval
		debugLog.SetDebug(j.Global.Debug)

		// 更新内存 Routers 模型（每服务器独立 handler）
		// 复用旧的 ServerHTTP 指针，使已运行 handler 闭包能立即感知跨域/路由词库/映射目录等非重启型配置变化
		oldRouters := dto.ServerConfig.Routers
		newRouters := make([]*dto.ServerHTTP, 0, len(j.Servers))
		for i, s := range j.Servers {
			var router *dto.ServerHTTP
			if i < len(oldRouters) && oldRouters[i] != nil {
				router = oldRouters[i]
			} else {
				router = &dto.ServerHTTP{Http: &http.Server{}}
				if dto.WebHandlerFactory != nil {
					router.Http.Handler = dto.WebHandlerFactory(router)
				}
			}
			router.Http.Addr = s.Server
			router.Enabled = s.Enabled
			router.Domains = splitServerDomains(s.Domains)
			router.WebRoot = s.WebRoot
			router.RouterFile = s.RouterFile
			router.Cors = s.Cors
			router.CorsOrigins = s.CorsOrigins
			router.TLS = s.TLS
			router.CertFile = s.CertFile
			router.KeyFile = s.KeyFile
			router.TLSMode = s.TLSMode
			router.TLSDomains = s.TLSDomains
			router.TLSEmail = s.TLSEmail
			router.FrpOpen = s.FrpOpen
			router.FrpServerAddr = s.FrpAddr
			router.FrpToken = s.FrpToken
			router.FrpDebug = s.FrpDebug
			newRouters = append(newRouters, router)
		}
		dto.ServerConfig.Routers = newRouters

		// 逐台重建 BeerWebFrp 隧道（每个服务器独立穿透）
		ReconnectAllFrp(newRouters)

		if needRestart {
			if err := RestartHTTPServer(); err != nil {
				w.Write([]byte(`{"status":"error","error":` + strconvQuote("HTTPS 配置热更新失败: "+err.Error()) + `}`))
				return
			}
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "toggle_server":
		var req struct {
			Index int  `json:"index"`
			Open  bool `json:"open"`
		}
		if err := json.Unmarshal(h.Data, &req); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		routers := dto.ServerConfig.Routers
		if req.Index < 0 || req.Index >= len(routers) || routers[req.Index] == nil {
			w.Write([]byte(`{"status":"error","error":"服务器不存在"}`))
			return
		}
		// 实时开关：直接修改运行中的服务器指针，无需重启
		routers[req.Index].Enabled = req.Open

		// 持久化到对应 HTTP 段（HTTP / HTTP2 / HTTP3...）
		ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			w.Write([]byte(`{"status":"error","error":"读取系统配置失败"}`))
			return
		}
		name := "HTTP"
		if req.Index > 0 {
			name = fmt.Sprintf("HTTP%d", req.Index+1)
		}
		f.Section(name).Key("启用").SetValue(strconv.FormatBool(req.Open))
		if err := ff.SaveIni(f); err != nil {
			w.Write([]byte(`{"status":"error","error":"保存系统配置失败"}`))
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "gen_https_cert":
		opuiGenHttpsCert(w, r, h)
		return

	case "import_https_cert":
		opuiImportHttpsCert(w, r, h)
		return

	case "list_system_certs":
		opuiListSystemCerts(w, r, h)
		return

	case "extract_system_cert":
		opuiExtractSystemCert(w, r, h)
		return

	case "get_opui":
		ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			debugLog.Errorf("[OPUI] get_opui LoadIni failed: %v", err)
			w.Write([]byte(`{"open":false,"path":"","secret":"","cors":false}`))
			return
		}
		d := f.Section("管理面板")
		var j HttpOpUiConfig_opui
		j.Open = d.Key("启用").MustBool(false)
		j.Path = d.Key("访问路径").String()
		j.Secret = d.Key("密钥").String()
		j.Cors = d.Key("跨域").MustBool(false)
		r, err := json.Marshal(j)
		if err != nil {
			debugLog.Errorf("[OPUI] get_opui json.Marshal failed: %v", err)
			w.Write([]byte(`{"open":false,"path":"","secret":""}`))
			return
		}
		w.Write(r)
		return

	case "save_opui":
		var j HttpOpUiConfig_opui
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := f.Section("管理面板")
		d.Key("启用").SetValue(strconv.FormatBool(j.Open))
		d.Key("访问路径").SetValue(j.Path)
		d.Key("密钥").SetValue(j.Secret)
		d.Key("跨域").SetValue(strconv.FormatBool(j.Cors))
		ff.SaveIni(f)

		if j.Open {
			dto.ServerConfig.OPUI = &dto.OPUI{
				Addr:   "/" + j.Path,
				Secret: j.Secret,
				Cors:   j.Cors,
			}
		} else {
			dto.ServerConfig.OPUI = nil
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "get_cloud_tool":
		ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			w.Write([]byte(`{"addr":"","connected":false,"debug":false}`))
			return
		}
		var j HttpOpUiConfig_cloud_tool
		j.Addr = cloudToolAddr()
		j.Connected = cloudToolConnected()
		j.Reconnecting = cloudToolReconnecting.Load()
		j.Debug = f.Section("云工具").Key("调试").MustBool(false)
		j.OfflineNotify = cloudToolGetOfflineNotify()
		j.MaxDevices = cloudToolGetMaxDevices()
		j.Email = cloudToolGetEmail()
		cloudToolDebug = j.Debug
		r, err := json.Marshal(j)
		if err != nil {
			w.Write([]byte(`{"addr":"","connected":false,"debug":false}`))
			return
		}
		w.Write(r)
		return

	case "get_cloud_funcs":
		infos := getCloudFuncs()
		if infos == nil {
			// 缓存未就绪（连接刚建立尚未完成 list_funcs），兜底拉取一次并缓存
			var ok bool
			infos, ok = fetchCloudFuncs()
			if !ok {
				resp, _ := json.Marshal(map[string]string{"status": "error", "error": "获取云函数列表失败"})
				w.Write(resp)
				return
			}
		}
		resp, _ := json.Marshal(map[string]any{"status": "ok", "funcs": infos})
		w.Write(resp)
		return

	case "cloud_money_logs":
		var req struct {
			Page     int `json:"page"`
			PageSize int `json:"page_size"`
		}
		if err := json.Unmarshal(h.Data, &req); err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if req.Page < 1 {
			req.Page = 1
		}
		if req.PageSize <= 0 {
			req.PageSize = 20
		}
		offset := (req.Page - 1) * req.PageSize
		data, _ := json.Marshal(map[string]int{"offset": offset, "limit": req.PageSize})
		res, err := cloudToolCall(cloudToolMsg{Type: "money_logs", Data: string(data)}, 15*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		var page struct {
			Items json.RawMessage `json:"items"`
			Total int             `json:"total"`
		}
		if err := json.Unmarshal([]byte(res.Data), &page); err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]any{"status": "ok", "items": page.Items, "total": page.Total})
		w.Write(resp)
		return

	case "cloud_account_info":
		res, err := cloudToolCall(cloudToolMsg{Type: "account_info"}, 15*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		var info struct {
			DiskSize      int64  `json:"disk_size"`
			Money         string `json:"money"`
			Coupon        string `json:"coupon"`
			TotalMoney    string `json:"total_money"`
			OnlineSeconds int64  `json:"online_seconds"`
			Devices       []struct {
				IP    string `json:"ip"`
				Start int64  `json:"start"`
			} `json:"devices"`
		}
		if err := json.Unmarshal([]byte(res.Data), &info); err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]any{
			"status":         "ok",
			"disk_size":      info.DiskSize,
			"money":          info.Money,
			"coupon":         info.Coupon,
			"total_money":    info.TotalMoney,
			"online_seconds": info.OnlineSeconds,
			"devices":        info.Devices,
		})
		w.Write(resp)
		return

	case "save_cloud_tool":
		var j HttpOpUiConfig_cloud_tool
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		newAddr := strings.TrimSpace(j.Addr)
		oldAddr := strings.TrimSpace(f.Section("云工具").Key("连接地址").String())
		f.Section("云工具").Key("连接地址").SetValue(newAddr)
		f.Section("云工具").Key("调试").SetValue(strconv.FormatBool(j.Debug))
		cloudToolDebug = j.Debug
		ff.SaveIni(f)
		if newAddr != oldAddr {
			// 地址变化：断开旧连接，等待下次登录/断线重连使用新地址
			cloudToolSetWantConn(false)
			cloudToolDisconnect()
			broadcastCloudToolStatus(false)
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "get_cloudtool_server":
		ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := f.Section("云工具服务端")
		var j HttpOpUiConfig_cloudtool_server
		j.Open = d.Key("启用").MustBool(false)
		j.Addr = d.Key("访问路径").String()
		if j.Addr == "" {
			j.Addr = "cloudtool"
		}
		j.AllowRegister = d.Key("任意账号注册").MustBool(true)
		j.DicDir = d.Key("词库目录").String()
		if j.DicDir == "" {
			j.DicDir = "cloudtool"
		}
		j.LogoutSec = d.Key("断开注销时长").MustInt(30)
		j.Debug = d.Key("调试").MustBool(false)
		r, _ := json.Marshal(j)
		w.Write(r)
		return

	case "save_cloudtool_server":
		var j HttpOpUiConfig_cloudtool_server
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		addr := strings.TrimPrefix(strings.TrimSpace(j.Addr), "/")
		if addr == "" {
			addr = "cloudtool"
		}
		dicDir := strings.TrimSpace(j.DicDir)
		if dicDir == "" {
			dicDir = "cloudtool"
		}
		if j.LogoutSec < 0 {
			j.LogoutSec = 0
		}
		ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := f.Section("云工具服务端")
		d.Key("启用").SetValue(strconv.FormatBool(j.Open))
		d.Key("访问路径").SetValue(addr)
		d.Key("任意账号注册").SetValue(strconv.FormatBool(j.AllowRegister))
		d.Key("词库目录").SetValue(dicDir)
		d.Key("断开注销时长").SetValue(strconv.Itoa(j.LogoutSec))
		d.Key("调试").SetValue(strconv.FormatBool(j.Debug))
		ff.SaveIni(f)

		// 更新内存配置并即时生效（Open=false 时仅拒绝新连接）；白名单由白名单配置页单独管理，不在此覆盖
		dto.ServerConfig.CloudTool = &dto.CloudTool{
			Open:          j.Open,
			Addr:          "/" + addr,
			AllowRegister: j.AllowRegister,
			Whitelist:     CloudToolSplitWhitelist(d.Key("白名单").String()),
			DicDir:        dicDir,
			LogoutSec:     j.LogoutSec,
			Debug:         j.Debug,
		}
		StartCloudToolServer()
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "get_cloudtool_accounts":
		// 云工具服务端账号列表（分页 + 账号模糊搜索，含实时连接状态）
		var req struct {
			Page     int    `json:"page"`
			PageSize int    `json:"page_size"`
			Keyword  string `json:"keyword"`
		}
		if err := json.Unmarshal(h.Data, &req); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if req.Page < 1 {
			req.Page = 1
		}
		if req.PageSize <= 0 {
			req.PageSize = 20
		}
		if req.PageSize > 100 {
			req.PageSize = 100
		}
		items, total, err := CloudToolListAccounts(req.Keyword, req.Page, req.PageSize)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]any{"status": "ok", "items": items, "total": total})
		w.Write(resp)
		return

	case "disconnect_cloudtool_account":
		// 强制断开云工具账号（远程下线）
		var j struct {
			Username string `json:"username"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if err := CloudToolDisconnectAccount(j.Username); err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "delete_cloudtool_account":
		// 删除云工具账号（可附带删除云工具余额，默认删除）
		var j struct {
			Username      string `json:"username"`
			DeleteBalance bool   `json:"delete_balance"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if err := CloudToolDeleteAccount(j.Username, j.DeleteBalance); err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "get_cloudtool_whitelist":
		// 云工具服务端白名单列表（分页 + 账号模糊搜索）
		var req struct {
			Page     int    `json:"page"`
			PageSize int    `json:"page_size"`
			Keyword  string `json:"keyword"`
		}
		if err := json.Unmarshal(h.Data, &req); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if req.Page < 1 {
			req.Page = 1
		}
		if req.PageSize <= 0 {
			req.PageSize = 20
		}
		if req.PageSize > 100 {
			req.PageSize = 100
		}
		items, total, err := CloudToolListWhitelist(req.Keyword, req.Page, req.PageSize)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]any{"status": "ok", "items": items, "total": total})
		w.Write(resp)
		return

	case "add_cloudtool_whitelist":
		// 把账号加入云工具服务端白名单
		var j struct {
			Username string `json:"username"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		whitelisted, err := CloudToolAddWhitelist(j.Username)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]any{"status": "ok", "whitelisted": whitelisted})
		w.Write(resp)
		return

	case "remove_cloudtool_whitelist":
		// 把账号移出云工具服务端白名单
		var j struct {
			Username string `json:"username"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		whitelisted, err := CloudToolRemoveWhitelist(j.Username)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]any{"status": "ok", "whitelisted": whitelisted})
		w.Write(resp)
		return

	case "rename_cloudtool_account":
		// 重命名云工具服务端账号（在线连接将先被断开，白名单自动迁移）
		var j struct {
			Username string `json:"username"`
			NewName  string `json:"new_name"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if err := CloudToolRenameAccount(j.Username, j.NewName); err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "reset_cloudtool_password":
		// 重置云工具服务端账号密码（在线连接将先被断开）
		var j struct {
			Username    string `json:"username"`
			NewPassword string `json:"new_password"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if err := CloudToolResetPassword(j.Username, j.NewPassword); err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "clear_cloudtool_accounts":
		// 清空全部云工具账号（白名单内账号保留）
		deleted, kept, err := CloudToolClearAccounts()
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]any{"status": "ok", "deleted": deleted, "kept": kept})
		w.Write(resp)
		return

	case "cloud_connect":
		var j struct {
			Username string `json:"username"`
			Password string `json:"password"` // 已 SHA-256 加密后的密码哈希
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if j.Username == "" || j.Password == "" {
			http.Error(w, `{"status":"error","error":"账号、密码均不能为空"}`, http.StatusBadRequest)
			return
		}
		token, offlineNotify, email, debug, err := cloudToolAuth(j.Username, j.Password, "")
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]any{
			"status":         "ok",
			"connected":      true,
			"token":          token,
			"offline_notify": offlineNotify,
			"debug":          debug,
			"max_devices":    cloudToolGetMaxDevices(),
			"email":          email,
		})
		w.Write(resp)
		return

	case "cloud_resume":
		var j struct {
			Username string `json:"username"`
			Token    string `json:"token"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if j.Username == "" || j.Token == "" {
			http.Error(w, `{"status":"error","error":"账号、token 均不能为空"}`, http.StatusBadRequest)
			return
		}
		token, offlineNotify, email, debug, err := cloudToolAuth(j.Username, "", j.Token)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]any{
			"status":         "ok",
			"connected":      true,
			"token":          token,
			"offline_notify": offlineNotify,
			"debug":          debug,
			"max_devices":    cloudToolGetMaxDevices(),
			"email":          email,
		})
		w.Write(resp)
		return

	case "cloud_disconnect":
		cloudToolSetWantConn(false)
		cloudToolDisconnect()
		broadcastCloudToolStatus(false)
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "cloud_cancel_reconnect":
		cloudToolSetWantConn(false)
		cloudToolDisconnect()
		cloudToolClearAuth()
		broadcastCloudToolStatus(false)
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "cloud_exec":
		var j struct {
			Func string `json:"func"`
			Data string `json:"data"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		res, err := cloudToolCall(cloudToolMsg{Type: "exec", Func: j.Func, Data: j.Data}, 15*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]string{"status": "ok", "result": res.Data})
		w.Write(resp)
		return

	case "cloud_change_password":
		var j struct {
			OldPassword string `json:"old_password"`
			NewPassword string `json:"new_password"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		res, err := cloudToolCall(cloudToolMsg{Type: "change_password", OldPassword: j.OldPassword, NewPassword: j.NewPassword}, 15*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]string{"status": "ok", "result": res.Data})
		w.Write(resp)
		return

	case "cloud_change_username":
		var j struct {
			OldPassword string `json:"old_password"`
			NewUsername string `json:"new_username"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		j.NewUsername = strings.TrimSpace(j.NewUsername)
		res, err := cloudToolCall(cloudToolMsg{Type: "change_username", OldPassword: j.OldPassword, NewUsername: j.NewUsername}, 15*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		// 更新本地保存的用户名，保证后续断线重连使用新用户名
		cloudToolMu.Lock()
		cloudToolUsername = j.NewUsername
		token := cloudToolToken
		cloudToolMu.Unlock()
		cloudToolSaveAuth(j.NewUsername, token)
		resp, _ := json.Marshal(map[string]string{"status": "ok", "result": res.Data})
		w.Write(resp)
		return

	case "cloud_set_offline_notify":
		var j struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		data := "0"
		if j.Enabled {
			data = "1"
		}
		res, err := cloudToolCall(cloudToolMsg{Type: "set_offline_notify", Data: data}, 15*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		cloudToolMu.Lock()
		cloudToolOfflineNotify = j.Enabled
		cloudToolMu.Unlock()
		resp, _ := json.Marshal(map[string]string{"status": "ok", "result": res.Data})
		w.Write(resp)
		return

	case "cloud_set_max_devices":
		var j struct {
			MaxDevices int `json:"max_devices"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if j.MaxDevices < 0 {
			j.MaxDevices = 0
		}
		res, err := cloudToolCall(cloudToolMsg{Type: "set_max_devices", Data: strconv.Itoa(j.MaxDevices)}, 15*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		cloudToolMu.Lock()
		cloudToolMaxDevices = j.MaxDevices
		cloudToolMu.Unlock()
		resp, _ := json.Marshal(map[string]string{"status": "ok", "result": res.Data})
		w.Write(resp)
		return

	case "cloud_bind_email":
		var j struct {
			Email string `json:"email"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		j.Email = strings.TrimSpace(j.Email)
		res, err := cloudToolCall(cloudToolMsg{Type: "send_email_code", Data: j.Email}, 15*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		// 仅发送验证码，绑定邮箱在验证码校验通过后完成
		resp, _ := json.Marshal(map[string]string{"status": "ok", "result": res.Data})
		w.Write(resp)
		return

	case "cloud_confirm_email":
		var j struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		res, err := cloudToolCall(cloudToolMsg{Type: "confirm_email", Data: j.Code}, 15*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		cloudToolMu.Lock()
		if res.Email != "" {
			cloudToolEmail = res.Email
		}
		cloudToolMu.Unlock()
		resp, _ := json.Marshal(map[string]string{"status": "ok", "result": res.Data, "email": res.Email})
		w.Write(resp)
		return

	case "cloud_delete_account":
		var j struct {
			Password string `json:"password"` // 已 SHA-256 加密后的密码哈希
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		res, err := cloudToolCall(cloudToolMsg{Type: "delete_account", Data: j.Password}, 15*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		// 注销成功：清除本地认证与连接，前端随之回到未登录状态
		cloudToolSetWantConn(false)
		cloudToolDisconnect()
		cloudToolClearAuth()
		broadcastCloudToolStatus(false)
		resp, _ := json.Marshal(map[string]string{"status": "ok", "result": res.Data})
		w.Write(resp)
		return

	case "cloud_logout":
		cloudToolMu.Lock()
		username := cloudToolUsername
		cloudToolMu.Unlock()
		if username != "" {
			// logout 触发云工具端删除 token 并断开，这里只需发送即可，随后关闭本地代理
			_ = cloudToolSend(cloudToolMsg{Type: "logout"})
		}
		cloudToolSetWantConn(false)
		cloudToolDisconnect()
		cloudToolClearAuth()
		broadcastCloudToolStatus(false)
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "get_bg":
		db, err := dic_funcs.GetGlobalDB()
		if err != nil {
			w.Write([]byte(`{"light":{"type":"","data":""},"dark":{"type":"","data":""}}`))
			return
		}
		if e := dic_funcs.EnsureFsTable(db, "opui_bg"); e != nil {
			w.Write([]byte(`{"light":{"type":"","data":""},"dark":{"type":"","data":""}}`))
			return
		}
		var data string
		err = db.QueryRow(`SELECT data FROM "opui_bg" WHERE key='bg'`).Scan(&data)
		if err != nil {
			w.Write([]byte(`{"light":{"type":"","data":""},"dark":{"type":"","data":""}}`))
			return
		}

		bgData := HttpOpUiConfig_bg{}
		// 新格式：{"light":{...},"dark":{...}}
		if err := json.Unmarshal([]byte(data), &bgData); err != nil ||
			(bgData.Light.Type == "" && bgData.Light.Data == "" && bgData.Light.Color == "" &&
				bgData.Dark.Type == "" && bgData.Dark.Data == "" && bgData.Dark.Color == "") {
			// 旧格式：{"type":..,"data":..} 或纯文本，迁移为亮暗共用同一背景
			var old HttpOpUiConfig_bgItem
			if e := json.Unmarshal([]byte(data), &old); e != nil {
				// 纯文本旧格式，需同时读取 type 列
				var bgType string
				_ = db.QueryRow(`SELECT type FROM "opui_bg" WHERE key='bg'`).Scan(&bgType)
				old = HttpOpUiConfig_bgItem{Type: bgType, Data: data}
			}
			bgData.Light = old
			bgData.Dark = old
		}
		r, _ := json.Marshal(bgData)
		w.Write(r)
		return

	case "save_bg":
		var j HttpOpUiConfig_bg
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		db, err := dic_funcs.GetGlobalDB()
		if err != nil {
			http.Error(w, `{"status":"error","error":"db not ready"}`, http.StatusInternalServerError)
			return
		}
		if e := dic_funcs.EnsureFsTable(db, "opui_bg"); e != nil {
			http.Error(w, `{"status":"error","error":"db init failed"}`, http.StatusInternalServerError)
			return
		}
		bgJson, _ := json.Marshal(j)
		now := time.Now().Unix()
		// 尝试新表结构（无 type 列），失败则回退旧表结构
		_, err = db.Exec(`
				INSERT INTO "opui_bg" (key, data, updated_at)
				VALUES ('bg', ?, ?)
				ON CONFLICT(key) DO UPDATE SET
					data = excluded.data,
					updated_at = excluded.updated_at
			`, string(bgJson), now)
		if err != nil {
			_, err = db.Exec(`
					INSERT INTO "opui_bg" (key, type, data, updated_at)
					VALUES ('bg', ?, ?, ?)
					ON CONFLICT(key) DO UPDATE SET
						type = excluded.type,
						data = excluded.data,
						updated_at = excluded.updated_at
				`, "", string(bgJson), now)
		}
		if err != nil {
			http.Error(w, `{"status":"error","error":"save failed"}`, http.StatusInternalServerError)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "check_opui_key":
		var j struct {
			Key string `json:"key"`
		}
		if len(h.Data) == 0 {
			debugLog.Errorf("[OPUI] check_opui_key: h.Data is nil or empty")
			w.Write([]byte(`{"valid":false}`))
			return
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			debugLog.Errorf("[OPUI] check_opui_key: json.Unmarshal failed, data=%s, err=%v", string(h.Data), err)
			w.Write([]byte(`{"valid":false}`))
			return
		}
		ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			w.Write([]byte(`{"valid":false}`))
			return
		}
		d := f.Section("管理面板")
		storedKey := d.Key("密钥").String()
		clientIP := utils.GetClientIP(r)
		if storedKey == "" || storedKey == j.Key {
			addLoginEvent("admin_login", "OPUI 管理员登录成功", clientIP)
			w.Write([]byte(`{"valid":true}`))
		} else {
			addLoginEvent("admin_login_fail", "OPUI 登录失败: 密钥错误", clientIP)
			w.Write([]byte(`{"valid":false}`))
		}
		return

	case "get_websocket":
		list := dto.ServerConfig.WsListSnapshot()
		items := make([]HttpOpUiWebSocketItem, 0, len(list)+1)
		for _, ws := range list {
			items = append(items, HttpOpUiWebSocketItem{
				Addr:     ws.Addr,
				Cors:     ws.Cors,
				Open:     ws.Open,
				Closable: true,
			})
		}
		// OPUI 本身也是一个 WebSocket 服务，纳入监听列表（但不可关闭，关闭等于关闭面板自身）
		if opui := dto.ServerConfig.OPUI; opui != nil {
			items = append(items, HttpOpUiWebSocketItem{
				Addr:     opui.Addr,
				Cors:     opui.Cors,
				Open:     true,
				Closable: false,
			})
		}
		r, _ := json.Marshal(map[string]any{"list": items})
		w.Write(r)
		return

	case "close_websocket":
		var j struct {
			Addr string `json:"addr"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if j.Addr == "" {
			http.Error(w, `{"status":"error","error":"addr is empty"}`, http.StatusBadRequest)
			return
		}
		// OPUI 是管理面板自身的 WebSocket，不可通过面板关闭
		if dto.ServerConfig.OPUI != nil && j.Addr == dto.ServerConfig.OPUI.Addr {
			http.Error(w, `{"status":"error","error":"opui cannot be closed"}`, http.StatusBadRequest)
			return
		}
		dto.ServerConfig.RemoveWs(j.Addr)
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "get_ngrok":
		ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := f.Section("Ngrok")
		var j HttpOpUiConfig_ngrok
		j.Open = d.Key("启用").MustBool(false)
		j.Token = d.Key("密钥").String()
		j.Domain = d.Key("访问链接").String()
		j.ServerAddr = d.Key("服务器").String()
		r, _ := json.Marshal(j)
		w.Write(r)
		return

	case "save_ngrok":
		var j HttpOpUiConfig_ngrok
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := f.Section("Ngrok")
		d.Key("启用").SetValue(strconv.FormatBool(j.Open))
		d.Key("密钥").SetValue(j.Token)
		d.Key("访问链接").SetValue(j.Domain)
		d.Key("服务器").SetValue(j.ServerAddr)
		ff.SaveIni(f)

		// 保存即生效：按启用状态启停隧道（服务器地址变更时先停旧隧道再重连）
		if j.Open {
			if dto.ServerConfig.NgrokListener != nil || dto.ServerConfig.NgrokCancel != nil {
				StopNgrok()
			}
			dto.ServerConfig.Ngrok = &dto.NgrokConfig{
				Addr:       j.Domain,
				Token:      j.Token,
				ServerAddr: j.ServerAddr,
			}
			url, err := StartNgrok(j.Token, j.Domain)
			if err != nil {
				w.Write([]byte(`{"status":"error","error":"` + err.Error() + `"}`))
				return
			}
			w.Write([]byte(`{"status":"ok","url":"` + url + `"}`))
		} else {
			StopNgrok()
			dto.ServerConfig.Ngrok = nil
			w.Write([]byte(`{"status":"ok"}`))
		}
		return

	case "toggle_ngrok":
		var j struct {
			Open bool `json:"open"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := f.Section("Ngrok")
		d.Key("启用").SetValue(strconv.FormatBool(j.Open))
		ff.SaveIni(f)

		if j.Open {
			token := d.Key("密钥").String()
			domain := d.Key("访问链接").String()
			serverAddr := d.Key("服务器").String()
			dto.ServerConfig.Ngrok = &dto.NgrokConfig{
				Addr:       domain,
				Token:      token,
				ServerAddr: serverAddr,
			}
			url, err := StartNgrok(token, domain)
			if err != nil {
				w.Write([]byte(`{"status":"error","error":"` + err.Error() + `"}`))
				return
			}
			w.Write([]byte(`{"status":"ok","url":"` + url + `"}`))
		} else {
			StopNgrok()
			dto.ServerConfig.Ngrok = nil
			w.Write([]byte(`{"status":"ok"}`))
		}
		return

	case "get_ftp":
		ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := f.Section("FTP")
		var j HttpOpUiConfig_ftp
		j.Open = d.Key("启用").MustBool(false)
		j.Port = d.Key("端口").MustInt(21)
		j.Username = d.Key("用户名").String()
		j.Password = d.Key("密码").String()
		j.Debug = d.Key("调试").MustBool(false)
		j.Tls = d.Key("TLS").MustBool(false)
		j.PasvPortStart = d.Key("PASV端口起始").MustInt(32000)
		j.PasvPortEnd = d.Key("PASV端口结束").MustInt(32005)
		r, _ := json.Marshal(j)
		w.Write(r)
		return

	case "save_ftp":
		var j HttpOpUiConfig_ftp
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		// 校验数据端口范围
		if j.PasvPortStart < 1 || j.PasvPortStart > 65535 || j.PasvPortEnd < 1 || j.PasvPortEnd > 65535 {
			http.Error(w, `{"status":"error","error":"数据端口范围必须在 1-65535 之间"}`, http.StatusBadRequest)
			return
		}
		if j.PasvPortStart > j.PasvPortEnd {
			http.Error(w, `{"status":"error","error":"起始端口不能大于结束端口"}`, http.StatusBadRequest)
			return
		}
		ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := f.Section("FTP")
		d.Key("启用").SetValue(strconv.FormatBool(j.Open))
		d.Key("端口").SetValue(strconv.Itoa(j.Port))
		d.Key("用户名").SetValue(j.Username)
		d.Key("密码").SetValue(j.Password)
		d.Key("调试").SetValue(strconv.FormatBool(j.Debug))
		d.Key("TLS").SetValue(strconv.FormatBool(j.Tls))
		d.Key("PASV端口起始").SetValue(strconv.Itoa(j.PasvPortStart))
		d.Key("PASV端口结束").SetValue(strconv.Itoa(j.PasvPortEnd))
		ff.SaveIni(f)

		if j.Open {
			StartFtp(j.Port, j.Debug, j.Username, j.Password, j.Tls, j.PasvPortStart, j.PasvPortEnd)
		} else {
			StopFtp()
		}

		w.Write([]byte(`{"status":"ok"}`))
		return

	case "get_sftp":
		ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := f.Section("SFTP")
		var j HttpOpUiConfig_sftp
		j.Open = d.Key("启用").MustBool(false)
		j.Port = d.Key("端口").MustInt(22)
		j.Username = d.Key("用户名").String()
		j.Password = d.Key("密码").String()
		j.Debug = d.Key("调试").MustBool(false)
		r, _ := json.Marshal(j)
		w.Write(r)
		return

	case "save_sftp":
		var j HttpOpUiConfig_sftp
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := f.Section("SFTP")
		d.Key("启用").SetValue(strconv.FormatBool(j.Open))
		d.Key("端口").SetValue(strconv.Itoa(j.Port))
		d.Key("用户名").SetValue(j.Username)
		d.Key("密码").SetValue(j.Password)
		d.Key("调试").SetValue(strconv.FormatBool(j.Debug))
		ff.SaveIni(f)

		if j.Open {
			StartSftp(j.Port, j.Debug, j.Username, j.Password)
		} else {
			StopSftp()
		}

		w.Write([]byte(`{"status":"ok"}`))
		return

	case "get_qq", "get_qq_list":
		ff := utils.NewFileQueue(dto.CONFIG_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		var list HttpOpUiConfig_qq_list
		for _, sec := range f.Sections() {
			secName := sec.Name()
			if secName == "QQ" || (strings.HasPrefix(secName, "QQ") && len(secName) > 2) {
				d := f.Section(secName)
				var j HttpOpUiConfig_qq
				j.Open = d.Key("启用").MustBool(false)
				j.Dic = d.Key("词库").String()
				j.Path = d.Key("访问路径").String()
				j.Appid = d.Key("APPID").String()
				j.Secret = d.Key("密钥").String()
				j.AtCompat = d.Key("全量艾特兼容").MustBool(true)
				j.FilterSlash = d.Key("过滤开头斜杠").MustBool(true)
				j.Debug = d.Key("调试打印").MustBool(false)
				j.Ws = d.Key("WebSocket").MustBool(false)
				j.WsIntents = d.Key("监听码").MustInt(0)
				j.Remark = d.Key("备注").String()
				j.Robot = d.Key("Robot").String()
				if dto.ServerConfig.QQBots != nil {
					if bot := dto.ServerConfig.QQBots[secName]; bot != nil {
						j.Connected = bot.WsConn != nil
						if bot.API != nil {
							j.BotName = bot.API.BotUsername
							j.BotAvatar = bot.API.BotAvatar
						}
					}
				}
				// 未启用或尚未上线时，从本地 botinfo.json 兜底恢复头像昵称
				if j.BotName == "" && j.BotAvatar == "" {
					if info := loadQQBotInfoCache(j.Dic); info != nil {
						j.BotName = info.Username
						j.BotAvatar = info.Avatar
					}
				}
				list.Instances = append(list.Instances, HttpOpUiConfig_qq_instance{
					Section: secName,
					Config:  j,
				})
			}
		}
		r, _ := json.Marshal(list)
		w.Write(r)
		return

	case "save_qq":
		var j HttpOpUiConfig_qq_instance
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		sectionName := j.Section
		if sectionName == "" {
			sectionName = "QQ"
		}
		ff := utils.NewFileQueue(dto.CONFIG_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		// 备注唯一性检查
		if j.Config.Remark != "" {
			for _, sec := range f.Sections() {
				secName := sec.Name()
				if secName != sectionName && (secName == "QQ" || (strings.HasPrefix(secName, "QQ") && len(secName) > 2)) {
					if f.Section(secName).Key("备注").String() == j.Config.Remark {
						http.Error(w, `{"status":"error","error":"备注名已存在"}`, http.StatusConflict)
						return
					}
				}
			}
		}
		d := f.Section(sectionName)
		d.Key("启用").SetValue(strconv.FormatBool(j.Config.Open))
		d.Key("词库").SetValue(j.Config.Dic)
		d.Key("访问路径").SetValue(j.Config.Path)
		d.Key("APPID").SetValue(j.Config.Appid)
		d.Key("密钥").SetValue(j.Config.Secret)
		d.Key("全量艾特兼容").SetValue(strconv.FormatBool(j.Config.AtCompat))
		d.Key("过滤开头斜杠").SetValue(strconv.FormatBool(j.Config.FilterSlash))
		d.Key("调试打印").SetValue(strconv.FormatBool(j.Config.Debug))
		d.Key("WebSocket").SetValue(strconv.FormatBool(j.Config.Ws))
		d.Key("监听码").SetValue(strconv.Itoa(j.Config.WsIntents))
		d.Key("备注").SetValue(j.Config.Remark)
		d.Key("Robot").SetValue(j.Config.Robot)
		dto.LoadConfig_qq(d, sectionName)
		ff.SaveIni(f)
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "toggle_qq_debug":
		var j struct {
			Section string `json:"section"`
			Debug   bool   `json:"debug"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		// 更新配置文件
		ff := utils.NewFileQueue(dto.CONFIG_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := f.Section(j.Section)
		d.Key("调试打印").SetValue(strconv.FormatBool(j.Debug))
		ff.SaveIni(f)
		// 仅更新运行中 bot 的 Debug 标志，不重连
		if dto.ServerConfig.QQBots != nil {
			if bot := dto.ServerConfig.QQBots[j.Section]; bot != nil {
				bot.Debug = j.Debug
				if bot.API != nil {
					bot.API.Debug = j.Debug
				}
			}
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "add_qq":
		ff := utils.NewFileQueue(dto.CONFIG_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		// 找到下一个可用的编号
		maxNum := 0
		for _, sec := range f.Sections() {
			name := sec.Name()
			if strings.HasPrefix(name, "QQ") {
				if name == "QQ" {
					if maxNum < 1 {
						maxNum = 1
					}
				} else {
					numStr := name[2:]
					if num, err := strconv.Atoi(numStr); err == nil && num > maxNum {
						maxNum = num
					}
				}
			}
		}
		newNum := maxNum + 1
		newSection := "QQ" + strconv.Itoa(newNum)
		d := f.Section(newSection)
		d.Key("启用").SetValue("false")
		d.Key("词库").SetValue("private/bot/qq" + strconv.Itoa(newNum))
		d.Key("访问路径").SetValue("qq-bot" + strconv.Itoa(newNum))
		d.Key("APPID").SetValue("")
		d.Key("密钥").SetValue("")
		d.Key("全量艾特兼容").SetValue("true")
		d.Key("过滤开头斜杠").SetValue("true")
		d.Key("调试打印").SetValue("false")
		d.Key("WebSocket").SetValue("true")
		d.Key("监听码").SetValue("0")
		d.Key("备注").SetValue("")
		d.Key("Robot").SetValue("")
		ff.SaveIni(f)
		j := HttpOpUiConfig_qq_instance{
			Section: newSection,
			Config: HttpOpUiConfig_qq{
				Open:        false,
				Dic:         "private/bot/qq" + strconv.Itoa(newNum),
				Path:        "qq-bot" + strconv.Itoa(newNum),
				Appid:       "",
				Secret:      "",
				AtCompat:    true,
				FilterSlash: true,
				Debug:       false,
				Ws:          true,
				WsIntents:   0,
				Remark:      "",
				Robot:       "",
			},
		}
		r, _ := json.Marshal(j)
		w.Write(r)
		return

	case "del_qq":
		var j struct {
			Section string `json:"section"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if j.Section == "" {
			http.Error(w, `{"status":"error","error":"section is empty"}`, http.StatusBadRequest)
			return
		}
		ff := utils.NewFileQueue(dto.CONFIG_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		f.DeleteSection(j.Section)
		// 从运行中移除
		if dto.ServerConfig.QQBots != nil {
			delete(dto.ServerConfig.QQBots, j.Section)
		}
		ff.SaveIni(f)
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "qq_sandbox_run":
		var j struct {
			Section string `json:"section"`
			// Dic 自定义词库路径（优先于 section，无需选择实例即可测试）
			Dic string `json:"dic"`
			Msg string `json:"msg"`
			// Private 按群私聊模拟（词库 #私聊# 触发词生效），默认群聊
			Private bool `json:"private"`
			// ReplyID 前端右键回复指定的被引用消息 ID，空则回退为本次消息自身
			ReplyID string `json:"reply_id"`
			// GroupID 自定义模拟群号（写入 %群号%），空则使用默认 sandbox_group
			GroupID string `json:"group_id"`
			// Images 用户发送的图片（data URL 列表），注入词库 $IMG$ 附件
			Images []string `json:"images"`
			// User 自定义模拟用户（ID 写入 QQ/qq 变量，Name 写入 昵称 变量）
			User struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"user"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}

		dicPath := j.Dic
		appid, secret := "", ""
		atCompat, filterSlash := true, true
		debug := false
		robot := ""

		// 未直接传词库路径时，回退到按 section 从配置读取（兼容旧调用）
		if dicPath == "" {
			if j.Section == "" {
				http.Error(w, `{"status":"error","error":"dic或section不能为空"}`, http.StatusBadRequest)
				return
			}
			ff := utils.NewFileQueue(dto.CONFIG_PATH)
			f, err := ff.LoadIni()
			if err != nil {
				utils.ErrorStop("系统配置不存在")
			}
			d := f.Section(j.Section)
			dicPath = d.Key("词库").String()
			appid = d.Key("APPID").String()
			secret = d.Key("密钥").String()
			atCompat = d.Key("全量艾特兼容").MustBool(true)
			filterSlash = d.Key("过滤开头斜杠").MustBool(true)
			debug = d.Key("调试打印").MustBool(false)
			robot = d.Key("Robot").String()
		}
		if dicPath == "" {
			http.Error(w, `{"status":"error","error":"词库路径不能为空"}`, http.StatusBadRequest)
			return
		}

		capture := qqbot_msg.NewSandboxCapture()
		api := qqbot_msg.NewQQBot(appid, secret)
		api.Debug = debug
		api.Sandbox = capture

		bot := &qqbot_msg.RouterQQBot{
			FilePath:    dicPath,
			API:         api,
			AtCompat:    atCompat,
			FilterSlash: filterSlash,
			Debug:       debug,
			Robot:       robot,
		}

		msgID, messages := qqbot.SandboxRun(bot, j.Msg, qqbot.SandboxUser{ID: j.User.ID, Name: j.User.Name}, j.Private, j.ReplyID, j.GroupID, j.Images)
		if messages == nil {
			messages = []qqbot_msg.SandboxMessage{}
		}
		resp, _ := json.Marshal(map[string]any{"status": "ok", "msg_id": msgID, "messages": messages})
		w.Write(resp)
		return

	case "get_napcat":
		ff := utils.NewFileQueue(dto.CONFIG_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := f.Section("NapCat")
		var j HttpOpUiConfig_napcat
		j.Open = d.Key("启用").MustBool(false)
		j.Dic = d.Key("词库").String()
		j.Path = d.Key("访问路径").String()
		j.Secret = d.Key("密钥").String()
		j.Api = d.Key("发送消息接口").String()
		r, _ := json.Marshal(j)
		w.Write(r)
		return

	case "save_napcat":
		var j HttpOpUiConfig_napcat
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		ff := utils.NewFileQueue(dto.CONFIG_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := f.Section("NapCat")
		d.Key("启用").SetValue(strconv.FormatBool(j.Open))
		d.Key("词库").SetValue(j.Dic)
		d.Key("访问路径").SetValue(j.Path)
		d.Key("密钥").SetValue(j.Secret)
		d.Key("发送消息接口").SetValue(j.Api)
		dto.LoadConfig_napcat(d)
		ff.SaveIni(f)
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "get_yunhu":
		ff := utils.NewFileQueue(dto.CONFIG_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := f.Section("云湖")
		var j HttpOpUiConfig_yunhu
		j.Open = d.Key("启用").MustBool(false)
		j.Dic = d.Key("词库").String()
		j.Path = d.Key("访问路径").String()
		j.Secret = d.Key("密钥").String()
		r, _ := json.Marshal(j)
		w.Write(r)
		return

	case "save_yunhu":
		var j HttpOpUiConfig_yunhu
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		ff := utils.NewFileQueue(dto.CONFIG_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := f.Section("云湖")
		d.Key("启用").SetValue(strconv.FormatBool(j.Open))
		d.Key("词库").SetValue(j.Dic)
		d.Key("访问路径").SetValue(j.Path)
		d.Key("密钥").SetValue(j.Secret)
		dto.LoadConfig_yunhu(d)
		ff.SaveIni(f)
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "get_feishu":
		ff := utils.NewFileQueue(dto.CONFIG_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := f.Section("飞书")
		var j HttpOpUiConfig_feishu
		j.Open = d.Key("启用").MustBool(false)
		j.Dic = d.Key("词库").String()
		j.Path = d.Key("访问路径").String()
		j.Appid = d.Key("APPID").String()
		j.Secret = d.Key("密钥").String()
		r, _ := json.Marshal(j)
		w.Write(r)
		return

	case "save_feishu":
		var j HttpOpUiConfig_feishu
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		ff := utils.NewFileQueue(dto.CONFIG_PATH)
		f, err := ff.LoadIni()
		if err != nil {

			utils.ErrorStop("系统配置不存在")
		}
		d := f.Section("飞书")
		d.Key("启用").SetValue(strconv.FormatBool(j.Open))
		d.Key("词库").SetValue(j.Dic)
		d.Key("访问路径").SetValue(j.Path)
		d.Key("APPID").SetValue(j.Appid)
		d.Key("密钥").SetValue(j.Secret)
		dto.LoadConfig_feishu(d)
		ff.SaveIni(f)
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "get_secluded":
		ff := utils.NewFileQueue(dto.CONFIG_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := f.Section("Secluded")
		var j HttpOpUiConfig_secluded
		j.Open = d.Key("启用").MustBool(false)
		j.Dic = d.Key("词库").String()
		j.Address = d.Key("对接地址").String()
		j.Token = d.Key("令牌").String()
		j.Debug = d.Key("调试打印").MustBool(false)
		r, _ := json.Marshal(j)
		w.Write(r)
		return

	case "save_secluded":
		var j HttpOpUiConfig_secluded
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		ff := utils.NewFileQueue(dto.CONFIG_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := f.Section("Secluded")
		d.Key("启用").SetValue(strconv.FormatBool(j.Open))
		d.Key("词库").SetValue(j.Dic)
		d.Key("对接地址").SetValue(j.Address)
		d.Key("令牌").SetValue(j.Token)
		d.Key("调试打印").SetValue(strconv.FormatBool(j.Debug))
		dto.LoadConfig_secluded(d)
		ff.SaveIni(f)
		if j.Open {
			if dto.ServerConfig.SecludedBot != nil && dto.ServerConfig.SecludedBot.Addr != "" {
				secludedbot.Start(dto.ServerConfig.SecludedBot.Addr, dto.ServerConfig.SecludedBot.Token)
			}
		} else {
			secludedbot.Stop()
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "dic_encrypt_file":
		// 生成加密词库：读取 .n 词库文件，加密后在原目录生成同名 .bak 文件
		var j struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !checkFilePath(j.Path) {
			http.Error(w, `{"status":"error","error":"文件路径不合法"}`, http.StatusBadRequest)
			return
		}
		src := filepath.Join(utils.GetAppDir(), filepath.FromSlash(j.Path))
		data, err := os.ReadFile(src)
		if err != nil {
			http.Error(w, `{"status":"error","error":"文件读取失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		encodeDic, err := utils.Encrypt(string(data), appfiles.Key)
		if err != nil {
			http.Error(w, `{"status":"error","error":"加密失败"}`, http.StatusBadRequest)
			return
		}
		dst := j.Path + ".bak"
		full := filepath.Join(utils.GetAppDir(), filepath.FromSlash(dst))
		if err := os.WriteFile(full, []byte("// "+appfiles.Version+"\n"+encodeDic), 0o644); err != nil {
			http.Error(w, `{"status":"error","error":"文件写入失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		var rj struct {
			Status string `json:"status"`
			Path   string `json:"path"`
		}
		rj.Status = "ok"
		rj.Path = dst
		r, _ := json.Marshal(rj)
		w.Write(r)
		return

	case "install_php":
		appDir := utils.GetAppDir()
		destDir := filepath.Join(appDir, "private", "extensions", "php")
		if fileExists(filepath.Join(destDir, "php.exe")) {
			resp := HttpOpUiInstallResponse{
				Status: "ok",
				Output: []string{"PHP 已安装"},
			}
			jsonResp, _ := json.Marshal(resp)
			w.Write(jsonResp)
			return
		}
		// 防止重复安装：检查是否已有同组件运行中的任务
		if existingTask := findRunningTaskForComponent("php"); existingTask != nil {
			jsonResp, _ := json.Marshal(map[string]string{"status": "ok", "task_id": existingTask.ID})
			w.Write(jsonResp)
			return
		}
		taskID := generateTaskID()
		task := &InstallTask{
			ID:        taskID,
			Component: "php",
			Status:    "running",
			Progress:  0,
		}
		installTaskStore.Store(taskID, task)
		go func() {
			var output []string
			progressFn := func(p float64) { task.setProgress(p) }
			err := installPHP(destDir, &output, progressFn)
			for _, line := range output {
				task.addOutput(line)
			}
			if task.IsCancelled() {
				task.addOutput("⚠ 安装已取消")
				task.finish(nil)
				return
			}
			task.finish(err)
		}()
		jsonResp, _ := json.Marshal(map[string]string{"status": "ok", "task_id": taskID})
		w.Write(jsonResp)
		return

	case "install_ffmpeg":
		appDir := utils.GetAppDir()
		destDir := filepath.Join(appDir, "private", "extensions", "ffmpeg")
		if utils.FindFfmpegExe(destDir) != "" {
			resp := HttpOpUiInstallResponse{
				Status: "ok",
				Output: []string{"FFmpeg 已安装"},
			}
			jsonResp, _ := json.Marshal(resp)
			w.Write(jsonResp)
			return
		}
		// 防止重复安装
		if existingTask := findRunningTaskForComponent("ffmpeg"); existingTask != nil {
			jsonResp, _ := json.Marshal(map[string]string{"status": "ok", "task_id": existingTask.ID})
			w.Write(jsonResp)
			return
		}
		taskID := generateTaskID()
		task := &InstallTask{
			ID:        taskID,
			Component: "ffmpeg",
			Status:    "running",
			Progress:  0,
		}
		installTaskStore.Store(taskID, task)
		go func() {
			var output []string
			progressFn := func(p float64) { task.setProgress(p) }
			err := installFFmpeg(destDir, &output, progressFn)
			for _, line := range output {
				task.addOutput(line)
			}
			if task.IsCancelled() {
				task.addOutput("⚠ 安装已取消")
				task.finish(nil)
				return
			}
			task.finish(err)
		}()
		jsonResp, _ := json.Marshal(map[string]string{"status": "ok", "task_id": taskID})
		w.Write(jsonResp)
		return

	case "install_silk_v3":
		appDir := utils.GetAppDir()
		destDir := filepath.Join(appDir, "private", "extensions")
		if fileExists(filepath.Join(destDir, "silk_v3", "silk_v3_encoder.exe")) {
			resp := HttpOpUiInstallResponse{
				Status: "ok",
				Output: []string{"silk_v3 已安装"},
			}
			jsonResp, _ := json.Marshal(resp)
			w.Write(jsonResp)
			return
		}
		// 防止重复安装
		if existingTask := findRunningTaskForComponent("silk_v3"); existingTask != nil {
			jsonResp, _ := json.Marshal(map[string]string{"status": "ok", "task_id": existingTask.ID})
			w.Write(jsonResp)
			return
		}
		taskID := generateTaskID()
		task := &InstallTask{
			ID:        taskID,
			Component: "silk_v3",
			Status:    "running",
			Progress:  0,
		}
		installTaskStore.Store(taskID, task)
		go func() {
			var output []string
			progressFn := func(p float64) { task.setProgress(p) }
			err := installSilkV3(destDir, &output, progressFn)
			for _, line := range output {
				task.addOutput(line)
			}
			if task.IsCancelled() {
				task.addOutput("⚠ 安装已取消")
				task.finish(nil)
				return
			}
			task.finish(err)
		}()
		jsonResp, _ := json.Marshal(map[string]string{"status": "ok", "task_id": taskID})
		w.Write(jsonResp)
		return

	case "install_napcat_bot":
		var config HttpOpUiConfig_install
		if err := json.Unmarshal(h.Data, &config); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		qq, ok := config.Params["qq"]
		if !ok || qq == "" {
			w.Write([]byte(`{"status":"error","error":"missing qq parameter"}`))
			return
		}
		appDir := utils.GetAppDir()
		destDir := filepath.Join(appDir, "private", "extensions", "NapCat.Shell")
		if fileExists(filepath.Join(destDir, "launcher.bat")) {
			resp := HttpOpUiInstallResponse{
				Status: "ok",
				Output: []string{"napcat_bot 已安装"},
			}
			jsonResp, _ := json.Marshal(resp)
			w.Write(jsonResp)
			return
		}
		// 防止重复安装
		if existingTask := findRunningTaskForComponent("napcat_bot"); existingTask != nil {
			jsonResp, _ := json.Marshal(map[string]string{"status": "ok", "task_id": existingTask.ID})
			w.Write(jsonResp)
			return
		}
		taskID := generateTaskID()
		task := &InstallTask{
			ID:        taskID,
			Component: "napcat_bot",
			Status:    "running",
			Progress:  0,
		}
		installTaskStore.Store(taskID, task)
		go func() {
			var output []string
			progressFn := func(p float64) { task.setProgress(p) }
			err := installNapCatBot(destDir, qq, &output, progressFn)
			for _, line := range output {
				task.addOutput(line)
			}
			if task.IsCancelled() {
				task.addOutput("⚠ 安装已取消")
				task.finish(nil)
				return
			}
			task.finish(err)
		}()
		jsonResp, _ := json.Marshal(map[string]string{"status": "ok", "task_id": taskID})
		w.Write(jsonResp)
		return

	case "install_python":
		appDir := utils.GetAppDir()
		destDir := filepath.Join(appDir, "private", "extensions", "python")
		if isPythonInstalled(destDir) {
			resp := HttpOpUiInstallResponse{
				Status: "ok",
				Output: []string{"Python 已安装"},
			}
			jsonResp, _ := json.Marshal(resp)
			w.Write(jsonResp)
			return
		}
		// Linux 等非 Windows 平台使用系统 python3，无法通过此处下载安装
		if runtime.GOOS != "windows" {
			resp := HttpOpUiInstallResponse{
				Status: "error",
				Error:  "未检测到 Python 运行环境，请通过系统包管理器安装 python3",
			}
			jsonResp, _ := json.Marshal(resp)
			w.Write(jsonResp)
			return
		}
		// 防止重复安装
		if existingTask := findRunningTaskForComponent("python"); existingTask != nil {
			jsonResp, _ := json.Marshal(map[string]string{"status": "ok", "task_id": existingTask.ID})
			w.Write(jsonResp)
			return
		}
		taskID := generateTaskID()
		task := &InstallTask{
			ID:        taskID,
			Component: "python",
			Status:    "running",
			Progress:  0,
		}
		installTaskStore.Store(taskID, task)
		go func() {
			var output []string
			progressFn := func(p float64) { task.setProgress(p) }
			err := installPython(destDir, &output, progressFn)
			for _, line := range output {
				task.addOutput(line)
			}
			if task.IsCancelled() {
				task.addOutput("⚠ 安装已取消")
				task.finish(nil)
				return
			}
			task.finish(err)
		}()
		jsonResp, _ := json.Marshal(map[string]string{"status": "ok", "task_id": taskID})
		w.Write(jsonResp)
		return

	case "get_install_status":
		appDir := utils.GetAppDir()
		extDir := filepath.Join(appDir, "private", "extensions")
		allStatus := map[string]bool{
			"php":        fileExists(filepath.Join(extDir, "php", "php.exe")),
			"python":     isPythonInstalled(filepath.Join(extDir, "python")),
			"napcat_bot": fileExists(filepath.Join(extDir, "NapCat.Shell", "launcher.bat")),
			"ffmpeg":     utils.FindFfmpegExe(filepath.Join(extDir, "ffmpeg")) != "",
			"silk_v3":    fileExists(filepath.Join(extDir, "silk_v3", "silk_v3_encoder.exe")),
		}
		jsonResp, _ := json.Marshal(allStatus)
		w.Write(jsonResp)
		return

	case "install_progress":
		var j HttpOpUiConfig_installStatus
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if val, ok := installTaskStore.Load(j.TaskID); ok {
			task, ok := val.(*InstallTask)
			if !ok {
				http.Error(w, `{"status":"error","error":"invalid task"}`, http.StatusInternalServerError)
				return
			}
			status, output, errMsg, progress := task.snapshot()
			resp := map[string]any{
				"status":    status,
				"component": task.Component,
				"output":    output,
				"error":     errMsg,
				"progress":  progress,
			}
			jsonResp, _ := json.Marshal(resp)
			w.Write(jsonResp)
		} else {
			w.Write([]byte(`{"status":"not_found","error":"task not found"}`))
		}
		return

	case "install_cancel":
		var j HttpOpUiConfig_installStatus
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if val, ok := installTaskStore.Load(j.TaskID); ok {
			task, ok := val.(*InstallTask)
			if !ok {
				http.Error(w, `{"status":"error","error":"invalid task"}`, http.StatusInternalServerError)
				return
			}
			task.Cancel()
			w.Write([]byte(`{"status":"ok"}`))
		} else {
			w.Write([]byte(`{"status":"not_found","error":"task not found"}`))
		}
		return

	case "uninstall":
		var config HttpOpUiConfig_install
		if err := json.Unmarshal(h.Data, &config); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		appDir := utils.GetAppDir()
		// 非 Windows 仅支持卸载 Python
		if runtime.GOOS != "windows" && config.Component != "python" {
			resp := HttpOpUiInstallResponse{Status: "error", Error: "该组件仅支持 Windows 端"}
			jsonResp, _ := json.Marshal(resp)
			w.Write(jsonResp)
			return
		}
		var rmDir string
		switch config.Component {
		case "php":
			rmDir = filepath.Join(appDir, "private", "extensions", "php")
		case "ffmpeg":
			rmDir = filepath.Join(appDir, "private", "extensions", "ffmpeg")
		case "silk_v3":
			rmDir = filepath.Join(appDir, "private", "extensions", "silk_v3")
		case "napcat_bot":
			rmDir = filepath.Join(appDir, "private", "extensions", "NapCat.Shell")
		case "python":
			// Linux 使用系统 python3，仅当存在内置扩展时才可删除
			if runtime.GOOS != "windows" && !fileExists(filepath.Join(appDir, "private", "extensions", "python", "python3")) {
				resp := HttpOpUiInstallResponse{Status: "error", Error: "当前使用系统 python3，请通过系统包管理器卸载"}
				jsonResp, _ := json.Marshal(resp)
				w.Write(jsonResp)
				return
			}
			rmDir = filepath.Join(appDir, "private", "extensions", "python")
		default:
			http.Error(w, `{"status":"error","error":"unknown component"}`, http.StatusBadRequest)
			return
		}
		if err := os.RemoveAll(rmDir); err != nil {
			resp := HttpOpUiInstallResponse{Status: "error", Error: "卸载失败: " + err.Error()}
			jsonResp, _ := json.Marshal(resp)
			w.Write(jsonResp)
			return
		}
		resp := HttpOpUiInstallResponse{Status: "ok", Output: []string{config.Component + " 已卸载"}}
		jsonResp, _ := json.Marshal(resp)
		w.Write(jsonResp)
		return

	case "get_dic_doc":
		data, err := appfiles.GetFile("dic.md")
		if err != nil {
			http.Error(w, `{"status":"error","error":"embedded file not found"}`, http.StatusInternalServerError)
			return
		}
		html := markdown.ToHTML(data, nil, nil)
		resp := map[string]string{"content": string(html)}
		jsonResp, _ := json.Marshal(resp)
		w.Write(jsonResp)
		return

	case "get_dic_doc_raw":
		data, err := appfiles.GetFile("dic.md")
		if err != nil {
			http.Error(w, `{"status":"error","error":"embedded file not found"}`, http.StatusInternalServerError)
			return
		}
		resp := map[string]string{"content": string(data)}
		jsonResp, _ := json.Marshal(resp)
		w.Write(jsonResp)
		return

	case "get_autostart":
		enabled, err := GetAutoStart()
		if err != nil {
			w.Write([]byte(`{"enabled":false}`))
			return
		}
		jsonResp, _ := json.Marshal(map[string]bool{"enabled": enabled})
		w.Write(jsonResp)
		return

	case "set_autostart":
		if err := SetAutoStart(); err != nil {
			jsonResp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(jsonResp)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "cancel_autostart":
		if err := CancelAutoStart(); err != nil {
			jsonResp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(jsonResp)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "get_dic_list":
		// 词库打开：读取指定目录下的直接子项（文件夹 + .n 文件），逐层浏览，不递归扫描
		var j struct {
			Path string `json:"path"` // 当前目录，相对应用目录，空表示应用目录根
		}
		_ = json.Unmarshal(h.Data, &j)
		dirPath := strings.TrimSpace(j.Path)
		root := utils.GetAppDir()
		if dirPath != "" {
			if !checkFilePath(dirPath) {
				jsonResp, _ := json.Marshal(map[string]any{"entries": []any{}})
				w.Write(jsonResp)
				return
			}
			root = filepath.Join(utils.GetAppDir(), filepath.FromSlash(dirPath))
		}
		items, err := os.ReadDir(root)
		if err != nil {
			jsonResp, _ := json.Marshal(map[string]any{"entries": []any{}})
			w.Write(jsonResp)
			return
		}
		type dicEntry struct {
			Name string `json:"name"`
			Path string `json:"path"`
			Dir  bool   `json:"dir"`
		}
		entries := make([]dicEntry, 0, len(items))
		for _, it := range items {
			name := it.Name()
			if !it.IsDir() && !strings.HasSuffix(strings.ToLower(name), ".n") {
				continue // 只显示文件夹与 .n 词库文件
			}
			entries = append(entries, dicEntry{
				Name: name,
				Path: filepath.ToSlash(filepath.Join(dirPath, name)),
				Dir:  it.IsDir(),
			})
		}
		// 文件夹在前，文件夹/文件各自按名称排序
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].Dir != entries[j].Dir {
				return entries[i].Dir
			}
			return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
		})
		jsonResp, _ := json.Marshal(map[string]any{"entries": entries})
		w.Write(jsonResp)
		return

	case "dic_get_content":
		var j struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if j.Path == "" {
			http.Error(w, `{"status":"error","error":"词库路径不能为空"}`, http.StatusBadRequest)
			return
		}
		if !checkDicPath(j.Path) {
			http.Error(w, `{"status":"error","error":"词库路径不合法"}`, http.StatusBadRequest)
			return
		}
		content, err := utils.NewFileQueue(j.Path).ReadFromFile()
		if err != nil {
			if os.IsNotExist(err) {
				w.Write([]byte(`{"status":"not_found","error":"词库文件不存在"}`))
				return
			}
			http.Error(w, `{"status":"error","error":"词库读取失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}

		// 编译检测：打开词库时也编译一次，返回编译问题（error 红色 / warning 黄色）供前端高亮
		resp := map[string]any{"content": content}
		if dicCheck, err := dic_dto.RunDicNoCache(j.Path); err == nil && dicCheck != nil {
			defer dicCheck.Close()
			if len(dicCheck.Data.Warnings) > 0 {
				resp["warnings"] = dicCheck.Data.Warnings
			}
		}
		jsonResp, _ := json.Marshal(resp)
		w.Write(jsonResp)
		return

	case "dic_save_content":
		var j struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if j.Path == "" {
			http.Error(w, `{"status":"error","error":"词库路径不能为空"}`, http.StatusBadRequest)
			return
		}
		if !checkDicPath(j.Path) {
			http.Error(w, `{"status":"error","error":"词库路径不合法"}`, http.StatusBadRequest)
			return
		}
		utils.NewFileQueue(j.Path).WriteToFile(j.Content)

		// 编译检测：保存后重新编译词库，返回编译问题（error 红色 / warning 黄色）供前端高亮
		resp := map[string]any{"status": "ok"}
		if dicCheck, err := dic_dto.RunDicNoCache(j.Path); err == nil && dicCheck != nil {
			defer dicCheck.Close()
			if len(dicCheck.Data.Warnings) > 0 {
				resp["warnings"] = dicCheck.Data.Warnings
			}
		}
		jsonResp, _ := json.Marshal(resp)
		w.Write(jsonResp)
		return

	case "file_list":
		// 文件管理：列出指定目录（应用目录内）的直接子项（文件夹 + 文件），逐层浏览
		var j struct {
			Path string `json:"path"` // 当前目录，相对应用目录，空表示应用目录根
		}
		_ = json.Unmarshal(h.Data, &j)
		dirPath := strings.TrimSpace(j.Path)
		root := utils.GetAppDir()
		if dirPath != "" {
			if !checkFilePath(dirPath) {
				http.Error(w, `{"status":"error","error":"目录路径不合法"}`, http.StatusBadRequest)
				return
			}
			root = filepath.Join(utils.GetAppDir(), filepath.FromSlash(dirPath))
		}
		items, err := os.ReadDir(root)
		if err != nil {
			http.Error(w, `{"status":"error","error":"读取目录失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		type fileEntry struct {
			Name  string `json:"name"`
			Path  string `json:"path"`
			Dir   bool   `json:"dir"`
			Size  int64  `json:"size"`
			Mtime int64  `json:"mtime"`
		}
		entries := make([]fileEntry, 0, len(items))
		for _, it := range items {
			name := it.Name()
			info, ierr := it.Info()
			if ierr != nil {
				continue
			}
			entries = append(entries, fileEntry{
				Name:  name,
				Path:  filepath.ToSlash(filepath.Join(dirPath, name)),
				Dir:   it.IsDir(),
				Size:  info.Size(),
				Mtime: info.ModTime().Unix(),
			})
		}
		// 文件夹在前，文件夹/文件各自按名称排序
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].Dir != entries[j].Dir {
				return entries[i].Dir
			}
			return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
		})
		jsonResp, _ := json.Marshal(map[string]any{
			"entries": entries,
			"root":    filepath.Base(utils.GetAppDir()), // 应用数据目录名，供前端面包屑根节点显示真实目录名
		})
		w.Write(jsonResp)
		return

	case "file_search":
		// 文件管理搜索：默认搜索当前目录下的直接子项（文件夹+文件），
		// 开启深度搜索则递归子目录；默认按名称包含关键字（忽略大小写），
		// 开启正则则按正则表达式匹配名称（忽略大小写）
		var j struct {
			Path    string `json:"path"`
			Keyword string `json:"keyword"`
			Deep    bool   `json:"deep"`
			Regex   bool   `json:"regex"`
		}
		_ = json.Unmarshal(h.Data, &j)
		dirPath := strings.TrimSpace(j.Path)
		root := utils.GetAppDir()
		if dirPath != "" {
			if !checkFilePath(dirPath) {
				http.Error(w, `{"status":"error","error":"目录路径不合法"}`, http.StatusBadRequest)
				return
			}
			root = filepath.Join(utils.GetAppDir(), filepath.FromSlash(dirPath))
		}
		kw := strings.TrimSpace(j.Keyword)
		var re *regexp.Regexp
		if j.Regex && kw != "" {
			var rerr error
			re, rerr = regexp.Compile("(?i)" + kw)
			if rerr != nil {
				http.Error(w, `{"status":"error","error":"正则表达式无效: `+rerr.Error()+`"}`, http.StatusBadRequest)
				return
			}
		}
		matchName := func(name string) bool {
			if kw == "" {
				return true
			}
			if re != nil {
				return re.MatchString(name)
			}
			return strings.Contains(strings.ToLower(name), strings.ToLower(kw))
		}
		type fileEntry struct {
			Name  string `json:"name"`
			Path  string `json:"path"`
			Dir   bool   `json:"dir"`
			Size  int64  `json:"size"`
			Mtime int64  `json:"mtime"`
		}
		entries := make([]fileEntry, 0)
		if j.Deep {
			_ = filepath.Walk(root, func(full string, info os.FileInfo, err error) error {
				if err != nil || full == root {
					return nil
				}
				name := info.Name()
				if !matchName(name) {
					return nil
				}
				rel, _ := filepath.Rel(root, full)
				relPath := filepath.ToSlash(rel)
				if dirPath != "" {
					relPath = filepath.ToSlash(filepath.Join(dirPath, rel))
				}
				entries = append(entries, fileEntry{
					Name:  name,
					Path:  relPath,
					Dir:   info.IsDir(),
					Size:  info.Size(),
					Mtime: info.ModTime().Unix(),
				})
				return nil
			})
		} else {
			items, err := os.ReadDir(root)
			if err != nil {
				http.Error(w, `{"status":"error","error":"读取目录失败: `+err.Error()+`"}`, http.StatusBadRequest)
				return
			}
			for _, it := range items {
				name := it.Name()
				if !matchName(name) {
					continue
				}
				info, ierr := it.Info()
				if ierr != nil {
					continue
				}
				entries = append(entries, fileEntry{
					Name:  name,
					Path:  filepath.ToSlash(filepath.Join(dirPath, name)),
					Dir:   it.IsDir(),
					Size:  info.Size(),
					Mtime: info.ModTime().Unix(),
				})
			}
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].Dir != entries[j].Dir {
				return entries[i].Dir
			}
			return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
		})
		jsonResp, _ := json.Marshal(map[string]any{"entries": entries})
		w.Write(jsonResp)
		return

	case "file_read":
		// 读取应用目录下任意文件（.n 词库请使用 dic_get_content）
		var j struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !checkFilePath(j.Path) {
			http.Error(w, `{"status":"error","error":"文件路径不合法"}`, http.StatusBadRequest)
			return
		}
		full := filepath.Join(utils.GetAppDir(), filepath.FromSlash(j.Path))
		data, err := os.ReadFile(full)
		if err != nil {
			http.Error(w, `{"status":"error","error":"文件读取失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		// 检测二进制：含 NUL 字节或非 UTF-8 文本，避免前端直接编辑损坏内容
		binary := bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data)
		resp := map[string]any{
			"path":   j.Path,
			"size":   int64(len(data)),
			"binary": binary,
		}
		if !binary {
			resp["content"] = string(data)
		}
		jsonResp, _ := json.Marshal(resp)
		w.Write(jsonResp)
		return

	case "file_image":
		// 读取图片文件并返回 base64，供前端在线预览
		var j struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !checkFilePath(j.Path) {
			http.Error(w, `{"status":"error","error":"文件路径不合法"}`, http.StatusBadRequest)
			return
		}
		full := filepath.Join(utils.GetAppDir(), filepath.FromSlash(j.Path))
		data, err := os.ReadFile(full)
		if err != nil {
			http.Error(w, `{"status":"error","error":"文件读取失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		mime := "application/octet-stream"
		switch strings.ToLower(filepath.Ext(j.Path)) {
		case ".png":
			mime = "image/png"
		case ".jpg", ".jpeg":
			mime = "image/jpeg"
		case ".gif":
			mime = "image/gif"
		case ".webp":
			mime = "image/webp"
		case ".bmp":
			mime = "image/bmp"
		case ".svg":
			mime = "image/svg+xml"
		case ".ico":
			mime = "image/x-icon"
		case ".avif":
			mime = "image/avif"
		}
		jsonResp, _ := json.Marshal(map[string]any{
			"path": j.Path,
			"mime": mime,
			"data": base64.StdEncoding.EncodeToString(data),
		})
		w.Write(jsonResp)
		return

	case "file_write":
		// 写入应用目录下任意文件（.n 词库请使用 dic_save_content）
		var j struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !checkFilePath(j.Path) {
			http.Error(w, `{"status":"error","error":"文件路径不合法"}`, http.StatusBadRequest)
			return
		}
		full := filepath.Join(utils.GetAppDir(), filepath.FromSlash(j.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			http.Error(w, `{"status":"error","error":"创建目录失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		if err := os.WriteFile(full, []byte(j.Content), 0o644); err != nil {
			http.Error(w, `{"status":"error","error":"文件写入失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
		w.Write(jsonResp)
		return

	case "file_db_tables":
		// 文件管理：列出 SQLite 数据库中的表/视图及建表语句
		var j struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		db := opuiOpenSqlite(w, j.Path)
		if db == nil {
			return
		}
		defer db.Close()
		rows, err := db.Query(`SELECT name, type, sql FROM sqlite_master WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY name`)
		if err != nil {
			http.Error(w, `{"status":"error","error":"数据库读取失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		defer rows.Close()
		tables := make([]map[string]string, 0)
		for rows.Next() {
			var name, typ string
			var sqlText *string
			if err := rows.Scan(&name, &typ, &sqlText); err != nil {
				continue
			}
			createSQL := ""
			if sqlText != nil {
				createSQL = *sqlText
			}
			tables = append(tables, map[string]string{"name": name, "type": typ, "sql": createSQL})
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok", "tables": tables})
		w.Write(jsonResp)
		return

	case "file_db_query":
		// 文件管理：对 SQLite 数据库执行 SQL（SELECT/WITH/PRAGMA 返回表格，其余为执行语句）
		var j struct {
			Path string `json:"path"`
			Sql  string `json:"sql"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		db := opuiOpenSqlite(w, j.Path)
		if db == nil {
			return
		}
		defer db.Close()
		trimmed := strings.ToUpper(stripLeadingSQLComments(j.Sql))
		isQuery := strings.HasPrefix(trimmed, "SELECT") || strings.HasPrefix(trimmed, "WITH") ||
			strings.HasPrefix(trimmed, "PRAGMA") || strings.HasPrefix(trimmed, "EXPLAIN") ||
			strings.HasPrefix(trimmed, "VALUES") || strings.Contains(trimmed, " RETURNING ")
		if !isQuery {
			res, err := db.Exec(j.Sql)
			if err != nil {
				jsonResp, _ := json.Marshal(map[string]any{"status": "ok", "error": err.Error()})
				w.Write(jsonResp)
				return
			}
			affected, _ := res.RowsAffected()
			id, _ := res.LastInsertId()
			jsonResp, _ := json.Marshal(map[string]any{"status": "ok", "rows_affected": affected, "last_insert_id": id})
			w.Write(jsonResp)
			return
		}
		// 查询路径：单表 SELECT 且为 rowid 表时自动前置 rowid，使结果支持内联编辑
		execSQL := j.Sql
		editable := false
		table := ""
		if t, ok := detectEditableTable(j.Sql); ok && isRowidTable(db, t) {
			execSQL = prependRowid(j.Sql)
			editable = true
			table = t
		}
		rows, err := db.Query(execSQL)
		if err != nil {
			// 改写后的查询失败则回退原始查询（只读）
			if execSQL != j.Sql {
				rows, err = db.Query(j.Sql)
				editable = false
				table = ""
			}
			if err != nil {
				jsonResp, _ := json.Marshal(map[string]any{"status": "ok", "error": err.Error()})
				w.Write(jsonResp)
				return
			}
		}
		defer rows.Close()
		columns, _ := rows.Columns()
		out := make([][]any, 0)
		for rows.Next() {
			// 结果行数上限，避免大表拖垮前端
			if len(out) >= 1000 {
				break
			}
			vals := make([]any, len(columns))
			ptrs := make([]any, len(columns))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				continue
			}
			row := make([]any, len(columns))
			for i, v := range vals {
				row[i] = sqliteCellValue(v)
			}
			out = append(out, row)
		}
		jsonResp, _ := json.Marshal(map[string]any{
			"status": "ok", "columns": columns, "rows": out,
			"editable": editable, "table": table,
		})
		w.Write(jsonResp)
		return

	case "file_db_update_cell":
		// 文件管理：内联编辑表格单元格（通过 rowid 定位行，仅限简单标识符表/列）
		var j struct {
			Path   string `json:"path"`
			Table  string `json:"table"`
			Rowid  int64  `json:"rowid"`
			Column string `json:"column"`
			Value  any    `json:"value"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !isSimpleIdent(j.Table) || !isSimpleIdent(j.Column) {
			http.Error(w, `{"status":"error","error":"参数不合法"}`, http.StatusBadRequest)
			return
		}
		db := opuiOpenSqlite(w, j.Path)
		if db == nil {
			return
		}
		defer db.Close()
		// 值绑定：空字符串视为 NULL；其余原样按字符串绑定，交由 SQLite 列类型亲和性自动转换，
		// 避免后端强转数字导致 TEXT 列前导零（如 "007"）被截断
		var bind any
		if s, ok := j.Value.(string); ok {
			bind = s
			if s == "" {
				bind = nil
			}
		} else {
			bind = j.Value
		}
		updateSQL := fmt.Sprintf(`UPDATE "%s" SET "%s" = ? WHERE rowid = ?`, j.Table, j.Column)
		res, err := db.Exec(updateSQL, bind, j.Rowid)
		if err != nil {
			jsonResp, _ := json.Marshal(map[string]any{"status": "ok", "error": err.Error()})
			w.Write(jsonResp)
			return
		}
		affected, _ := res.RowsAffected()
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok", "rows_affected": affected})
		w.Write(jsonResp)
		return

	case "file_create":
		// 文件管理：在指定目录下创建新文件（可选初始内容，默认空文件）
		var j struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !checkFilePath(j.Path) {
			http.Error(w, `{"status":"error","error":"文件路径不合法"}`, http.StatusBadRequest)
			return
		}
		full := filepath.Join(utils.GetAppDir(), filepath.FromSlash(j.Path))
		if _, err := os.Stat(full); err == nil {
			http.Error(w, `{"status":"error","error":"文件已存在"}`, http.StatusBadRequest)
			return
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			http.Error(w, `{"status":"error","error":"创建目录失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		if err := os.WriteFile(full, []byte(j.Content), 0o644); err != nil {
			http.Error(w, `{"status":"error","error":"文件创建失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
		w.Write(jsonResp)
		return

	case "file_delete":
		// 文件管理：删除指定文件或目录（递归删除）
		var j struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !checkFilePath(j.Path) {
			http.Error(w, `{"status":"error","error":"文件路径不合法"}`, http.StatusBadRequest)
			return
		}
		full := filepath.Join(utils.GetAppDir(), filepath.FromSlash(j.Path))
		if err := os.RemoveAll(full); err != nil {
			http.Error(w, `{"status":"error","error":"删除失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
		w.Write(jsonResp)
		return

	case "file_upload":
		// 文件管理：上传文件（内容为 base64 编码），同名文件覆盖
		var j struct {
			Path string `json:"path"`
			Data string `json:"data"` // base64 编码的文件内容
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !checkFilePath(j.Path) {
			http.Error(w, `{"status":"error","error":"文件路径不合法"}`, http.StatusBadRequest)
			return
		}
		raw, err := base64.StdEncoding.DecodeString(j.Data)
		if err != nil {
			http.Error(w, `{"status":"error","error":"文件内容解码失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		full := filepath.Join(utils.GetAppDir(), filepath.FromSlash(j.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			http.Error(w, `{"status":"error","error":"创建目录失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		if err := os.WriteFile(full, raw, 0o644); err != nil {
			http.Error(w, `{"status":"error","error":"文件写入失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
		w.Write(jsonResp)
		return

	case "file_upload_status":
		// 文件管理：查询分块上传进度（已上传的分块序号）
		var j struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !checkFilePath(j.Path) {
			http.Error(w, `{"status":"error","error":"文件路径不合法"}`, http.StatusBadRequest)
			return
		}
		chunks := []int{}
		if items, err := os.ReadDir(uploadTmpDir(j.Path)); err == nil {
			for _, e := range items {
				name := e.Name()
				if !strings.HasPrefix(name, "chunk_") {
					continue
				}
				idx, err := strconv.Atoi(strings.TrimPrefix(name, "chunk_"))
				if err == nil {
					chunks = append(chunks, idx)
				}
			}
		}
		sort.Ints(chunks)
		jsonResp, _ := json.Marshal(map[string]any{"chunks": chunks})
		w.Write(jsonResp)
		return

	case "file_upload_chunk":
		// 文件管理：上传单个分块（base64）
		var j struct {
			Path  string `json:"path"`
			Index int    `json:"index"`
			Total int    `json:"total"`
			Data  string `json:"data"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !checkFilePath(j.Path) {
			http.Error(w, `{"status":"error","error":"文件路径不合法"}`, http.StatusBadRequest)
			return
		}
		if j.Index < 0 || j.Total <= 0 || j.Index >= j.Total {
			http.Error(w, `{"status":"error","error":"分块参数不合法"}`, http.StatusBadRequest)
			return
		}
		raw, err := base64.StdEncoding.DecodeString(j.Data)
		if err != nil {
			http.Error(w, `{"status":"error","error":"分块内容解码失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		tmpDir := uploadTmpDir(j.Path)
		if err := os.MkdirAll(tmpDir, 0o755); err != nil {
			http.Error(w, `{"status":"error","error":"创建临时目录失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		chunkFile := filepath.Join(tmpDir, fmt.Sprintf("chunk_%d", j.Index))
		if err := os.WriteFile(chunkFile, raw, 0o644); err != nil {
			http.Error(w, `{"status":"error","error":"分块写入失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
		w.Write(jsonResp)
		return

	case "file_upload_merge":
		// 文件管理：合并所有分块为最终文件
		var j struct {
			Path  string `json:"path"`
			Total int    `json:"total"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !checkFilePath(j.Path) {
			http.Error(w, `{"status":"error","error":"文件路径不合法"}`, http.StatusBadRequest)
			return
		}
		if j.Total <= 0 {
			http.Error(w, `{"status":"error","error":"分块总数不合法"}`, http.StatusBadRequest)
			return
		}
		tmpDir := uploadTmpDir(j.Path)
		full := filepath.Join(utils.GetAppDir(), filepath.FromSlash(j.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			http.Error(w, `{"status":"error","error":"创建目录失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		out, err := os.Create(full)
		if err != nil {
			http.Error(w, `{"status":"error","error":"创建目标文件失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		var mergeErr error
		for i := 0; i < j.Total; i++ {
			chunkFile := filepath.Join(tmpDir, fmt.Sprintf("chunk_%d", i))
			data, err := os.ReadFile(chunkFile)
			if err != nil {
				mergeErr = fmt.Errorf("分块 %d 缺失或读取失败: %v", i, err)
				break
			}
			if _, err := out.Write(data); err != nil {
				mergeErr = fmt.Errorf("写入目标文件失败: %v", err)
				break
			}
		}
		out.Close()
		if mergeErr != nil {
			_ = os.Remove(full)
			http.Error(w, `{"status":"error","error":"`+mergeErr.Error()+`"}`, http.StatusBadRequest)
			return
		}
		_ = os.RemoveAll(tmpDir) // 清理临时分块
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
		w.Write(jsonResp)
		return

	case "file_mkdir":
		// 文件管理：新建文件夹
		var j struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !checkFilePath(j.Path) {
			http.Error(w, `{"status":"error","error":"路径不合法"}`, http.StatusBadRequest)
			return
		}
		full := filepath.Join(utils.GetAppDir(), filepath.FromSlash(j.Path))
		if _, err := os.Stat(full); err == nil {
			http.Error(w, `{"status":"error","error":"已存在同名项"}`, http.StatusBadRequest)
			return
		}
		if err := os.MkdirAll(full, 0o755); err != nil {
			http.Error(w, `{"status":"error","error":"创建文件夹失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
		w.Write(jsonResp)
		return

	case "file_rename":
		// 文件管理：重命名文件/目录
		var j struct {
			Path    string `json:"path"`
			NewName string `json:"newName"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !checkFilePath(j.Path) {
			http.Error(w, `{"status":"error","error":"路径不合法"}`, http.StatusBadRequest)
			return
		}
		newName := strings.TrimSpace(j.NewName)
		if newName == "" {
			http.Error(w, `{"status":"error","error":"名称不能为空"}`, http.StatusBadRequest)
			return
		}
		if newName == "." || newName == ".." || strings.ContainsAny(newName, `/\`) {
			http.Error(w, `{"status":"error","error":"名称不合法"}`, http.StatusBadRequest)
			return
		}
		oldFull := filepath.Join(utils.GetAppDir(), filepath.FromSlash(j.Path))
		if _, err := os.Stat(oldFull); err != nil {
			http.Error(w, `{"status":"error","error":"原文件不存在"}`, http.StatusBadRequest)
			return
		}
		newFull := filepath.Join(filepath.Dir(oldFull), newName)
		if _, err := os.Stat(newFull); err == nil {
			http.Error(w, `{"status":"error","error":"已存在同名项"}`, http.StatusBadRequest)
			return
		}
		if err := os.Rename(oldFull, newFull); err != nil {
			http.Error(w, `{"status":"error","error":"重命名失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
		w.Write(jsonResp)
		return

	case "file_copy":
		// 文件管理：复制多个文件/目录到目标目录
		var j struct {
			Paths  []string `json:"paths"`
			Target string   `json:"target"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if len(j.Paths) == 0 {
			http.Error(w, `{"status":"error","error":"未选择文件"}`, http.StatusBadRequest)
			return
		}
		for _, p := range j.Paths {
			if !checkFilePath(p) {
				http.Error(w, `{"status":"error","error":"源路径不合法"}`, http.StatusBadRequest)
				return
			}
		}
		if j.Target != "" && !checkFilePath(j.Target) {
			http.Error(w, `{"status":"error","error":"目标路径不合法"}`, http.StatusBadRequest)
			return
		}
		appDir := utils.GetAppDir()
		targetDir := filepath.Join(appDir, filepath.FromSlash(j.Target))
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			http.Error(w, `{"status":"error","error":"创建目标目录失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		for _, p := range j.Paths {
			src := filepath.Join(appDir, filepath.FromSlash(p))
			base := filepath.Base(src)
			dst := filepath.Join(targetDir, base)
			if _, err := os.Stat(dst); err == nil {
				http.Error(w, `{"status":"error","error":"目标已存在同名项: `+base+`"}`, http.StatusBadRequest)
				return
			}
			if err := copyPath(src, dst); err != nil {
				http.Error(w, `{"status":"error","error":"复制失败: `+err.Error()+`"}`, http.StatusBadRequest)
				return
			}
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
		w.Write(jsonResp)
		return

	case "file_move":
		// 文件管理：移动（剪切）多个文件/目录到目标目录
		var j struct {
			Paths  []string `json:"paths"`
			Target string   `json:"target"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if len(j.Paths) == 0 {
			http.Error(w, `{"status":"error","error":"未选择文件"}`, http.StatusBadRequest)
			return
		}
		for _, p := range j.Paths {
			if !checkFilePath(p) {
				http.Error(w, `{"status":"error","error":"源路径不合法"}`, http.StatusBadRequest)
				return
			}
		}
		if j.Target != "" && !checkFilePath(j.Target) {
			http.Error(w, `{"status":"error","error":"目标路径不合法"}`, http.StatusBadRequest)
			return
		}
		appDir := utils.GetAppDir()
		targetDir := filepath.Join(appDir, filepath.FromSlash(j.Target))
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			http.Error(w, `{"status":"error","error":"创建目标目录失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		for _, p := range j.Paths {
			src := filepath.Join(appDir, filepath.FromSlash(p))
			base := filepath.Base(src)
			dst := filepath.Join(targetDir, base)
			if filepath.Clean(src) == filepath.Clean(dst) {
				continue // 源就在目标目录，无需移动
			}
			if _, err := os.Stat(dst); err == nil {
				http.Error(w, `{"status":"error","error":"目标已存在同名项: `+base+`"}`, http.StatusBadRequest)
				return
			}
			if err := os.Rename(src, dst); err != nil {
				http.Error(w, `{"status":"error","error":"移动失败: `+err.Error()+`"}`, http.StatusBadRequest)
				return
			}
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
		w.Write(jsonResp)
		return

	case "get_dic_funcs":
		// 返回全部已注册词库函数（实时读取注册表），供前端代码补全
		jsonResp, _ := json.Marshal(map[string]any{"cmds": dic_funcs.ListFuncs()})
		w.Write(jsonResp)
		return

	case "get_dic_tasks":
		// 返回定时任务（分页），避免任务过多时一次性返回全部
		var j struct {
			Limit int `json:"limit"`
			Skip  int `json:"skip"`
		}
		_ = json.Unmarshal(h.Data, &j)
		if j.Limit <= 0 {
			j.Limit = 50
		}
		if j.Skip < 0 {
			j.Skip = 0
		}
		all := dic_funcs.ListScheduledTasks()
		total := len(all)
		start := j.Skip
		if start > total {
			start = total
		}
		end := j.Skip + j.Limit
		if end > total {
			end = total
		}
		jsonResp, _ := json.Marshal(map[string]any{
			"list":    all[start:end],
			"total":   total,
			"hasMore": end < total,
		})
		w.Write(jsonResp)
		return

	case "add_dic_task":
		var j struct {
			DicPath    string `json:"dic_path"`
			Trigger    string `json:"trigger"`
			Interval   string `json:"interval"`
			Once       bool   `json:"once"`
			RunAtStart bool   `json:"run_at_start"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		// 词库路径留空时使用 system.ini 中 [词库调试] 默认词库
		if j.DicPath == "" {
			if p, ok := loadDicDebugDefaults()["path"].(string); ok {
				j.DicPath = p
			}
		}
		if !checkDicPath(j.DicPath) {
			http.Error(w, `{"status":"error","error":"词库路径不合法"}`, http.StatusBadRequest)
			return
		}
		id, err := dic_funcs.AddScheduledTask(j.DicPath, j.Trigger, j.Interval, j.Once, j.RunAtStart)
		if err != nil {
			http.Error(w, `{"status":"error","error":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok", "id": id})
		w.Write(jsonResp)
		return

	case "del_dic_task":
		var j struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if err := dic_funcs.DelScheduledTask(j.ID); err != nil {
			http.Error(w, `{"status":"error","error":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
		w.Write(jsonResp)
		return

	case "get_dic_config":
		// 读取词库调试运行配置（system.ini 的 [词库调试] 节）
		jsonResp, _ := json.Marshal(loadDicDebugDefaults())
		w.Write(jsonResp)
		return

	case "save_dic_config":
		// 保存词库调试运行配置到 system.ini 的 [词库调试] 节
		var cfg map[string]any
		if err := json.Unmarshal(h.Data, &cfg); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		file := utils.NewFile()
		file.SetPath("private/system/system.ini")
		iniFile, err := file.LoadIni()
		if err != nil {
			http.Error(w, `{"status":"error","error":"读取 system.ini 失败"}`, http.StatusInternalServerError)
			return
		}
		sec := iniFile.Section("词库调试")
		if v, ok := cfg["path"].(string); ok && v != "" {
			sec.Key("默认词库").SetValue(v)
		}
		if tabs, ok := cfg["tabs"].([]any); ok {
			var items []string
			for _, it := range tabs {
				if s, ok := it.(string); ok && s != "" {
					items = append(items, s)
				}
			}
			// 路径列表整体 JSON 编码存储，保持 ini 单行值
			if b, err := json.Marshal(items); err == nil {
				sec.Key("打开的标签").SetValue(string(b))
			}
		}
		if v, ok := cfg["trigger"].(string); ok {
			sec.Key("触发文本").SetValue(v)
		}
		if v, ok := cfg["timeout"].(float64); ok {
			sec.Key("超时").SetValue(strconv.Itoa(int(v)))
		}
		if v, ok := cfg["historyMax"].(float64); ok && v > 0 {
			sec.Key("历史记录数量").SetValue(strconv.Itoa(int(v)))
		}
		if v, ok := cfg["saveRun"].(bool); ok {
			sec.Key("保存运行").SetValue(strconv.FormatBool(v))
		}
		if v, ok := cfg["autoSave"].(bool); ok {
			sec.Key("实时保存").SetValue(strconv.FormatBool(v))
		}
		if g, ok := cfg["g"].([]any); ok {
			var items []string
			for _, it := range g {
				if s, ok := it.(string); ok {
					items = append(items, s)
				}
			}
			// 值可含任意换行：整体 JSON 编码存储（ini 值保持单行，避免按行拆分时被截断）
			if b, err := json.Marshal(items); err == nil {
				sec.Key("全局变量").SetValue(string(b))
			}
		}
		if err := file.SaveIni(iniFile); err != nil {
			http.Error(w, `{"status":"error","error":"写入 system.ini 失败: `+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
		w.Write(jsonResp)
		return

	case "dic_debug_run":
		var j struct {
			Path    string            `json:"path"`
			Trigger string            `json:"trigger"`
			G       map[string]string `json:"g"`
			// 超时（秒），0 表示不限时；超时后强行打断词库执行
			Timeout int `json:"timeout"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if j.Path == "" {
			http.Error(w, `{"status":"error","error":"词库路径不能为空"}`, http.StatusBadRequest)
			return
		}
		if !checkDicPath(j.Path) {
			http.Error(w, `{"status":"error","error":"词库路径不合法"}`, http.StatusBadRequest)
			return
		}
		dic, err := dic_dto.RunDic(j.Path)
		if err != nil {
			http.Error(w, `{"status":"error","error":"词库加载失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		defer dic.Close()

		// 编译存在 error 级诊断（框配对错误、触发词正则错误等）时拒绝运行：
		// 不执行词库，直接把错误与编译诊断回传前端高亮。
		for _, warn := range dic.Data.Warnings {
			if warn.Level == "error" {
				jsonResp, _ := json.Marshal(map[string]any{
					"output":       "",
					"timedOut":     false,
					"segments":     []any{},
					"vars":         map[string]any{"P": map[string]any{}, "G": map[string]any{}, "GV": map[string]any{}},
					"warnings":     dic.Data.Warnings,
					"compileError": "编译存在错误，无法运行",
				})
				w.Write(jsonResp)
				return
			}
		}

		// 注入词库路径，便于错误日志显示来源（顶层词库默认没有 _词库路径_）
		dic.Val.P.Set("_词库路径_", j.Path)

		// 注入全局变量
		for k, v := range j.G {
			dic.Val.G.Set(k, v)
		}

		var output string
		var timedOut bool
		if j.Timeout > 0 {
			output, timedOut = dic_api.Api.DicRunTimeout(dic, j.Trigger, time.Duration(j.Timeout)*time.Second)
		} else {
			output = dic_api.Api.DicRun(dic, j.Trigger)
		}

		// 从输出中提取错误行号（格式：funcName(line:N)：error 或 JS错误(line:N)：error）
		var errorLine int
		if re := regexp.MustCompile(`\(line:(\d+)\)`); re != nil {
			if m := re.FindStringSubmatch(output); len(m) >= 2 {
				errorLine, _ = strconv.Atoi(m[1])
			}
		}

		// 收集运行后的局部/全局变量（值 + 类型，类实例携带成员变量供前端折叠）
		pVars := make(map[string]any)
		for k, v := range dic.Val.P.GetAll() {
			pVars[k] = varDebugItem(v)
		}
		gVars := make(map[string]any)
		for k, v := range dic.Val.G.GetAll() {
			gVars[k] = varDebugItem(v)
		}
		gvVars := make(map[string]any)
		for k, v := range dto.GV.GetAll() {
			// 系统内部线程变量（_ 前缀）不纳入监控，避免与用户线程变量混淆
			if strings.HasPrefix(k, "_") {
				continue
			}
			gvVars[k] = varDebugItem(v)
		}

		resp := map[string]any{
			"output":   output,
			"timedOut": timedOut,
			"segments": parseOutputSegments(output),
			"vars": map[string]any{
				"P":  pVars,
				"G":  gVars,
				"GV": gvVars,
			},
		}
		if errorLine > 0 {
			resp["errorLine"] = errorLine
		}
		// 编译警告（如循环引入），前端以黄色警告展示
		if len(dic.Data.Warnings) > 0 {
			resp["warnings"] = dic.Data.Warnings
		}
		jsonResp, _ := json.Marshal(resp)
		w.Write(jsonResp)
		return

	case "get_thread_vars":
		// 返回线程变量（分页），只在请求区间内做类型识别/展开，避免变量过多时全量解析拖慢响应
		var j struct {
			Limit int `json:"limit"`
			Skip  int `json:"skip"`
		}
		_ = json.Unmarshal(h.Data, &j)
		if j.Limit <= 0 {
			j.Limit = 50
		}
		if j.Skip < 0 {
			j.Skip = 0
		}
		all := dto.GV.GetAll()
		keys := make([]string, 0, len(all))
		for k := range all {
			if strings.HasPrefix(k, "_") {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		total := len(keys)
		start := j.Skip
		if start > total {
			start = total
		}
		end := j.Skip + j.Limit
		if end > total {
			end = total
		}
		items := make([]map[string]any, 0, end-start)
		for _, k := range keys[start:end] {
			item := varDebugItem(all[k])
			item["key"] = k
			items = append(items, item)
		}
		r, _ := json.Marshal(map[string]any{
			"list":    items,
			"total":   total,
			"hasMore": end < total,
		})
		w.Write(r)
		return

	case "set_thread_var":
		var j struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if j.Key == "" {
			http.Error(w, `{"status":"error","error":"线程变量键不能为空"}`, http.StatusBadRequest)
			return
		}
		dto.SetThreadVar(j.Key, j.Value)
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "del_thread_var":
		var j struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if j.Key == "" {
			http.Error(w, `{"status":"error","error":"线程变量键不能为空"}`, http.StatusBadRequest)
			return
		}
		dto.DeleteThreadVar(j.Key)
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "dic_thread_vars_clear":
		dto.ClearThreadVars()
		jsonResp, _ := json.Marshal(map[string]any{"ok": true})
		w.Write(jsonResp)
		return

	case "get_server_logs":
		var j struct {
			Limit int `json:"limit"`
			Skip  int `json:"skip"`
		}
		_ = json.Unmarshal(h.Data, &j)
		logs, hasMore := readServerLogs(j.Limit, j.Skip)
		jsonResp, _ := json.Marshal(map[string]any{"logs": logs, "hasMore": hasMore})
		w.Write(jsonResp)
		return

	case "clear_server_logs":
		ClearServerLogs()
		jsonResp, _ := json.Marshal(map[string]any{"ok": true})
		w.Write(jsonResp)
		return

	case "terminal_input":
		var j struct {
			Input string `json:"input"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		input := strings.TrimSpace(j.Input)
		if input == "" {
			http.Error(w, `{"status":"error","error":"输入不能为空"}`, http.StatusBadRequest)
			return
		}
		go runTerminalDic(input)
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "get_sys_status":
		data, err := getSysStatus()
		if err != nil {
			debugLog.Errorf("[OPUI] get_sys_status failed: %v", err)
			http.Error(w, `{"status":"error","error":"collect failed"}`, http.StatusInternalServerError)
			return
		}
		if r, err := json.Marshal(data); err == nil {
			w.Write(r)
		} else {
			http.Error(w, `{"status":"error","error":"marshal failed"}`, http.StatusInternalServerError)
		}
		return

	case "check_update":
		jsonResp, _ := json.Marshal(checkUpdate())
		w.Write(jsonResp)
		return

	case "online_update":
		// 启动/继续在线更新下载（幂等：下载中则忽略，暂停则继续）
		go startOnlineUpdate()
		resp, _ := json.Marshal(map[string]any{"status": "ok", "msg": "开始下载更新", "data": getUpdateStatus()})
		w.Write(resp)
		return

	case "pause_update":
		pauseOnlineUpdate()
		resp, _ := json.Marshal(map[string]any{"status": "ok", "data": getUpdateStatus()})
		w.Write(resp)
		return

	case "resume_update":
		go startOnlineUpdate()
		resp, _ := json.Marshal(map[string]any{"status": "ok", "data": getUpdateStatus()})
		w.Write(resp)
		return

	case "get_update_status":
		resp, _ := json.Marshal(getUpdateStatus())
		w.Write(resp)
		return

	case "security_info":
		info := SecurityInfo{
			ServerStart: serverStartTime.Format("2006-01-02 15:04:05"),
			Uptime:      formatDuration(time.Since(serverStartTime)),
		}

		// 登录事件
		loginEventsMu.Lock()
		info.LoginEvents = make([]LoginEvent, len(loginEvents))
		copy(info.LoginEvents, loginEvents)
		loginEventsMu.Unlock()

		// 在线列表 = OPUI 已连接用户
		rawClients := GetOpuiOnlineClients()
		info.OnlineList = make([]OnlineItem, 0, len(rawClients))
		for _, c := range rawClients {
			item := OnlineItem{}
			if v, ok := c["name"].(string); ok {
				item.Name = v
			}
			if v, ok := c["type"].(string); ok {
				item.Type = v
			}
			if v, ok := c["online"].(bool); ok {
				item.Online = v
			}
			if v, ok := c["detail"].(string); ok {
				item.Detail = v
			}
			info.OnlineList = append(info.OnlineList, item)
		}

		if r, err := json.Marshal(info); err == nil {
			w.Write(r)
		} else {
			http.Error(w, `{"status":"error","error":"marshal failed"}`, http.StatusInternalServerError)
		}
		return

	case "ip_blacklist_list":
		ipBlacklistMu.Lock()
		list := make([]string, 0, len(ipBlacklist))
		for ip := range ipBlacklist {
			list = append(list, ip)
		}
		ipBlacklistMu.Unlock()
		r, _ := json.Marshal(list)
		w.Write(r)
		return

	case "ip_blacklist_add":
		var j struct {
			IP string `json:"ip"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil || j.IP == "" {
			http.Error(w, `{"status":"error","error":"invalid ip"}`, http.StatusBadRequest)
			return
		}
		ipBlacklistMu.Lock()
		ipBlacklist[strings.TrimSpace(j.IP)] = true
		ipBlacklistMu.Unlock()
		saveIPBlacklist()
		// 广播安全事件
		notifyData, _ := json.Marshal(map[string]string{
			"type":   "ip_blacklist",
			"action": "add",
			"ip":     j.IP,
		})
		broadcastOpuiNotify(notifyData)
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "ip_blacklist_remove":
		var j struct {
			IP string `json:"ip"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil || j.IP == "" {
			http.Error(w, `{"status":"error","error":"invalid ip"}`, http.StatusBadRequest)
			return
		}
		ipBlacklistMu.Lock()
		delete(ipBlacklist, strings.TrimSpace(j.IP))
		ipBlacklistMu.Unlock()
		saveIPBlacklist()
		notifyData, _ := json.Marshal(map[string]string{
			"type":   "ip_blacklist",
			"action": "remove",
			"ip":     j.IP,
		})
		broadcastOpuiNotify(notifyData)
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "firewall_get_config":
		ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
		cf, err := ff.LoadIni()
		if err != nil {
			w.Write([]byte(`{"enabled":false,"dic_path":""}`))
			return
		}
		fwSec := cf.Section("防火墙")
		var enabled bool
		var dicPath string
		if fwSec != nil {
			enabled = fwSec.Key("启用").MustBool(false)
			dicPath = fwSec.Key("词库").String()
		}
		r, _ := json.Marshal(map[string]any{
			"enabled":  enabled,
			"dic_path": dicPath,
		})
		w.Write(r)
		return

	case "firewall_save_config":
		var j struct {
			Enabled bool   `json:"enabled"`
			DicPath string `json:"dic_path"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
		cf, err := ff.LoadIni()
		if err != nil {
			http.Error(w, `{"status":"error","error":"config load failed"}`, http.StatusInternalServerError)
			return
		}
		cf.Section("防火墙").Key("启用").SetValue(strconv.FormatBool(j.Enabled))
		cf.Section("防火墙").Key("词库").SetValue(j.DicPath)
		ff.SaveIni(cf)
		w.Write([]byte(`{"status":"ok"}`))
		return

	default:
		http.Error(w, `{"status":"error","error":"invalid type"}`, http.StatusBadRequest)
		return
	}
}
