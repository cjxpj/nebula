package qqbottool

import (
	"fmt"
	"regexp"
	"strings"
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

// 移除开头一个空格
func RemoveLeadingSpace(s string) string {
	if strings.HasPrefix(s, " ") {
		return s[1:]
	}
	return s
}
