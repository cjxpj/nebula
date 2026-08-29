package qqbot

import (
	"fmt"
	"strings"
	"time"

	qqbot_msg "github.com/cjxpj/nebula/bot/qqbot/msg"
)

// SandboxUser 沙箱模拟的用户信息（由前端自定义）
type SandboxUser struct {
	// ID 模拟用户的 OpenID/QQ 号
	ID string
	// Name 模拟用户昵称（写入词库变量 昵称）
	Name string
}

// SandboxRun 在沙箱中模拟一条消息（群聊或私聊），返回机器人将要发送的消息列表（不真正发往 QQ）。
// 同时返回本次模拟的入站消息 ID，供前端还原 ±atMsg=消息ID± 引用回复。
// replyID 为前端指定的被引用消息 ID（右键回复），空则回退为本次入站消息自身，
// 保证 ref_msg_idx 始终有值可用于还原引用回复场景。
// private 为 true 时按群私聊处理（词库 #私聊# 触发词生效）。
// groupID 为模拟的群号（写入 %群号%），空则使用默认 sandbox_group。
// images 为前端发送的图片（data URL 列表），注入为消息附件，供 $IMG$ 等词库函数读取。
// bot 需已设置 FilePath；API 需为沙箱模式（bot.API.Sandbox 非 nil）。
func SandboxRun(bot *qqbot_msg.RouterQQBot, msg string, user SandboxUser, private bool, replyID, groupID string, images []string) (string, []qqbot_msg.SandboxMessage) {
	userID := user.ID
	if userID == "" {
		userID = "sandbox_user"
	}
	username := user.Name
	if username == "" {
		username = "测试用户"
	}
	if groupID == "" {
		groupID = "sandbox_group"
	}

	// 纳秒级唯一 ID：同一秒内连续多条测试消息也不会撞 ID（CheckOnce 去重依赖）
	msgID := fmt.Sprintf("sandbox_%d", time.Now().UnixNano())

	// 被引用消息 ID：优先前端右键指定的目标，否则回退为本次入站消息自身
	refID := replyID
	if refID == "" {
		refID = msgID
	}

	// 前端发送的图片转为消息附件，$IMG$ / $IMG 1$ 等词库函数据此读取
	var attachments []qqbot_msg.Attachment
	for _, img := range images {
		if att, ok := parseImageDataURL(img); ok {
			attachments = append(attachments, att)
		}
	}

	m := &qqbot_msg.GroupMessageEvent{
		ID:      msgID,
		Content: msg,
		Author: qqbot_msg.GroupAuthor{
			ID:           userID,
			Username:     username,
			UserOpenID:   userID,
			MemberOpenID: userID,
		},
		Attachments: attachments,
		GroupOpenID: groupID,
		// 沙箱模拟「引用回复」场景，与真实 QQ 对齐：
		// msg_idx 为当前消息自身的引用索引，ref_msg_idx 为被引用消息的引用索引。
		// 词库引用回复统一用 %消息ID%（当前消息 ID），经 msgRefIdxMap 还原为引用索引。
		MessageScene: qqbot_msg.GroupMessageScene{Ext: []string{"msg_idx=" + msgID, "ref_msg_idx=" + refID}},
	}

	if private {
		qqBOTGroupPrivateRunEvent(m, bot)
	} else {
		qqBOTGroupRunEvent(m, bot)
	}

	waitSandboxMessages(bot)
	if bot == nil || bot.API == nil || bot.API.Sandbox == nil {
		return m.ID, nil
	}
	return m.ID, bot.API.Sandbox.List()
}

// parseImageDataURL 把前端传入的 data URL（data:image/png;base64,xxx）解析为消息附件。
// 解析失败（非 data URL 或缺少数据）返回 ok=false，上层跳过该图片。
func parseImageDataURL(s string) (qqbot_msg.Attachment, bool) {
	s = strings.TrimSpace(s)
	const prefix = "data:"
	if !strings.HasPrefix(s, prefix) {
		return qqbot_msg.Attachment{}, false
	}
	rest := s[len(prefix):]
	before, after, ok := strings.Cut(rest, ",")
	if !ok {
		return qqbot_msg.Attachment{}, false
	}
	meta := before
	data := after
	if data == "" {
		return qqbot_msg.Attachment{}, false
	}
	mime := meta
	if before, _, ok := strings.Cut(meta, ";"); ok {
		mime = before
	}
	if mime == "" {
		mime = "image/png"
	}
	return qqbot_msg.Attachment{
		URL:         s,
		ContentType: mime,
		Content:     data,
	}, true
}

// waitSandboxMessages 等待异步发送结束：大多数回复函数在 goroutine 里发送，
// 部分还带延迟（如 $调用 延迟毫秒 触发$）。以「连续 quietGap 无新捕获」判定发送结束，
// 未捕获到任何消息时最多等 noReplyWait；整体上限 maxWait 防止长延迟拖死接口。
func waitSandboxMessages(bot *qqbot_msg.RouterQQBot) {
	if bot == nil || bot.API == nil || bot.API.Sandbox == nil {
		return
	}
	const (
		pollInterval = 50 * time.Millisecond
		quietGap     = 400 * time.Millisecond // 连续无新消息视为发送结束
		noReplyWait  = time.Second            // 从未捕获过消息的兜底等待
		maxWait      = 5 * time.Second        // 整体上限（如 $调用 5000$）
	)
	c := bot.API.Sandbox
	start := time.Now()
	lastSeen := c.LastAdd() // 已见过的最近捕获时间（零值为「尚未捕获」）
	for {
		time.Sleep(pollInterval)
		cur := c.LastAdd()
		now := time.Now()
		if cur != lastSeen {
			lastSeen = cur
		}
		switch {
		case cur.IsZero() && now.Sub(start) >= noReplyWait:
			return // 始终无回复：快速返回
		case !cur.IsZero() && now.Sub(cur) >= quietGap:
			return // 已捕获过且持续静默：发送结束
		case now.Sub(start) >= maxWait:
			return // 延迟发送的兜底上限
		}
	}
}
