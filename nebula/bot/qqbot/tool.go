package qqbot

import (
	"bytes"
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
		case isReplyImageData([]byte(src)):
			// 标记值本身就是图片二进制（如 ±img=%画布数据%±）
			data = src
		default:
			data, err = utils.NewFileQueue(src).ReadFile()
		}
		if err == nil && data != "" {
			imgs = append(imgs, data)
		}
	}

	// 去掉首尾空白：±img=/±atMsg= 标记移除后可能残留空格/换行，
	// 纯空白视为空消息，交由上层「空消息不发送」判断拦截。
	s = strings.TrimSpace(s)
	// 词库正文里直接输出的图片二进制（如 $画布.获取$ 返回的 PNG/JPEG 字节）
	// 会与文本混在一起，这里按文件头魔数拆分，避免把二进制当文本发出。
	s, imgs = splitReplyImages(s, imgs)
	return s, imgs, atMsgID
}

// isReplyImageData 按文件头判断数据是否为常见图片格式。
// 注意不能用 http.DetectContentType：它把任何以 "BM" 开头的文本都判成 image/bmp，
// 会导致正文里普通文字被误当成图片切走，因此这里按真实文件头结构校验。
func isReplyImageData(data []byte) bool {
	switch {
	case bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}): // PNG
		return true
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}): // JPEG
		return true
	case bytes.HasPrefix(data, []byte("GIF87a")), bytes.HasPrefix(data, []byte("GIF89a")): // GIF
		return true
	case bytes.HasPrefix(data, []byte("RIFF")) && len(data) >= 12 && bytes.Equal(data[8:12], []byte("WEBP")): // WebP
		return true
	case bytes.HasPrefix(data, []byte{0x00, 0x00, 0x01, 0x00}), // ICO
		bytes.HasPrefix(data, []byte{0x00, 0x00, 0x02, 0x00}): // CUR
		return true
	case len(data) >= 14 && data[0] == 'B' && data[1] == 'M' &&
		data[6] == 0 && data[7] == 0 && data[8] == 0 && data[9] == 0: // BMP：保留字段（6~9 字节）必须为 0
		return true
	}
	return false
}

// imageFirstBytes 可能是图片文件头首字节的集合，用于快速跳过普通文本
var imageFirstBytes = [256]bool{
	0x89: true, 0xFF: true, 'G': true, 'R': true, 0x00: true, 'B': true,
}

// findReplyImageStart 在文本中查找第一张图片二进制的起始位置，找不到返回 -1
func findReplyImageStart(data []byte) int {
	for i := 0; i < len(data); i++ {
		if imageFirstBytes[data[i]] && isReplyImageData(data[i:]) {
			return i
		}
	}
	return -1
}

// findReplyImageEnd 返回从 start 开始图片二进制的结束位置（不含）。无法确定时返回 len(data)
func findReplyImageEnd(data []byte, start int) int {
	rest := data[start:]
	switch {
	case bytes.HasPrefix(rest, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}):
		// PNG 以 IEND chunk（00 00 00 00 49 45 4E 44 AE 42 60 82）结束
		if idx := bytes.Index(rest, []byte{0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82}); idx >= 0 {
			return start + idx + 12
		}
	case bytes.HasPrefix(rest, []byte{0xFF, 0xD8, 0xFF}):
		// JPEG 以 FFD9 结束
		if idx := bytes.Index(rest, []byte{0xFF, 0xD9}); idx >= 0 {
			return start + idx + 2
		}
	case bytes.HasPrefix(rest, []byte("GIF87a")) || bytes.HasPrefix(rest, []byte("GIF89a")):
		// GIF 以 0x3B 结束
		if idx := bytes.IndexByte(rest, 0x3B); idx >= 0 {
			return start + idx + 1
		}
	case bytes.HasPrefix(rest, []byte("RIFF")) && len(rest) >= 12 && bytes.Equal(rest[8:12], []byte("WEBP")):
		// WebP：RIFF 头部 4~7 字节为整个文件长度（含 8 字节头）
		size := int(rest[4]) | int(rest[5])<<8 | int(rest[6])<<16 | int(rest[7])<<24
		if size >= 8 && len(rest) >= 8+size {
			return start + 8 + size
		}
	}
	return len(data)
}

// splitReplyImages 把回复文本中直接输出的图片二进制拆出来追加到 imgs，
// 返回去掉图片后的文本。未识别到图片时原样返回，避免影响普通文本回复。
func splitReplyImages(s string, imgs []string) (string, []string) {
	data := []byte(s)
	if findReplyImageStart(data) < 0 {
		return s, imgs
	}
	var text bytes.Buffer
	for {
		imgStart := findReplyImageStart(data)
		if imgStart < 0 {
			text.Write(data)
			break
		}
		text.Write(data[:imgStart])
		imgEnd := findReplyImageEnd(data, imgStart)
		imgs = append(imgs, string(data[imgStart:imgEnd]))
		if imgEnd >= len(data) {
			break
		}
		data = data[imgEnd:]
	}
	return strings.TrimSpace(text.String()), imgs
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
