package qqbot

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	qqbot_msg "github.com/cjxpj/nebula/bot/qqbot/msg"
	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/dto"
)

// QQBot处理
func BotMessage(w http.ResponseWriter, r *http.Request) {
	httpBody, err := io.ReadAll(r.Body)
	if err != nil {
		w.Write([]byte("Bot Post"))
		return
	}
	payload := &qqbot_msg.Payload{}
	if err = json.Unmarshal(httpBody, payload); err != nil {
		w.Write([]byte("Bot ErrorData"))
		return
	}
	fmt.Println("QQBot收到消息")
	fmt.Println("QQBot消息内容:", string(httpBody))
	fmt.Println("处理:", qqbot_msg.Op(payload.Op))

	switch qqbot_msg.Op(payload.Op) {
	case "开放平台对机器人服务端进行验证":
		// 验证数据
		data := &qqbot_msg.ValidationRequest{}
		if err = json.Unmarshal(payload.Data, data); err == nil {
			// 效验签名
			if sign, err := qqbot_msg.GenerateValidationResult(dto.ServerConfig.QQBot.API.ClientSecret, data.Event, data.Token); err == nil {
				// 返回效验结果
				w.Write([]byte(sign))
				debugLog.Infof("QQBot数据验证成功")
				return
			}
			debugLog.Infof("QQBot数据签名失败")
			return
		}
		debugLog.Infof("QQBot数据验证失败")
		return
	case "服务端进行消息推送":
		// 处理消息
		switch payload.Type {

		case "MESSAGE_CREATE": // 频道-私域全局消息
			qqBOTChannelRun(payload)
			w.Write([]byte("Bot Message"))

		case "AT_MESSAGE_CREATE": // 频道-公域艾特消息
			qqBOTChannelRun(payload)
			w.Write([]byte("Bot Message"))

		case "DIRECT_MESSAGE_CREATE": // 频道-私聊消息
			qqBOTChannelPrivateRun(payload)
			w.Write([]byte("Bot Message"))

		case "GROUP_AT_MESSAGE_CREATE": // 群-艾特消息
			qqBOTGroupATRun(payload)
			w.Write([]byte("Bot Message"))

		case "C2C_MESSAGE_CREATE": // 群-私聊
			qqBOTGroupPrivateRun(payload)
			w.Write([]byte("Bot Message"))

		default:
			debugLog.Infof("QQBot消息类型未支持")
			return
		}
	}
}
