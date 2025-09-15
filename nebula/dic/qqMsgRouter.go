package dic

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/qqbottool"
	"github.com/cjxpj/nebula/utils"
)

// 频道处理
func (s *ServeRouter) QQBOTChannelRun(payload *qqbottool.Payload) {
	// 解析消息数据
	m := &qqbottool.GuildMessageEvent{}
	err := json.Unmarshal([]byte(payload.Data), m)
	if err != nil {
		fmt.Println("QQBot消息数据验证失败")
		return
	}

	// 处理重复消息ID
	if !s.QQBot.CheckOnce(fmt.Sprint(m.Seq)) {
		// fmt.Println("QQBot消息ID重复", m.Seq)
		return
	}

	// 处理消息次数
	s.QQBot.MsgCount++

	// 新建副本消息
	msg := m.Content

	// 词库
	BotDic := utils.NewFileQueue(s.QQBot.FilePath)
	FileData, err := BotDic.ReadFromFile()
	if err != nil {
		utils.Error("读取机器人词库出错")
		return
	}

	// 去除艾特
	msg = qqbottool.RemoveLeadingMentionOnce(msg)

	// 回复消息
	dic := NewDic(s.QQBot.FilePath, FileData).
		// 变量
		SetGlobal_v(dto.NewVal().
			Set("来源", "频道").
			Set("群号", m.GuildID).
			Set("子群号", m.ChannelID).
			Set("昵称", m.Author.Username).
			Set("QQ", m.Author.ID).
			Set("头像", m.Author.Avatar))
	dic.SetFunc("调用", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("2..") {
			return "参数错误", nil
		}
		go func() {
			qqVal := dic.NewDicVal()
			sleepTime := inputs.Int(1)
			time.Sleep(time.Duration(sleepTime) * time.Millisecond)
			rMsg := dic.RunPrivateVal(inputs.StringAfter(2), qqVal)
			rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
			if img := qqVal.P.GetStr("发送图片"); img != "" {
				s.QQBot.API.ReplyChannelImgMessage(m.ID, m.ChannelID, img, rMsg)
			} else if rMsg != "" {
				s.QQBot.API.ReplyChannelMessage(m.ID, m.ChannelID, rMsg)
			}
		}()
		return "", nil
	})
	rMsg := dic.Run(msg)
	// 替换‘\r’换行
	rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")

	// fmt.Println("QQBot回复:", rMsg)
	if img := dic.Val.P.GetStr("发送图片"); img != "" {
		_, mErr := s.QQBot.API.ReplyChannelImgMessage(m.ID, m.ChannelID, img, rMsg)
		if mErr != nil {
			fmt.Println("QQBot回复图文失败", mErr)
		}
		return
	}
	if rMsg != "" {
		_, mErr := s.QQBot.API.ReplyChannelMessage(m.ID, m.ChannelID, rMsg)
		if mErr != nil {
			fmt.Println("QQBot回复失败", mErr)
		}
	}
}

