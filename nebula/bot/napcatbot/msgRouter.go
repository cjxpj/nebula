package napcatbot

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	napcatbot_dto "github.com/cjxpj/nebula/bot/napcatbot/dto"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// 群上传文件处理
func napCatBOTGroupUploadFileRun(msgData *MessagePayload) {
	groupID := msgData.GroupID // 群号
	// 群列表
	groupList := utils.NewFileQueue(dto.ServerConfig.NapCatBot.FilePath + "/groups.txt")
	FileData, err := groupList.ReadFromFile()
	if err != nil {
		return
	}
	if FileData != "all" {
		found := false
		for s := range strings.SplitSeq(FileData, ",") {
			id, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
			if groupID == id {
				found = true
				break
			}
		}
		if !found {
			return // 没匹配到，直接拦截
		}
	}

	botDicPath := utils.NewFileQueue(dto.ServerConfig.NapCatBot.FilePath + "/dic")
	botDicList, err := botDicPath.GetFileList()
	if err != nil {
		return
	}

	// 取出需要的数据
	userID := msgData.UserID // QQ

	isAdmin := "null" // 是否是管理员
	// 主人列表
	if adminList, err := utils.NewFileQueue(dto.ServerConfig.NapCatBot.FilePath + "/admin.txt").ReadFromFile(); err == nil {
		for s := range strings.SplitSeq(adminList, ",") {
			id, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
			if userID == id {
				isAdmin = s
				break
			}
		}
	}

	// 文件数据
	fileData, _ := json.Marshal(msgData.File)

	valData := dto.NewVal().
		Set("来源", "群上传文件").
		Set("群号", utils.AnyToString(groupID)).
		Set("robot", utils.AnyToString(msgData.SelfID)).
		Set("QQ", utils.AnyToString(userID)).
		Set("文件数据", string(fileData)).
		Set("主人", isAdmin)

	for _, v := range botDicList {
		go func() {
			FileData, err := utils.NewFileQueue(dto.ServerConfig.NapCatBot.FilePath + "/dic/" + v).ReadFromFile()
			if err != nil {
				return
			}

			// 回复消息
			dic := dic_dto.NewDic(dto.ServerConfig.NapCatBot.FilePath, FileData).
				SetGlobal_v(valData)

			dic.AddFuncs(Funcs)

			dic.SetFunc("调用", dto.DicFunc{
				L: "2..",
				Fn: func(d *dto.DicInputs) (any, error) {
					go func() {
						qqVal := dic.NewDicVal()
						sleepTime := d.Inputs.Int(1)
						time.Sleep(time.Duration(sleepTime) * time.Millisecond)
						rMsg := dic_api.Api.DicRunPrivateVal(dic, d.Inputs.StringAfter(2), qqVal)
						if rMsg != "" {
							rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
							SendGroupText(groupID, rMsg)
						}
					}()
					return "", nil
				}})

			rMsg := dic_api.Api.DicRunPrivate(dic, "上传文件")
			if rMsg != "" {
				rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
				debugLog.Infof("%v", rMsg)
				body, err := SendGroupText(groupID, rMsg)
				if err != nil {
					debugLog.Infof("%v", err)
				}
				debugLog.Infof("%v", string(body))
			}
		}()
	}
}

