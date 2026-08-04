package qqbot

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	qqbot_msg "github.com/cjxpj/nebula/bot/qqbot/msg"
	"github.com/cjxpj/nebula/debugLog"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

var mdRe = regexp.MustCompile(`(?s)\[((?:\\.|[^\]\\])+)\]\(((?:\\.|[^)\\])+)\)`)
var mdReAt = regexp.MustCompile(`<(.+?)(\/)?>`)

// 群消息处理
func qqBOTGroupRun(payload *qqbot_msg.Payload, bot *qqbot_msg.RouterQQBot) {
	// 解析消息数据
	m := &qqbot_msg.GroupMessageEvent{}
	err := json.Unmarshal([]byte(payload.Data), m)
	if err != nil {
		debugLog.Infof("QQBot消息数据验证失败")
		return
	}

	botDicPath := utils.NewFileQueue(filepath.Join(bot.FilePath, "dic"))
	botDicList, err := botDicPath.GetFileList()
	if err != nil {
		return
	}

	// 取出需要的数据
	userID := m.Author.ID // QQ
	username := m.Author.Username
	if username == "" {
		username = "未知"
	}

	isAdmin := "null" // 是否是管理员
	// 主人列表
	if adminList, err := utils.NewFileQueue(filepath.Join(bot.FilePath, "admin.txt")).ReadFromFile(); err == nil {
		for s := range strings.SplitSeq(adminList, ",") {
			if userID == s {
				isAdmin = s
				break
			}
		}
	}

	// 处理重复消息ID
	if !bot.CheckOnce(m.ID) {
		// fmt.Println("QQBot消息ID重复", m.ID)
		return
	}

	// 处理消息次数
	qqbot_msg.MsgCount++

	// 新建副本消息
	msg := m.Content

	// 艾特消息格式转换，将ID替换为用户名
	var atIDs []string
	var atUsernames []string
	msg, atIDs, atUsernames = ConvertATMessageWithMentions(msg, m.Mentions)

	// 全量消息艾特兼容：去除开头 @用户名 并调整 AT 顺序
	if bot.AtCompat {
		msg = RemoveLeadingAtMentions(msg)
		// 反转 AT 列表，使 AT0 对应最后一个 @（实际触发者）
		for i, j := 0, len(atIDs)-1; i < j; i, j = i+1, j-1 {
			atIDs[i], atIDs[j] = atIDs[j], atIDs[i]
			atUsernames[i], atUsernames[j] = atUsernames[j], atUsernames[i]
		}
	}

	appId := ""
	if bot.API != nil {
		appId = bot.API.AppId
	}

	valData := dto.NewVal().
		Set("来源", "群聊").
		Set("昵称", username).
		Set("群号", m.GroupOpenID).
		Set("qq", userID).
		Set("QQ", userID).
		Set("主人", isAdmin).
		Set("管理", func() string {
			switch m.Author.MemberRole {
			case "owner":
				return "1"
			case "admin":
				return "2"
			default:
				return "0"
			}
		}()).
		Set("robot", appId).
		Set("Robot", appId).
		Set("头像", "http://q.qlogo.cn/qqapp/"+appId+"/"+userID+"/640")

	for i, id := range atIDs {
		valData.Set(fmt.Sprintf("AT%d", i), id)
	}
	for i, username := range atUsernames {
		valData.Set(fmt.Sprintf("AtName%d", i), username)
	}

	// 记录群和用户
	RecordGroup(bot, m.GroupOpenID)
	RecordUser(bot, userID, m.Author.Username)

	// 词库
	for _, v := range botDicList {
		go func(v string) {
			dicPath := filepath.Join(bot.FilePath, "dic", v)
			FileData, err := utils.NewFileQueue(dicPath).ReadFromFile()
			if err != nil {
				return
			}

			// 设置PushContext供 #引入=QQBot 函数使用
			SetPushContext(&PushContext{
				Bot:         bot,
				MsgID:       m.ID,
				GroupOpenID: m.GroupOpenID,
			})
			defer ClearPushContext()

			// 回复消息
			dic := dic_dto.NewDic(dicPath, FileData).
				SetGlobal_v(valData)

			dic.AddFuncs(ReplyFuncs)

			dic.SetFunc("IMG", dto.DicFunc{
				L: "0|1",
				Fn: func(d *dto.DicInputs) (any, error) {
					if d.Inputs.Len() == 0 {
						if len(m.Attachments) == 0 {
							return "[]", nil
						}
						data, _ := utils.Marshal(m.Attachments)
						return string(data), nil
					}
					if len(m.Attachments) == 0 {
						return "null", nil
					}
					index := d.Inputs.Int(1)
					if index <= 0 || index > len(m.Attachments) {
						return "null", nil
					}
					return m.Attachments[index-1].URL, nil
				},
			})

			dic.SetFunc("调用", dto.DicFunc{
				L: "2..",
				Fn: func(d *dto.DicInputs) (any, error) {
					go func() {
						qqVal := dic.NewDicVal()
						// 读取参数1休眠
						sleepTime := d.Inputs.Int(1)
						time.Sleep(time.Duration(sleepTime) * time.Millisecond)

						// 调用参数2
						rMsg := dic_api.Api.DicRunPrivateVal(dic, d.Inputs.StringAfter(2), qqVal)
						// 替换'\r'换行
						rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")

						// fmt.Println("QQBot回复:", rMsg)
						if rMsg, imgs := stripImgTags(rMsg); len(imgs) != 0 {
							for i, img := range imgs {
								if i == 1 {
									rMsg = ""
								}
								_, mErr := bot.API.ReplyGroupImgMessage(m.ID, m.GroupOpenID, img, rMsg)
								if mErr != nil {
									debugLog.Infof("QQBot回复图文失败%v", mErr)
								}
							}
							return
						}

						if rMsg != "" {
							_, mErr := bot.API.ReplyGroupMessage(m.ID, m.GroupOpenID, rMsg)
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
						_, mErr := bot.API.
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

					_, mErr := bot.API.
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
								_, mErr := bot.API.ReplyGroupMessage(m.ID, m.GroupOpenID, "\n"+rMsg)
								if mErr != nil {
									fmt.Println("QQBot回复失败", mErr)
								}
							}
						} else {
							_, mErr := bot.API.ReplyGroupImgMessage(m.ID, m.GroupOpenID, d.Inputs.String(2), rMsg)
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
						_, mErr := bot.API.ReplyGroupVideoMessage(m.ID, m.GroupOpenID, d.Inputs.String(1))
						if mErr != nil {
							fmt.Println("QQBot回复视频失败", mErr)
						}
					}()
					return "", nil
				}})
			dic.SetFunc("发送语音", dto.DicFunc{
				L: "1",
				Fn: func(d *dto.DicInputs) (any, error) {
					go func() {
						_, mErr := bot.API.ReplyGroupVoiceMessage(m.ID, m.GroupOpenID, d.Inputs.String(1))
						if mErr != nil {
							fmt.Println("QQBot回复语音失败", mErr)
						}
					}()
					return "", nil
				}})
			rMsg := dic_api.Api.DicRun(dic, msg)
			// 替换'\r'换行
			rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")

			// fmt.Println("QQBot回复:", rMsg)

			if rMsg, imgs := stripImgTags(rMsg); len(imgs) != 0 {
				for i, img := range imgs {
					if i == 1 {
						rMsg = ""
					}
					_, mErr := bot.API.ReplyGroupImgMessage(m.ID, m.GroupOpenID, img, rMsg)
					if mErr != nil {
						fmt.Println("QQBot回复图文失败", mErr)
					}
				}
				return
			}

			if rMsg != "" {
				_, mErr := bot.API.ReplyGroupMessage(m.ID, m.GroupOpenID, rMsg)
				if mErr != nil {
					debugLog.Infof("QQBot回复失败%v", mErr)
				}
			}
		}(v)
	}
}

