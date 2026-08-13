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
	Http                *http.Server
	Cors                bool
	CorsOrigins         string
	TempCleanupInterval int
	TLS                 bool
	CertFile            string
	KeyFile             string
}

type OPUI struct {
	// 地址
	Addr string
	// 密钥
	Secret string
	// 跨域开关
	Cors bool
}

type ServerConfigInfo struct {
	// HTTP地址
	Router *ServerHTTP
	// OPUI
	OPUI *OPUI
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
