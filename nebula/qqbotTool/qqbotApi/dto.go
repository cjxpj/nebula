package qqbotapi

import (
	"time"

	"github.com/patrickmn/go-cache"
)

const APIURL = "https://api.sgroup.qq.com"

// QQBot 封装机器人鉴权和发消息流程
type QQBot struct {
	AppId        string
	ClientSecret string
	Key          *AccessTokenResponse
	TokenTime    time.Time
	// 处理次数
	Count int
}

// =============QQBot路由数据================
type RouterQQBot struct {
	// 是否开启
	Open bool
	// 地址
	Addr string
	// 密钥
	Secret string
	// 词库路径
	FilePath string
	// 消息处理次数
	MsgCount int
	// 缓存器-清重复数据
	LastMsg *cache.Cache
	// API工具
	API *QQBot
}

// AccessTokenResponse 表示 access_token 响应
type AccessTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   string `json:"expires_in"`
}

// 群富媒体
type GroupMessageFile struct {
	// 	媒体类型：1 图片，2 视频，3 语音，4 文件（暂不开放）
	// 资源格式要求
	//  图片：png/jpg，视频：mp4，语音：silk
	Type int    `json:"file_type"`
	Url  string `json:"file_url"`
	Srv  bool   `json:"srv_send_msg"` // 是否发送到群内
	// base64编码
	Data string `json:"file_data"`
}

// 群富媒体返回
type GroupMessageFileResponse struct {
	Uuid string `json:"file_uuid"`
	Info string `json:"file_info"`
	TTL  int    `json:"ttl"`
	ID   string `json:"id"`
}

// ChannelSend 频道发送
type ChannelSend struct {
	Content string `json:"content"`
	MsgId   string `json:"msg_id"`
}

// MessageToSend 是发送的文本消息体
type MessageToSend struct {
	// 消息类型：0 是文本，2 是 markdown， 3 ark，4 embed，7 media 富媒体
	MsgType int    `json:"msg_type"`
	Content string `json:"content"`
	// 群富媒体
	Media *GroupMessageFileResponse `json:"media"`
	// 被动消息必填
	MsgId  string `json:"msg_id"`
	MsgSeq int    `json:"msg_seq"`
}

// MessageResponse 是发送成功后返回的结构体
type MessageResponse struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	Content   string `json:"content"`
}
