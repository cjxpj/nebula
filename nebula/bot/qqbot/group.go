package qqbot

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
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
var groupInviterMap sync.Map // key: botPath+groupOpenID, value: inviter OpenID

// popMDKeyboard 检测"按钮"关键字，其后的参数为文本按钮定义
// "标签" → 点击发送 "/标签"；"标签|数据" → 点击发送 "数据"
// 自定义按钮仅简单文本模式（$发送MD "文本" 按钮 ...$）支持；
// 模板模式（$发送MD 模板ID 键1 值1...$）不支持自定义按钮，仅插入文本，"按钮"按普通键/值处理
func popMDKeyboard(d *dto.DicInputs) (int, *qqbot_msg.Keyboard) {
	l := d.Inputs.Len()
	if l == 0 {
		return 0, nil
	}
	// "按钮"仅在参数2位置（简单文本模式）作为按钮分隔符；
	// 其后解析不出按钮定义时按普通参数处理
	if l >= 2 && d.Inputs.String(2) == "按钮" {
		// "按钮"是最后一个参数时视为分隔符（空键盘），保证 $发送MD "文本" 按钮$ 走简单文本模式
		if l == 2 {
			return 1, nil
		}
		if kb := parseTextButtons(d, 3, l); kb != nil {
			return 1, kb
		}
	}
	return l, nil
}

// parseTextButtons 从参数 start..l 解析文本按钮定义
// 格式: label;key=val;key=val...  尾部 \r 表示该按钮后换行
// 支持 key: type(0/1/2)、data、enter(true/false)
func parseTextButtons(d *dto.DicInputs, start, l int) *qqbot_msg.Keyboard {
	var rows []*qqbot_msg.KeyboardRow
	var curButtons []*qqbot_msg.Button
	for i := start; i <= l; i++ {
		s := d.Inputs.String(i)
		if s == "" {
			continue
		}
		// 用 ; 分割参数
		parts := strings.Split(s, ";")
		label := strings.TrimSpace(parts[0])
		if label == "" {
			continue
		}
		// 检测尾部 \r 用于换行
		newRow := false
		if before, ok := strings.CutSuffix(label, "\\r"); ok {
			label = before
			newRow = true
		}
		// 默认值
		btnType := 2
		btnData := label
		var btnEnter bool
		hasType := false
		hasData := false
		// | 拆分：标签|数据，label 的 # 前缀移到 btnData 上由后续统一处理
		if before, after, ok := strings.Cut(label, "|"); ok {
			label = strings.TrimSpace(before)
			btnData = strings.TrimSpace(after)
			hasData = true
			if strings.HasPrefix(label, "#") {
				btnData = "#" + btnData
				label = strings.TrimPrefix(label, "#")
			}
		}
		// label 最多 10 字符
		if len([]rune(label)) > 10 {
			label = string([]rune(label)[:10])
		}
		// 解析 key=value
		for _, part := range parts[1:] {
			k, v, ok := strings.Cut(part, "=")
			if !ok {
				continue
			}
			k = strings.TrimSpace(k)
			v = strings.TrimSpace(v)
			switch k {
			case "type":
				if t, err := strconv.Atoi(v); err == nil {
					btnType = t
					hasType = true
				}
			case "data":
				btnData = v
				hasData = true
			case "enter":
				btnEnter = v == "true" || v == "1"
			}
		}
		// 未显式指定 type 时，根据 data 前缀自动判断类型
		if !hasType {
			if strings.HasPrefix(btnData, "http://") || strings.HasPrefix(btnData, "https://") {
				btnType = 0 // 链接
			} else if strings.HasPrefix(btnData, "#") {
				btnType = 1 // 回调
			}
		}
		// 根据类型清理 data 和 label
		if btnType == 1 && strings.HasPrefix(btnData, "#") {
			btnData = strings.TrimPrefix(btnData, "#")
			if after, ok := strings.CutPrefix(label, "#"); ok {
				label = after
			}
		} else if btnType == 0 && (strings.HasPrefix(btnData, "http://") || strings.HasPrefix(btnData, "https://")) {
			if !hasData {
				// 用 # 分隔：前面是链接，后面是显示文本
				if idx := strings.Index(btnData, "#"); idx != -1 {
					label = btnData[idx+1:]
					btnData = btnData[:idx]
				} else if u, err := url.Parse(btnData); err == nil && u.Host != "" {
					label = u.Host
				}
				if len([]rune(label)) > 10 {
					label = string([]rune(label)[:10])
				}
			}
		}
		action := &qqbot_msg.ButtonAction{
			Type:       btnType,
			Permission: &qqbot_msg.ButtonPermission{Type: 2},
			Data:       btnData,
		}
		if btnEnter {
			action.Enter = true
		}
		curButtons = append(curButtons, &qqbot_msg.Button{
			ID:         fmt.Sprintf("btn_%d", i-1),
			RenderData: &qqbot_msg.ButtonRenderData{Label: label},
			Action:     action,
		})
		if newRow {
			rows = append(rows, &qqbot_msg.KeyboardRow{Buttons: curButtons})
			curButtons = nil
		}
	}
	if len(curButtons) > 0 {
		rows = append(rows, &qqbot_msg.KeyboardRow{Buttons: curButtons})
	}
	if len(rows) == 0 {
		return nil
	}
	return &qqbot_msg.Keyboard{
		Content: &qqbot_msg.KeyboardContent{
			Rows: rows,
		},
	}
}

