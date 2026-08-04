package qqbot

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	qqbot_msg "github.com/cjxpj/nebula/bot/qqbot/msg"
	"github.com/cjxpj/nebula/debugLog"
)

// QQBot处理
func BotMessage(w http.ResponseWriter, r *http.Request, bot *qqbot_msg.RouterQQBot) {
	httpBody, err := io.ReadAll(r.Body)
	if err != nil {
		w.Write([]byte("Bot Post"))
		return
	}

	// Webhook 签名校验：服务端进行消息推送时必须验证签名
	// 开放平台验证 (op=13) 不需要签名校验（平台还未确认机器人身份）
	payload := &qqbot_msg.Payload{}
	if err = json.Unmarshal(httpBody, payload); err != nil {
		w.Write([]byte("Bot ErrorData"))
		return
	}
	if bot != nil && bot.API != nil && payload.Op != 13 {
		signature := r.Header.Get("X-Signature-Ed25519")
		timestamp := r.Header.Get("X-Signature-Timestamp")
		if signature == "" || timestamp == "" {
			debugLog.Infof("QQBot Webhook 消息缺少签名头, 拒绝处理")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if !qqbot_msg.VerifyWebhookSignature(bot.API.ClientSecret, signature, timestamp, httpBody) {
			debugLog.Infof("QQBot Webhook 签名校验失败, 拒绝处理")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if bot.Debug {
			fmt.Println("[QQBot] Webhook 签名校验通过")
		}
	}
	if bot != nil && bot.Debug {
		fmt.Println("[QQBot] ========== 收到消息 ==========")
		fmt.Printf("[QQBot 来源] %s | IP: %s\n", r.RemoteAddr, r.Header.Get("X-Forwarded-For"))
		fmt.Printf("[QQBot 操作] %s\n", qqbot_msg.Op(payload.Op))
	}

	switch qqbot_msg.Op(payload.Op) {
	case "开放平台对机器人服务端进行验证":
		// 验证数据
		if bot == nil || bot.API == nil {
			debugLog.Infof("QQBot验证失败: bot 未初始化")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		data := &qqbot_msg.ValidationRequest{}
		if err = json.Unmarshal(payload.Data, data); err == nil {
			if bot.Debug {
				fmt.Printf("[QQBot 验证] Token=%s Event=%s\n", data.Token, data.Event)
			}
			// 效验签名
			if sign, err := qqbot_msg.GenerateValidationResult(bot.API.ClientSecret, data.Event, data.Token); err == nil {
				// 返回效验结果（必须先设置 Content-Type，Cloudflare 隧道才正确转发）
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(sign))
				if bot.Debug {
					fmt.Printf("[QQBot 返回] 验证签名: %s\n", sign)
				}
				debugLog.Infof("QQBot数据验证成功")
				return
			}
			debugLog.Infof("QQBot数据签名失败")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		debugLog.Infof("QQBot数据验证失败")
		w.WriteHeader(http.StatusBadRequest)
		return
	case "服务端进行消息推送":
		// 处理消息
		switch payload.Type {
		case "MESSAGE_CREATE": // 频道-私域全局消息
			if bot.Debug {
				m := &qqbot_msg.GuildMessageEvent{}
				json.Unmarshal(payload.Data, m)
				fmt.Printf("[QQBot 来源] 频道消息 | 频道=%s 子频道=%s 用户=%s(%s) 内容=%s\n",
					m.GuildID, m.ChannelID, m.Author.Username, m.Author.ID, m.Content)
			}
			qqBOTChannelRun(payload, bot)
			w.Write([]byte("Bot Message"))
			if bot.Debug {
				fmt.Println("[QQBot 返回] HTTP 200 OK (Bot Message)")
				fmt.Println("[QQBot] ================================")
			}

		case "AT_MESSAGE_CREATE": // 频道-公域艾特消息
			if bot.Debug {
				m := &qqbot_msg.GuildMessageEvent{}
				json.Unmarshal(payload.Data, m)
				fmt.Printf("[QQBot 来源] 频道艾特 | 频道=%s 子频道=%s 用户=%s(%s) 内容=%s\n",
					m.GuildID, m.ChannelID, m.Author.Username, m.Author.ID, m.Content)
			}
			qqBOTChannelRun(payload, bot)
			w.Write([]byte("Bot Message"))
			if bot.Debug {
				fmt.Println("[QQBot 返回] HTTP 200 OK (Bot Message)")
				fmt.Println("[QQBot] ================================")
			}

		case "DIRECT_MESSAGE_CREATE": // 频道-私聊消息
			if bot.Debug {
				m := &qqbot_msg.GuildMessageEvent{}
				json.Unmarshal(payload.Data, m)
				fmt.Printf("[QQBot 来源] 频道私聊 | 频道=%s 用户=%s(%s) 内容=%s\n",
					m.GuildID, m.Author.Username, m.Author.ID, m.Content)
			}
			qqBOTChannelPrivateRun(payload, bot)
			w.Write([]byte("Bot Message"))
			if bot.Debug {
				fmt.Println("[QQBot 返回] HTTP 200 OK (Bot Message)")
				fmt.Println("[QQBot] ================================")
			}

		case "GROUP_AT_MESSAGE_CREATE": // 群-艾特消息
			if bot.Debug {
				m := &qqbot_msg.GroupMessageEvent{}
				json.Unmarshal(payload.Data, m)
				fmt.Printf("[QQBot 来源] 群艾特 | 群=%s 用户=%s(%s) 内容=%s\n",
					m.GroupOpenID, m.Author.Username, m.Author.ID, m.Content)
			}
			qqBOTGroupATRun(payload, bot)
			w.Write([]byte("Bot Message"))
			if bot.Debug {
				fmt.Println("[QQBot 返回] HTTP 200 OK (Bot Message)")
				fmt.Println("[QQBot] ================================")
			}

		case "GROUP_MESSAGE_CREATE": // 群-全部消息
			if bot.Debug {
				m := &qqbot_msg.GroupMessageEvent{}
				json.Unmarshal(payload.Data, m)
				fmt.Printf("[QQBot 来源] 群消息 | 群=%s 用户=%s(%s) 内容=%s\n",
					m.GroupOpenID, m.Author.Username, m.Author.ID, m.Content)
			}
			qqBOTGroupRun(payload, bot)
			w.Write([]byte("Bot Message"))
			if bot.Debug {
				fmt.Println("[QQBot 返回] HTTP 200 OK (Bot Message)")
				fmt.Println("[QQBot] ================================")
			}

		case "C2C_MESSAGE_CREATE": // 群-私聊
			if bot.Debug {
				m := &qqbot_msg.GroupMessageEvent{}
				json.Unmarshal(payload.Data, m)
				fmt.Printf("[QQBot 来源] 私聊 | 用户=%s(%s) 内容=%s\n",
					m.Author.Username, m.Author.UserOpenID, m.Content)
			}
			qqBOTGroupPrivateRun(payload, bot)
			w.Write([]byte("Bot Message"))
			if bot.Debug {
				fmt.Println("[QQBot 返回] HTTP 200 OK (Bot Message)")
				fmt.Println("[QQBot] ================================")
			}

		case "GROUP_MEMBER_ADD", "GROUP_ADD_ROBOT": // 群成员加入
			if bot.Debug {
				m := &qqbot_msg.GroupMemberEvent{}
				json.Unmarshal(payload.Data, m)
				fmt.Printf("[QQBot 来源] 入群事件 | 群=%s 成员=%s 用户=%s\n",
					m.GroupOpenID, m.MemberOpenID, m.UserOpenID)
			}
			qqBOTGroupEventRun(payload, bot)
			w.Write([]byte("Bot Message"))
			if bot.Debug {
				fmt.Println("[QQBot 返回] HTTP 200 OK (Bot Message)")
				fmt.Println("[QQBot] ================================")
			}

		case "GROUP_MEMBER_REMOVE", "GROUP_DEL_ROBOT": // 群成员退出
			if bot.Debug {
				m := &qqbot_msg.GroupMemberEvent{}
				json.Unmarshal(payload.Data, m)
				fmt.Printf("[QQBot 来源] 退群事件 | 群=%s 成员=%s 用户=%s\n",
					m.GroupOpenID, m.MemberOpenID, m.UserOpenID)
			}
			qqBOTGroupEventRun(payload, bot)
			w.Write([]byte("Bot Message"))
			if bot.Debug {
				fmt.Println("[QQBot 返回] HTTP 200 OK (Bot Message)")
				fmt.Println("[QQBot] ================================")
			}

		default:
			if bot.Debug {
				fmt.Printf("[QQBot] 未支持的消息类型: %s\n", payload.Type)
			}
			debugLog.Infof("QQBot消息类型未支持")
			return
		}
	}
}
