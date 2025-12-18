package dic

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/napcatbottool"
	"github.com/cjxpj/nebula/napcatbottool/napcatbotapi"
	"github.com/cjxpj/nebula/utils"
)

// 加载函数
func (s *ServeRouter) NapCatBOTLoadDicFuncs(dic *Dic) {
	n := s.NapCatBot // 获取机器人实例

	dic.SetFunc("群列表", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("0") {
			return "参数错误", nil
		}
		list, err := n.GetGroupList()
		return string(list), err
	})

	dic.SetFunc("禁言", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("3") {
			return "参数错误", nil
		}
		n.GroupMute(inputs.Int64(1), inputs.Int64(2), inputs.Int64(3))
		return "", nil
	})

	dic.SetFunc("点赞", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("2") {
			return "参数错误", nil
		}
		n.Like(inputs.Int64(1), inputs.Int64(2))
		return "", nil
	})

	dic.SetFunc("戳一戳", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("2") {
			return "参数错误", nil
		}
		n.Nudge(inputs.Int64(1), inputs.Int64(2))
		return "", nil
	})

	dic.SetFunc("撤回", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("1") {
			return "参数错误", nil
		}
		n.Recall(inputs.Int64(1))
		return "", nil
	})

	dic.SetFunc("全体禁言", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("1") {
			return "参数错误", nil
		}
		n.GroupMuteAll(inputs.Int64(1), true)
		return "", nil
	})

	dic.SetFunc("全体解禁", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("1") {
			return "参数错误", nil
		}
		n.GroupMuteAll(inputs.Int64(1), false)
		return "", nil
	})

	dic.SetFunc("设置群头衔", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("3") {
			return "参数错误", nil
		}
		n.SetGroupSpecialTitle(inputs.Int64(1), inputs.Int64(2), inputs.String(3))
		return "", nil
	})

	dic.SetFunc("设置群管理", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("2") {
			return "参数错误", nil
		}
		n.SetGroupAdmin(inputs.Int64(1), inputs.Int64(2), true)
		return "", nil
	})

	dic.SetFunc("取消群管理", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("2") {
			return "参数错误", nil
		}
		n.SetGroupAdmin(inputs.Int64(1), inputs.Int64(2), false)
		return "", nil
	})

	dic.SetFunc("获取群信息", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("1") {
			return "参数错误", nil
		}
		data, err := n.GetGroupInfo(inputs.Int64(1))
		return string(data), err
	})

	dic.SetFunc("获取群成员信息", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("2") {
			return "参数错误", nil
		}
		data, err := n.GetGroupMemberInfo(inputs.Int64(1), inputs.Int64(2))
		return string(data), err
	})

	dic.SetFunc("获取群成员列表", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("1") {
			return "参数错误", nil
		}
		data, err := n.GetGroupMemberList(inputs.Int64(1), false)
		return string(data), err
	})

	dic.SetFunc("踢", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("2") {
			return "参数错误", nil
		}
		n.GroupKick(inputs.Int64(1), inputs.Int64(2), false)
		return "", nil
	})

	dic.SetFunc("设置群名", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("2") {
			return "参数错误", nil
		}
		n.SetGroupName(inputs.Int64(1), inputs.String(2))
		return "", nil
	})

	dic.SetFunc("设置群成员名字", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("3") {
			return "参数错误", nil
		}
		n.SetGroupMemberCard(inputs.Int64(1), inputs.Int64(2), inputs.String(3))
		return "", nil
	})

	dic.SetFunc("获取消息详情", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("1") {
			return "参数错误", nil
		}
		data, err := n.GetMsg(inputs.Int64(1))
		return string(data), err
	})

	dic.SetFunc("获取好友列表", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("0") {
			return "参数错误", nil
		}
		data, err := n.GetFriendList()
		return string(data), err
	})

	dic.SetFunc("群单发", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("2") {
			return "参数错误", nil
		}
		rMsg := inputs.String(2)
		if rMsg != "" {
			rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
			n.SendGroupText(inputs.Int64(1), rMsg)
		}
		return "", nil
	})

	dic.SetFunc("发送视频", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("2") {
			return "参数错误", nil
		}
		n.SendGroupVideo(inputs.Int64(1), inputs.String(2))
		return "", nil
	})

	dic.SetFunc("发送语音", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("2") {
			return "参数错误", nil
		}
		n.SendGroupRecord(inputs.Int64(1), inputs.String(2))
		return "", nil
	})

}

