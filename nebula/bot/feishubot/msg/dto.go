package feishubot_msg

import lark "github.com/larksuite/oapi-sdk-go/v3"

type RouterFeishubot struct {
	// 是否开启
	Open bool
	// 地址
	Addr string
	// API
	API *lark.Client
	// 词库路径
	FilePath string
	// 消息次数
	Count int
}

type SlackURLVerification struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
	Token     string `json:"token"`
	Encrypt   string `json:"encrypt"`
}
