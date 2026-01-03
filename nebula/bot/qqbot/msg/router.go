package qqbot_msg

import (
	"github.com/patrickmn/go-cache"
)

const APIURL = "https://api.sgroup.qq.com"

// =============QQBot路由数据================
type RouterQQBot struct {
	// 是否开启
	Open bool
	// 地址
	Addr string
	// 词库路径
	FilePath string
	// 消息处理次数
	MsgCount int
	// 缓存器-清重复数据
	LastMsg *cache.Cache
	// 接口
	API *QQBot
}
