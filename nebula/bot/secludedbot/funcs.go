package secludedbot

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// Funcs 是 Secluded 插件向词库暴露的函数集合
var Funcs = map[string]dto.DicFunc{
	"群单发": {
		L: "1|2",
		Fn: func(d *dto.DicInputs) (any, error) {
			rMsg := d.Inputs.String(2)
			if rMsg != "" {
				rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
				groupId := d.Inputs.String(1)
				account := getCurrentAccount()
				// debugLog.Infof("[secluded] 群单发: groupId=%s, message=%s, account=%s", groupId, rMsg, account)
				if err := SendTextWithAccount("group", groupId, rMsg, account); err != nil {
					debugLog.Infof("[secluded] 群单发失败: %v", err)
				}
			}
			return "", nil
		},
	},
	"私聊": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			rMsg := d.Inputs.String(2)
			if rMsg != "" {
				rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
				userId := d.Inputs.String(1)
				account := getCurrentAccount()
				// debugLog.Infof("[secluded] 私聊: userId=%s, message=%s, account=%s", userId, rMsg, account)
				if err := SendTextWithAccount("friend", userId, rMsg, account); err != nil {
					debugLog.Infof("[secluded] 私聊失败: %v", err)
				}
			}
			return "", nil
		},
	},
	"SEC打印": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			return "", Log(d.Inputs.String(1), d.Inputs.String(2))
		},
	},
	"发送语音": {
		L: "3",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupId := d.Inputs.String(1)
			pttUrl := d.Inputs.String(2)
			duration := d.Inputs.String(3)
			if groupId == "" || pttUrl == "" {
				return "", nil
			}
			account := getCurrentAccount()
			if account == "" {
				return "", nil
			}
			seq := nextSeq()
			packet := map[string]any{
				"seq": seq,
				"cmd": "SendOicqMsg",
				"rsp": true,
				"data": []any{map[string]string{
					"Account":      account,
					"Group":        "Group",
					"GroupId":      groupId,
					"Ptt":          pttUrl,
					"Time":         duration,
					"Value":        "0",
					"ProgressPush": fmt.Sprintf("%d", time.Now().UnixNano()),
				}},
			}
			// debugLog.Infof("[secluded] 发送语音: groupId=%s, url=%s, duration=%s", groupId, pttUrl, duration)
			sendRaw(packet)
			return "", nil
		},
	},
	"发送视频": {
		L: "4",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupId := d.Inputs.String(1)
			videoUrl := d.Inputs.String(2)
			duration := d.Inputs.String(3)
			coverUrl := d.Inputs.String(4)
			if groupId == "" || videoUrl == "" {
				return "", nil
			}
			account := getCurrentAccount()
			if account == "" {
				return "", nil
			}
			data := []any{map[string]string{
				"Account":      account,
				"Group":        "Group",
				"GroupId":      groupId,
				"Video":        videoUrl,
				"Time":         duration,
				"Name":         "video",
				"ProgressPush": fmt.Sprintf("%d", time.Now().UnixNano()),
			}}
			if coverUrl != "" {
				data = append(data, map[string]string{"Img": coverUrl})
			}
			seq := nextSeq()
			packet := map[string]any{
				"seq":  seq,
				"cmd":  "SendOicqMsg",
				"rsp":  true,
				"data": data,
			}
			// debugLog.Infof("[secluded] 发送视频: groupId=%s, duration=%s, coverUrl=%s", groupId, duration, coverUrl)
			sendRaw(packet)
			return "", nil
		},
	},
	"获取群列表": {
		L: "0",
		Fn: func(d *dto.DicInputs) (any, error) {
			account := getCurrentAccount()
			if account == "" {
				return "", nil
			}
			seq := nextSeq()
			packet := map[string]any{
				"seq": seq,
				"cmd": "SendOicqMsg",
				"rsp": true,
				"data": []any{map[string]string{
					"Account":      account,
					"GroupListGet": "GroupListGet",
					"GroupId":      "0",
				}},
			}
			debugLog.Infof("[secluded] 获取群列表: account=%s", account)
			rsp, err := sendAndWait(packet, seq)
			if err != nil {
				debugLog.Infof("[secluded] 获取群列表失败: %v", err)
				return "", err
			}
			debugLog.Infof("[secluded] 获取群列表成功: %s", string(rsp.Data))
			return string(rsp.Data), nil
		},
	},
	"SEC发包": {
		L: "1|2",
		Fn: func(d *dto.DicInputs) (any, error) {
			jsonStr := d.Inputs.String(1)
			if jsonStr == "" {
				return "", nil
			}
			account := d.Inputs.String(2)
			if account == "" {
				account = getCurrentAccount()
			}
			if account == "" {
				return "", nil
			}

			var data []map[string]string
			if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
				debugLog.Infof("[secluded] 发包 JSON 解析失败: %v", err)
				return "", fmt.Errorf("invalid json")
			}

			seq := nextSeq()
			packet := map[string]any{
				"seq": seq,
				"cmd": "SendOicqMsg",
				"rsp": true,
				"data": func() any {
					result := make([]any, 0, len(data))
					for _, item := range data {
						if _, ok := item["Account"]; !ok {
							item["Account"] = account
						}
						result = append(result, item)
					}
					return result
				}(),
			}

			// debugLog.Infof("[secluded] 自定义发包: %s", jsonStr)
			rsp, err := sendAndWait(packet, seq)
			if err != nil {
				debugLog.Infof("[secluded] 发包失败: %v", err)
				return "", err
			}
			// debugLog.Infof("[secluded] 发包成功: %s", string(rsp.Data))
			return string(rsp.Data), nil
		},
	},
	"撤回": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupId := d.Inputs.String(1)
			msgId := d.Inputs.String(2)
			account := getCurrentAccount()
			if account == "" || groupId == "" || msgId == "" {
				return "", nil
			}
			seq := nextSeq()
			packet := map[string]any{
				"seq": seq,
				"cmd": "SendOicqMsg",
				"rsp": true,
				"data": []any{map[string]string{
					"Account":  account,
					"Group":    "Group",
					"GroupId":  groupId,
					"Withdraw": msgId,
				}},
			}
			rsp, err := sendAndWait(packet, seq)
			if err != nil {
				debugLog.Infof("[secluded] 撤回失败: %v", err)
				return "", err
			}
			return string(rsp.Data), nil
		},
	},
	"禁": {
		L: "2|3",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupId := d.Inputs.String(1)
			uin := d.Inputs.String(2)
			muteTime := d.Inputs.String(3)
			account := getCurrentAccount()
			if account == "" || groupId == "" || muteTime == "" {
				return "", nil
			}

			dataMap := map[string]string{
				"Account":       account,
				"Group":         "Group",
				"GroupId":       groupId,
				"GroupProhibit": "GroupProhibit",
				"Time":          muteTime,
			}
			if uin != "" {
				dataMap["People"] = "People"
				dataMap["Uin"] = uin
			}
			packet := map[string]any{
				"seq":  nextSeq(),
				"cmd":  "SendOicqMsg",
				"rsp":  true,
				"data": []any{dataMap},
			}
			sendRaw(packet)
			return "", nil
		},
	},
	"踢": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupId := d.Inputs.String(1)
			uin := d.Inputs.String(2)
			account := getCurrentAccount()
			if account == "" || groupId == "" || uin == "" {
				return "", nil
			}
			packet := map[string]any{
				"seq": nextSeq(),
				"cmd": "SendOicqMsg",
				"rsp": true,
				"data": []any{map[string]string{
					"Account":         account,
					"Group":           "Group",
					"GroupId":         groupId,
					"GroupMemberExit": "GroupMemberExit",
					"Uin":             uin,
				}},
			}
			sendRaw(packet)
			return "", nil
		},
	},
	"邀请加群": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupId := d.Inputs.String(1)
			uin := d.Inputs.String(2)
			account := getCurrentAccount()
			if account == "" || groupId == "" || uin == "" {
				return "", nil
			}
			packet := map[string]any{
				"seq": nextSeq(),
				"cmd": "SendOicqMsg",
				"rsp": true,
				"data": []any{map[string]string{
					"Account":         account,
					"Group":           "Group",
					"GroupId":         groupId,
					"GroupMemberJoin": "GroupMemberJoin",
					"Uin":             uin,
				}},
			}
			sendRaw(packet)
			return "", nil
		},
	},
	"修改群名": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupId := d.Inputs.String(1)
			groupName := d.Inputs.String(2)
			account := getCurrentAccount()
			if account == "" || groupId == "" || groupName == "" {
				return "", nil
			}
			packet := map[string]any{
				"seq": nextSeq(),
				"cmd": "SendOicqMsg",
				"rsp": true,
				"data": []any{map[string]string{
					"Account":         account,
					"GroupId":         groupId,
					"GroupModifyName": "GroupModifyName",
					"GroupName":       groupName,
				}},
			}
			sendRaw(packet)
			return "", nil
		},
	},
	"管理员变动": {
		L: "3",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupId := d.Inputs.String(1)
			uin := d.Inputs.String(2)
			action := d.Inputs.String(3)
			account := getCurrentAccount()
			if account == "" || groupId == "" || uin == "" || action == "" {
				return "", nil
			}

			dataMap := map[string]string{
				"Account":          account,
				"Group":            "Group",
				"GroupId":          groupId,
				"GroupModifyAdmin": "GroupModifyAdmin",
				"Uin":              uin,
			}
			switch action {
			case "add", "Add", "添加":
				dataMap["Add"] = "Add"
			case "del", "Del", "删除", "取消":
				dataMap["Del"] = "Del"
			default:
				return "", nil
			}
			packet := map[string]any{
				"seq":  nextSeq(),
				"cmd":  "SendOicqMsg",
				"rsp":  true,
				"data": []any{dataMap},
			}
			sendRaw(packet)
			return "", nil
		},
	},
	"拍一拍": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupId := d.Inputs.String(1)
			uin := d.Inputs.String(2)
			account := getCurrentAccount()
			if account == "" || groupId == "" || uin == "" {
				return "", nil
			}
			packet := map[string]any{
				"seq": nextSeq(),
				"cmd": "SendOicqMsg",
				"rsp": true,
				"data": []any{map[string]string{
					"Account":        account,
					"Group":          "Group",
					"GroupId":        groupId,
					"GroupBeatABeat": "GroupBeatABeat",
					"Uin":            uin,
				}},
			}
			sendRaw(packet)
			return "", nil
		},
	},
	"群打卡": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupId := d.Inputs.String(1)
			account := getCurrentAccount()
			if account == "" || groupId == "" {
				return "", nil
			}
			packet := map[string]any{
				"seq": nextSeq(),
				"cmd": "SendOicqMsg",
				"rsp": true,
				"data": []any{map[string]string{
					"Account":      account,
					"GroupId":      groupId,
					"GroupClockin": "GroupClockin",
				}},
			}
			sendRaw(packet)
			return "", nil
		},
	},
	"群通知处理": {
		L: "3|4",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupId := d.Inputs.String(1)
			msgId := d.Inputs.String(2)
			code := d.Inputs.String(3)
			action := d.Inputs.String(4)
			account := getCurrentAccount()
			if account == "" || groupId == "" || msgId == "" || code == "" {
				return "", nil
			}

			dataMap := map[string]string{
				"Account":     account,
				"GroupId":     groupId,
				"GroupNotify": "GroupNotify",
				"MsgId":       msgId,
				"Code":        code,
			}
			switch action {
			case "yes", "Yes", "同意", "通过":
				dataMap["Yes"] = "Yes"
			case "no", "No", "拒绝":
				dataMap["No"] = "No"
				reason := d.Inputs.String(5)
				if reason != "" {
					dataMap["Text"] = reason
				}
			default:
				return "", nil
			}
			packet := map[string]any{
				"seq":  nextSeq(),
				"cmd":  "SendOicqMsg",
				"rsp":  true,
				"data": []any{dataMap},
			}
			sendRaw(packet)
			return "", nil
		},
	},
	"灰色消息": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupId := d.Inputs.String(1)
			text := d.Inputs.String(2)
			account := getCurrentAccount()
			if account == "" || groupId == "" || text == "" {
				return "", nil
			}
			packet := map[string]any{
				"seq": nextSeq(),
				"cmd": "SendOicqMsg",
				"rsp": true,
				"data": []any{map[string]string{
					"Account": account,
					"Group":   "Group",
					"GroupId": groupId,
					"GrayTip": "GrayTip",
					"Text":    text,
				}},
			}
			sendRaw(packet)
			return "", nil
		},
	},
	"添加好友": {
		L: "1|2",
		Fn: func(d *dto.DicInputs) (any, error) {
			uin := d.Inputs.String(1)
			info := d.Inputs.String(2)
			account := getCurrentAccount()
			if account == "" || uin == "" {
				return "", nil
			}
			dataMap := map[string]string{
				"Account":       account,
				"UserAddFriend": "UserAddFriend",
				"Uin":           uin,
			}
			if info != "" {
				dataMap["Info"] = info
			}
			packet := map[string]any{
				"seq":  nextSeq(),
				"cmd":  "SendOicqMsg",
				"rsp":  true,
				"data": []any{dataMap},
			}
			sendRaw(packet)
			return "", nil
		},
	},
	"删除好友": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			uin := d.Inputs.String(1)
			account := getCurrentAccount()
			if account == "" || uin == "" {
				return "", nil
			}
			packet := map[string]any{
				"seq": nextSeq(),
				"cmd": "SendOicqMsg",
				"rsp": true,
				"data": []any{map[string]string{
					"Account":       account,
					"UserDelFriend": "UserDelFriend",
					"Uin":           uin,
				}},
			}
			sendRaw(packet)
			return "", nil
		},
	},
	"设置好友备注": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			uin := d.Inputs.String(1)
			name := d.Inputs.String(2)
			account := getCurrentAccount()
			if account == "" || uin == "" || name == "" {
				return "", nil
			}
			packet := map[string]any{
				"seq": nextSeq(),
				"cmd": "SendOicqMsg",
				"rsp": true,
				"data": []any{map[string]string{
					"Account":         account,
					"FriendSetRemark": "FriendSetRemark",
					"Uin":             uin,
					"Name":            name,
				}},
			}
			sendRaw(packet)
			return "", nil
		},
	},
	"获取好友列表": {
		L: "0",
		Fn: func(d *dto.DicInputs) (any, error) {
			account := getCurrentAccount()
			if account == "" {
				return "", nil
			}
			seq := nextSeq()
			packet := map[string]any{
				"seq": seq,
				"cmd": "SendOicqMsg",
				"rsp": true,
				"data": []any{map[string]string{
					"Account":       account,
					"FriendListGet": "FriendListGet",
				}},
			}
			rsp, err := sendAndWait(packet, seq)
			if err != nil {
				debugLog.Infof("[secluded] 获取好友列表失败: %v", err)
				return "", err
			}
			debugLog.Infof("[secluded] 获取好友列表成功: %s", string(rsp.Data))
			return string(rsp.Data), nil
		},
	},
	"获取群成员列表": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupId := d.Inputs.String(1)
			account := getCurrentAccount()
			if account == "" || groupId == "" {
				return "", nil
			}
			seq := nextSeq()
			packet := map[string]any{
				"seq": seq,
				"cmd": "SendOicqMsg",
				"rsp": true,
				"data": []any{map[string]string{
					"Account":            account,
					"GroupMemberListGet": "GroupMemberListGet",
					"GroupId":            groupId,
				}},
			}
			rsp, err := sendAndWait(packet, seq)
			if err != nil {
				debugLog.Infof("[secluded] 获取群成员列表失败: %v", err)
				return "", err
			}
			return string(rsp.Data), nil
		},
	},
	"获取不活跃列表": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupId := d.Inputs.String(1)
			account := getCurrentAccount()
			if account == "" || groupId == "" {
				return "", nil
			}
			seq := nextSeq()
			packet := map[string]any{
				"seq": seq,
				"cmd": "SendOicqMsg",
				"rsp": true,
				"data": []any{map[string]string{
					"Account":                    account,
					"GroupMemberListGetInactive": "GroupMemberListGetInactive",
					"GroupId":                    groupId,
				}},
			}
			rsp, err := sendAndWait(packet, seq)
			if err != nil {
				debugLog.Infof("[secluded] 获取不活跃列表失败: %v", err)
				return "", err
			}
			return string(rsp.Data), nil
		},
	},
	"获取用户信息": {
		L: "0",
		Fn: func(d *dto.DicInputs) (any, error) {
			account := getCurrentAccount()
			if account == "" {
				return "", nil
			}
			seq := nextSeq()
			packet := map[string]any{
				"seq": seq,
				"cmd": "SendOicqMsg",
				"rsp": true,
				"data": []any{map[string]string{
					"Account":     account,
					"UserInfoGet": "UserInfoGet",
				}},
			}
			rsp, err := sendAndWait(packet, seq)
			if err != nil {
				debugLog.Infof("[secluded] 获取用户信息失败: %v", err)
				return "", err
			}
			return string(rsp.Data), nil
		},
	},
	"设置消息接收模式": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupId := d.Inputs.String(1)
			mode := d.Inputs.String(2)
			account := getCurrentAccount()
			if account == "" || groupId == "" || mode == "" {
				return "", nil
			}
			packet := map[string]any{
				"seq": nextSeq(),
				"cmd": "SendOicqMsg",
				"rsp": true,
				"data": []any{map[string]string{
					"Account":            account,
					"GroupId":            groupId,
					"GroupReceivingMode": "GroupReceivingMode",
					"Value":              mode,
				}},
			}
			sendRaw(packet)
			return "", nil
		},
	},
	"群待办": {
		L: "3",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupId := d.Inputs.String(1)
			msgId := d.Inputs.String(2)
			action := d.Inputs.String(3)
			account := getCurrentAccount()
			if account == "" || groupId == "" || msgId == "" || action == "" {
				return "", nil
			}

			dataMap := map[string]string{
				"Account":    account,
				"Group":      "Group",
				"GroupId":    groupId,
				"GroupTodos": "GroupTodos",
				"MsgId":      msgId,
			}
			switch action {
			case "add", "Add", "添加":
				dataMap["Add"] = "Add"
			case "del", "Del", "删除":
				dataMap["Del"] = "Del"
			default:
				return "", nil
			}
			packet := map[string]any{
				"seq":  nextSeq(),
				"cmd":  "SendOicqMsg",
				"rsp":  true,
				"data": []any{dataMap},
			}
			sendRaw(packet)
			return "", nil
		},
	},
	"Xml卡片": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupId := d.Inputs.String(1)
			xml := d.Inputs.String(2)
			account := getCurrentAccount()
			if account == "" || groupId == "" || xml == "" {
				return "", nil
			}
			packet := map[string]any{
				"seq": nextSeq(),
				"cmd": "SendOicqMsg",
				"rsp": true,
				"data": []any{map[string]string{
					"Account": account,
					"GroupId": groupId,
					"Xml":     xml,
				}},
			}
			sendRaw(packet)
			return "", nil
		},
	},
	"Json卡片": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupId := d.Inputs.String(1)
			jsonCard := d.Inputs.String(2)
			account := getCurrentAccount()
			if account == "" || groupId == "" || jsonCard == "" {
				return "", nil
			}
			packet := map[string]any{
				"seq": nextSeq(),
				"cmd": "SendOicqMsg",
				"rsp": true,
				"data": []any{map[string]string{
					"Account": account,
					"GroupId": groupId,
					"Json":    jsonCard,
				}},
			}
			sendRaw(packet)
			return "", nil
		},
	},
	"空间点赞": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			uin := d.Inputs.String(1)
			msgId := d.Inputs.String(2)
			account := getCurrentAccount()
			if account == "" || uin == "" || msgId == "" {
				return "", nil
			}
			packet := map[string]any{
				"seq": nextSeq(),
				"cmd": "SendOicqMsg",
				"rsp": true,
				"data": []any{map[string]string{
					"Account": account,
					"Qzone":   "Qzone",
					"Like":    "Like",
					"Uin":     uin,
					"MsgId":   msgId,
				}},
			}
			sendRaw(packet)
			return "", nil
		},
	},
	"添加群白名单": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupId := d.Inputs.String(1)
			if groupId == "" || dto.ServerConfig.SecludedBot == nil {
				return "", nil
			}
			path := dto.ServerConfig.SecludedBot.FilePath + "/groups.txt"
			groupFile := utils.NewFileQueue(path)
			content, err := groupFile.ReadFromFile()
			if err != nil {
				return "", nil
			}
			content = strings.TrimSpace(content)
			if content == "all" {
				groupFile.WriteFileByte([]byte(groupId))
				return "添加成功", nil
			}
			groups := strings.Split(content, ",")
			for _, g := range groups {
				if strings.TrimSpace(g) == groupId {
					return "已存在", nil
				}
			}
			groups = append(groups, groupId)
			groupFile.WriteFileByte([]byte(strings.Join(groups, ",")))
			return "添加成功", nil
		},
	},
	"删除群白名单": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupId := d.Inputs.String(1)
			if groupId == "" || dto.ServerConfig.SecludedBot == nil {
				return "", nil
			}
			path := dto.ServerConfig.SecludedBot.FilePath + "/groups.txt"
			groupFile := utils.NewFileQueue(path)
			content, err := groupFile.ReadFromFile()
			if err != nil {
				return "", nil
			}
			content = strings.TrimSpace(content)
			if content == "all" {
				return "当前为全部允许", nil
			}
			groups := strings.Split(content, ",")
			newGroups := slices.DeleteFunc(groups, func(g string) bool {
				return strings.TrimSpace(g) == groupId
			})
			if len(newGroups) == 0 {
				groupFile.WriteFileByte([]byte("all"))
				return "已清空，恢复全部允许", nil
			}
			groupFile.WriteFileByte([]byte(strings.Join(newGroups, ",")))
			return "删除成功", nil
		},
	},
	"全体禁言": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupId := d.Inputs.String(1)
			account := getCurrentAccount()
			if account == "" || groupId == "" {
				return "", nil
			}
			packet := map[string]any{
				"seq": nextSeq(),
				"cmd": "SendOicqMsg",
				"rsp": true,
				"data": []any{map[string]string{
					"Account":       account,
					"All":           "All",
					"Group":         "Group",
					"GroupId":       groupId,
					"GroupProhibit": "GroupProhibit",
					"Open":          "Open",
				}},
			}
			sendRaw(packet)
			return "", nil
		},
	},
	"全体解禁": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupId := d.Inputs.String(1)
			account := getCurrentAccount()
			if account == "" || groupId == "" {
				return "", nil
			}
			packet := map[string]any{
				"seq": nextSeq(),
				"cmd": "SendOicqMsg",
				"rsp": true,
				"data": []any{map[string]string{
					"Account":       account,
					"All":           "All",
					"Group":         "Group",
					"GroupId":       groupId,
					"GroupProhibit": "GroupProhibit",
					"Close":         "Close",
				}},
			}
			sendRaw(packet)
			return "", nil
		},
	},
	"贴表情": {
		L: "3",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupId := d.Inputs.String(1)
			msgId := d.Inputs.String(2)
			emoReply := d.Inputs.String(3)
			account := getCurrentAccount()
			if account == "" || groupId == "" || msgId == "" || emoReply == "" {
				return "", nil
			}
			packet := map[string]any{
				"seq": nextSeq(),
				"cmd": "SendOicqMsg",
				"rsp": true,
				"data": []any{map[string]string{
					"Account":  account,
					"Group":    "Group",
					"GroupId":  groupId,
					"MsgId":    msgId,
					"EmoReply": emoReply,
				}},
			}
			sendRaw(packet)
			return "", nil
		},
	},
	"创建群聊": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupName := d.Inputs.String(1)
			account := getCurrentAccount()
			if account == "" || groupName == "" {
				return "", nil
			}
			seq := nextSeq()
			packet := map[string]any{
				"seq": seq,
				"cmd": "SendOicqMsg",
				"rsp": true,
				"data": []any{map[string]string{
					"Account":     account,
					"GroupCreate": "GroupCreate",
					"GroupName":   groupName,
				}},
			}
			rsp, err := sendAndWait(packet, seq)
			if err != nil {
				debugLog.Infof("[secluded] 创建群聊失败: %v", err)
				return "", err
			}
			return string(rsp.Data), nil
		},
	},
	"点赞": {
		L: "3",
		Fn: func(d *dto.DicInputs) (any, error) {
			uin := d.Inputs.String(1)
			uid := d.Inputs.String(2)
			value := d.Inputs.String(3)
			account := getCurrentAccount()
			if account == "" || uin == "" || uid == "" || value == "" {
				return "", nil
			}
			packet := map[string]any{
				"seq": nextSeq(),
				"cmd": "SendOicqMsg",
				"rsp": true,
				"data": []any{map[string]string{
					"Account":      account,
					"FavoriteCard": "FavoriteCard",
					"Uin":          uin,
					"Uid":          uid,
					"Value":        value,
				}},
			}
			sendRaw(packet)
			return "", nil
		},
	},
	"加群": {
		L: "1|2",
		Fn: func(d *dto.DicInputs) (any, error) {
			groupId := d.Inputs.String(1)
			info := d.Inputs.String(2)
			account := getCurrentAccount()
			if account == "" || groupId == "" {
				return "", nil
			}
			dataMap := map[string]string{
				"Account":       account,
				"UserJoinGroup": "UserJoinGroup",
				"GroupId":       groupId,
			}
			if info != "" {
				dataMap["Info"] = info
			}
			packet := map[string]any{
				"seq":  nextSeq(),
				"cmd":  "SendOicqMsg",
				"rsp":  true,
				"data": []any{dataMap},
			}
			sendRaw(packet)
			return "", nil
		},
	},
}

// getCurrentAccount 获取当前机器人账户
func getCurrentAccount() string {
	// 优先从消息上下文获取（实时可靠）
	if pushContext.current != nil && pushContext.current.Account != "" {
		return pushContext.current.Account
	}
	// 其次从配置获取（上线时保存的）
	if dto.ServerConfig.SecludedBot != nil && dto.ServerConfig.SecludedBot.Account != "" {
		return dto.ServerConfig.SecludedBot.Account
	}
	return ""
}