func qqBOTGroupATRun(payload *qqbot_msg.Payload, bot *qqbot_msg.RouterQQBot) {
	// 解析消息数据
	m := &qqbot_msg.GroupMessageEvent{}
	err := json.Unmarshal([]byte(payload.Data), m)
	if err != nil {
		debugLog.Infof("QQBot消息数据验证失败")
		return
	}

	botDicPath := utils.NewFileQueue(filepath.Join(bot.FilePath, "dic"))
	botDicList, err := botDicPath.GetFileList()
	if err != nil {
		return
	}

	// 取出需要的数据
	userID := m.Author.ID // QQ
	username := m.Author.Username
	if username == "" {
		username = "未知"
	}

	isAdmin := "null" // 是否是管理员
	// 主人列表
	if adminList, err := utils.NewFileQueue(filepath.Join(bot.FilePath, "admin.txt")).ReadFromFile(); err == nil {
		for s := range strings.SplitSeq(adminList, ",") {
			if userID == s {
				isAdmin = s
				break
			}
		}
	}

	// 处理重复消息ID
	if !bot.CheckOnce(m.ID) {
		// fmt.Println("QQBot消息ID重复", m.ID)
		return
	}

	// 处理消息次数
	qqbot_msg.MsgCount++

	// 新建副本消息
	msg := m.Content

	// 去除开头第一个空格
	msg = RemoveLeadingSpace(msg)

	appId := ""
	if bot.API != nil {
		appId = bot.API.AppId
	}

	valData := dto.NewVal().
		Set("来源", "群聊").
		Set("昵称", username).
		Set("群号", m.GroupOpenID).
		Set("qq", userID).
		Set("QQ", userID).
		Set("主人", isAdmin).
		Set("管理", func() string {
			switch m.Author.MemberRole {
			case "owner":
				return "1"
			case "admin":
				return "2"
			default:
				return "0"
			}
		}()).
		Set("robot", appId).
		Set("Robot", appId).
		Set("头像", "http://q.qlogo.cn/qqapp/"+appId+"/"+userID+"/640")

	// 记录可用群
	RecordGroup(bot, m.GroupOpenID)

	// 词库
	for _, v := range botDicList {
		go func(v string) {
			dicPath := filepath.Join(bot.FilePath, "dic", v)
			FileData, err := utils.NewFileQueue(dicPath).ReadFromFile()
			if err != nil {
				return
			}

			// 设置PushContext供 #引入=QQBot 函数使用
			SetPushContext(&PushContext{
				Bot:         bot,
				MsgID:       m.ID,
				GroupOpenID: m.GroupOpenID,
			})
			defer ClearPushContext()

			// 回复消息
			dic := dic_dto.NewDic(dicPath, FileData).
				SetGlobal_v(valData)

			dic.AddFuncs(ReplyFuncs)

			dic.SetFunc("调用", dto.DicFunc{
				L: "2..",
				Fn: func(d *dto.DicInputs) (any, error) {
					go func() {
						qqVal := dic.NewDicVal()
						// 读取参数1休眠
						sleepTime := d.Inputs.Int(1)
						time.Sleep(time.Duration(sleepTime) * time.Millisecond)

						// 调用参数2
						rMsg := dic_api.Api.DicRunPrivateVal(dic, d.Inputs.StringAfter(2), qqVal)
						// 替换'\r'换行
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
								_, mErr := bot.API.ReplyGroupImgMessage(m.ID, m.GroupOpenID, img, rMsg)
								if mErr != nil {
									debugLog.Infof("QQBot回复图文失败%v", mErr)
								}
							}
							return
						}

						if rMsg != "" {
							// 开头附带\n
							rMsg = "\n" + rMsg
							_, mErr := bot.API.ReplyGroupMessage(m.ID, m.GroupOpenID, rMsg)
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
						_, mErr := bot.API.
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

					_, mErr := bot.API.
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
								_, mErr := bot.API.ReplyGroupMessage(m.ID, m.GroupOpenID, "\n"+rMsg)
								if mErr != nil {
									fmt.Println("QQBot回复失败", mErr)
								}
							}
						} else {
							if rMsg != "" {
								rMsg = "\n" + rMsg
							}
							_, mErr := bot.API.ReplyGroupImgMessage(m.ID, m.GroupOpenID, d.Inputs.String(2), rMsg)
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
						_, mErr := bot.API.ReplyGroupVideoMessage(m.ID, m.GroupOpenID, d.Inputs.String(1))
						if mErr != nil {
							fmt.Println("QQBot回复视频失败", mErr)
						}
					}()
					return "", nil
				}})
			dic.SetFunc("发送语音", dto.DicFunc{
				L: "1",
				Fn: func(d *dto.DicInputs) (any, error) {
					go func() {
						_, mErr := bot.API.ReplyGroupVoiceMessage(m.ID, m.GroupOpenID, d.Inputs.String(1))
						if mErr != nil {
							fmt.Println("QQBot回复语音失败", mErr)
						}
					}()
					return "", nil
				}})
			rMsg := dic_api.Api.DicRun(dic, msg)
			// 替换'\r'换行
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
					_, mErr := bot.API.ReplyGroupImgMessage(m.ID, m.GroupOpenID, img, rMsg)
					if mErr != nil {
						fmt.Println("QQBot回复图文失败", mErr)
					}
				}
				return
			}

			if rMsg != "" {
				// 开头附带\n
				rMsg = "\n" + rMsg
				_, mErr := bot.API.ReplyGroupMessage(m.ID, m.GroupOpenID, rMsg)
				if mErr != nil {
					debugLog.Infof("QQBot回复失败%v", mErr)
				}
			}
		}(v)
	}
}