// 群撤回消息处理
func napCatBOTGroupRecallRun(msgData *MessagePayload) {
	groupID := msgData.GroupID // 群号
	// 群列表
	groupList := utils.NewFileQueue(dto.ServerConfig.NapCatBot.FilePath + "/groups.txt")
	FileData, err := groupList.ReadFromFile()
	if err != nil {
		return
	}
	if FileData != "all" {
		found := false
		for s := range strings.SplitSeq(FileData, ",") {
			id, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
			if groupID == id {
				found = true
				break
			}
		}
		if !found {
			return // 没匹配到，直接拦截
		}
	}

	botDicPath := utils.NewFileQueue(dto.ServerConfig.NapCatBot.FilePath + "/dic")
	botDicList, err := botDicPath.GetFileList()
	if err != nil {
		return
	}

	// 取出需要的数据
	userID := msgData.UserID // QQ

	isAdmin := "null" // 是否是管理员
	// 主人列表
	if adminList, err := utils.NewFileQueue(dto.ServerConfig.NapCatBot.FilePath + "/admin.txt").ReadFromFile(); err == nil {
		for s := range strings.SplitSeq(adminList, ",") {
			id, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
			if userID == id {
				isAdmin = s
				break
			}
		}
	}

	msgId := msgData.MessageID

	recallMsg := "获取失败" // 撤回消息
	nick := ""          // 昵称
	groupName := ""     // 群名

	valData := dto.NewVal().
		Set("来源", "群撤回消息").
		Set("群号", utils.AnyToString(groupID)).
		Set("robot", utils.AnyToString(msgData.SelfID)).
		Set("QQ", utils.AnyToString(userID)).
		Set("MsgId", utils.AnyToString(msgId)).
		Set("MessageID", utils.AnyToString(msgId)).
		Set("撤回消息", recallMsg).
		Set("昵称", nick).
		Set("群名", groupName).
		Set("主人", isAdmin)

	for _, v := range botDicList {
		go func() {
			FileData, err := utils.NewFileQueue(dto.ServerConfig.NapCatBot.FilePath + "/dic/" + v).ReadFromFile()
			if err != nil {
				return
			}

			// 回复消息
			dic := dic_dto.NewDic(dto.ServerConfig.NapCatBot.FilePath, FileData).
				SetGlobal_v(valData)

			dic.AddFuncs(Funcs)

			dic.SetFunc("调用", dto.DicFunc{
				L: "2..",
				Fn: func(d *dto.DicInputs) (any, error) {
					go func() {
						qqVal := dic.NewDicVal()
						sleepTime := d.Inputs.Int(1)
						time.Sleep(time.Duration(sleepTime) * time.Millisecond)
						rMsg := dic_api.Api.DicRunPrivateVal(dic, d.Inputs.StringAfter(2), qqVal)
						if rMsg != "" {
							rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
							SendGroupText(groupID, rMsg)
						}
					}()
					return "", nil
				}})

			rMsg := dic_api.Api.DicRunPrivate(dic, "撤回")
			if rMsg != "" {
				rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
				debugLog.Infof("%v", rMsg)
				body, err := SendGroupText(groupID, rMsg)
				if err != nil {
					debugLog.Infof("%v", err)
				}
				debugLog.Infof("%v", string(body))
			}
		}()
	}
}

// 点赞处理
func napCatBOTProfileLikeRun(msgData *MessagePayload) {
	botDicPath := utils.NewFileQueue(dto.ServerConfig.NapCatBot.FilePath + "/dic")
	botDicList, err := botDicPath.GetFileList()
	if err != nil {
		return
	}

	// 取出需要的数据
	userID := msgData.OperatorId // QQ

	isAdmin := "null" // 是否是管理员
	// 主人列表
	if adminList, err := utils.NewFileQueue(dto.ServerConfig.NapCatBot.FilePath + "/admin.txt").ReadFromFile(); err == nil {
		for s := range strings.SplitSeq(adminList, ",") {
			id, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
			if userID == id {
				isAdmin = s
				break
			}
		}
	}

	valData := dto.NewVal().
		Set("来源", "点赞").
		Set("群号", "0").
		Set("robot", utils.AnyToString(msgData.SelfID)).
		Set("QQ", utils.AnyToString(userID)).
		Set("AT0", utils.AnyToString(msgData.TargetID)).
		Set("主人", isAdmin)

	for _, v := range botDicList {
		go func() {
			FileData, err := utils.NewFileQueue(dto.ServerConfig.NapCatBot.FilePath + "/dic/" + v).ReadFromFile()
			if err != nil {
				return
			}

			// 回复消息
			dic := dic_dto.NewDic(dto.ServerConfig.NapCatBot.FilePath, FileData).
				SetGlobal_v(valData)

			dic.SetFunc("调用", dto.DicFunc{
				L: "2..",
				Fn: func(d *dto.DicInputs) (any, error) {
					go func() {
						qqVal := dic.NewDicVal()
						sleepTime := d.Inputs.Int(1)
						time.Sleep(time.Duration(sleepTime) * time.Millisecond)
						rMsg := dic_api.Api.DicRunPrivateVal(dic, d.Inputs.StringAfter(2), qqVal)
						if rMsg != "" {
							rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
							SendPrivateText(userID, rMsg)
						}
					}()
					return "", nil
				}})

			dic.AddFuncs(Funcs)

			rMsg := dic_api.Api.DicRunPrivate(dic, fmt.Sprintf("点赞 %d", msgData.Times))
			if rMsg != "" {
				rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
				body, err := SendPrivateText(userID, rMsg)
				if err != nil {
					fmt.Println(err)
				}
				fmt.Println(string(body))
			}
		}()
	}
}

