package napcatbot

import "strings"

func DeMsg(msg string) string {
	msg = strings.ReplaceAll(msg, "&amp;", "&")
	return msg
}
