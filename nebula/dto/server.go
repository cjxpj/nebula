package dto

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"sort"
	"sync"

	feishubot_msg "github.com/cjxpj/nebula/bot/feishubot/msg"
	napcatbot_dto "github.com/cjxpj/nebula/bot/napcatbot/dto"
	qqbot_msg "github.com/cjxpj/nebula/bot/qqbot/msg"
	secludedbot_dto "github.com/cjxpj/nebula/bot/secludedbot/dto"
	yunhubot_dto "github.com/cjxpj/nebula/bot/yunhubot/dto"
	"github.com/gorilla/websocket"
)

// ==============Server================

// 默认路由词库文件与默认网站根目录
const (
	DefaultRouterFile = "private/system/router.n"
	DefaultWebRoot    = "public"
)

// WS连接
type ServerRouterWebSocket struct {
	// 是否开启
	Open bool
	// 地址
	Addr string
	// 跨域
	Cors bool
	// 词库路径
	FilePath string
	// 连接
	Conn *websocket.Upgrader
}

type OPUI struct {
	// 地址
	Addr string
	// 密钥
	Secret string
	// 跨域开关
	Cors bool
}

// CloudTool 内置云工具服务端配置（config.yaml [云工具服务端] 节）。
type CloudTool struct {
	// 是否开启
	Open bool
	// 访问路径（含前导 /，如 /cloudtool）
	Addr string
	// 是否允许任意账号注册（关闭时仅白名单账号可登录/注册）
	AllowRegister bool
	// 账号白名单（用户名 -> true）
	Whitelist map[string]bool
	// 云工具词库目录（相对 private/，如 cloudtool）
	DicDir string
	// 断开自动注销时长（秒）：连接断开时本次在线时长低于该值即注销，0 表示关闭
	LogoutSec int
	// 调试打印
	Debug bool
}

type ServerConfigInfo struct {
	// Debug 全局调试开关：控制打印词库缓存等调试信息
	Debug bool
	// TempCleanupInterval 全局临时读写清理周期（秒）
	TempCleanupInterval int
	// DicCache 是否启用词库编译磁盘缓存（private/.dic_cache），默认关闭
	DicCache bool
	// OPUI
	OPUI *OPUI
	// 内置云工具
	CloudTool *CloudTool
	// 正在监听的WS列表
	WsList map[string]*ServerRouterWebSocket
	// WS列表锁
	WsListMu sync.Mutex
	// QQBot地址（多开支持，key为INI section名如"QQ"、"QQ2"等）
	QQBots map[string]*qqbot_msg.RouterQQBot
	// YunHuBot地址
	YunHuBot *yunhubot_dto.RouterYunHuBot
	// NapCatBot地址
	NapCatBot *napcatbot_dto.RouterNapCatBot
	FeiShuBot *feishubot_msg.RouterFeishubot
	// SecludedBot 对接
	SecludedBot *secludedbot_dto.RouterSecludedBot
	// Ngrok地址
	Ngrok *NgrokConfig
	// Ngrok 隧道监听器（运行时启停用）
	NgrokListener net.Listener
	// Ngrok 取消上下文（运行时启停用）
	NgrokCancel context.CancelFunc
}

// AddWs 添加或更新一个正在监听的 WS 服务
func (s *ServerConfigInfo) AddWs(ws *ServerRouterWebSocket) {
	if ws == nil {
		return
	}
	s.WsListMu.Lock()
	defer s.WsListMu.Unlock()
	if s.WsList == nil {
		s.WsList = make(map[string]*ServerRouterWebSocket)
	}
	s.WsList[ws.Addr] = ws
}

// RemoveWs 移除指定地址的 WS 服务
func (s *ServerConfigInfo) RemoveWs(addr string) {
	s.WsListMu.Lock()
	defer s.WsListMu.Unlock()
	if s.WsList != nil {
		delete(s.WsList, addr)
	}
}

// WsListSnapshot 返回当前监听中的 WS 服务快照（按地址排序）
func (s *ServerConfigInfo) WsListSnapshot() []*ServerRouterWebSocket {
	s.WsListMu.Lock()
	defer s.WsListMu.Unlock()
	list := make([]*ServerRouterWebSocket, 0, len(s.WsList))
	for _, ws := range s.WsList {
		list = append(list, ws)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Addr < list[j].Addr })
	return list
}

type NgrokConfig struct {
	// 地址
	Addr string
	// Token
	Token string
	// ServerAddr 要转发的本地 HTTP 服务器监听地址（空则转发到核心服务器）
	ServerAddr string
}

type HTTPRequestInfo struct {
	Path        string                 `json:"路径"`
	Type        string                 `json:"来源"`
	QueryParams url.Values             `json:"GET,omitempty"`
	Headers     http.Header            `json:"请求头"`
	IP          string                 `json:"IP"`
	Host        string                 `json:"Host"`
	Post        any                    `json:"POST,omitempty"`
	PostFile    map[string][]*PostFile `json:"POSTFile,omitempty"`
}

type PostFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Data string `json:"data"`
}

