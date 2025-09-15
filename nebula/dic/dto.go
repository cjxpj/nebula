package dic

import (
	"net/http"
	"net/url"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/qqbottool/qqbotapi"
	"github.com/cjxpj/nebula/utils"
	yunhubotapi "github.com/cjxpj/nebula/yunhuBotTool/yunhubotApi"

	"github.com/gorilla/websocket"
)

type Dic struct {
	text      string
	Val       *dto.DicVal
	id        int16
	Path      string
	FuncText  []*dto.BuildDic
	ClassText map[string]*dto.DicClass
	// 自义定函数
	MyFunc map[string]func(val *dto.DicVal, inputs *utils.DicInputs) (any, error)
}

type WebDic struct {
	Text   string
	Val    *dto.DicVal
	Path   string
	MyFunc map[string]func(val *dto.DicVal, inputs *utils.DicInputs) (any, error)
}

// run
type DicEntry struct {
	// 返回信息
	Output  *dto.SingleValue
	Val     *dto.DicVal
	Sys_v   *dto.LocalDicValue
	Trigger bool
	Dic     *dto.BuildValue
}

func (d *DicEntry) Close() {
	d.Output.Clear()
	// 回收局部变量
	d.Val.P.Close()
	// 清空指针
	d.Val = nil
	d.Sys_v = nil
	// 清空回收
	d.Dic.Close()
}

// if
type IfText struct{}

// build
type Build struct {
	Val *dto.DicVal
}

// func
type DicFunc struct {
	// 变量
	Val *dto.DicVal
	// 系统变量
	Sys *dto.LocalDicValue
	// 准备输出内容
	Output *dto.SingleValue
	Dic    *dto.BuildValue
}

// ==============Server================

// WS连接
type ServeRouterWebSocket struct {
	// 是否开启
	Open bool
	// 地址
	Addr string
	// 连接
	Conn *websocket.Upgrader
}

type ServeRouter struct {
	// WS地址
	Ws *ServeRouterWebSocket
	// QQBot地址
	QQBot *qqbotapi.RouterQQBot
	// YunHuBot地址
	YunHuBot *yunhubotapi.RouterYunHuBot
}

type RequestInfo struct {
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