// 频道私信处理
func (s *ServeRouter) QQBOTChannelPrivateRun(payload *qqbottool.Payload) {
	// 解析消息数据
	m := &qqbottool.GuildMessageEvent{}
	err := json.Unmarshal([]byte(payload.Data), m)
	if err != nil {
		fmt.Println("QQBot消息数据验证失败")
		return
	}

	// 处理重复消息ID
	if !s.QQBot.CheckOnce(fmt.Sprint(m.Seq)) {
		// fmt.Println("QQBot消息ID重复", m.Seq)
		return
	}

	// 处理消息次数
	s.QQBot.MsgCount++

	// 新建副本消息
	msg := m.Content

	// 词库
	BotDic := utils.NewFileQueue(s.QQBot.FilePath)
	FileData, err := BotDic.ReadFromFile()
	if err != nil {
		utils.Error("读取机器人词库出错")
		return
	}

	// 去除艾特
	msg = qqbottool.RemoveLeadingMentionOnce(msg)

	// 回复消息
	dic := NewDic(s.QQBot.FilePath, FileData).
		// 变量
		SetGlobal_v(dto.NewVal().
			Set("来源", "频道").
			Set("群号", m.GuildID).
			Set("子群号", m.ChannelID).
			Set("昵称", m.Author.Username).
			Set("QQ", m.Author.ID).
			Set("头像", m.Author.Avatar))

	dic.SetFunc("调用", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("2..") {
			return "参数错误", nil
		}
		go func() {
			qqVal := dic.NewDicVal()
			sleepTime := inputs.Int(1)
			time.Sleep(time.Duration(sleepTime) * time.Millisecond)
			rMsg := dic.RunPrivateVal(inputs.StringAfter(2), qqVal)
			rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
			if img := qqVal.P.GetStr("发送图片"); img != "" {
				s.QQBot.API.ReplyChannelPrivateMessage(m.ID, m.GuildID, img, rMsg)
			} else if rMsg != "" {
				s.QQBot.API.ReplyPrivateMessage(m.ID, m.GuildID, rMsg)
			}
		}()
		return "", nil
	})

	rMsg := dic.Run(msg)
	// 替换‘\r’换行
	rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")

	// fmt.Println("QQBot回复:", rMsg)

	if img := dic.Val.P.GetStr("发送图片"); img != "" {
		_, mErr := s.QQBot.API.ReplyChannelPrivateMessage(m.ID, m.GuildID, img, rMsg)
		if mErr != nil {
			fmt.Println("QQBot回复图文失败", mErr)
		}
		return
	}
	if rMsg != "" {
		_, mErr := s.QQBot.API.ReplyPrivateMessage(m.ID, m.GuildID, rMsg)
		if mErr != nil {
			fmt.Println("QQBot回复失败", mErr)
		}
	}
}

// 群消息处理
func (s *ServeRouter) QQBOTGroupATRun(payload *qqbottool.Payload) {
	// 解析消息数据
	m := &qqbottool.GroupMessageEvent{}
	err := json.Unmarshal([]byte(payload.Data), m)
	if err != nil {
		fmt.Println("QQBot消息数据验证失败")
		return
	}

	// 处理重复消息ID
	if !s.QQBot.CheckOnce(m.ID) {
		// fmt.Println("QQBot消息ID重复", m.ID)
		return
	}

	// 处理消息次数
	s.QQBot.MsgCount++

	// 新建副本消息
	msg := m.Content

	// 词库
	BotDic := utils.NewFileQueue(s.QQBot.FilePath)
	FileData, err := BotDic.ReadFromFile()
	if err != nil {
		utils.Error("读取机器人词库出错")
		return
	}

	// 去除开头第一个空格
	msg = qqbottool.RemoveLeadingSpace(msg)

	// 回复消息
	dic := NewDic(s.QQBot.FilePath, FileData).
		// 变量
		SetGlobal_v(dto.NewVal().
			Set("来源", "群私聊").
			Set("昵称", "未知").
			Set("群号", m.GroupOpenID).
			Set("QQ", m.Author.ID).
			Set("头像", "http://q.qlogo.cn/qqapp/"+s.QQBot.API.AppId+"/"+m.Author.ID+"/640"))
	dic.SetFunc("调用", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("2..") {
			return "参数错误", nil
		}
		go func() {
			qqVal := dic.NewDicVal()
			// inputs.IntOk()
			// 读取参数1休眠
			sleepTime := inputs.Int(1)
			time.Sleep(time.Duration(sleepTime) * time.Millisecond)

			// 调用参数2
			rMsg := dic.RunPrivateVal(inputs.StringAfter(2), qqVal)
			// 替换‘\r’换行
			rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")

			// fmt.Println("QQBot回复:", rMsg)
			if img := qqVal.P.GetStr("发送图片"); img != "" {
				if rMsg != "" {
					rMsg = "\n" + rMsg
				}
				_, mErr := s.QQBot.API.ReplyGroupImgMessage(m.ID, m.GroupOpenID, img, rMsg)
				if mErr != nil {
					fmt.Println("QQBot回复图文失败", mErr)
				}
				return
			}

			if rMsg != "" {
				// 开头附带\n
				rMsg = "\n" + rMsg
				_, mErr := s.QQBot.API.ReplyGroupMessage(m.ID, m.GroupOpenID, rMsg)
				if mErr != nil {
					fmt.Println("QQBot回复失败", mErr)
				}
			}
		}()
		return "", nil
	})
	dic.SetFunc("发送文本", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk(1, 2) {
			return "参数错误", nil
		}
		go func() {
			rMsg := strings.ReplaceAll(inputs.String(1), "\\r", "\n")
			if inputs.LenOk(1) {
				if rMsg != "" {
					_, mErr := s.QQBot.API.ReplyGroupMessage(m.ID, m.GroupOpenID, "\n"+rMsg)
					if mErr != nil {
						fmt.Println("QQBot回复失败", mErr)
					}
				}
			} else {
				if rMsg != "" {
					rMsg = "\n" + rMsg
				}
				_, mErr := s.QQBot.API.ReplyGroupImgMessage(m.ID, m.GroupOpenID, inputs.String(2), rMsg)
				if mErr != nil {
					fmt.Println("QQBot回复图文失败", mErr)
				}
			}
		}()
		return "", nil
	})
	dic.SetFunc("发送视频", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk(1) {
			return "参数错误", nil
		}
		go func() {
			_, mErr := s.QQBot.API.ReplyGroupVideoMessage(m.ID, m.GroupOpenID, inputs.String(1))
			if mErr != nil {
				fmt.Println("QQBot回复语音失败", mErr)
			}
		}()
		return "", nil
	})
	dic.SetFunc("发送语音", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk(1) {
			return "参数错误", nil
		}
		go func() {
			_, mErr := s.QQBot.API.ReplyGroupVoiceMessage(m.ID, m.GroupOpenID, inputs.String(1))
			if mErr != nil {
				fmt.Println("QQBot回复语音失败", mErr)
			}
		}()
		return "", nil
	})
	rMsg := dic.Run(msg)
	// 替换‘\r’换行
	rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")

	// fmt.Println("QQBot回复:", rMsg)
	if img := dic.Val.P.GetStr("发送图片"); img != "" {
		if rMsg != "" {
			rMsg = "\n" + rMsg
		}
		_, mErr := s.QQBot.API.ReplyGroupImgMessage(m.ID, m.GroupOpenID, img, rMsg)
		if mErr != nil {
			fmt.Println("QQBot回复图文失败", mErr)
		}
		return
	}

	if rMsg != "" {
		// 开头附带\n
		rMsg = "\n" + rMsg
		_, mErr := s.QQBot.API.ReplyGroupMessage(m.ID, m.GroupOpenID, rMsg)
		if mErr != nil {
			fmt.Println("QQBot回复失败", mErr)
		}
	}
}

