package qqbot

import (
	"fmt"
	"strings"
	"sync"

	qqbot_msg "github.com/cjxpj/nebula/bot/qqbot/msg"
	"github.com/cjxpj/nebula/dto"
)

func init() {
	dto.BotFuncsRegistry["QQBot"] = ActiveFuncs
}

// PushContext 存储当前机器人及消息上下文，供 ReplyFuncs 中的回复函数使用
type PushContext struct {
	Bot         *qqbot_msg.RouterQQBot
	MsgID       string
	GroupOpenID string
	ChannelID   string
	UserOpenID  string
}

var (
	pushMu      sync.RWMutex
	currentPush *PushContext
)

// SetPushContext 设置当前上下文
func SetPushContext(ctx *PushContext) {
	pushMu.Lock()
	defer pushMu.Unlock()
	currentPush = ctx
}

// ClearPushContext 清除当前上下文
func ClearPushContext() {
	pushMu.Lock()
	defer pushMu.Unlock()
	currentPush = nil
}

// GetPushContext 获取当前上下文
func GetPushContext() *PushContext {
	pushMu.RLock()
	defer pushMu.RUnlock()
	return currentPush
}

// getActiveBotAPI 从全局配置取第一个启用的 QQBot API（主动发送，无 PushContext 依赖）
func getActiveBotAPI() *qqbot_msg.QQBot {
	if dto.ServerConfig.QQBots == nil || len(dto.ServerConfig.QQBots) == 0 {
		return nil
	}
	for _, bot := range dto.ServerConfig.QQBots {
		if bot != nil && bot.Open && bot.API != nil {
			return bot.API
		}
	}
	return nil
}

// ========== ActiveFuncs：主动发送（#引入=@QQBot 注入），通过全局 Bot API，无需 PushContext ==========
var ActiveFuncs = map[string]dto.DicFunc{
	"群单发": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			bot := getActiveBotAPI()
			if bot == nil {
				return "QQBot未启用或未配置", nil
			}
			groupOpenID := d.Inputs.String(1)
			rMsg := d.Inputs.String(2)
			if rMsg == "" {
				return "内容为空", nil
			}
			rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
			if _, err := bot.ReplyGroupMessage("", groupOpenID, rMsg); err != nil {
				return "发送失败: " + err.Error(), nil
			}
			return "发送成功", nil
		},
	},
	"群发图": {
		L: "3",
		Fn: func(d *dto.DicInputs) (any, error) {
			bot := getActiveBotAPI()
			if bot == nil {
				return "QQBot未启用或未配置", nil
			}
			if _, err := bot.ReplyGroupImgMessage("", d.Inputs.String(1), d.Inputs.String(2), d.Inputs.String(3)); err != nil {
				return "发送失败: " + err.Error(), nil
			}
			return "发送成功", nil
		},
	},
	"群发MD": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			bot := getActiveBotAPI()
			if bot == nil {
				return "QQBot未启用或未配置", nil
			}
			if _, err := bot.ReplyGroupAnyMarkdownMessage("", d.Inputs.String(1), d.Inputs.String(2)); err != nil {
				return "发送失败: " + err.Error(), nil
			}
			return "发送成功", nil
		},
	},
	"群发语音": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			bot := getActiveBotAPI()
			if bot == nil {
				return "QQBot未启用或未配置", nil
			}
			if _, err := bot.ReplyGroupVoiceMessage("", d.Inputs.String(1), d.Inputs.String(2)); err != nil {
				return "发送失败: " + err.Error(), nil
			}
			return "发送成功", nil
		},
	},
	"群发视频": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			bot := getActiveBotAPI()
			if bot == nil {
				return "QQBot未启用或未配置", nil
			}
			if _, err := bot.ReplyGroupVideoMessage("", d.Inputs.String(1), d.Inputs.String(2)); err != nil {
				return "发送失败: " + err.Error(), nil
			}
			return "发送成功", nil
		},
	},
	"私聊": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			bot := getActiveBotAPI()
			if bot == nil {
				return "QQBot未启用或未配置", nil
			}
			rMsg := d.Inputs.String(2)
			if rMsg == "" {
				return "内容为空", nil
			}
			rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
			if _, err := bot.ReplyGroupPrivateMessage("", d.Inputs.String(1), rMsg); err != nil {
				return "发送失败: " + err.Error(), nil
			}
			return "发送成功", nil
		},
	},
	"私聊图": {
		L: "3",
		Fn: func(d *dto.DicInputs) (any, error) {
			bot := getActiveBotAPI()
			if bot == nil {
				return "QQBot未启用或未配置", nil
			}
			if _, err := bot.ReplyGroupPrivateImgMessage("", d.Inputs.String(1), d.Inputs.String(2), d.Inputs.String(3)); err != nil {
				return "发送失败: " + err.Error(), nil
			}
			return "发送成功", nil
		},
	},
}

