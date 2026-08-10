package qqbot_msg

import (
	"net/http"

	"github.com/patrickmn/go-cache"
)

// 去重 true 表示首次，false 表示重复
func (d *RouterQQBot) CheckOnce(key string) bool {
	if d.LastMsg == nil {
		// 未初始化的 LastMsg，当作不重复处理（跳过去重）
		return true
	}
	if _, found := d.LastMsg.Get(key); found {
		return false
	}
	d.LastMsg.Set(key, true, cache.DefaultExpiration)
	return true
}

// GetAccessToken 获取 QQBot Access Token
func GetAccessToken(appId, clientSecret string) (*AccessTokenResponse, error) {
	url := "https://api.bot.qq.com/app/getAppAccessToken"
	body := map[string]string{
		"appId":        appId,
		"clientSecret": clientSecret,
	}

	// fmt.Println("GetAccessToken", url, body)

	var res *AccessTokenResponse
	if err := postJson(url, body, nil, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// GetQQBotAuthHeader 构造 Authorization 头
func GetQQBotAuthHeader(token string) http.Header {
	header := http.Header{}
	header.Set("Authorization", "QQBot "+token)
	return header
}