// 群事件处理（入群/退群）
func qqBOTGroupEventRun(payload *qqbot_msg.Payload, bot *qqbot_msg.RouterQQBot) {
	m := &qqbot_msg.GroupMemberEvent{}
	err := json.Unmarshal([]byte(payload.Data), m)
	if err != nil {
		debugLog.Infof("QQBot群事件数据验证失败")
		return
	}

	botDicPath := utils.NewFileQueue(filepath.Join(bot.FilePath, "dic"))
	botDicList, err := botDicPath.GetFileList()
	if err != nil {
		return
	}

	// 处理消息次数
	qqbot_msg.MsgCount++

	// 根据事件类型确定消息内容和来源
	eventType := payload.Type
	var msg string
	var source string
	switch eventType {
	case "GROUP_MEMBER_ADD":
		msg = "群成员加入"
		source = "入群"
	case "GROUP_MEMBER_REMOVE":
		msg = "群成员退出"
		source = "退群"
	default:
		return
	}

	appId := ""
	if bot.API != nil {
		appId = bot.API.AppId
	}

	// 取成员ID：GROUP_MEMBER_ADD/REMOVE 用 member_openid，GROUP_ADD_ROBOT/DEL_ROBOT 用 op_member_openid
	memberID := m.MemberOpenID
	if memberID == "" {
		memberID = m.OpMemberOpenID
	}

	valData := dto.NewVal().
		Set("来源", source).
		Set("群号", m.GroupOpenID).
		Set("成员", memberID).
		Set("用户", m.UserOpenID).
		Set("robot", appId).
		Set("Robot", appId)

	// 记录群
	RecordGroup(bot, m.GroupOpenID)

	// 词库
	for _, v := range botDicList {
		go func(v string) {
			dicPath := filepath.Join(bot.FilePath, "dic", v)
			FileData, err := utils.NewFileQueue(dicPath).ReadFromFile()
			if err != nil {
				return
			}

			SetPushContext(&PushContext{
				Bot:         bot,
				MsgID:       payload.Id,
				GroupOpenID: m.GroupOpenID,
			})
			defer ClearPushContext()

			dic := dic_dto.NewDic(dicPath, FileData).
				SetGlobal_v(valData)

			dic.AddFuncs(ReplyFuncs)

			// 内部事件使用 DicRunPrivate
			rMsg := dic_api.Api.DicRunPrivate(dic, msg)
			rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")

			if rMsg, imgs := stripImgTags(rMsg); len(imgs) != 0 {
				for i, img := range imgs {
					if i == 1 {
						rMsg = ""
					}
					_, mErr := bot.API.ReplyGroupImgMessage(payload.Id, m.GroupOpenID, img, rMsg)
					if mErr != nil {
						debugLog.Infof("QQBot回复图文失败%v", mErr)
					}
				}
				return
			}

			if rMsg != "" {
				_, mErr := bot.API.ReplyGroupMessage(payload.Id, m.GroupOpenID, rMsg)
				if mErr != nil {
					debugLog.Infof("QQBot回复失败%v", mErr)
				}
			}
		}(v)
	}
}