// 群戳一戳处理
func napCatBOTGroupNudgeRun(msgData *MessagePayload) {
	groupID := msgData.GroupID // 群号
	// 群列表
	groupList := utils.NewFileQueue(dto.ServerConfig.NapCatBot.FilePath + "/groups.txt")
	FileData, err := groupList.ReadFromFile()
	if err != nil {
		return
	}
	if FileData != "all" {
		found := false
		for s := range strings.SplitSeq(FileData, ",") {
			id, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
			if groupID == id {
				found = true
				break
			}
		}
		if !found {
			return // 没匹配到，直接拦截
		}
	}

	botDicPath := utils.NewFileQueue(dto.ServerConfig.NapCatBot.FilePath + "/dic")
	botDicList, err := botDicPath.GetFileList()
	if err != nil {
		return
	}

	// 取出需要的数据
	userID := msgData.UserID // QQ

	isAdmin := "null" // 是否是管理员
	// 主人列表
	if adminList, err := utils.NewFileQueue(dto.ServerConfig.NapCatBot.FilePath + "/admin.txt").ReadFromFile(); err == nil {
		for s := range strings.SplitSeq(adminList, ",") {
			id, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
			if userID == id {
				isAdmin = s
				break
			}
		}
	}

	valData := dto.NewVal().
		Set("来源", "群戳一戳").
		Set("群号", utils.AnyToString(groupID)).
		Set("robot", utils.AnyToString(msgData.SelfID)).
		Set("QQ", utils.AnyToString(userID)).
		Set("AT0", utils.AnyToString(msgData.TargetID)).
		Set("主人", isAdmin)

	for _, v := range botDicList {
		go func() {
			FileData, err := utils.NewFileQueue(dto.ServerConfig.NapCatBot.FilePath + "/dic/" + v).ReadFromFile()
			if err != nil {
				return
			}

			// 回复消息
			dic := dic_dto.NewDic(dto.ServerConfig.NapCatBot.FilePath, FileData).
				SetGlobal_v(valData)

			dic.SetFunc("调用", dto.DicFunc{
				L: "2..",
				Fn: func(d *dto.DicInputs) (any, error) {
					go func() {
						qqVal := dic.NewDicVal()
						sleepTime := d.Inputs.Int(1)
						time.Sleep(time.Duration(sleepTime) * time.Millisecond)
						rMsg := dic_api.Api.DicRunPrivateVal(dic, d.Inputs.StringAfter(2), qqVal)
						if rMsg != "" {
							rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
							SendGroupText(groupID, rMsg)
						}
					}()
					return "", nil
				}})

			dic.AddFuncs(Funcs)

			rMsg := dic_api.Api.DicRunPrivate(dic, "戳一戳")
			if rMsg != "" {
				rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
				debugLog.Infof("%v", rMsg)
				body, err := SendGroupText(groupID, rMsg)
				if err != nil {
					debugLog.Infof("%v", err)
				}
				debugLog.Infof("%v", string(body))
			}
		}()
	}
}

