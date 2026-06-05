package napcatbot

import (
	"encoding/json"
	"regexp"
	"sync"
)

var reMsgData = regexp.MustCompile(`±(at|img|atMsg)=([^±]+)±|([^±]+)`)

// =============== 聊天记录（合并转发）构建器 ===============

type forwardNode struct {
	Name    string
	Content string
}

var forwardBuilder = struct {
	mu    sync.Mutex
	nodes map[int64]forwardNode
	seq   int64
}{nodes: make(map[int64]forwardNode)}

// MultiMsgPut 将一条消息放入合并转发构建器，返回节点序号
func MultiMsgPut(name, content string) int64 {
	forwardBuilder.mu.Lock()
	defer forwardBuilder.mu.Unlock()

	forwardBuilder.seq++
	id := forwardBuilder.seq
	forwardBuilder.nodes[id] = forwardNode{Name: name, Content: content}
	return id
}

// MultiMsg 发送合并转发消息，返回 resId|fileName
func MultiMsg(groupId int64, timestamp int64, ids []int64) (string, error) {
	forwardBuilder.mu.Lock()
	var messages []map[string]any
	for _, id := range ids {
		if node, ok := forwardBuilder.nodes[id]; ok {
			messages = append(messages, map[string]any{
				"type": "node",
				"data": map[string]any{
					"name":    node.Name,
					"content": node.Content,
					"time":    timestamp,
				},
			})
			delete(forwardBuilder.nodes, id)
		}
	}
	// 清空所有节点，防止 MultiMsgPut 后未调用 MultiMsg 导致的内存泄漏
	forwardBuilder.nodes = make(map[int64]forwardNode)
	forwardBuilder.mu.Unlock()

	if len(messages) == 0 {
		return "", nil
	}

	body := map[string]any{
		"group_id": groupId,
		"messages": messages,
	}

	data, err := postJson("/send_forward_msg", body)
	if err != nil {
		return "", err
	}

	// 解析返回包，截取 Id 和 Name（m_fileName 和 m_resid）
	var resp struct {
		Data struct {
			MFileName string `json:"m_fileName"`
			MResid    string `json:"m_resid"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err == nil {
		if resp.Data.MResid != "" && resp.Data.MFileName != "" {
			return resp.Data.MResid + "|" + resp.Data.MFileName, nil
		}
	}

	return string(data), nil
}

// ParseMixedText 把一段带 ±at=xxx± / ±img=url± 的混合文本
// 解析成 []map[string]any 的切片。
func parseMixedText(input string) []map[string]any {
	// 正则：捕获 ±type=value± 或者普通文本
	var res []map[string]any
	for _, m := range reMsgData.FindAllStringSubmatch(input, -1) {
		switch {
		case m[1] == "at": // 捕获到 at
			res = append(res, map[string]any{
				"type": "at",
				"data": map[string]string{"qq": m[2]},
			})
		case m[1] == "img": // 捕获到 img
			res = append(res, map[string]any{
				"type": "image",
				"data": map[string]string{"file": m[2]},
			})
		case m[1] == "atMsg": // 捕获到 atMsg
			res = append(res, map[string]any{
				"type": "reply",
				"data": map[string]string{"id": m[2]},
			})
		case m[3] != "": // 普通文本
			res = append(res, map[string]any{
				"type": "text",
				"data": map[string]string{"text": m[3]},
			})
		}
	}
	return res
}

// 发送群消息
func SendGroupText(groupId int64, text string) ([]byte, error) {
	url := "/send_group_msg"

	body := map[string]any{
		"group_id": groupId,
		"message":  parseMixedText(text),
	}
	return postJson(url, body)
}

// 发送私聊消息
func SendPrivateText(userId int64, text string) ([]byte, error) {
	url := "/send_private_msg"

	body := map[string]any{
		"user_id": userId,
		"message": parseMixedText(text),
	}
	return postJson(url, body)
}

// 发送音乐卡片
func SendGroupMusic(groupId int64, title, jumpUrl, imageUrl, musicUrl string) ([]byte, error) {
	url := "/send_group_msg"

	body := map[string]any{
		"group_id": groupId,
		"message": []map[string]any{
			{
				"type": "music",
				"data": map[string]string{
					"type":  "custom",
					"title": title,
					"url":   jumpUrl,
					"audio": musicUrl,
					"image": imageUrl,
				},
			},
		},
	}
	return postJson(url, body)
}

/// 发群语音
func SendGroupRecord(groupId int64, file string) ([]byte, error) {
	url := "/send_group_msg"

	body := map[string]any{
		"group_id": groupId,
		"message": []map[string]any{
			{
				"type": "record",
				"data": map[string]string{"file": file},
			},
		},
	}
	return postJson(url, body)
}

// 发送群视频
func SendGroupVideo(groupId int64, file string) ([]byte, error) {
	url := "/send_group_msg"

	body := map[string]any{
		"group_id": groupId,
		"message": []map[string]any{
			{
				"type": "video",
				"data": map[string]string{"file": file},
			},
		},
	}
	return postJson(url, body)
}

// 获取群列表
func GetGroupList() ([]byte, error) {
	url := "/get_group_list"
	return postJson(url, nil)
}

// 禁
func GroupMute(groupId, userId, duration int64) ([]byte, error) {
	url := "/set_group_ban"
	body := map[string]any{
		"group_id": groupId,
		"user_id":  userId,
		"duration": duration,
	}
	return postJson(url, body)
}

// 点赞
func Like(userId, num int64) ([]byte, error) {
	url := "/send_like"
	body := map[string]any{
		"user_id": userId,
		"times":   num,
	}
	return postJson(url, body)
}

// 戳一戳
func Nudge(groupId, userId int64) ([]byte, error) {
	url := "/send_poke"
	body := map[string]any{
		"group_id": groupId,
		"user_id":  userId,
	}
	return postJson(url, body)
}

// 私聊戳一戳
func NudgePrivate(userId int64) ([]byte, error) {
	url := "/send_poke"
	body := map[string]any{
		"user_id": userId,
	}
	return postJson(url, body)
}

// 撤回消息
func Recall(messageId int64) ([]byte, error) {
	url := "/delete_msg"
	body := map[string]any{
		"message_id": messageId,
	}
	return postJson(url, body)
}

// 全体禁言
func GroupMuteAll(groupId int64, mute bool) ([]byte, error) {
	url := "/set_group_whole_ban"
	body := map[string]any{
		"group_id": groupId,
		"enable":   mute,
	}
	return postJson(url, body)
}

// 设置群头衔
func SetGroupSpecialTitle(groupId, userId int64, title string) ([]byte, error) {
	url := "/set_group_special_title"
	body := map[string]any{
		"group_id":      groupId,
		"user_id":       userId,
		"special_title": title,
	}
	return postJson(url, body)
}

// 设置群管理
func SetGroupAdmin(groupId, userId int64, admin bool) ([]byte, error) {
	url := "/set_group_admin"
	body := map[string]any{
		"group_id": groupId,
		"user_id":  userId,
		"enable":   admin,
	}
	return postJson(url, body)
}

// 获取群信息
func GetGroupInfo(groupId int64) ([]byte, error) {
	url := "/get_group_info"
	body := map[string]any{
		"group_id": groupId,
	}
	return postJson(url, body)
}

// 获取群成员信息
func GetGroupMemberInfo(groupId, userId int64) ([]byte, error) {
	url := "/get_group_member_info"
	body := map[string]any{
		"group_id": groupId,
		"user_id":  userId,
	}
	return postJson(url, body)
}

// 获取群成员列表
func GetGroupMemberList(groupId int64, noCache bool) ([]byte, error) {
	url := "/get_group_member_list"
	body := map[string]any{
		"group_id": groupId,
		"no_cache": noCache,
	}
	return postJson(url, body)
}

// 群踢人
func GroupKick(groupId, userId int64, rejectAddRequest bool) ([]byte, error) {
	url := "/set_group_kick"
	body := map[string]any{
		"group_id":           groupId,
		"user_id":            userId,
		"reject_add_request": rejectAddRequest,
	}
	return postJson(url, body)
}

// 设置群名
func SetGroupName(groupId int64, groupName string) ([]byte, error) {
	url := "/set_group_name"
	body := map[string]any{
		"group_id":   groupId,
		"group_name": groupName,
	}
	return postJson(url, body)
}

// 设置群成员名字
func SetGroupMemberCard(groupId, userId int64, card string) ([]byte, error) {
	url := "/set_group_card"
	body := map[string]any{
		"group_id": groupId,
		"user_id":  userId,
		"card":     card,
	}
	return postJson(url, body)
}

// 获取消息内容
func GetMsg(messageId int64) ([]byte, error) {
	url := "/get_msg"
	body := map[string]any{
		"message_id": messageId,
	}
	return postJson(url, body)
}

// 获取好友列表
func GetFriendList() ([]byte, error) {
	url := "/get_friend_list"
	return postJson(url, nil)
}
