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
	re := regexp.MustCompile(`^<@!\d+>\s*`)
	loc := re.FindStringIndex(s)
	if loc == nil {
		return s // 没找到，原样返回
	}
	return s[:loc[0]] + s[loc[1]:] // 替换掉匹配的那一段
}

// 移除开头指定@账号
func RemoveLeadingMention(s string, qq int) string {
	// 检测带空格
	if after, ok := strings.CutPrefix(s, fmt.Sprintf("<@!%d> ", qq)); ok {
		return after // 移除开头
	}
	// 检测开头
	if after, ok := strings.CutPrefix(s, fmt.Sprintf("<@!%d>", qq)); ok {
		return after // 移除开头
	}
	return s
}

// 艾特消息格式转换（十六进制ID格式）
func ConvertATMessageToMD(s string) string {
	re := regexp.MustCompile(`<@([0-9A-Fa-f]+)>`)
	return re.ReplaceAllString(s, "@$1")
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
	if strings.HasPrefix(s, " ") {
		return s[1:]
	}
	return s
}

var reLeadingAt = regexp.MustCompile(`^(@\S+\s*)+`)

// RemoveLeadingAtMentions 移除消息开头的 @用户名 前缀，用于全量消息艾特兼容
func RemoveLeadingAtMentions(s string) string {
	return reLeadingAt.ReplaceAllString(s, "")
}

// 去掉所有 ±img=...± 段，并返回净化后文本 + 提取到的 img 值列表
func stripImgTags(s string) (string, []string) {
	re := regexp.MustCompile(`±img=(.+?)±`)
	var imgs []string

	dst := re.ReplaceAllStringFunc(s, func(raw string) string {
		m := re.FindStringSubmatch(raw)
		if len(m) < 2 {
			return ""
		}

		src := m[1]

		var data string
		var err error

		// 判断是否为 http / https
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
			data, err = utils.Get(src)
		} else {
			data, err = utils.NewFileQueue(src).ReadFile()
		}

		if err == nil {
			imgs = append(imgs, data)
		}

		return ""
	})

	return dst, imgs
}
