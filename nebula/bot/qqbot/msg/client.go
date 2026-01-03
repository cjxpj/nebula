package qqbot_msg

import (
	"strconv"
	"time"
)

// NewQQBot 创建一个 QQBot 客户端
func NewQQBot(appId, clientSecret string) *QQBot {
	return &QQBot{
		AppId:        appId,
		ClientSecret: clientSecret,
	}
}

// ensureToken 确保 token 有效（自动刷新）
func (b *QQBot) ensureToken() error {
	// 尝试从字符串转换 ExpiresIn 为 int
	var expiresIn int
	if b.Key != nil {
		if sec, err := strconv.Atoi(b.Key.ExpiresIn); err == nil {
			expiresIn = sec
		}
	}

	// 如果 token 存在，且未过期，则复用
	if b.Key != nil && expiresIn > 0 && time.Since(b.TokenTime) < time.Duration(expiresIn)*time.Second {
		return nil
	}

	// 获取新的 token
	token, err := GetAccessToken(b.AppId, b.ClientSecret)
	if err != nil {
		return err
	}

	b.Key = token
	b.TokenTime = time.Now()

	return nil
}