// 群私聊处理
func qqBOTGroupPrivateRun(payload *qqbot_msg.Payload, bot *qqbot_msg.RouterQQBot) {
	// 解析消息数据
	m := &qqbot_msg.GroupMessageEvent{}
	err := json.Unmarshal([]byte(payload.Data), m)
	if err != nil {
		debugLog.Infof("QQBot消息数据验证失败")
		return
	}

	botDicPath := utils.NewFileQueue(filepath.Join(bot.FilePath, "dic"))
	botDicList, err := botDicPath.GetFileList()
	if err != nil {
		return
	}

	// 取出需要的数据
	userID := m.Author.UserOpenID // QQ

	isAdmin := "null" // 是否是管理员
	// 主人列表
	if adminList, err := utils.NewFileQueue(filepath.Join(bot.FilePath, "admin.txt")).ReadFromFile(); err == nil {
		for s := range strings.SplitSeq(adminList, ",") {
			if userID == s {
				isAdmin = s
				break
			}
		}
	}

	// 处理重复消息ID
	if !bot.CheckOnce(m.ID) {
		// fmt.Println("QQBot消息ID重复", m.ID)
		return
	}

	// 处理消息次数
	qqbot_msg.MsgCount++

	appId := ""
	if bot.API != nil {
		appId = bot.API.AppId
	}

	valData := dto.NewVal().
		Set("来源", "群私聊").
		Set("昵称", "未知").
		Set("qq", userID).
		Set("QQ", userID).
		Set("主人", isAdmin).
		Set("robot", appId).
		Set("Robot", appId).
		Set("头像", "http://q.qlogo.cn/qqapp/"+appId+"/"+userID+"/640")

	// 词库
	for _, v := range botDicList {
		go func(v string) {
			dicPath := filepath.Join(bot.FilePath, "dic", v)
			FileData, err := utils.NewFileQueue(dicPath).ReadFromFile()
			if err != nil {
				return
			}

			// 设置PushContext供 #引入=QQBot 函数使用
			SetPushContext(&PushContext{
				Bot:        bot,
				MsgID:      m.ID,
				UserOpenID: userID,
			})
			defer ClearPushContext()

			// 回复消息
			dic := dic_dto.NewDic(dicPath, FileData).
				SetGlobal_v(valData)

			dic.AddFuncs(ReplyFuncs)

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
								bot.API.ReplyGroupPrivateImgMessage(m.ID, userID, img, rMsg)
							}
							return
						}

						if rMsg != "" {
							bot.API.ReplyGroupPrivateMessage(m.ID, userID, rMsg)
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

					_, mErr := bot.API.
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
								bot.API.ReplyGroupPrivateMessage(m.ID, userID, rMsg)
							}
						} else {
							bot.API.ReplyGroupPrivateImgMessage(m.ID, userID, d.Inputs.String(2), rMsg)
						}
					}()
					return "", nil
				}})
			dic.SetFunc("发送语音", dto.DicFunc{
				L: "1",
				Fn: func(d *dto.DicInputs) (any, error) {
					go func() {
						bot.API.ReplyGroupPrivateVoiceMessage(m.ID, userID, d.Inputs.String(1))
					}()
					return "", nil
				}})
			dic.SetFunc("发送视频", dto.DicFunc{
				L: "1",
				Fn: func(d *dto.DicInputs) (any, error) {
					go func() {
						bot.API.ReplyGroupPrivateVideoMessage(m.ID, userID, d.Inputs.String(1))
					}()
					return "", nil
				}})

			rMsg := dic_api.Api.DicRun(dic, "#私聊#"+m.Content)
			// 替换'\r'换行
			rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")

			// fmt.Println("QQBot回复:", rMsg)
			if rMsg, imgs := stripImgTags(rMsg); len(imgs) != 0 {
				for i, img := range imgs {
					if i == 1 {
						rMsg = ""
					}
					bot.API.ReplyGroupPrivateImgMessage(m.ID, userID, img, rMsg)
				}
				return
			}
			if rMsg != "" {
				bot.API.ReplyGroupPrivateMessage(m.ID, userID, rMsg)
			}
		}(v)
	}
}
