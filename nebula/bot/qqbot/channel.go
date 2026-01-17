package qqbot

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	qqbot_msg "github.com/cjxpj/nebula/bot/qqbot/msg"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// 频道处理
func qqBOTChannelRun(payload *qqbot_msg.Payload) {
	// 解析消息数据
	m := &qqbot_msg.GuildMessageEvent{}
	err := json.Unmarshal([]byte(payload.Data), m)
	if err != nil {
		fmt.Println("QQBot消息数据验证失败")
		return
	}

	// 处理重复消息ID
	if !dto.ServerConfig.QQBot.CheckOnce(fmt.Sprint(m.Seq)) {
		// fmt.Println("QQBot消息ID重复", m.Seq)
		return
	}

	// 处理消息次数
	qqbot_msg.MsgCount++

	// 新建副本消息
	msg := m.Content

	// 词库
	BotDic := utils.NewFileQueue(dto.ServerConfig.QQBot.FilePath)
	FileData, err := BotDic.ReadFromFile()
	if err != nil {
		utils.Error("读取机器人词库出错")
		return
	}

	// 去除艾特
	msg = RemoveLeadingMentionOnce(msg)

	// 回复消息
	dic := dic_dto.NewDic(dto.ServerConfig.QQBot.FilePath, FileData).
		// 变量
		SetGlobal_v(dto.NewVal().
			Set("来源", "频道").
			Set("群号", m.GuildID).
			Set("子群号", m.ChannelID).
			Set("昵称", m.Author.Username).
			Set("QQ", m.Author.ID).
			Set("头像", m.Author.Avatar))
	dic.SetFunc("调用", dto.DicFunc{
		L: "2..",
		Fn: func(d *dto.DicInputs) (any, error) {
			go func() {
				qqVal := dic.NewDicVal()
				sleepTime := d.Inputs.Int(1)
				time.Sleep(time.Duration(sleepTime) * time.Millisecond)
				rMsg := dic_api.Api.DicRunPrivateVal(dic, d.Inputs.StringAfter(2), qqVal)
				rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
				if img := qqVal.P.GetStr("发送图片"); img != "" {
					dto.ServerConfig.QQBot.API.ReplyChannelImgMessage(m.ID, m.ChannelID, img, rMsg)
				} else if rMsg != "" {
					dto.ServerConfig.QQBot.API.ReplyChannelMessage(m.ID, m.ChannelID, rMsg)
				}
			}()
			return "", nil
		}})
	rMsg := dic_api.Api.DicRun(dic, msg)
	// 替换‘\r’换行
	rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")

	// fmt.Println("QQBot回复:", rMsg)
	if img := dic.Val.P.GetStr("发送图片"); img != "" {
		_, mErr := dto.ServerConfig.QQBot.API.ReplyChannelImgMessage(m.ID, m.ChannelID, img, rMsg)
		if mErr != nil {
			fmt.Println("QQBot回复图文失败", mErr)
		}
		return
	}
	if rMsg != "" {
		_, mErr := dto.ServerConfig.QQBot.API.ReplyChannelMessage(m.ID, m.ChannelID, rMsg)
		if mErr != nil {
			fmt.Println("QQBot回复失败", mErr)
		}
	}
}

// 频道私信处理
func qqBOTChannelPrivateRun(payload *qqbot_msg.Payload) {
	// 解析消息数据
	m := &qqbot_msg.GuildMessageEvent{}
	err := json.Unmarshal([]byte(payload.Data), m)
	if err != nil {
		fmt.Println("QQBot消息数据验证失败")
		return
	}

	// 处理重复消息ID
	if !dto.ServerConfig.QQBot.CheckOnce(fmt.Sprint(m.Seq)) {
		// fmt.Println("QQBot消息ID重复", m.Seq)
		return
	}

	// 处理消息次数
	qqbot_msg.MsgCount++

	// 新建副本消息
	msg := m.Content

	// 词库
	BotDic := utils.NewFileQueue(dto.ServerConfig.QQBot.FilePath)
	FileData, err := BotDic.ReadFromFile()
	if err != nil {
		utils.Error("读取机器人词库出错")
		return
	}

	// 去除艾特
	msg = RemoveLeadingMentionOnce(msg)

	// 回复消息
	dic := dic_dto.NewDic(dto.ServerConfig.QQBot.FilePath, FileData).
		// 变量
		SetGlobal_v(dto.NewVal().
			Set("来源", "频道").
			Set("群号", m.GuildID).
			Set("子群号", m.ChannelID).
			Set("昵称", m.Author.Username).
			Set("QQ", m.Author.ID).
			Set("头像", m.Author.Avatar))

	dic.SetFunc("调用", dto.DicFunc{
		L: "2..",
		Fn: func(d *dto.DicInputs) (any, error) {
			go func() {
				qqVal := dic.NewDicVal()
				sleepTime := d.Inputs.Int(1)
				time.Sleep(time.Duration(sleepTime) * time.Millisecond)
				rMsg := dic_api.Api.DicRunPrivateVal(dic, d.Inputs.StringAfter(2), qqVal)
				rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
				if img := qqVal.P.GetStr("发送图片"); img != "" {
					dto.ServerConfig.QQBot.API.ReplyChannelPrivateMessage(m.ID, m.GuildID, img, rMsg)
				} else if rMsg != "" {
					dto.ServerConfig.QQBot.API.ReplyPrivateMessage(m.ID, m.GuildID, rMsg)
				}
			}()
			return "", nil
		}})

	rMsg := dic_api.Api.DicRun(dic, msg)
	// 替换‘\r’换行
	rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")

	// fmt.Println("QQBot回复:", rMsg)

	if img := dic.Val.P.GetStr("发送图片"); img != "" {
		_, mErr := dto.ServerConfig.QQBot.API.ReplyChannelPrivateMessage(m.ID, m.GuildID, img, rMsg)
		if mErr != nil {
			fmt.Println("QQBot回复图文失败", mErr)
		}
		return
	}
	if rMsg != "" {
		_, mErr := dto.ServerConfig.QQBot.API.ReplyPrivateMessage(m.ID, m.GuildID, rMsg)
		if mErr != nil {
			fmt.Println("QQBot回复失败", mErr)
		}
	}
}