// 群私聊处理
func (s *ServeRouter) QQBOTGroupPrivateRun(payload *qqbottool.Payload) {
	// 解析消息数据
	m := &qqbottool.GroupMessageEvent{}
	err := json.Unmarshal([]byte(payload.Data), m)
	if err != nil {
		fmt.Println("QQBot消息数据验证失败")
		return
	}

	// 处理重复消息ID
	if !s.QQBot.CheckOnce(m.ID) {
		// fmt.Println("QQBot消息ID重复", m.ID)
		return
	}

	// 处理消息次数
	s.QQBot.MsgCount++

	// 词库
	BotDic := utils.NewFileQueue(s.QQBot.FilePath)
	FileData, err := BotDic.ReadFromFile()
	if err != nil {
		utils.Error("读取机器人词库出错")
		return
	}

	// 回复消息
	dic := NewDic(s.QQBot.FilePath, FileData).
		// 变量
		SetGlobal_v(dto.NewVal().
			Set("来源", "群私聊").
			Set("昵称", "未知").
			Set("QQ", m.Author.ID).
			Set("头像", "http://q.qlogo.cn/qqapp/"+s.QQBot.API.AppId+"/"+m.Author.ID+"/640"))

	dic.SetFunc("调用", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("2..") {
			return "参数错误", nil
		}
		go func() {
			qqVal := dic.NewDicVal()
			sleepTime := inputs.Int(1)
			time.Sleep(time.Duration(sleepTime) * time.Millisecond)
			rMsg := dic.RunPrivateVal(inputs.StringAfter(2), qqVal)
			rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
			if img := qqVal.P.GetStr("发送图片"); img != "" {
				s.QQBot.API.ReplyGroupPrivateImgMessage(m.ID, m.Author.UserOpenID, img, rMsg)
			} else if rMsg != "" {
				s.QQBot.API.ReplyGroupPrivateMessage(m.ID, m.Author.UserOpenID, rMsg)
			}
		}()
		return "", nil
	})

	dic.SetFunc("发送文本", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk(1, 2) {
			return "参数错误", nil
		}
		go func() {
			rMsg := strings.ReplaceAll(inputs.String(1), "\\r", "\n")
			if inputs.LenOk(1) {
				if rMsg != "" {
					s.QQBot.API.ReplyGroupPrivateMessage(m.ID, m.Author.UserOpenID, rMsg)
				}
			} else {
				s.QQBot.API.ReplyGroupPrivateImgMessage(m.ID, m.Author.UserOpenID, inputs.String(2), rMsg)
			}
		}()
		return "", nil
	})
	dic.SetFunc("发送语音", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk(1) {
			return "参数错误", nil
		}
		go func() {
			s.QQBot.API.ReplyGroupPrivateVoiceMessage(m.ID, m.Author.UserOpenID, inputs.String(1))
		}()
		return "", nil
	})
	dic.SetFunc("发送视频", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk(1) {
			return "参数错误", nil
		}
		go func() {
			s.QQBot.API.ReplyGroupPrivateVideoMessage(m.ID, m.Author.UserOpenID, inputs.String(1))
		}()
		return "", nil
	})

	rMsg := dic.Run(m.Content)
	// 替换‘\r’换行
	rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")

	// fmt.Println("QQBot回复:", rMsg)
	if img := dic.Val.P.GetStr("发送图片"); img != "" {
		_, mErr := s.QQBot.API.ReplyGroupPrivateImgMessage(m.ID, m.Author.UserOpenID, img, rMsg)
		if mErr != nil {
			fmt.Println("QQBot回复图文失败", mErr)
		}
		return
	}
	if rMsg != "" {
		_, mErr := s.QQBot.API.ReplyGroupPrivateMessage(m.ID, m.Author.UserOpenID, rMsg)
		if mErr != nil {
			fmt.Println("QQBot回复失败", mErr)
		}
	}
}

