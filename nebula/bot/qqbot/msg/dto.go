package qqbot_msg

import (
	"encoding/json"
	"time"
)

// QQBot 封装机器人鉴权和发消息流程
type QQBot struct {
	AppId        string
	ClientSecret string
	Key          *AccessTokenResponse
	TokenTime    time.Time
	// 处理次数
	Count int
}

// AccessTokenResponse 表示 access_token 响应
type AccessTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   string `json:"expires_in"`
}

// 事件
type Payload struct {
	// 事件ID
	Id string `json:"id"`
	// 事件类型
	Op int `json:"op"`
	// 数据
	Data json.RawMessage `json:"d"`
	// 唯一标识
	Seq int `json:"s"`
	// 请求类型
	Type string `json:"t"`
}

// 签名数据来源
type ValidationRequest struct {
	// 请求的token
	Token string `json:"plain_token"`
	// 请求的签名
	Event string `json:"event_ts"`
}

// 返回签名验证结果
type ValidationResult struct {
	PlainToken string `json:"plain_token"`
	Signature  string `json:"signature"`
}

// =============消息处理===============

// =============频道消息处理================

// Author 表示消息发送者信息
type Author struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Avatar      string `json:"avatar"`
	Bot         bool   `json:"bot"`
	UnionOpenID string `json:"union_openid"`
}

// Member 表示成员信息
type Member struct {
	Nick     string   `json:"nick"`
	Roles    []string `json:"roles"`
	JoinedAt string   `json:"joined_at"`
}

// Mention 表示一个被提及的用户
type Mention struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	// 可扩展其他字段，比如头像、是否bot等
}

// 频道消息事件
type GuildMessageEvent struct {
	ID string `json:"id"`
	// 子频道ID
	ChannelID string `json:"channel_id"`
	// 频道ID
	GuildID string `json:"guild_id"`
	// 消息内容
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
	Author    Author `json:"author"`
	Member    Member `json:"member"`
	// 被@的用户列表
	Mentions     []Mention `json:"mentions"`
	Seq          int       `json:"seq"`
	SeqInChannel string    `json:"seq_in_channel"`
}

// =============群消息处理================

// 群用户
type GroupAuthor struct {
	ID string `json:"id"`
	// 群
	MemberOpenID string `json:"member_openid"`
	// 私聊
	UserOpenID  string `json:"user_openid"`
	UnionOpenID string `json:"union_openid"`
}

// 群消息场景
type GroupMessageScene struct {
	Source string `json:"source"`
}

// 群消息事件
type GroupMessageEvent struct {
	ID        string      `json:"id"`
	Content   string      `json:"content"`
	Timestamp string      `json:"timestamp"`
	Author    GroupAuthor `json:"author"`
	// 群
	GroupID string `json:"group_id"`
	// 群
	GroupOpenID  string            `json:"group_openid"`
	MessageScene GroupMessageScene `json:"message_scene"`
	MessageType  int               `json:"message_type"`
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
