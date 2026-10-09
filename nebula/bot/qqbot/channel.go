package qqbot

import (
	"encoding/json"
	"fmt"

	botdic "github.com/cjxpj/nebula/bot/botdic"
	qqbot_msg "github.com/cjxpj/nebula/bot/qqbot/msg"
	"github.com/cjxpj/nebula/debugLog"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
)

// 频道处理
func qqBOTChannelRun(payload *qqbot_msg.Payload, bot *qqbot_msg.RouterQQBot) {
	// 解析消息数据
	m := &qqbot_msg.GuildMessageEvent{}
	err := json.Unmarshal([]byte(payload.Data), m)
	if err != nil {
		debugLog.Infof("QQBot消息数据验证失败")
		return
	}

	// 处理重复消息ID
	if !bot.CheckOnce(fmt.Sprint(m.Seq)) {
		// fmt.Println("QQBot消息ID重复", m.Seq)
		return
	}

	// 处理消息次数
	qqbot_msg.MsgCount++

	// 去除艾特
	msg := RemoveLeadingMentionOnce(m.Content)

	// 词库
	botdic.Run{
		FilePath:   bot.FilePath,
		SingleFile: true,
		Val: dto.NewVal().
			Set("来源", "频道").
			Set("群号", m.GuildID).
			Set("子群号", m.ChannelID).
			Set("昵称", m.Author.Username).
			Set("QQ", m.Author.ID).
			Set("管理", getChannelAdminRole(m.Member.Roles)).
			Set("头像", m.Author.Avatar).
			Set("MsgId", m.ID).
			Set("MessageID", m.ID),
		Funcs: ReplyFuncs,
		Prepare: func(dic *dic_dto.Dic) {
			// 设置PushContext供 #引入=QQBot 函数使用
			SetPushContext(dic, &PushContext{
				Bot:       bot,
				MsgID:     m.ID,
				ChannelID: m.ChannelID,
			})
		},
		Trigger:   msg,
		AsyncCall: true,
		// 频道消息可能只设置「发送图片」而没有文本，空文本也需送达
		DeliverOnEmpty: true,
		Deliver: func(rMsg string, val *dto.DicVal) {
			if img := val.P.GetStr("发送图片"); img != "" {
				if _, mErr := bot.API.ReplyChannelImgMessage(m.ID, m.ChannelID, img, rMsg); mErr != nil {
					debugLog.Infof("QQBot回复图文失败%v", mErr)
				}
				return
			}
			if rMsg != "" {
				if _, mErr := bot.API.ReplyChannelMessage(m.ID, m.ChannelID, rMsg); mErr != nil {
					debugLog.Infof("QQBot回复失败%v", mErr)
				}
			}
		},
	}.Exec()
}

// 频道私信处理
func qqBOTChannelPrivateRun(payload *qqbot_msg.Payload, bot *qqbot_msg.RouterQQBot) {
	// 解析消息数据
	m := &qqbot_msg.GuildMessageEvent{}
	err := json.Unmarshal([]byte(payload.Data), m)
	if err != nil {
		debugLog.Infof("QQBot私聊消息数据验证失败")
		return
	}

	if !bot.CheckOnce(fmt.Sprint(m.Seq)) {
		fmt.Println("QQBot私聊消息ID重复")
		return
	}
	// 处理消息次数
	qqbot_msg.MsgCount++

	// 去除艾特
	msg := RemoveLeadingMentionOnce(m.Content)

	botdic.Run{
		FilePath:   bot.FilePath,
		SingleFile: true,
		Val: dto.NewVal().
			Set("来源", "频道").Set("群号", m.GuildID).Set("子群号", m.ChannelID).
			Set("昵称", m.Author.Username).Set("QQ", m.Author.ID).
			Set("管理", getChannelAdminRole(m.Member.Roles)).Set("头像", m.Author.Avatar).
			Set("MsgId", m.ID).Set("MessageID", m.ID),
		Funcs: ReplyFuncs,
		Prepare: func(dic *dic_dto.Dic) {
			SetPushContext(dic, &PushContext{Bot: bot, MsgID: m.ID, ChannelID: m.ChannelID})
		},
		Trigger:   msg,
		AsyncCall: true,
		// 频道私信同样支持「只发送图片」的空文本回复
		DeliverOnEmpty: true,
		Deliver: func(rMsg string, val *dto.DicVal) {
			if img := val.P.GetStr("发送图片"); img != "" {
				if _, mErr := bot.API.ReplyChannelPrivateMessage(m.ID, m.GuildID, img, rMsg); mErr != nil {
					debugLog.Infof("QQBot回复图文失败%v", mErr)
				}
				return
			}
			if rMsg != "" {
				if _, mErr := bot.API.ReplyPrivateMessage(m.ID, m.GuildID, rMsg); mErr != nil {
					debugLog.Infof("QQBot回复失败%v", mErr)
				}
			}
		},
	}.Exec()
}

// getChannelAdminRole 从频道成员角色列表判断管理权限
// 返回 "1" (创建者), "2" (管理员), "0" (普通成员)
func getChannelAdminRole(roles []string) string {
	for _, r := range roles {
		switch r {
		case "4": // 频道创建者
			return "1"
		case "2": // 频道管理员
			return "2"
		}
	}
	return "0"
}