// QQBot处理
func (s *ServeRouter) QQBotRun(w http.ResponseWriter, r *http.Request) {
	httpBody, err := io.ReadAll(r.Body)
	if err != nil {
		w.Write([]byte("Bot Post"))
		return
	}
	payload := &qqbottool.Payload{}
	if err = json.Unmarshal(httpBody, payload); err != nil {
		w.Write([]byte("Bot ErrorData"))
		return
	}
	// fmt.Println("QQBot收到消息")
	// fmt.Println("QQBot消息内容:", string(httpBody))
	// fmt.Println("处理:", qqbottool.Op(payload.Op))
	switch qqbottool.Op(payload.Op) {
	case "开放平台对机器人服务端进行验证":
		// 验证数据
		data := &qqbottool.ValidationRequest{}
		if err = json.Unmarshal(payload.Data, data); err == nil {
			// 效验签名
			if sign, err := qqbottool.GenerateValidationResult(s.QQBot.Secret, data.Event, data.Token); err == nil {
				// 返回效验结果
				w.Write([]byte(sign))
				fmt.Println("QQBot数据验证成功")
				return
			}
			fmt.Println("QQBot数据签名失败")
			return
		}
		fmt.Println("QQBot数据验证失败")
		return
	case "服务端进行消息推送":
		// 处理消息
		switch payload.Type {

		case "MESSAGE_CREATE": // 频道-私域全局消息
			s.QQBOTChannelRun(payload)
			w.Write([]byte("Bot Message"))

		case "AT_MESSAGE_CREATE": // 频道-公域艾特消息
			s.QQBOTChannelRun(payload)
			w.Write([]byte("Bot Message"))

		case "DIRECT_MESSAGE_CREATE": // 频道-私聊消息
			s.QQBOTChannelPrivateRun(payload)
			w.Write([]byte("Bot Message"))

		case "GROUP_AT_MESSAGE_CREATE": // 群-艾特消息
			s.QQBOTGroupATRun(payload)
			w.Write([]byte("Bot Message"))

		case "C2C_MESSAGE_CREATE": // 群-私聊
			s.QQBOTGroupPrivateRun(payload)
			w.Write([]byte("Bot Message"))

		default:
			fmt.Println("QQBot消息类型未支持")
			return
		}

	}
}
