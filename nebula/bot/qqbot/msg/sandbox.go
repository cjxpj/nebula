package qqbot_msg

import (
	"encoding/base64"
	"strings"
	"sync"
	"time"
)

// SandboxMessage 沙箱捕获到的一条机器人将要发送的消息。
type SandboxMessage struct {
	// Type 消息类型：text（文本）、markdown（Markdown）、image（图片）、video（视频）、voice（语音）、media（其它富媒体）
	Type string `json:"type"`
	// Content 文本正文、Markdown 正文或图片/视频的 data URL
	Content string `json:"content"`
	// MsgId 引用回复的目标消息 ID（词库 ±atMsg=消息ID± 或被动回复的当前消息），为空表示未引用
	MsgId string `json:"msg_id,omitempty"`
}

// SandboxCapture 沙箱消息捕获器，拦截 QQBot 的发送动作（不真正发往 QQ）。
type SandboxCapture struct {
	mu       sync.Mutex
	messages []SandboxMessage
	lastAdd  time.Time // 最近一次捕获时间，供上层「静默检测」判断异步发送是否结束
}

// NewSandboxCapture 创建一个空的沙箱捕获器。
func NewSandboxCapture() *SandboxCapture {
	return &SandboxCapture{}
}

// Add 追加一条捕获消息。
func (c *SandboxCapture) Add(m SandboxMessage) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, m)
	c.lastAdd = time.Now()
}

// List 返回已捕获消息的副本。
func (c *SandboxCapture) List() []SandboxMessage {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]SandboxMessage, len(c.messages))
	copy(out, c.messages)
	return out
}

// LastAdd 返回最近一次捕获的时间（零值表示尚未捕获过）。
func (c *SandboxCapture) LastAdd() time.Time {
	if c == nil {
		return time.Time{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastAdd
}

// sandboxTrim 去掉真实发送时附加的前导换行（被动回复格式），避免沙箱气泡顶部出现空行
func sandboxTrim(s string) string {
	return strings.TrimLeft(s, "\n\r")
}

// imageDataURL 把 base64 图片数据转为 data URL（按魔数嗅探 png/jpeg）
func imageDataURL(data string) string {
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil || len(raw) == 0 {
		return ""
	}
	mime := "image/png"
	if len(raw) >= 2 && raw[0] == 0xFF && raw[1] == 0xD8 {
		mime = "image/jpeg"
	}
	return "data:" + mime + ";base64," + data
}

// captureSend 在沙箱模式下拦截发送体，提取将要发出的消息内容。
// 注意：各 Reply* 方法传给 Send 的是值类型（MessageToSend/GroupMessageFile），
// 因此类型分支必须同时兼容值与指针，否则匹配不到、捕获为空。
func (b *QQBot) captureSend(body any) {
	if b == nil || b.Sandbox == nil {
		return
	}
	switch v := body.(type) {
	case MessageToSend:
		b.captureMessageToSend(&v)
	case *MessageToSend:
		if v != nil {
			b.captureMessageToSend(v)
		}
	case GroupMessageFile:
		b.captureGroupMessageFile(&v)
	case *GroupMessageFile:
		if v != nil {
			b.captureGroupMessageFile(v)
		}
	case ChannelSend:
		if content := sandboxTrim(v.Content); content != "" {
			b.Sandbox.Add(SandboxMessage{Type: "text", Content: content})
		}
	case *ChannelSend:
		if v != nil {
			if content := sandboxTrim(v.Content); content != "" {
				b.Sandbox.Add(SandboxMessage{Type: "text", Content: content})
			}
		}
	}
}

// sandboxReplyID 提取发送体引用的目标：仅显式引用回复（message_reference）才算引用，
// 被动回复的 msg_id 只是「回复当前消息」的必填字段，不应展示为引用条。
func sandboxReplyID(v *MessageToSend) string {
	if v.MessageReference != nil && v.MessageReference.MessageID != "" {
		return v.MessageReference.MessageID
	}
	return ""
}

// captureMessageToSend 从消息发送体中提取文本/Markdown/富媒体正文
func (b *QQBot) captureMessageToSend(v *MessageToSend) {
	replyID := sandboxReplyID(v)
	switch v.MsgType {
	case 0: // 文本
		if content := sandboxTrim(v.Content); content != "" {
			b.Sandbox.Add(SandboxMessage{Type: "text", Content: content, MsgId: replyID})
		}
	case 2: // Markdown
		if v.Markdown != nil {
			if v.Markdown.Content != "" {
				b.Sandbox.Add(SandboxMessage{Type: "markdown", Content: v.Markdown.Content, MsgId: replyID})
			} else {
				// 模板消息：无正文，只有模板 ID，前端按提示文案展示
				b.Sandbox.Add(SandboxMessage{Type: "media", Content: "Markdown 模板 " + v.Markdown.CustomTemplateId, MsgId: replyID})
			}
		}
	case 7: // 富媒体（图片/视频/语音）的正文（附带的文字）
		if content := sandboxTrim(v.Content); content != "" {
			b.Sandbox.Add(SandboxMessage{Type: "text", Content: content, MsgId: replyID})
		}
	}
}

// captureGroupMessageFile 从富媒体上传体中提取图片/视频数据（真实流程中消息体仅携带 uuid）
func (b *QQBot) captureGroupMessageFile(v *GroupMessageFile) {
	if v.Data == "" {
		return
	}
	switch v.Type {
	case 1: // 图片
		if url := imageDataURL(v.Data); url != "" {
			b.Sandbox.Add(SandboxMessage{Type: "image", Content: url})
		} else {
			b.Sandbox.Add(SandboxMessage{Type: "media", Content: "图片"})
		}
	case 2: // 视频
		b.Sandbox.Add(SandboxMessage{Type: "video", Content: "data:video/mp4;base64," + v.Data})
	case 3: // 语音
		b.Sandbox.Add(SandboxMessage{Type: "voice", Content: ""})
	}
}

// fillSandboxResp 在沙箱模式下填充一个假的成功响应，避免后续流程报错。
func (b *QQBot) fillSandboxResp(respObj any) {
	if respObj == nil {
		return
	}
	switch r := respObj.(type) {
	case *MessageResponse:
		r.ID = "sandbox"
		r.Content = "sandbox"
	case *GroupMessageFileResponse:
		r.Uuid = "sandbox"
		r.ID = "sandbox"
	}
}