// 群上传文件处理
func (s *ServeRouter) NapCatBOTGroupUploadFileRun(msgData *napcatbottool.MessagePayload) {
	groupID := msgData.GroupID // 群号
	// 群列表
	groupList := utils.NewFileQueue(s.NapCatBot.FilePath + "/groups.txt")
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

	botDicPath := utils.NewFileQueue(s.NapCatBot.FilePath + "/dic")
	botDicList, err := botDicPath.GetFileList()
	if err != nil {
		return
	}

	// 取出需要的数据
	userID := msgData.UserID // QQ

	isAdmin := "null" // 是否是管理员
	// 主人列表
	if adminList, err := utils.NewFileQueue(s.NapCatBot.FilePath + "/admin.txt").ReadFromFile(); err == nil {
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
			FileData, err := utils.NewFileQueue(s.NapCatBot.FilePath + "/dic/" + v).ReadFromFile()
			if err != nil {
				return
			}

			// 回复消息
			dic := NewDic(s.NapCatBot.FilePath, FileData).
				SetGlobal_v(valData)

			dic.SetFunc("调用", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
				if !inputs.LenOk("2..") {
					return "参数错误", nil
				}
				go func() {
					qqVal := dic.NewDicVal()
					sleepTime := inputs.Int(1)
					time.Sleep(time.Duration(sleepTime) * time.Millisecond)
					rMsg := dic.RunPrivateVal(inputs.StringAfter(2), qqVal)
					if rMsg != "" {
						rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
						s.NapCatBot.SendGroupText(groupID, rMsg)
					}
				}()
				return "", nil
			})

			s.NapCatBOTLoadDicFuncs(dic)

			rMsg := dic.RunPrivate("上传文件")
			if rMsg != "" {
				rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
				fmt.Println(rMsg)
				body, err := s.NapCatBot.SendGroupText(groupID, rMsg)
				if err != nil {
					fmt.Println(err)
				}
				fmt.Println(string(body))
			}
		}()
	}
}

// 群撤回消息处理
func (s *ServeRouter) NapCatBOTGroupRecallRun(msgData *napcatbottool.MessagePayload) {
	groupID := msgData.GroupID // 群号
	// 群列表
	groupList := utils.NewFileQueue(s.NapCatBot.FilePath + "/groups.txt")
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

	botDicPath := utils.NewFileQueue(s.NapCatBot.FilePath + "/dic")
	botDicList, err := botDicPath.GetFileList()
	if err != nil {
		return
	}

	// 取出需要的数据
	userID := msgData.UserID // QQ

	isAdmin := "null" // 是否是管理员
	// 主人列表
	if adminList, err := utils.NewFileQueue(s.NapCatBot.FilePath + "/admin.txt").ReadFromFile(); err == nil {
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
			FileData, err := utils.NewFileQueue(s.NapCatBot.FilePath + "/dic/" + v).ReadFromFile()
			if err != nil {
				return
			}

			// 回复消息
			dic := NewDic(s.NapCatBot.FilePath, FileData).
				SetGlobal_v(valData)

			dic.SetFunc("调用", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
				if !inputs.LenOk("2..") {
					return "参数错误", nil
				}
				go func() {
					qqVal := dic.NewDicVal()
					sleepTime := inputs.Int(1)
					time.Sleep(time.Duration(sleepTime) * time.Millisecond)
					rMsg := dic.RunPrivateVal(inputs.StringAfter(2), qqVal)
					if rMsg != "" {
						rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
						s.NapCatBot.SendGroupText(groupID, rMsg)
					}
				}()
				return "", nil
			})

			s.NapCatBOTLoadDicFuncs(dic)

			rMsg := dic.RunPrivate("撤回")
			if rMsg != "" {
				rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
				fmt.Println(rMsg)
				body, err := s.NapCatBot.SendGroupText(groupID, rMsg)
				if err != nil {
					fmt.Println(err)
				}
				fmt.Println(string(body))
			}
		}()
	}
}