// mdFormatVal 格式化 MD 参数值（换行/链接转义等）
func mdFormatVal(val string) string {
	val = strings.ReplaceAll(val, "\n", "\r")
	val = mdRe.ReplaceAllString(val, "[\r\n$1]($2)")
	val = mdReAt.ReplaceAllString(val, "<$1\r\n$2>")
	val = strings.ReplaceAll(val, "```", "'''")
	if strings.HasPrefix(val, "#") {
		val = " " + val
	}
	return val
}

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

	// 过滤开头 /（独立开关，默认启用）
	if bot.FilterSlash {
		msg = RemoveLeadingSlash(msg)
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
		Set("头像", "http://q.qlogo.cn/qqapp/"+appId+"/"+userID+"/640").
		Set("MsgId", m.ID).
		Set("MessageID", m.ID)

	for i, id := range atIDs {
		valData.Set(fmt.Sprintf("AT%d", i), id)
	}
	for i, username := range atUsernames {
		valData.Set(fmt.Sprintf("AtName%d", i), username)
	}

	// 记录用户
	RecordUser(bot, userID, m.Author.Username)

	// 词库
	for _, v := range botDicList {
		if !strings.HasSuffix(v, ".n") {
			continue
		}
		dicPath := filepath.Join(bot.FilePath, "dic", v)
		FileData, err := utils.NewFileQueue(dicPath).ReadFromFile()
		if err != nil {
			continue
		}

		// 回复消息
		dic := dic_dto.NewDic(dicPath, FileData).
			SetGlobal_v(valData)
		dic.Val.P.Set("_词库路径_", dicPath)

		// 设置PushContext供 #引入=QQBot 函数使用
		SetPushContext(dic, &PushContext{
			Bot:         bot,
			MsgID:       m.ID,
			GroupOpenID: m.GroupOpenID,
		})

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
					if strippedMsg, imgs := stripImgTags(rMsg); len(imgs) != 0 {
						for i, img := range imgs {
							if i == 1 {
								strippedMsg = ""
							}
							_, mErr := bot.API.ReplyGroupImgMessage(m.ID, m.GroupOpenID, img, strippedMsg)
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
				pLen, kb := popMDKeyboard(d)

				if pLen == 1 || (pLen-1)%2 != 0 {
					_, mErr := bot.API.
						ReplyGroupAnyMarkdownWithKeyboard(m.ID, m.GroupOpenID, d.Inputs.String(1), kb)
					if mErr != nil {
						fmt.Println("QQBot回复失败", mErr)
					}
					return "", nil
				}

				// 2 开始，必须是 key-value 成对
				params := make([]*qqbot_msg.MarkdownParams, 0)
				for i := 2; i <= pLen; i += 2 {
					key := d.Inputs.String(i)
					val := mdFormatVal(d.Inputs.String(i + 1))
					params = append(params, &qqbot_msg.MarkdownParams{Key: key, Values: strings.Split(val, "\r\n")})
				}

				md := &qqbot_msg.Markdown{
					CustomTemplateId: d.Inputs.String(1),
					Params:           params,
				}

				_, mErr := bot.API.
					ReplyGroupMarkdownWithKeyboard(m.ID, m.GroupOpenID, md, kb)

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

		if strippedMsg, imgs := stripImgTags(rMsg); len(imgs) != 0 {
			for i, img := range imgs {
				if i == 1 {
					strippedMsg = ""
				}
				_, mErr := bot.API.ReplyGroupImgMessage(m.ID, m.GroupOpenID, img, strippedMsg)
				if mErr != nil {
					fmt.Println("QQBot回复图文失败", mErr)
				}
			}
		} else if rMsg != "" {
			_, mErr := bot.API.ReplyGroupMessage(m.ID, m.GroupOpenID, rMsg)
			if mErr != nil {
				debugLog.Infof("QQBot回复失败%v", mErr)
			}
		}
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
		Set("头像", "http://q.qlogo.cn/qqapp/"+appId+"/"+userID+"/640").
		Set("MsgId", m.ID).
		Set("MessageID", m.ID)

	// 词库
	for _, v := range botDicList {
		if !strings.HasSuffix(v, ".n") {
			continue
		}
		dicPath := filepath.Join(bot.FilePath, "dic", v)
		FileData, err := utils.NewFileQueue(dicPath).ReadFromFile()
		if err != nil {
			continue
		}

		// 回复消息
		dic := dic_dto.NewDic(dicPath, FileData).
			SetGlobal_v(valData)
		dic.Val.P.Set("_词库路径_", dicPath)

		// 设置PushContext供 #引入=QQBot 函数使用
		SetPushContext(dic, &PushContext{
			Bot:         bot,
			MsgID:       m.ID,
			GroupOpenID: m.GroupOpenID,
		})

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
					if strippedMsg, imgs := stripImgTags(rMsg); len(imgs) != 0 {
						if strippedMsg != "" {
							strippedMsg = "\n" + strippedMsg
						}
						for i, img := range imgs {
							if i == 1 {
								strippedMsg = ""
							}
							_, mErr := bot.API.ReplyGroupImgMessage(m.ID, m.GroupOpenID, img, strippedMsg)
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
				pLen, kb := popMDKeyboard(d)

				if pLen == 1 || (pLen-1)%2 != 0 {
					_, mErr := bot.API.
						ReplyGroupAnyMarkdownWithKeyboard(m.ID, m.GroupOpenID, d.Inputs.String(1), kb)
					if mErr != nil {
						fmt.Println("QQBot回复失败", mErr)
					}
					return "", nil
				}

				// 2 开始，必须是 key-value 成对
				params := make([]*qqbot_msg.MarkdownParams, 0)
				for i := 2; i <= pLen; i += 2 {
					key := d.Inputs.String(i)
					val := mdFormatVal(d.Inputs.String(i + 1))
					params = append(params, &qqbot_msg.MarkdownParams{Key: key, Values: strings.Split(val, "\r\n")})
				}

				md := &qqbot_msg.Markdown{
					CustomTemplateId: d.Inputs.String(1),
					Params:           params,
				}

				_, mErr := bot.API.
					ReplyGroupMarkdownWithKeyboard(m.ID, m.GroupOpenID, md, kb)

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

		if strippedMsg, imgs := stripImgTags(rMsg); len(imgs) != 0 {
			if strippedMsg != "" {
				strippedMsg = "\n" + strippedMsg
			}
			for i, img := range imgs {
				if i == 1 {
					strippedMsg = ""
				}
				_, mErr := bot.API.ReplyGroupImgMessage(m.ID, m.GroupOpenID, img, strippedMsg)
				if mErr != nil {
					fmt.Println("QQBot回复图文失败", mErr)
				}
			}
		} else if rMsg != "" {
			// 开头附带\n
			rMsg = "\n" + rMsg
			_, mErr := bot.API.ReplyGroupMessage(m.ID, m.GroupOpenID, rMsg)
			if mErr != nil {
				debugLog.Infof("QQBot回复失败%v", mErr)
			}
		}
	}
}

// 群事件处理（入群/退群/入群申请）
// qqBOTGroupEventRun 群事件分发入口
func qqBOTGroupEventRun(payload *qqbot_msg.Payload, bot *qqbot_msg.RouterQQBot) {
	qqbot_msg.MsgCount++

	appId := ""
	if bot.API != nil {
		appId = bot.API.AppId
	}

	valData, msg, groupOpenID, event, ok := parseGroupEvent(payload, appId)
	if !ok {
		return
	}

	msgID := ""
	eventID := ""
	var interactionEvent *qqbot_msg.InteractionEvent

	// event_id 被动回复支持事件：
	// INTERACTION_CREATE / GROUP_ADD_ROBOT / GROUP_MEMBER_ADD。
	// 群成员退群（GROUP_MEMBER_REMOVE）、入群申请（GROUP_JOIN_REQUEST）为主动推送事件，
	// 不支持 event_id 被动回复（报 40034027），走主动消息。
	switch payload.Type {
	case "INTERACTION_CREATE":
		interactionEvent = &qqbot_msg.InteractionEvent{}
		if err := json.Unmarshal([]byte(payload.Data), interactionEvent); err == nil {
			// 交互事件无用户消息可回复，被动消息使用 event_id 而非 msg_id
			// event_id 是 WS 帧的事件 ID（payload.Id），不是 d 中的 interaction id
			eventID = payload.Id
		} else {
			interactionEvent = nil
		}
	case "GROUP_ADD_ROBOT", "GROUP_MEMBER_ADD":
		eventID = payload.Id
	}

	inviterKey := bot.FilePath + "|" + groupOpenID
	privateUserID := ""
	switch payload.Type {
	case "GROUP_ADD_ROBOT":
		// 机器人进群：在群里面发言
		inviter := valData.GetStr("操作者")
		groupInviterMap.Store(inviterKey, inviter)
	case "GROUP_DEL_ROBOT":
		privateUserID = valData.GetStr("操作者")
		groupInviterMap.Delete(inviterKey)
	case "INTERACTION_CREATE":
		// 单聊场景（自定义菜单/消息按钮）无群上下文，回复走私信
		if interactionEvent != nil && interactionEvent.Scene == "c2c" {
			privateUserID = interactionEvent.UserOpenID
		}
	}
	ctx := &PushContext{
		Bot:           bot,
		MsgID:         msgID,
		EventID:       eventID,
		GroupOpenID:   groupOpenID,
		PrivateUserID: privateUserID,
	}
	runGroupEventDic(bot, ctx, valData, event, msg)

	// RespondInteraction 必须在发送被动回复（带 event_id）之后调用
	// 因为 PUT /interactions/{id} 会消耗 interaction，之后再发带 event_id 的消息会被拒绝
	if interactionEvent != nil {
		code := qqbot_msg.InteractionCodeSuccess
		code = qqbot_msg.InteractionResponseCode(ctx.InteractionCode)
		bot.API.RespondInteraction(interactionEvent, code)
	}
}

// parseGroupEvent 解析群事件 payload，返回 valData、触发词、群号、特殊触发类别
func parseGroupEvent(payload *qqbot_msg.Payload, appId string) (*dto.Val, string, string, string, bool) {
	switch payload.Type {
	case "GROUP_MEMBER_ADD", "GROUP_MEMBER_REMOVE",
		"GROUP_ADD_ROBOT", "GROUP_DEL_ROBOT":
		m := &qqbot_msg.GroupMemberEvent{}
		if err := json.Unmarshal([]byte(payload.Data), m); err != nil {
			debugLog.Infof("QQBot群事件数据验证失败")
			return nil, "", "", "", false
		}

		source := "群成员退出"
		msg := "群成员退群"
		eventType := "成员"
		switch payload.Type {
		case "GROUP_ADD_ROBOT":
			source = "机器人进群"
			msg = "机器人进群"
			eventType = "机器人"
		case "GROUP_DEL_ROBOT":
			source = "机器人退群"
			msg = "机器人退群"
			eventType = "机器人"
		case "GROUP_MEMBER_ADD":
			source = "群成员加入"
			msg = "群成员进群"
		}

		memberID := m.MemberOpenID
		operatorID := m.OpMemberOpenID
		if memberID == "" {
			memberID = m.OpMemberOpenID
		}

		// 成员事件（ADD/REMOVE）用 member_openid；机器人事件用操作者 op_member_openid
		// 优先级：user_openid → member_openid → op_member_openid
		userQQ := m.UserOpenID
		if userQQ == "" {
			userQQ = m.MemberOpenID
		}
		if userQQ == "" {
			userQQ = m.OpMemberOpenID
		}

		return dto.NewVal().
			Set("来源", source).
			Set("事件类型", eventType).
			Set("群号", m.GroupOpenID).
			Set("成员", memberID).
			Set("操作者", operatorID).
			Set("QQ", userQQ).
			Set("qq", userQQ).
			Set("robot", appId).
			Set("Robot", appId), msg, m.GroupOpenID, "群事件", true

	case "GROUP_JOIN_REQUEST":
		m := &qqbot_msg.JoinRequestEvent{}
		if err := json.Unmarshal([]byte(payload.Data), m); err != nil {
			debugLog.Infof("QQBot入群申请数据验证失败")
			return nil, "", "", "", false
		}
		// 申请理由：验证消息模式取 verify_message 内容
		reason := m.VerifyInfo.VerifyMessage
		// 申请问题：问答模式拼接 问题:答案
		var qas []string
		if m.VerifyInfo.Method == "admin_review_qa" {
			for _, qa := range m.VerifyInfo.ReviewQAList {
				qas = append(qas, qa.Question+":"+qa.Answer)
			}
		}
		qaMsg := strings.Join(qas, ";")
		return dto.NewVal().
			Set("来源", "入群申请").
			Set("群号", m.GroupOpenID).
			Set("QQ", m.MemberOpenID).
			Set("qq", m.MemberOpenID).
			Set("昵称", m.Username).
			Set("申请理由", reason).
			Set("申请问题", qaMsg).
			Set("申请时间", m.ApplyAt).
			Set("申请来源", m.ApplySource).
			Set("申请ID", m.JoinRequestID).
			Set("robot", appId).
			Set("Robot", appId), "入群申请", m.GroupOpenID, "群事件", true

	case "INTERACTION_CREATE":
		m := &qqbot_msg.InteractionEvent{}
		if err := json.Unmarshal([]byte(payload.Data), m); err != nil || m.Data == nil || m.Data.Resolved == nil {
			return nil, "", "", "", false
		}

		interactionType := m.Type
		if interactionType == 0 {
			interactionType = m.Data.Type
		}

		groupOpenID := m.GroupOpenID
		userID := m.UserOpenID
		if userID == "" {
			userID = m.GroupMemberOpenID // 群聊场景没有 user_openid，用 member_openid
		}

		// 快捷菜单回调 (type=12)：以功能 ID (feature_id) 作为触发词
		if interactionType == 12 {
			featureID := m.Data.Resolved.FeatureID
			if featureID == "" {
				return nil, "", "", "", false
			}
			return dto.NewVal().
				Set("来源", "快捷菜单").
				Set("群号", groupOpenID).
				Set("QQ", userID).
				Set("qq", userID).
				Set("功能ID", featureID).
				Set("robot", appId).
				Set("Robot", appId), featureID, groupOpenID, "菜单事件", true
		}

		// 消息按钮回调 (type=11)：以按钮 data 作为触发词
		if interactionType != 11 {
			return nil, "", "", "", false
		}
		btnData := strings.TrimPrefix(m.Data.Resolved.ButtonData, "/")
		if btnData == "" {
			return nil, "", "", "", false
		}
		return dto.NewVal().
			Set("来源", "按钮").
			Set("群号", groupOpenID).
			Set("QQ", userID).
			Set("qq", userID).
			Set("robot", appId).
			Set("Robot", appId), btnData, groupOpenID, "按钮事件", true
	}
	return nil, "", "", "", false
}

// ============= 好友事件处理 ============

// qqBOTFriendEventRun 好友事件分发入口（添加/删除好友）
func qqBOTFriendEventRun(payload *qqbot_msg.Payload, bot *qqbot_msg.RouterQQBot) {
	qqbot_msg.MsgCount++

	appId := ""
	if bot.API != nil {
		appId = bot.API.AppId
	}

	valData, msg, userOpenID, ok := parseFriendEvent(payload, appId)
	if !ok {
		return
	}

	eventID := payload.Id
	// 好友事件无群聊上下文，以私信形式回复给该用户
	ctx := &PushContext{
		Bot:           bot,
		MsgID:         "",
		EventID:       eventID,
		GroupOpenID:   "",
		PrivateUserID: userOpenID,
	}
	runGroupEventDic(bot, ctx, valData, "", msg)
}

// parseFriendEvent 解析好友事件 payload，返回 valData、触发词、用户 OpenID
func parseFriendEvent(payload *qqbot_msg.Payload, appId string) (*dto.Val, string, string, bool) {
	switch payload.Type {
	case "FRIEND_ADD":
		m := &qqbot_msg.FriendAddEvent{}
		if err := json.Unmarshal([]byte(payload.Data), m); err != nil {
			debugLog.Infof("QQBot好友添加事件数据验证失败")
			return nil, "", "", false
		}
		return dto.NewVal().
			Set("来源", "好友添加").
			Set("QQ", m.OpenID).
			Set("qq", m.OpenID).
			Set("robot", appId).
			Set("Robot", appId), "好友添加", m.OpenID, true

	case "FRIEND_DEL":
		m := &qqbot_msg.FriendDelEvent{}
		if err := json.Unmarshal([]byte(payload.Data), m); err != nil {
			debugLog.Infof("QQBot好友删除事件数据验证失败")
			return nil, "", "", false
		}
		return dto.NewVal().
			Set("来源", "好友删除").
			Set("QQ", m.OpenID).
			Set("qq", m.OpenID).
			Set("robot", appId).
			Set("Robot", appId), "好友删除", m.OpenID, true
	}
	return nil, "", "", false
}

// runGroupEventDic 遍历词库并执行群事件回复
func runGroupEventDic(bot *qqbot_msg.RouterQQBot, ctx *PushContext, valData *dto.Val, event string, msg string) {
	botDicList, err := utils.NewFileQueue(filepath.Join(bot.FilePath, "dic")).GetFileList()
	if err != nil {
		return
	}

	for _, v := range botDicList {
		if !strings.HasSuffix(v, ".n") {
			continue
		}
		FileData, err := utils.NewFileQueue(filepath.Join(bot.FilePath, "dic", v)).ReadFromFile()
		if err != nil {
			continue
		}

		dic := dic_dto.NewDic(filepath.Join(bot.FilePath, "dic", v), FileData).
			SetGlobal_v(valData)
		// 设置PushContext供 #引入=QQBot 函数使用
		SetPushContext(dic, ctx)
		dic.AddFuncs(ReplyFuncs)

		dic.SetFunc("设置状态", dto.DicFunc{
			L: "1",
			Fn: func(d *dto.DicInputs) (any, error) {
				code := d.Inputs.Int(1)
				if pc := GetPushContext(d); pc != nil {
					pc.InteractionCode = code
				}
				return "", nil
			},
		})

		var rMsg string
		if event != "" {
			rMsg = dic_api.Api.DicRunEvent(dic, event, msg)
		} else {
			rMsg = dic_api.Api.DicRunPrivate(dic, msg)
		}
		rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")

		if strippedMsg, imgs := stripImgTags(rMsg); len(imgs) != 0 {
			for i, img := range imgs {
				if i == 1 {
					strippedMsg = ""
				}
				if ctx.PrivateUserID != "" {
					if _, mErr := bot.API.ReplyGroupPrivateImgMessage(ctx.MsgID, ctx.PrivateUserID, img, strippedMsg); mErr != nil {
						fmt.Println("QQBot私信回复图文失败", mErr)
					}
				} else {
					if _, mErr := bot.API.ReplyGroupImgMessage(ctx.MsgID, ctx.GroupOpenID, img, strippedMsg, ctx.EventID); mErr != nil {
						fmt.Println("QQBot回复图文失败", mErr)
					}
				}
				// event_id 只能使用一次，后续图片和词库不能再用
				ctx.EventID = ""
			}
		} else if rMsg != "" {
			if ctx.PrivateUserID != "" {
				if _, mErr := bot.API.ReplyGroupPrivateMessage(ctx.MsgID, ctx.PrivateUserID, rMsg); mErr != nil {
					debugLog.Infof("QQBot私信回复失败%v", mErr)
				}
			} else {
				if _, mErr := bot.API.ReplyGroupMessage(ctx.MsgID, ctx.GroupOpenID, rMsg, ctx.EventID); mErr != nil {
					debugLog.Infof("QQBot回复失败%v", mErr)
				}
			}
			// event_id 只能使用一次，后续词库不能再用
			ctx.EventID = ""
		}
	}
}

// triggerStartupCallback 机器人上线（READY）后触发 [系统]启动
func triggerStartupCallback(bot *qqbot_msg.RouterQQBot) {
	botDicList, err := utils.NewFileQueue(filepath.Join(bot.FilePath, "dic")).GetFileList()
	if err != nil {
		return
	}

	appId := ""
	if bot.API != nil {
		appId = bot.API.AppId
	}

	for _, v := range botDicList {
		if !strings.HasSuffix(v, ".n") {
			continue
		}
		FileData, err := utils.NewFileQueue(filepath.Join(bot.FilePath, "dic", v)).ReadFromFile()
		if err != nil {
			continue
		}

		valData := dto.NewVal().
			Set("来源", "机器人上线").
			Set("robot", appId).
			Set("Robot", appId)

		dic := dic_dto.NewDic(filepath.Join(bot.FilePath, "dic", v), FileData).
			SetGlobal_v(valData)
		// 设置PushContext供 #引入=QQBot 函数使用
		SetPushContext(dic, &PushContext{Bot: bot})
		dic.AddFuncs(ReplyFuncs)

		rMsg := dic_api.Api.DicRunEvent(dic, "系统", "启动")
		rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
		if rMsg != "" {
			fmt.Println(rMsg)
		}
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
		Set("头像", "http://q.qlogo.cn/qqapp/"+appId+"/"+userID+"/640").
		Set("MsgId", m.ID).
		Set("MessageID", m.ID)

	// 词库
	for _, v := range botDicList {
		if !strings.HasSuffix(v, ".n") {
			continue
		}
		dicPath := filepath.Join(bot.FilePath, "dic", v)
		FileData, err := utils.NewFileQueue(dicPath).ReadFromFile()
		if err != nil {
			continue
		}

		// 回复消息
		dic := dic_dto.NewDic(dicPath, FileData).
			SetGlobal_v(valData)
		dic.Val.P.Set("_词库路径_", dicPath)

		// 设置PushContext供 #引入=QQBot 函数使用
		SetPushContext(dic, &PushContext{
			Bot:        bot,
			MsgID:      m.ID,
			UserOpenID: userID,
		})

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

					if strippedMsg, imgs := stripImgTags(rMsg); len(imgs) != 0 {
						for i, img := range imgs {
							if i == 1 {
								strippedMsg = ""
							}
							bot.API.ReplyGroupPrivateImgMessage(m.ID, userID, img, strippedMsg)
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
			L: "1..",
			Fn: func(d *dto.DicInputs) (any, error) {
				pLen, kb := popMDKeyboard(d)

				// 简单MD文本发送（含换行符才是文本，不含则视为模板ID）
				if pLen == 1 || (pLen-1)%2 != 0 {
					_, mErr := bot.API.
						ReplyPrivateAnyMarkdownWithKeyboard(m.ID, userID, d.Inputs.String(1), kb)
					if mErr != nil {
						fmt.Println("QQBot回复失败", mErr)
					}
					return "", nil
				}

				// 2 开始，必须是 key-value 成对
				params := make([]*qqbot_msg.MarkdownParams, 0)
				for i := 2; i <= pLen; i += 2 {
					key := d.Inputs.String(i)
					val := mdFormatVal(d.Inputs.String(i + 1))
					params = append(params, &qqbot_msg.MarkdownParams{Key: key, Values: strings.Split(val, "\r\n")})
				}

				md := &qqbot_msg.Markdown{
					CustomTemplateId: d.Inputs.String(1),
					Params:           params,
				}

				_, mErr := bot.API.
					ReplyPrivateMarkdownWithKeyboard(m.ID, userID, md, kb)

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
		if strippedMsg, imgs := stripImgTags(rMsg); len(imgs) != 0 {
			for i, img := range imgs {
				if i == 1 {
					strippedMsg = ""
				}
				bot.API.ReplyGroupPrivateImgMessage(m.ID, userID, img, strippedMsg)
			}
		} else if rMsg != "" {
			bot.API.ReplyGroupPrivateMessage(m.ID, userID, rMsg)
		}
	}
}
