package qqbot

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"sync"

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
		switch {
		case strings.HasPrefix(src, "data:"):
			// 沙箱场景 $IMG 1$ 返回的是 data URL，直接解码为图片原始字节
			data, err = dataURLBytes(src)
		case strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://"):
			data, err = utils.Get(src)
		default:
			data, err = utils.NewFileQueue(src).ReadFile()
		}
		if err == nil && data != "" {
			imgs = append(imgs, data)
		}
	}

	// 去掉首尾空白：±img=/±atMsg= 标记移除后可能残留空格/换行，
	// 纯空白视为空消息，交由上层「空消息不发送」判断拦截。
	return strings.TrimSpace(s), imgs, atMsgID
}

// dataURLBytes 把 data URL（如 data:image/png;base64,xxx）解码为图片原始字节。
// 沙箱场景下 $IMG 1$ 返回的附件 URL 是 data URL，需此处解码后才能作为图片发送。
func dataURLBytes(s string) (string, error) {
	meta, data, ok := strings.Cut(strings.TrimPrefix(s, "data:"), ",")
	if !ok || data == "" {
		return "", fmt.Errorf("无效的 data URL")
	}
	if strings.Contains(meta, ";base64") {
		raw, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return "", err
		}
		return string(raw), nil
	}
	return data, nil
}

// extValue 从群消息场景的 ext 中提取指定 key 的值（key=value 格式，如 msg_idx= / ref_msg_idx=），无则返回空串
func extValue(scene qqbot_msg.GroupMessageScene, key string) string {
	prefix := key + "="
	for _, ext := range scene.Ext {
		if v, ok := strings.CutPrefix(ext, prefix); ok {
			return v
		}
	}
	return ""
}

// msgRefIdxMap 记录「消息ID → 引用索引(msg_idx)」的映射，
// 用于把词库 ±atMsg=消息ID± 中的消息 ID 还原为引用回复所需的引用索引（回复ID）。
var msgRefIdxMap sync.Map

// storeMsgRefIdx 记录一条消息的 ID 与其引用索引（msg_idx）的对应关系。
func storeMsgRefIdx(msgID, refIdx string) {
	if msgID == "" || refIdx == "" {
		return
	}
	msgRefIdxMap.Store(msgID, refIdx)
}

// resolveAtMsgRefID 把 ±atMsg=消息ID± 中的值还原为引用回复所需的引用索引（回复ID）：
// 优先按消息ID在映射中查找对应引用索引；查找不到时原样返回，
// 此时该值本身已是引用索引（如 msg_idx/ref_idx）。
func resolveAtMsgRefID(atMsgID string) string {
	if atMsgID == "" {
		return ""
	}
	if v, ok := msgRefIdxMap.Load(atMsgID); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return atMsgID
}