// 私聊消息处理
func napCatBOTPrivateRun(msgData *MessagePayload) {

	botDicPath := utils.NewFileQueue(dto.ServerConfig.NapCatBot.FilePath + "/dic")
	botDicList, err := botDicPath.GetFileList()
	if err != nil {
		return
	}

	// 取出需要的数据
	userID := msgData.Sender.UserID // QQ

	isAdmin := "null" // 是否是管理员
	// 主人列表
	if adminList, err := utils.NewFileQueue(dto.ServerConfig.NapCatBot.FilePath + "/admin.txt").ReadFromFile(); err == nil {
		for s := range strings.SplitSeq(adminList, ",") {
			id, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
			if userID == id {
				isAdmin = s
				break
			}
		}
	}

	nick := msgData.Sender.Card // 昵称
	if nick == "" {
		nick = msgData.Sender.Nickname // 昵称
	}
	content := msgData.RawMessage  // 消息内容
	groupName := msgData.GroupName // 群名
	msgId := msgData.MessageID     // 消息ID

	// 正则替换艾特
	content = napcatbot_dto.ReQQAt.ReplaceAllString(content, "@${1}")
	// 正则替换图片链接
	content = napcatbot_dto.ReQQImg.ReplaceAllString(content, "[img=${5}]")
	// 解码
	content = DeMsg(content)
	// fmt.Println("触发：", content)

	valData := dto.NewVal().
		Set("来源", "私聊").
		Set("群名", groupName).
		Set("昵称", nick).
		Set("群号", "0").
		Set("robot", utils.AnyToString(msgData.SelfID)).
		Set("QQ", utils.AnyToString(userID)).
		Set("MsgId", utils.AnyToString(msgId)).
		Set("MessageID", utils.AnyToString(msgId)).
		Set("主人", isAdmin)

	for _, v := range botDicList {
		go func() {
			FileData, err := utils.NewFileQueue(dto.ServerConfig.NapCatBot.FilePath + "/dic/" + v).ReadFromFile()
			if err != nil {
				return
			}

			// 回复消息
			dic := dic_dto.NewDic(dto.ServerConfig.NapCatBot.FilePath, FileData).
				SetGlobal_v(valData)

			dic.SetFunc("调用", dto.DicFunc{
				L: "2..",
				Fn: func(d *dto.DicInputs) (any, error) {
					go func() {
						qqVal := dic.NewDicVal()
						sleepTime := d.Inputs.Int(1)
						time.Sleep(time.Duration(sleepTime) * time.Millisecond)
						rMsg := dic_api.Api.DicRunPrivateVal(dic, d.Inputs.StringAfter(2), qqVal)
						if rMsg != "" {
							rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
							SendGroupText(userID, rMsg)
						}
					}()
					return "", nil
				}})

			dic.AddFuncs(Funcs)

			rMsg := dic_api.Api.DicRun(dic, fmt.Sprintf("#私聊#%s", content))
			if rMsg != "" {
				rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
				fmt.Println(rMsg)
				body, err := SendPrivateText(userID, rMsg)
				if err != nil {
					fmt.Println(err)
				}
				fmt.Println(string(body))
			}
		}()
	}
}

