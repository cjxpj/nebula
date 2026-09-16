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
	// 事件订阅加密密钥（Encrypt Key），为空表示事件未加密
	EncryptKey string
	// 事件订阅验证令牌（Verification Token），为空表示不校验
	VerificationToken string
}

type SlackURLVerification struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
	Token     string `json:"token"`
	Encrypt   string `json:"encrypt"`
}
