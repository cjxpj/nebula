package qqbot

import (
	"fmt"
	"regexp"
	"strings"

	qqbot_msg "github.com/cjxpj/nebula/bot/qqbot/msg"
	"github.com/cjxpj/nebula/utils"
)

// 移除艾特消息开头的 @xxx
func RemoveLeadingMentionOnce(s string) string {
	if !strings.HasPrefix(s, "<@!") {
		return s
	}
	end := strings.IndexByte(s, '>')
	if end <= 3 {
		return s
	}
	for i := 3; i < end; i++ {
		if s[i] < '0' || s[i] > '9' {
			return s
		}
	}
	return strings.TrimLeft(s[end+1:], " \t\n\f\r")
}

// 移除开头指定@账号
func RemoveLeadingMention(s string, qq int) string {
	s = strings.TrimPrefix(s, fmt.Sprintf("<@!%d> ", qq))
	return strings.TrimPrefix(s, fmt.Sprintf("<@!%d>", qq))
}

var reATHex = regexp.MustCompile(`<@([0-9A-Fa-f]+)>`)

// 艾特消息格式转换（十六进制ID格式）
func ConvertATMessageToMD(s string) string {
	return reATHex.ReplaceAllString(s, "@$1")
}

var atMentionRe = regexp.MustCompile(`<@([^>]+)>`)

// ConvertATMessageWithMentions 将消息中的 <@ID> 替换为实际用户名，并返回替换后的消息、ID列表和用户名列表
func ConvertATMessageWithMentions(s string, mentions []qqbot_msg.Mention) (string, []string, []string) {
	var ids []string
	var usernames []string
	if len(mentions) == 0 {
		return s, ids, usernames
	}

	mentionMap := make(map[string]string, len(mentions))
	for _, m := range mentions {
		mentionMap[m.ID] = m.Username
	}

	s = atMentionRe.ReplaceAllStringFunc(s, func(match string) string {
		id := match[2 : len(match)-1] // 去掉 <@ 和 >
		if username, ok := mentionMap[id]; ok {
			ids = append(ids, id)
			usernames = append(usernames, username)
			return "@" + username
		}
		return match
	})
	return s, ids, usernames
}

// 移除开头一个空格
func RemoveLeadingSpace(s string) string {
	return strings.TrimPrefix(s, " ")
}

// RemoveLeadingAtMentions 移除消息开头的 @用户名 前缀，用于全量消息艾特兼容
func RemoveLeadingAtMentions(s string) string {
	const ws = " \t\n\f\r"
	for {
		if !strings.HasPrefix(s, "@") {
			return s
		}
		if len(s) < 2 || strings.IndexByte(ws, s[1]) >= 0 {
			return s
		}
		i := strings.IndexAny(s, ws)
		if i < 0 {
			return ""
		}
		s = strings.TrimLeft(s[i:], ws)
	}
}

// RemoveLeadingSlash 移除消息开头的 / 或 空格+ /，用于过滤斜杠指令前缀
func RemoveLeadingSlash(s string) string {
	s = RemoveLeadingSpace(s)
	return strings.TrimPrefix(s, "/")
}

const (
	tagAtMsg = "±atMsg="
	tagImg   = "±img="
	tagSep   = "±"
)

// stripReplyTags 解析回复文本中的 ±atMsg=消息id± 与 ±img=...± 标记，
// 返回净化后文本、图片列表、引用回复的消息ID（atMsgID 为空表示回复当前消息）
func stripReplyTags(s string) (string, []string, string) {
	var atMsgID string
	for {
		i := strings.Index(s, tagAtMsg)
		if i < 0 {
			break
		}
		j := strings.Index(s[i+len(tagAtMsg):], tagSep)
		if j < 0 {
			break
		}
		if atMsgID == "" {
			atMsgID = s[i+len(tagAtMsg) : i+len(tagAtMsg)+j]
		}
		s = s[:i] + s[i+len(tagAtMsg)+j+len(tagSep):]
	}

	var imgs []string
	for {
		i := strings.Index(s, tagImg)
		if i < 0 {
			break
		}
		j := strings.Index(s[i+len(tagImg):], tagSep)
		if j < 0 {
			break
		}
		src := s[i+len(tagImg) : i+len(tagImg)+j]
		s = s[:i] + s[i+len(tagImg)+j+len(tagSep):]

		var data string
		var err error
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
			data, err = utils.Get(src)
		} else {
			data, err = utils.NewFileQueue(src).ReadFile()
		}
		if err == nil {
			imgs = append(imgs, data)
		}
	}

	return s, imgs, atMsgID
}

// refIdxOf 从群消息场景的 ext 中提取引用索引（形如 "msg_idx=REFIDX_xxx"），无则返回空串
func refIdxOf(scene qqbot_msg.GroupMessageScene) string {
	for _, ext := range scene.Ext {
		if strings.HasPrefix(ext, "msg_idx=") {
			return strings.TrimPrefix(ext, "msg_idx=")
		}
	}
	return ""
}