type SetCookie struct {
	Name     string `json:"命名"`
	Value    string `json:"数据"`
	Path     string `json:"路径"`
	HttpOnly bool   `json:"禁止JS"`
	MaxAge   int    `json:"存活"`
}

// ==============函数启动的服务器================

// FuncServerInfo 由字典函数「服务器」启动的轻量 HTTP 词库服务器状态。
// Update/Close 为 dic 包注入的操作句柄，不参与 JSON 序列化。
type FuncServerInfo struct {
	Addr    string `json:"addr"`     // 监听地址（含端口）
	Cors    bool   `json:"cors"`     // 跨域开关
	DicData string `json:"dic_data"` // 词库源码文本
	Core    bool   `json:"core"`     // 是否核心服务器（由注册表 coreAddr 派生，供前端展示）
	// HTTPS（TLS）配置
	TLS      bool   `json:"tls"`       // 是否启用 HTTPS
	CertFile string `json:"cert_file"` // 证书文件路径（相对 private/https 或绝对路径）
	KeyFile  string `json:"key_file"`  // 密钥文件路径
	// BeerFrp 穿透配置
	FrpOpen       bool   `json:"frp_open"`        // 是否启用 BeerWebFrp 穿透
	FrpServerAddr string `json:"frp_server_addr"` // BeerWebFrp 服务端地址（ws/wss）
	FrpToken      string `json:"frp_token"`       // BeerWebFrp 隧道密钥
	FrpDebug      bool   `json:"frp_debug"`       // BeerWebFrp 调试日志

	Handler http.Handler                     `json:"-"` // 请求处理器（供 Ngrok 等转发复用）
	Update  func(upd FuncServerUpdate) error `json:"-"`
	Close   func()                           `json:"-"`
}

// FuncServerUpdate 前端编辑函数服务器时提交的增量更新（指针字段为 nil 表示不修改）。
type FuncServerUpdate struct {
	Cors          *bool   `json:"cors"`
	TLS           *bool   `json:"tls"`
	CertFile      *string `json:"cert_file"`
	KeyFile       *string `json:"key_file"`
	FrpOpen       *bool   `json:"frp_open"`
	FrpServerAddr *string `json:"frp_server_addr"`
	FrpToken      *string `json:"frp_token"`
	FrpDebug      *bool   `json:"frp_debug"`
}

// FuncServerRegistry 函数启动的服务器注册表（按监听地址索引，同名地址同时只会有一个存活实例）。
type FuncServerRegistry struct {
	mu       sync.Mutex
	mp       map[string]*FuncServerInfo
	coreAddr string // 核心服务器监听地址，空串表示无核心服务器
}

// FuncServers 全局函数服务器注册表，供前端查询/编辑。
var FuncServers = &FuncServerRegistry{mp: make(map[string]*FuncServerInfo)}

// Add 添加或更新一个函数服务器。核心身份不在此自动指定，须由词库显式调用「服务器.设置核心服务器」标记。
func (r *FuncServerRegistry) Add(s *FuncServerInfo) {
	if s == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mp[s.Addr] = s
}

// SetCore 将指定地址的服务器标记为核心服务器。返回是否设置成功。
// 核心地址单独记录在 coreAddr 字段中，取出即判断，无需遍历各服务器条目。
func (r *FuncServerRegistry) SetCore(addr string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.mp[addr]; !ok {
		return false
	}
	r.coreAddr = addr
	return true
}

// CoreAddr 返回当前核心服务器地址，无核心服务器时返回空串。
func (r *FuncServerRegistry) CoreAddr() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.coreAddr
}

// Remove 移除指定地址的函数服务器；若移除的是核心服务器则同时清空核心地址。
func (r *FuncServerRegistry) Remove(addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.mp, addr)
	if r.coreAddr == addr {
		r.coreAddr = ""
	}
}

// Get 返回指定地址的函数服务器，不存在时返回 nil。
func (r *FuncServerRegistry) Get(addr string) *FuncServerInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.mp[addr]
}

// UpdateData 同步指定服务器的词库源码快照。
func (r *FuncServerRegistry) UpdateData(addr, dicData string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.mp[addr]; ok {
		e.DicData = dicData
	}
}

// UpdateCors 同步指定服务器的跨域开关快照。
func (r *FuncServerRegistry) UpdateCors(addr string, cors bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.mp[addr]; ok {
		e.Cors = cors
	}
}

// Apply 对指定地址的服务器快照执行一次修改（用于同步运行中服务器的可编辑配置）。
func (r *FuncServerRegistry) Apply(addr string, fn func(*FuncServerInfo)) {
	if fn == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.mp[addr]; ok {
		fn(e)
	}
}

// Snapshot 返回当前全部函数服务器快照（按地址排序），Core 字段按 coreAddr 动态派生。
func (r *FuncServerRegistry) Snapshot() []FuncServerInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := make([]FuncServerInfo, 0, len(r.mp))
	for a, e := range r.mp {
		c := *e
		c.Core = a == r.coreAddr
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Addr < list[j].Addr })
	return list
}