// ========== ReplyFuncs：回复发送（依赖 PushContext，用于 Bot 消息处理） ==========
var ReplyFuncs = map[string]dto.DicFunc{

	"发送文本": {
		L: "1|2",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext()
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", fmt.Errorf("QQBot上下文未初始化")
			}
			go func() {
				rMsg := strings.ReplaceAll(d.Inputs.String(1), "\\r", "\n")
				if d.Inputs.LenOk(1) {
					if rMsg != "" {
						ctx.Bot.API.ReplyGroupMessage(ctx.MsgID, ctx.GroupOpenID, "\n"+rMsg)
					}
				} else {
					ctx.Bot.API.ReplyGroupImgMessage(ctx.MsgID, ctx.GroupOpenID, d.Inputs.String(2), rMsg)
				}
			}()
			return "", nil
		},
	},
	"发送MD": {
		L: "1..",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext()
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", fmt.Errorf("QQBot上下文未初始化")
			}
			// 简单MD文本发送
			if d.Inputs.LenOk(1) {
				ctx.Bot.API.ReplyGroupAnyMarkdownMessage(ctx.MsgID, ctx.GroupOpenID, d.Inputs.String(1))
				return "", nil
			}
			// CustomTemplateId + key=value 参数对
			if (d.Inputs.Len()-1)%2 != 0 {
				return nil, fmt.Errorf("要设置对应键跟值")
			}
			params := make([]*qqbot_msg.MarkdownParams, 0)
			list := d.Inputs.StringAfterList(2)
			for i := 0; i < len(list); i += 2 {
				key := list[i]
				val := list[i+1]
				val = strings.ReplaceAll(val, "\n", "\r")
				val = mdRe.ReplaceAllString(val, "[\r\n$1]($2)")
				val = mdReAt.ReplaceAllString(val, "<$1\r\n$2>")
				val = strings.ReplaceAll(val, "```", "'''")
				if strings.HasPrefix(val, "#") {
					val = " " + val
				}
				vals := strings.Split(val, "\r\n")
				params = append(params, &qqbot_msg.MarkdownParams{
					Key:    key,
					Values: vals,
				})
			}
			md := &qqbot_msg.Markdown{
				CustomTemplateId: d.Inputs.String(1),
				Params:           params,
			}
			ctx.Bot.API.ReplyGroupMarkdownMessage(ctx.MsgID, ctx.GroupOpenID, md)
			return "", nil
		},
	},
	"发送视频": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext()
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", fmt.Errorf("QQBot上下文未初始化")
			}
			go func() {
				ctx.Bot.API.ReplyGroupVideoMessage(ctx.MsgID, ctx.GroupOpenID, d.Inputs.String(1))
			}()
			return "", nil
		},
	},
	"发送语音": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext()
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", fmt.Errorf("QQBot上下文未初始化")
			}
			go func() {
				ctx.Bot.API.ReplyGroupVoiceMessage(ctx.MsgID, ctx.GroupOpenID, d.Inputs.String(1))
			}()
			return "", nil
		},
	},
}
