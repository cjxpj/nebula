package qqbot_msg

import (
	"context"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/patrickmn/go-cache"
)

const APIURL = "https://api.bot.qq.com"

// =============QQBot路由数据================
type RouterQQBot struct {
	// 是否开启
	Open bool
	// 地址
	Addr string
	// 词库路径
	FilePath string
	// 缓存器-清重复数据
	LastMsg *cache.Cache
	// 接口
	API *QQBot
	// 全量消息艾特兼容
	AtCompat bool
	// 过滤开头斜杠指令前缀
	FilterSlash bool
	// 调试打印
	Debug bool
	// 备注名
	Remark string
	// WebSocket 模式
	Ws bool
	// WebSocket 连接
	WsConn *websocket.Conn
	// WebSocket 取消上下文
	WsCancel context.CancelFunc
	// WebSocket 序列号（用于心跳）
	WsSeq int
	// WebSocket 意图值（默认 1073741825 公域 / 513 私域）
	WsIntents int
	// WebSocket session_id（用于 Resume 恢复）
	WsSessionID string
	// WebSocket 写锁
	WsMutex sync.Mutex
}

// WsStartFunc 和 WsStopFunc 由 qqbot 包注册，供 dto 等外部包调用
var (
	StartWsFunc func(bot *RouterQQBot)
	StopWsFunc  func(bot *RouterQQBot)
)
