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

// WebHandlerFactory 构建单个 HTTP 服务器的处理器。由 dic 包在初始化时注入，
// 供 dic 与 dic_server 两处（loadConfig 与 save_servers）统一创建每服务器独立 handler。
var WebHandlerFactory func(router *ServerHTTP) http.Handler

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

type ServerHTTP struct {
	Http *http.Server
	// Enabled 是否启用该服务器（实时开关，关闭后仅保留管理面板/机器人等内置入口）
	Enabled bool
	// Domains 绑定域名（多个，换行分隔），留空则用监听地址访问
	Domains []string
	// WebRoot 映射目录（网站根目录）：该服务器路由词库从中提供静态/网页/词库文件，留空默认 public
	WebRoot string
	// RouterFile 路由词库文件（.n）：该服务器使用的路由词库，留空默认 private/system/router.n
	RouterFile string
	// Cors 跨域开关
	Cors bool
	// CorsOrigins 跨域白名单
	CorsOrigins string
	TLS         bool
	// 证书文件/密钥文件路径（相对 private/https 或绝对路径）
	CertFile string
	KeyFile  string
	// TLSMode 证书来源：file（手动路径）/ self（自签名）/ upload（上传）/ system（系统证书库）/ acme（Let's Encrypt）
	TLSMode    string
	TLSDomains string // acme 域名列表（逗号分隔）
	TLSEmail   string // acme 邮箱（可选）
	// FrpOpen 是否启用 BeerWebFrp 穿透（每个服务器独立）
	FrpOpen bool
	// FrpServerAddr BeerWebFrp 服务端 WebSocket 地址（ws:// 或 wss://）
	FrpServerAddr string
	// FrpToken BeerWebFrp 隧道密钥
	FrpToken string
	// FrpDebug BeerWebFrp 调试日志
	FrpDebug bool
}

type OPUI struct {
	// 地址
	Addr string
	// 密钥
	Secret string
	// 跨域开关
	Cors bool
}

// CloudTool 内置云工具服务端配置（system.ini [云工具服务端] 节）。
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
	// Routers 多开 HTTP 服务器列表
	Routers []*ServerHTTP
	// Debug 全局调试开关：控制打印词库缓存等调试信息
	Debug bool
	// TempCleanupInterval 全局临时读写清理周期（秒）
	TempCleanupInterval int
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

// Primary 返回第一个 HTTP 服务器（主服务器），未配置时返回 nil
func (s *ServerConfigInfo) Primary() *ServerHTTP {
	if len(s.Routers) == 0 {
		return nil
	}
	return s.Routers[0]
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
	// ServerAddr 要转发的本地 HTTP 服务器监听地址（空则转发到主服务器 Primary）
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
