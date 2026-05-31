package qqbot

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	qqbot_msg "github.com/cjxpj/nebula/bot/qqbot/msg"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

var mdRe = regexp.MustCompile(`(?s)\[((?:\\.|[^\]\\])+)\]\(((?:\\.|[^)\\])+)\)`)
var mdReAt = regexp.MustCompile(`<(.+?)(\/)?>`)

// 群消息处理
func qqBOTGroupATRun(payload *qqbot_msg.Payload) {
	// 解析消息数据
	m := &qqbot_msg.GroupMessageEvent{}
	err := json.Unmarshal([]byte(payload.Data), m)
	if err != nil {
		fmt.Println("QQBot消息数据验证失败")
		return
	}

	botDicPath := utils.NewFileQueue(dto.ServerConfig.QQBot.FilePath + "/dic")
	botDicList, err := botDicPath.GetFileList()
	if err != nil {
		return
	}

	// 取出需要的数据
	userID := m.Author.ID // QQ

	isAdmin := "null" // 是否是管理员
	// 主人列表
	if adminList, err := utils.NewFileQueue(dto.ServerConfig.QQBot.FilePath + "/admin.txt").ReadFromFile(); err == nil {
		for s := range strings.SplitSeq(adminList, ",") {
			if userID == s {
				isAdmin = s
				break
			}
		}
	}

	// 处理重复消息ID
	if !dto.ServerConfig.QQBot.CheckOnce(m.ID) {
		// fmt.Println("QQBot消息ID重复", m.ID)
		return
	}

	// 处理消息次数
	qqbot_msg.MsgCount++

	// 新建副本消息
	msg := m.Content

	// 去除开头第一个空格
	msg = RemoveLeadingSpace(msg)

	valData := dto.NewVal().
		Set("来源", "群聊").
		Set("昵称", "未知").
		Set("群号", m.GroupOpenID).
		Set("qq", userID).
		Set("QQ", userID).
		Set("主人", isAdmin).
		Set("头像", "http://q.qlogo.cn/qqapp/"+dto.ServerConfig.QQBot.API.AppId+"/"+userID+"/640")

	// 词库
	for _, v := range botDicList {
		go func() {
			dicPath := dto.ServerConfig.QQBot.FilePath + "/dic/" + v
			FileData, err := utils.NewFileQueue(dicPath).ReadFromFile()
			if err != nil {
				return
			}

			// 回复消息
			dic := dic_dto.NewDic(dicPath, FileData).
				SetGlobal_v(valData)

			dic.SetFunc("调用", dto.DicFunc{
				L: "2..",
				Fn: func(d *dto.DicInputs) (any, error) {
					go func() {
						qqVal := dic.NewDicVal()
						// d.Inputs.IntOk()
						// 读取参数1休眠
						sleepTime := d.Inputs.Int(1)
						time.Sleep(time.Duration(sleepTime) * time.Millisecond)

						// 调用参数2
						rMsg := dic_api.Api.DicRunPrivateVal(dic, d.Inputs.StringAfter(2), qqVal)
						// 替换‘\r’换行
						rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")

						// fmt.Println("QQBot回复:", rMsg)
						if rMsg, imgs := stripImgTags(rMsg); len(imgs) != 0 {
							if rMsg != "" {
								rMsg = "\n" + rMsg
							}
							for i, img := range imgs {
								if i == 1 {
									rMsg = ""
								}
								_, mErr := dto.ServerConfig.QQBot.API.ReplyGroupImgMessage(m.ID, m.GroupOpenID, img, rMsg)
								if mErr != nil {
									fmt.Println("QQBot回复图文失败", mErr)
								}
							}
							return
						}

						if rMsg != "" {
							// 开头附带\n
							rMsg = "\n" + rMsg
							_, mErr := dto.ServerConfig.QQBot.API.ReplyGroupMessage(m.ID, m.GroupOpenID, rMsg)
							if mErr != nil {
								fmt.Println("QQBot回复失败", mErr)
							}
						}
					}()
					return "", nil
				}})

			dic.SetFunc("发送MD", dto.DicFunc{
				L: "1..",
				Fn: func(d *dto.DicInputs) (any, error) {

					if d.Inputs.LenOk(1) {
						_, mErr := dto.ServerConfig.QQBot.API.
							ReplyGroupAnyMarkdownMessage(m.ID, m.GroupOpenID, d.Inputs.String(1))
						if mErr != nil {
							fmt.Println("QQBot回复失败", mErr)
						}
						return "", nil
					}

					// 2 开始，必须是 key-value 成对
					if (d.Inputs.Len()-1)%2 != 0 {
						return nil, fmt.Errorf("要设置对应键跟值")
					}

					params := make([]*qqbot_msg.MarkdownParams, 0)

					list := d.Inputs.StringAfterList(2)
					for i := 0; i < len(list); i += 2 {
						key := list[i]
						// 分割换行
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

					_, mErr := dto.ServerConfig.QQBot.API.
						ReplyGroupMarkdownMessage(m.ID, m.GroupOpenID, md)

					if mErr != nil {
						fmt.Println("QQBot回复失败", mErr)
					}

					return "", nil
				},
			})

			dic.SetFunc("发送文本", dto.DicFunc{
				L: "1|2",
				Fn: func(d *dto.DicInputs) (any, error) {
					go func() {
						rMsg := strings.ReplaceAll(d.Inputs.String(1), "\\r", "\n")
						if d.Inputs.LenOk(1) {
							if rMsg != "" {
								_, mErr := dto.ServerConfig.QQBot.API.ReplyGroupMessage(m.ID, m.GroupOpenID, "\n"+rMsg)
								if mErr != nil {
									fmt.Println("QQBot回复失败", mErr)
								}
							}
						} else {
							if rMsg != "" {
								rMsg = "\n" + rMsg
							}
							_, mErr := dto.ServerConfig.QQBot.API.ReplyGroupImgMessage(m.ID, m.GroupOpenID, d.Inputs.String(2), rMsg)
							if mErr != nil {
								fmt.Println("QQBot回复图文失败", mErr)
							}
						}
					}()
					return "", nil
				}})
			dic.SetFunc("发送视频", dto.DicFunc{
				L: "1",
				Fn: func(d *dto.DicInputs) (any, error) {
					go func() {
						_, mErr := dto.ServerConfig.QQBot.API.ReplyGroupVideoMessage(m.ID, m.GroupOpenID, d.Inputs.String(1))
						if mErr != nil {
							fmt.Println("QQBot回复语音失败", mErr)
						}
					}()
					return "", nil
				}})
			dic.SetFunc("发送语音", dto.DicFunc{
				L: "1",
				Fn: func(d *dto.DicInputs) (any, error) {
					go func() {
						_, mErr := dto.ServerConfig.QQBot.API.ReplyGroupVoiceMessage(m.ID, m.GroupOpenID, d.Inputs.String(1))
						if mErr != nil {
							fmt.Println("QQBot回复语音失败", mErr)
						}
					}()
					return "", nil
				}})
			rMsg := dic_api.Api.DicRun(dic, msg)
			// 替换‘\r’换行
			rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")

			// fmt.Println("QQBot回复:", rMsg)

			if rMsg, imgs := stripImgTags(rMsg); len(imgs) != 0 {
				if rMsg != "" {
					rMsg = "\n" + rMsg
				}
				for i, img := range imgs {
					if i == 1 {
						rMsg = ""
					}
					_, mErr := dto.ServerConfig.QQBot.API.ReplyGroupImgMessage(m.ID, m.GroupOpenID, img, rMsg)
					if mErr != nil {
						fmt.Println("QQBot回复图文失败", mErr)
					}
				}
				return
			}

			if rMsg != "" {
				// 开头附带\n
				rMsg = "\n" + rMsg
				_, mErr := dto.ServerConfig.QQBot.API.ReplyGroupMessage(m.ID, m.GroupOpenID, rMsg)
				if mErr != nil {
					fmt.Println("QQBot回复失败", mErr)
				}
			}
		}()
	}
}

// 群私聊处理
func qqBOTGroupPrivateRun(payload *qqbot_msg.Payload) {
	// 解析消息数据
	m := &qqbot_msg.GroupMessageEvent{}
	err := json.Unmarshal([]byte(payload.Data), m)
	if err != nil {
		fmt.Println("QQBot消息数据验证失败")
		return
	}

	botDicPath := utils.NewFileQueue(dto.ServerConfig.QQBot.FilePath + "/dic")
	botDicList, err := botDicPath.GetFileList()
	if err != nil {
		return
	}

	// 取出需要的数据
	userID := m.Author.UserOpenID // QQ

	isAdmin := "null" // 是否是管理员
	// 主人列表
	if adminList, err := utils.NewFileQueue(dto.ServerConfig.QQBot.FilePath + "/admin.txt").ReadFromFile(); err == nil {
		for s := range strings.SplitSeq(adminList, ",") {
			if userID == s {
				isAdmin = s
				break
			}
		}
	}

	// 处理重复消息ID
	if !dto.ServerConfig.QQBot.CheckOnce(m.ID) {
		// fmt.Println("QQBot消息ID重复", m.ID)
		return
	}

	// 处理消息次数
	qqbot_msg.MsgCount++

	valData := dto.NewVal().
		Set("来源", "群私聊").
		Set("昵称", "未知").
		Set("qq", userID).
		Set("QQ", userID).
		Set("主人", isAdmin).
		Set("头像", "http://q.qlogo.cn/qqapp/"+dto.ServerConfig.QQBot.API.AppId+"/"+userID+"/640")

	// 词库
	for _, v := range botDicList {
		go func() {
			dicPath := dto.ServerConfig.QQBot.FilePath + "/dic/" + v
			FileData, err := utils.NewFileQueue(dicPath).ReadFromFile()
			if err != nil {
				return
			}

			// 回复消息
			dic := dic_dto.NewDic(dicPath, FileData).
				SetGlobal_v(valData)

			dic.SetFunc("调用", dto.DicFunc{
				L: "2..",
				Fn: func(d *dto.DicInputs) (any, error) {
					go func() {
						qqVal := dic.NewDicVal()
						sleepTime := d.Inputs.Int(1)
						time.Sleep(time.Duration(sleepTime) * time.Millisecond)
						rMsg := dic_api.Api.DicRunPrivateVal(dic, d.Inputs.StringAfter(2), qqVal)
						rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")

						if rMsg, imgs := stripImgTags(rMsg); len(imgs) != 0 {
							for i, img := range imgs {
								if i == 1 {
									rMsg = ""
								}
								dto.ServerConfig.QQBot.API.ReplyGroupPrivateImgMessage(m.ID, userID, img, rMsg)
							}
							return
						}

						if rMsg != "" {
							dto.ServerConfig.QQBot.API.ReplyGroupPrivateMessage(m.ID, userID, rMsg)
						}
					}()
					return "", nil
				}})

			dic.SetFunc("发送MD", dto.DicFunc{
				L: "2..",
				Fn: func(d *dto.DicInputs) (any, error) {

					// 2 开始，必须是 key-value 成对
					if (d.Inputs.Len()-1)%2 != 0 {
						return nil, fmt.Errorf("要设置对应键跟值")
					}

					params := make([]*qqbot_msg.MarkdownParams, 0)

					list := d.Inputs.StringAfterList(2)
					for i := 0; i < len(list); i += 2 {
						key := list[i]
						// 分割换行
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

					_, mErr := dto.ServerConfig.QQBot.API.
						ReplyPrivateMarkdownMessage(m.ID, userID, md)

					if mErr != nil {
						fmt.Println("QQBot回复失败", mErr)
					}

					return "", nil
				},
			})

			dic.SetFunc("私聊", dto.DicFunc{
				L: "1|2",
				Fn: func(d *dto.DicInputs) (any, error) {
					go func() {
						rMsg := strings.ReplaceAll(d.Inputs.String(1), "\\r", "\n")
						if d.Inputs.LenOk(1) {
							if rMsg != "" {
								dto.ServerConfig.QQBot.API.ReplyGroupPrivateMessage(m.ID, userID, rMsg)
							}
						} else {
							dto.ServerConfig.QQBot.API.ReplyGroupPrivateImgMessage(m.ID, userID, d.Inputs.String(2), rMsg)
						}
					}()
					return "", nil
				}})
			dic.SetFunc("发送语音", dto.DicFunc{
				L: "1",
				Fn: func(d *dto.DicInputs) (any, error) {
					go func() {
						dto.ServerConfig.QQBot.API.ReplyGroupPrivateVoiceMessage(m.ID, userID, d.Inputs.String(1))
					}()
					return "", nil
				}})
			dic.SetFunc("发送视频", dto.DicFunc{
				L: "1",
				Fn: func(d *dto.DicInputs) (any, error) {
					go func() {
						dto.ServerConfig.QQBot.API.ReplyGroupPrivateVideoMessage(m.ID, userID, d.Inputs.String(1))
					}()
					return "", nil
				}})

			rMsg := dic_api.Api.DicRun(dic, "#私聊#"+m.Content)
			// 替换‘\r’换行
			rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")

			// fmt.Println("QQBot回复:", rMsg)
			if rMsg, imgs := stripImgTags(rMsg); len(imgs) != 0 {
				for i, img := range imgs {
					if i == 1 {
						rMsg = ""
					}
					dto.ServerConfig.QQBot.API.ReplyGroupPrivateImgMessage(m.ID, userID, img, rMsg)
				}
				return
			}
			if rMsg != "" {
				dto.ServerConfig.QQBot.API.ReplyGroupPrivateMessage(m.ID, userID, rMsg)
			}
		}()
	}
}