// 群戳一戳处理
func (s *ServeRouter) NapCatBOTGroupNudgeRun(msgData *napcatbottool.MessagePayload) {
	groupID := msgData.GroupID // 群号
	// 群列表
	groupList := utils.NewFileQueue(s.NapCatBot.FilePath + "/groups.txt")
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

	botDicPath := utils.NewFileQueue(s.NapCatBot.FilePath + "/dic")
	botDicList, err := botDicPath.GetFileList()
	if err != nil {
		return
	}

	// 取出需要的数据
	userID := msgData.UserID // QQ

	isAdmin := "null" // 是否是管理员
	// 主人列表
	if adminList, err := utils.NewFileQueue(s.NapCatBot.FilePath + "/admin.txt").ReadFromFile(); err == nil {
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
			FileData, err := utils.NewFileQueue(s.NapCatBot.FilePath + "/dic/" + v).ReadFromFile()
			if err != nil {
				return
			}

			// 回复消息
			dic := NewDic(s.NapCatBot.FilePath, FileData).
				SetGlobal_v(valData)

			dic.SetFunc("调用", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
				if !inputs.LenOk("2..") {
					return "参数错误", nil
				}
				go func() {
					qqVal := dic.NewDicVal()
					sleepTime := inputs.Int(1)
					time.Sleep(time.Duration(sleepTime) * time.Millisecond)
					rMsg := dic.RunPrivateVal(inputs.StringAfter(2), qqVal)
					if rMsg != "" {
						rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
						s.NapCatBot.SendGroupText(groupID, rMsg)
					}
				}()
				return "", nil
			})

			s.NapCatBOTLoadDicFuncs(dic)

			rMsg := dic.RunPrivate("戳一戳")
			if rMsg != "" {
				rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
				fmt.Println(rMsg)
				body, err := s.NapCatBot.SendGroupText(groupID, rMsg)
				if err != nil {
					fmt.Println(err)
				}
				fmt.Println(string(body))
			}
		}()
	}
}

// 群消息处理
func (s *ServeRouter) NapCatBOTGroupRun(msgData *napcatbottool.MessagePayload) {
	groupID := msgData.GroupID // 群号
	// 群列表
	groupList := utils.NewFileQueue(s.NapCatBot.FilePath + "/groups.txt")
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

	botDicPath := utils.NewFileQueue(s.NapCatBot.FilePath + "/dic")
	botDicList, err := botDicPath.GetFileList()
	if err != nil {
		return
	}

	// 取出需要的数据
	userID := msgData.Sender.UserID // QQ

	isAdmin := "null" // 是否是管理员
	// 主人列表
	if adminList, err := utils.NewFileQueue(s.NapCatBot.FilePath + "/admin.txt").ReadFromFile(); err == nil {
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
	content = napcatbotapi.ReQQAt.ReplaceAllString(content, "@${1}")
	// 正则替换图片链接
	content = napcatbotapi.ReQQImg.ReplaceAllString(content, "[img=${5}]")
	// 解码
	content = napcatbottool.DeMsg(content)
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
				if userData, err := s.NapCatBot.GetGroupMemberInfo(groupID, qq); err == nil {
					user := &napcatbottool.APIResponse{}
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
			FileData, err := utils.NewFileQueue(s.NapCatBot.FilePath + "/dic/" + v).ReadFromFile()
			if err != nil {
				return
			}

			// 回复消息
			dic := NewDic(s.NapCatBot.FilePath, FileData).
				SetGlobal_v(valData)

			dic.SetFunc("调用", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
				if !inputs.LenOk("2..") {
					return "参数错误", nil
				}
				go func() {
					qqVal := dic.NewDicVal()
					sleepTime := inputs.Int(1)
					time.Sleep(time.Duration(sleepTime) * time.Millisecond)
					rMsg := dic.RunPrivateVal(inputs.StringAfter(2), qqVal)
					if rMsg != "" {
						rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
						s.NapCatBot.SendGroupText(groupID, rMsg)
					}
				}()
				return "", nil
			})

			s.NapCatBOTLoadDicFuncs(dic)

			rMsg := dic.Run(content)
			if rMsg != "" {
				rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
				fmt.Println(rMsg)
				body, err := s.NapCatBot.SendGroupText(groupID, rMsg)
				if err != nil {
					fmt.Println(err)
				}
				fmt.Println(string(body))
			}
		}()
	}
}

func (s *ServeRouter) NapCatBotRun(w http.ResponseWriter, r *http.Request) {
	httpBody, err := io.ReadAll(r.Body)
	if err != nil {
		w.Write([]byte("Bot Post"))
		return
	}

	fmt.Println(string(httpBody))
	payload := &napcatbottool.MessagePayload{}
	if err = json.Unmarshal(httpBody, payload); err != nil {
		w.Write([]byte("Bot ErrorData"))
		return
	}
	if payload.MessageType == "group" {
		s.NapCatBOTGroupRun(payload)
		w.Write([]byte(`{}`))
		return
	}

	// 戳一戳
	if payload.SubType == "poke" {
		s.NapCatBOTGroupNudgeRun(payload)
		w.Write([]byte(`{}`))
		return
	}

	// 群撤回消息
	if payload.NoticeType == "group_recall" {
		s.NapCatBOTGroupRecallRun(payload)
		w.Write([]byte(`{}`))
		return
	}

	// 上传文件
	if payload.NoticeType == "group_upload" {
		s.NapCatBOTGroupUploadFileRun(payload)
		w.Write([]byte(`{}`))
		return
	}

	fmt.Println("Bot消息类型未支持")
}
