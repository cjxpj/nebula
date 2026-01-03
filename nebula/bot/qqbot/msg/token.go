package qqbot_msg

import (
	"net/http"

	"github.com/patrickmn/go-cache"
)

// 去重 true 表示首次，false 表示重复
func (d *RouterQQBot) CheckOnce(key string) bool {
	if d.LastMsg == nil {
		// 报错
		panic("LastMsg is nil")
	}
	if _, found := d.LastMsg.Get(key); found {
		return false
	}
	d.LastMsg.Set(key, true, cache.DefaultExpiration)
	return true
}

// GetAccessToken 获取 QQBot Access Token
func GetAccessToken(appId, clientSecret string) (*AccessTokenResponse, error) {
	url := "https://bots.qq.com/app/getAppAccessToken"
	body := map[string]string{
		"appId":        appId,
		"clientSecret": clientSecret,
	}

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
