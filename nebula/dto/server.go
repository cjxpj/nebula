package dto

import (
	"net/http"
	"net/url"

	feishubot_msg "github.com/cjxpj/nebula/bot/feishubot/msg"
	napcatbot_dto "github.com/cjxpj/nebula/bot/napcatbot/dto"
	qqbot_msg "github.com/cjxpj/nebula/bot/qqbot/msg"
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
	// 连接
	Conn *websocket.Upgrader
}

type ServerHTTP struct {
	Cors bool
	Http *http.Server
}

type OPUI struct {
	// 地址
	Addr string
}

type ServerConfigInfo struct {
	// HTTP地址
	Router *ServerHTTP
	// OPUI
	OPUI *OPUI
	// WS地址
	Ws *ServerRouterWebSocket
	// QQBot地址
	QQBot *qqbot_msg.RouterQQBot
	// YunHuBot地址
	YunHuBot *yunhubot_dto.RouterYunHuBot
	// NapCatBot地址
	NapCatBot *napcatbot_dto.RouterNapCatBot
	FeiShuBot *feishubot_msg.RouterFeishubot
	// Ngrok地址
	Ngrok *NgrokConfig
	// JAVAMC
	JAVAMC *JAVAMC
}

type JAVAMC struct {
	// 启用
	Open bool
	// 地址
	Addr string
	// 词库地址
	DicPath string
	// WS
	Conn *websocket.Upgrader
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