// 群消息处理
func napCatBOTGroupRun(msgData *MessagePayload) {
	groupID := msgData.GroupID // 群号
	// 群列表
	groupList := utils.NewFileQueue(dto.ServerConfig.NapCatBot.FilePath + "/groups.txt")
	FileData, err := groupList.ReadFromFile()
	if err != nil {
		return
	}
	if FileData != "all" {
		found := false
		for s := range strings.SplitSeq(FileData, ",") {
			id, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
			if groupID == id {
				found = true
				break
			}
		}
		if !found {
			return // 没匹配到，直接拦截
		}
	}

	botDicPath := utils.NewFileQueue(dto.ServerConfig.NapCatBot.FilePath + "/dic")
	botDicList, err := botDicPath.GetFileList()
	if err != nil {
		return
	}

	// 取出需要的数据
	userID := msgData.Sender.UserID // QQ

	isAdmin := "null" // 是否是管理员
	// 主人列表
	if adminList, err := utils.NewFileQueue(dto.ServerConfig.NapCatBot.FilePath + "/admin.txt").ReadFromFile(); err == nil {
		for s := range strings.SplitSeq(adminList, ",") {
			id, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
			if userID == id {
				isAdmin = s
				break
			}
		}
	}

	nick := msgData.Sender.Card // 昵称
	if nick == "" {
		nick = msgData.Sender.Nickname // 昵称
	}
	content := msgData.RawMessage  // 消息内容
	groupName := msgData.GroupName // 群名
	msgId := msgData.MessageID     // 消息ID

	// 正则替换艾特
	content = napcatbot_dto.ReQQAt.ReplaceAllString(content, "@${1}")
	// 正则替换图片链接
	content = napcatbot_dto.ReQQImg.ReplaceAllString(content, "[img=${5}]")
	// 解码
	content = DeMsg(content)
	// fmt.Println("触发：", content)

	valData := dto.NewVal().
		Set("来源", "群聊").
		Set("群名", groupName).
		Set("昵称", nick).
		Set("群号", utils.AnyToString(groupID)).
		Set("robot", utils.AnyToString(msgData.SelfID)).
		Set("QQ", utils.AnyToString(userID)).
		Set("MsgId", utils.AnyToString(msgId)).
		Set("MessageID", utils.AnyToString(msgId)).
		Set("主人", isAdmin)

	qqNum := 0
	for _, elem := range msgData.Message {
		if elem.Type == "at" {
			if qq, ok := elem.Data["qq"]; ok {
				qqStr, _ := qq.(string)
				// string转int64
				valData.Set(fmt.Sprintf("AT%d", qqNum), qqStr)
				qq, _ := strconv.ParseInt(qqStr, 10, 64)
				if userData, err := GetGroupMemberInfo(groupID, qq); err == nil {
					user := &APIResponse{}
					if err := json.Unmarshal(userData, user); err == nil && user.Status == "ok" {
						nick := user.Data.Card
						if nick == "" {
							nick = user.Data.Nickname
						}
						valData.Set(fmt.Sprintf("ATName%d", qqNum), nick)
					}
				}
				qqNum++
			}
		}
	}
	for _, v := range botDicList {
		go func() {
			dicPath := dto.ServerConfig.NapCatBot.FilePath + "/dic/" + v
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
						if rMsg != "" {
							rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
							SendGroupText(groupID, rMsg)
						}
					}()
					return "", nil
				}})

			dic.AddFuncs(Funcs)

			rMsg := dic_api.Api.DicRun(dic, content)
			if rMsg != "" {
				rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
				debugLog.Infof("%v", rMsg)
				body, err := SendGroupText(groupID, rMsg)
				if err != nil {
					debugLog.Infof("%v", err)
				}
				debugLog.Infof("%v", string(body))
			}
		}()
	}
}

func BotMessage(w http.ResponseWriter, r *http.Request) {
	httpBody, err := io.ReadAll(r.Body)
	if err != nil {
		w.Write([]byte("Bot Post"))
		return
	}

	debugLog.Infof("%v", string(httpBody))
	payload := &MessagePayload{}
	if err = json.Unmarshal(httpBody, payload); err != nil {
		w.Write([]byte("Bot ErrorData"))
		return
	}

	// 群消息
	if payload.MessageType == "group" {
		napCatBOTGroupRun(payload)
		w.Write([]byte(`{}`))
		return
	}

	// 私聊消息
	if payload.MessageType == "private" {
		napCatBOTPrivateRun(payload)
		w.Write([]byte(`{}`))
		return
	}

	// 赞机器人
	// {"time":1766120429,"self_id":2807291283,"post_type":"notice","notice_type":"notify","sub_type":"profile_like","operator_id":2960965389,"operator_nick":"Super小啤酒","times":13}
	if payload.SubType == "profile_like" {
		napCatBOTProfileLikeRun(payload)
		w.Write([]byte(`{}`))
		return
	}

	// 戳一戳
	if payload.SubType == "poke" {
		napCatBOTGroupNudgeRun(payload)
		w.Write([]byte(`{}`))
		return
	}

	// 群撤回消息
	if payload.NoticeType == "group_recall" {
		napCatBOTGroupRecallRun(payload)
		w.Write([]byte(`{}`))
		return
	}

	// 上传文件
	if payload.NoticeType == "group_upload" {
		napCatBOTGroupUploadFileRun(payload)
		w.Write([]byte(`{}`))
		return
	}

	debugLog.Infof("Bot消息类型未支持")
}
