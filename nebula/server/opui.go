package dic_server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/cjxpj/nebula/appfiles"
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

type HttpOpUiConfig_server struct {
	Server              string `json:"server"`
	CORS                bool   `json:"cors"`
	CORSOrigins         string `json:"cors_origins"`
	TempCleanupInterval int    `json:"temp_cleanup_interval"`
	TLS                 bool   `json:"tls"`
	CertFile            string `json:"cert_file"`
	KeyFile             string `json:"key_file"`
}

type HttpOpUiWebSocketItem struct {
	Addr     string `json:"addr"`
	Cors     bool   `json:"cors"`
	Open     bool   `json:"open"`
	Closable bool   `json:"closable"`
}

type HttpOpUiConfig_ngrok struct {
	Open   bool   `json:"open"`
	Token  string `json:"token"`
	Domain string `json:"domain"`
}

type HttpOpUiConfig_frp struct {
	Open       bool   `json:"open"`
	ServerAddr string `json:"server_addr"`
	Token      string `json:"token"`
	Debug      bool   `json:"debug"`
}

type HttpOpUiConfig_opui struct {
	Open   bool   `json:"open"`
	Path   string `json:"path"`
	Secret string `json:"secret"`
	Cors   bool   `json:"cors"`
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
				// 回显到真实终端，保证开发调试时仍能看到输出
				_, _ = originalStdout.WriteString(raw)
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

// ClearServerLogs 清空 database/log 下的全部日志文件
func ClearServerLogs() {
	utils.NewFileQueue(serverLogDir()).DeleteFolder()
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
}

// writeJob 一次写任务
type writeJob struct {
	data []byte
	done chan error // nil = 异步（HTTP响应），非nil = 同步（WS帧/状态等结果）
}

// BeerWebFrp WebSocket 连接管理
var (
	frpConn       *frpCon
	frpMutex      sync.Mutex
	frpCancel     context.CancelFunc
	frpHTTPClient = &http.Client{
		// 超时需小于 BeerWebFrp 服务端 proxyRequestTimeout(90s)，避免服务端先超时导致 502
		Timeout: 85 * time.Second,
		// 不自动跟随重定向：3xx 及 Location 必须原样回传隧道服务端，
		// 由服务端改写 Location（如 "/" -> "/token/"）以适配隧道域名
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	frpDebug bool

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
	mu   sync.Mutex
}

// SetFrpDebug 设置 FRP 调试开关
func SetFrpDebug(debug bool) {
	frpDebug = debug
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

// ConnectFrp 连接到 BeerWebFrp 服务端
func ConnectFrp(serverAddr, token string) {
	serverAddr = strings.TrimSpace(serverAddr)
	token = strings.TrimSpace(token)
	if frpDebug {
		debugLog.Infof("[FRP] ConnectFrp 被调用, serverAddr=%s, token长度=%d", serverAddr, len(token))
	}

	frpMutex.Lock()
	if frpCancel != nil {
		if frpDebug {
			debugLog.Infof("[FRP] 取消已有连接上下文")
		}
		frpCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	frpCancel = cancel
	frpMutex.Unlock()

	go runFrpConn(ctx, serverAddr, token)
}

// runFrpConn 管理单条 FRP WebSocket 连接的生命周期，断线自动重连
func runFrpConn(ctx context.Context, serverAddr, token string) {
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
			if frpDebug {
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

		// 握手
		if err := conn.WriteJSON(map[string]string{"token": token}); err != nil {
			if frpDebug {
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
			if frpDebug {
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
			if frpDebug {
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
		if frpDebug {
			debugLog.Infof("[FRP] 握手成功, link=%s", hs.Link)
		}
		// 打印隧道访问链接（经启动词库 start.n 格式化输出，如 “BeerWebFrp：<链接>”）
		printFrpLink(hs.Link)

		// 注册到连接池，启动异步写入器
		fc := &frpCon{
			conn: conn,
			wch:  make(chan writeJob, 64),
		}
		fc.ctx, fc.cancel = context.WithCancel(ctx)
		frpMutex.Lock()
		frpConn = fc
		frpMutex.Unlock()

		go fc.writer()

		if frpDebug {
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
				if frpDebug {
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
				if frpDebug {
					debugLog.Infof("[FRP] 收到WS代理请求: %s (id=%s)", msg.Path, msg.ID)
				}
				go handleFrpWsProxy(fc, &msg)
			case "ws_frame":
				if frpDebug {
					debugLog.Infof("[FRP] 收到WS数据帧 (id=%s)", msg.ID)
				}
				go handleFrpWsFrame(&msg)
			case "ws_close":
				if frpDebug {
					debugLog.Infof("[FRP] 收到WS关闭通知 (id=%s)", msg.ID)
				}
				go handleFrpWsClose(&msg)
			default:
				if frpDebug {
					debugLog.Infof("[FRP] 收到HTTP代理请求: %s %s (id=%s)", msg.Method, msg.Path, msg.ID)
				}
				go handleFrpProxyRequest(fc, &msg)
			}
		}

		// 清理：取消 writer，停止 Ping，断开残留的本地 WS 代理流（避免重连后旧流残留）
		fc.cancel()
		close(pingDone)
		closeAllFrpWsStreams()

		frpMutex.Lock()
		if frpConn == fc {
			frpConn = nil
		}
		frpMutex.Unlock()

		if frpDebug {
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
				if frpDebug {
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
			if frpDebug {
				debugLog.Infof("[FRP] 写队列满，丢弃响应 (id=%s)", msg.ID)
			}
		}
	}()

	respStatusCode = 502
	respHeaders = map[string]string{"Content-Type": "text/html; charset=utf-8"}
	respBody = []byte("backend server not available")

	if dto.ServerConfig.Router == nil || dto.ServerConfig.Router.Http == nil {
		if frpDebug {
			debugLog.Infof("[FRP] 代理请求: Router.Http为空，返回502 (id=%s)", msg.ID)
		}
		return
	}

	// 构造本地 HTTP 地址
	localAddr := strings.Replace(dto.ServerConfig.Router.Http.Addr, "0.0.0.0", "127.0.0.1", 1)
	targetURL := "http://" + localAddr + msg.Path

	if frpDebug {
		debugLog.Infof("[FRP] 转发请求: %s %s -> %s (body长度=%d, headers数=%d)", msg.Method, msg.Path, targetURL, len(msg.Body), len(msg.Headers))
	}

	var bodyReader io.Reader
	if len(msg.Body) > 0 {
		bodyReader = bytes.NewReader(msg.Body)
	}
	// 注：服务端 Body 已是 []byte，JSON 反序列化时自动 base64 解码

	req, err := http.NewRequest(msg.Method, targetURL, bodyReader)
	if err != nil {
		if frpDebug {
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
		if frpDebug {
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
		if frpDebug {
			debugLog.Infof("[FRP] 代理请求: 读取响应体失败 (id=%s): %v", msg.ID, err)
		}
		respStatusCode = 502
		respBody = []byte("proxy read body error: " + err.Error())
		return
	}

	respStatusCode = httpResp.StatusCode

	if frpDebug {
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
		if frpDebug {
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
			if frpDebug {
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
	streamID := msg.ID

	if dto.ServerConfig.Router == nil || dto.ServerConfig.Router.Http == nil {
		sendFrpWsStatus(fc, streamID, -1)
		return
	}

	localAddr := strings.Replace(dto.ServerConfig.Router.Http.Addr, "0.0.0.0", "127.0.0.1", 1)
	wsURL := "ws://" + localAddr + msg.Path

	if frpDebug {
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
		if frpDebug {
			debugLog.Infof("[FRP] WS代理: 本地WS连接失败 (id=%s): %v", streamID, err)
		}
		sendFrpWsStatus(fc, streamID, -1)
		return
	}

	// 注册流
	st := &frpWsStream{conn: localConn}
	frpWsStreamsMu.Lock()
	frpWsStreams[streamID] = st
	frpWsStreamsMu.Unlock()

	if frpDebug {
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
				if frpDebug {
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
func handleFrpWsFrame(msg *frpWSMessage) {
	streamID := msg.ID

	frpWsStreamsMu.Lock()
	st, ok := frpWsStreams[streamID]
	frpWsStreamsMu.Unlock()

	if !ok {
		if frpDebug {
			debugLog.Infof("[FRP] WS代理: 收到ws_frame但流不存在 (id=%s)", streamID)
		}
		return
	}

	// 加锁串行化写入，避免多帧并发写导致 gorilla 连接数据竞争
	st.mu.Lock()
	err := st.conn.WriteMessage(websocket.BinaryMessage, msg.Body)
	st.mu.Unlock()
	if err != nil && frpDebug {
		debugLog.Infof("[FRP] WS代理: 写入本地WS失败 (id=%s): %v", streamID, err)
	}
}

// handleFrpWsClose 处理来自 BeerWebFrp 的 ws_close 消息
// 关闭本地 WS 连接并清理流
func handleFrpWsClose(msg *frpWSMessage) {
	streamID := msg.ID

	frpWsStreamsMu.Lock()
	st, ok := frpWsStreams[streamID]
	if ok {
		delete(frpWsStreams, streamID)
	}
	frpWsStreamsMu.Unlock()

	if ok && st != nil {
		if frpDebug {
			debugLog.Infof("[FRP] WS代理: 关闭本地WS流 (id=%s)", streamID)
		}
		st.mu.Lock()
		st.conn.Close()
		st.mu.Unlock()
	}
}

// sendFrpWsFrame 向服务端发送 WS 数据帧响应（同步等待写入结果）
func sendFrpWsFrame(fc *frpCon, streamID string, frame []byte) {
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
			if err != nil && frpDebug {
				debugLog.Infof("[FRP] WS代理: 发送数据帧失败 (id=%s): %v", streamID, err)
			}
		case <-time.After(10 * time.Second):
			// 连接已断开且写队列无人消费，放弃等待，避免 goroutine 泄漏
		}
	default:
		if frpDebug {
			debugLog.Infof("[FRP] WS代理: 发送数据帧失败（写队列满） (id=%s)", streamID)
		}
	}
}

// sendFrpWsStatus 向服务端发送 WS 状态通知（同步等待写入结果）
func sendFrpWsStatus(fc *frpCon, streamID string, statusCode int) {
	if frpDebug {
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

// closeAllFrpWsStreams 断开所有本地 WS 代理连接
func closeAllFrpWsStreams() {
	frpWsStreamsMu.Lock()
	defer frpWsStreamsMu.Unlock()
	if frpDebug && len(frpWsStreams) > 0 {
		debugLog.Infof("[FRP] WS代理: 清理%d个本地WS流", len(frpWsStreams))
	}
	for id, st := range frpWsStreams {
		st.mu.Lock()
		st.conn.Close()
		st.mu.Unlock()
		delete(frpWsStreams, id)
	}
}

// DisconnectFrp 断开所有 BeerWebFrp 连接
func DisconnectFrp() {
	if frpDebug {
		debugLog.Infof("[FRP] DisconnectFrp 被调用, 断开所有连接")
	}

	frpMutex.Lock()
	defer frpMutex.Unlock()

	if frpCancel != nil {
		frpCancel()
		frpCancel = nil
	}

	// 关闭连接
	if frpConn != nil {
		frpConn.cancel()
		frpConn = nil
	}

	closeAllFrpWsStreams()

	if frpDebug {
		debugLog.Infof("[FRP] DisconnectFrp 完成")
	}
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
}

type HttpOpUiConfig_qq_instance struct {
	Section string            `json:"section"`
	Config  HttpOpUiConfig_qq `json:"config"`
}

type HttpOpUiConfig_qq_list struct {
	Instances []HttpOpUiConfig_qq_instance `json:"instances"`
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

type HttpOpUiConfig_EncryptDic struct {
	Text string `json:"text"`
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

// listDicFilesInDir 扫描应用目录下指定目录（如 private、private/bot/qq/dic）中的词库文件（.n），
// 返回相对应用目录的路径
func listDicFilesInDir(dir string) ([]string, error) {
	appDir := utils.GetAppDir()
	var files []string

	root := filepath.Join(appDir, dir)
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return files, nil
	}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), ".n") {
			if rel, err := filepath.Rel(appDir, path); err == nil {
				files = append(files, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(files)
	return files, nil
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

func OpUI(w http.ResponseWriter, r *http.Request, getpath string) {
	// OPUI WebSocket 统一通信（API 请求 + 事件推送），挂在访问路径本身（如 /nebula）
	if getpath == "" || getpath == "/" {
		// 仅处理 WebSocket 升级请求，忽略普通 HTTP 请求（如浏览器直接访问访问路径）
		if !websocket.IsWebSocketUpgrade(r) {
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

	// OPUI 仅通过 WebSocket 通信，不再提供 HTTP API 口子
	http.NotFound(w, r)
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

	// 扩展部署相关接口仅支持 Windows 端，其他平台直接拒绝
	switch h.Type {
	case "install_php", "install_ffmpeg", "install_silk_v3", "install_napcat_bot", "install_python",
		"get_install_status", "install_progress", "cancel_install", "uninstall":
		if runtime.GOOS != "windows" {
			jsonResp, _ := json.Marshal(map[string]string{"status": "error", "error": "扩展部署功能仅支持 Windows 端"})
			w.Write(jsonResp)
			return
		}
	}

	switch h.Type {
	case "get_server":
		ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := f.Section("HTTP")
		var j HttpOpUiConfig_server
		j.Server = d.Key("server").String()
		j.CORS = d.Key("跨域").MustBool(false)
		j.CORSOrigins = d.Key("跨域白名单").String()
		j.TempCleanupInterval = d.Key("临时读写清理周期").MustInt(60)
		j.TLS = d.Key("TLS").MustBool(false)
		j.CertFile = d.Key("TLS证书文件").String()
		j.KeyFile = d.Key("TLS密钥文件").String()
		if r, err := json.Marshal(j); err != nil {
			w.Write([]byte(`{"server":"","cors":false,"cors_origins":"","temp_cleanup_interval":60,"tls":false,"cert_file":"","key_file":""}`))
		} else {
			w.Write(r)
		}
		return

	case "save_server":
		var j HttpOpUiConfig_server
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := f.Section("HTTP")
		d.Key("server").SetValue(j.Server)
		d.Key("跨域").SetValue(strconv.FormatBool(j.CORS))
		d.Key("跨域白名单").SetValue(j.CORSOrigins)
		d.Key("临时读写清理周期").SetValue(strconv.Itoa(j.TempCleanupInterval))
		d.Key("TLS").SetValue(strconv.FormatBool(j.TLS))
		d.Key("TLS证书文件").SetValue(j.CertFile)
		d.Key("TLS密钥文件").SetValue(j.KeyFile)
		if err := ff.SaveIni(f); err != nil {
			utils.ErrorStop("系统配置保存失败")
		}
		dto.ServerConfig.Router.Cors = j.CORS
		dto.ServerConfig.Router.CorsOrigins = j.CORSOrigins
		dto.ServerConfig.Router.TempCleanupInterval = j.TempCleanupInterval
		dto.ServerConfig.Router.TLS = j.TLS
		dto.ServerConfig.Router.CertFile = j.CertFile
		dto.ServerConfig.Router.KeyFile = j.KeyFile
		// 处理配置请求
		w.Write([]byte(`{"status":"ok"}`))
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
		ff.SaveIni(f)
		w.Write([]byte(`{"status":"ok"}`))
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
			dto.ServerConfig.Ngrok = &dto.NgrokConfig{
				Addr:  domain,
				Token: token,
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

	case "get_frp":
		ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := f.Section("FRP")
		var j HttpOpUiConfig_frp
		j.Open = d.Key("启用").MustBool(false)
		j.ServerAddr = d.Key("服务端地址").String()
		j.Token = d.Key("令牌").String()
		j.Debug = d.Key("调试").MustBool(false)
		r, _ := json.Marshal(j)
		w.Write(r)
		return

	case "save_frp":
		var j HttpOpUiConfig_frp
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		// 自动将 http/https 转换为 ws/wss
		if after, ok := strings.CutPrefix(j.ServerAddr, "https://"); ok {
			j.ServerAddr = "wss://" + after
		} else if after, ok := strings.CutPrefix(j.ServerAddr, "http://"); ok {
			j.ServerAddr = "ws://" + after
		}
		ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
		f, err := ff.LoadIni()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := f.Section("FRP")
		d.Key("启用").SetValue(strconv.FormatBool(j.Open))
		d.Key("服务端地址").SetValue(j.ServerAddr)
		d.Key("令牌").SetValue(j.Token)
		d.Key("调试").SetValue(strconv.FormatBool(j.Debug))
		ff.SaveIni(f)

		frpDebug = j.Debug

		if j.Open {
			// 开启：建立 WebSocket 连接
			ConnectFrp(j.ServerAddr, j.Token)
			w.Write([]byte(`{"status":"ok"}`))
		} else {
			// 关闭：断开连接
			DisconnectFrp()
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
		if j.Section == "" || j.Section == "QQ" {
			http.Error(w, `{"status":"error","error":"cannot delete primary QQ instance"}`, http.StatusBadRequest)
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

	case "encrypt_dic":
		var j HttpOpUiConfig_EncryptDic
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		encodeDic, err := utils.Encrypt(j.Text, appfiles.Key)
		if err != nil {
			http.Error(w, `{"status":"error","error":"加密失败"}`, http.StatusBadRequest)
			return
		}
		var rj struct {
			Status string `json:"status"`
			Text   string `json:"text"`
		}
		rj.Status = "ok"
		rj.Text = encodeDic
		r, _ := json.Marshal(rj)
		w.Write(r)
		return

	case "install_php":
		appDir := utils.GetAppDir()
		destDir := filepath.Join(appDir, "private", "extensions", "php")
		if utils.NewFileQueue(filepath.Join(destDir, "php.exe")).FileExists() {
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
		if utils.NewFileQueue(filepath.Join(destDir, "silk_v3", "silk_v3_encoder.exe")).FileExists() {
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
		if utils.NewFileQueue(filepath.Join(destDir, "launcher.bat")).FileExists() {
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
		if utils.NewFileQueue(filepath.Join(destDir, "python.exe")).FileExists() {
			resp := HttpOpUiInstallResponse{
				Status: "ok",
				Output: []string{"Python 已安装"},
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
			"php":        utils.NewFileQueue(filepath.Join(extDir, "php", "php.exe")).FileExists(),
			"python":     utils.NewFileQueue(filepath.Join(extDir, "python", "python.exe")).FileExists(),
			"napcat_bot": utils.NewFileQueue(filepath.Join(extDir, "NapCat.Shell", "launcher.bat")).FileExists(),
			"ffmpeg":     utils.FindFfmpegExe(filepath.Join(extDir, "ffmpeg")) != "",
			"silk_v3":    utils.NewFileQueue(filepath.Join(extDir, "silk_v3", "silk_v3_encoder.exe")).FileExists(),
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
		// 只在有输入关键字时才搜索；base 指定搜索目录（默认 private），只扫描该文件夹下的 .n 文件
		var j struct {
			Search string `json:"search"`
			Base   string `json:"base"`
			Limit  int    `json:"limit"`
		}
		_ = json.Unmarshal(h.Data, &j)
		kw := strings.ToLower(strings.TrimSpace(j.Search))
		if kw == "" {
			// 未输入关键字不返回列表
			jsonResp, _ := json.Marshal(map[string]any{"files": []string{}})
			w.Write(jsonResp)
			return
		}
		base := strings.TrimSpace(j.Base)
		if base == "" {
			base = "private"
		}
		// 限定搜索目录只能位于 private/public 下，防止越权扫描
		if base != "private" && base != "public" &&
			!strings.HasPrefix(base, "private/") && !strings.HasPrefix(base, "public/") {
			jsonResp, _ := json.Marshal(map[string]any{"files": []string{}})
			w.Write(jsonResp)
			return
		}
		files, err := listDicFilesInDir(base)
		if err != nil {
			http.Error(w, `{"status":"error","error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		var filtered []string
		for _, f := range files {
			if strings.Contains(strings.ToLower(f), kw) {
				filtered = append(filtered, f)
			}
		}
		if j.Limit > 0 && len(filtered) > j.Limit {
			filtered = filtered[:j.Limit]
		}
		jsonResp, _ := json.Marshal(map[string]any{"files": filtered})
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
			http.Error(w, `{"status":"error","error":"词库读取失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		jsonResp, _ := json.Marshal(map[string]any{"content": content})
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
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
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

		resp := map[string]any{
			"output":   output,
			"timedOut": timedOut,
			"segments": parseOutputSegments(output),
			"vars": map[string]any{
				"P": pVars,
				"G": gVars,
			},
		}
		if errorLine > 0 {
			resp["errorLine"] = errorLine
		}
		jsonResp, _ := json.Marshal(resp)
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
		go func() {
			time.Sleep(500 * time.Millisecond) // 等待响应发送完毕
			if err := doOnlineUpdate(); err != nil {
				fmt.Println("online_update failed:", err)
			}
		}()
		w.Write([]byte(`{"status":"ok","msg":"正在下载更新，完成后将自动重启"}`))
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
