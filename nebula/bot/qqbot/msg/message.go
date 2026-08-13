package qqbot_msg

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseKeyboardJSON 解析键盘 JSON 字符串，非 JSON 或空字符串返回 nil
func ParseKeyboardJSON(jsonStr string) *Keyboard {
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" || jsonStr[0] != '{' {
		return nil
	}
	var kb Keyboard
	if err := json.Unmarshal([]byte(jsonStr), &kb); err != nil {
		return nil
	}
	// 兼容 {"rows":[...]} 写法（自动包裹 content）
	if kb.Content == nil && kb.ID == "" && strings.Contains(jsonStr, `"rows"`) {
		wrapped := `{"content":` + jsonStr + `}`
		var kb2 Keyboard
		if err := json.Unmarshal([]byte(wrapped), &kb2); err == nil {
			return &kb2
		}
	}
	return &kb
}

// 回复子频道消息
func (b *QQBot) ReplyChannelMessage(messageID, channelID, content string) (*MessageResponse, error) {
	if channelID == "" || content == "" {
		return nil, fmt.Errorf("channelID或消息内容为空")
	}
	url := fmt.Sprintf("/channels/%s/messages", channelID)

	b.Count++
	msg := MessageToSend{
		Content: content,
		MsgId:   messageID,
		MsgSeq:  b.Count,
	}

	var resp MessageResponse
	if err := b.Send(url, msg, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// 回复子频道图文消息
func (b *QQBot) ReplyChannelImgMessage(messageID, channelID, img, content string) (*MessageResponse, error) {
	url := fmt.Sprintf("/channels/%s/messages", channelID)

	msg := ChannelSend{
		Content: content,
		MsgId:   messageID,
	}

	var resp MessageResponse
	if err := b.SendChannelImage(url, []byte(img), msg, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// 回复频道语音消息
func (b *QQBot) ReplyChannelVoiceMessage(messageID, channelID, voice string) (*MessageResponse, error) {
	url := fmt.Sprintf("/channels/%s/messages", channelID)
	base64Data := base64.StdEncoding.EncodeToString([]byte(voice))
	voice = base64Data
	media, err := b.GroupUploadFiles(3, channelID, voice)
	if err != nil {
		return nil, err
	}

	b.Count++

	msg := MessageToSend{
		MsgType: 7,
		Media:   media,
		MsgId:   messageID,
		MsgSeq:  b.Count,
	}

	var resp MessageResponse
	if err := b.Send(url, msg, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// 回复频道私信图文消息
func (b *QQBot) ReplyChannelPrivateMessage(messageID, userID, img, content string) (*MessageResponse, error) {
	url := fmt.Sprintf("/dms/%s/messages", userID)

	msg := ChannelSend{
		Content: content,
		MsgId:   messageID,
	}

	var resp MessageResponse
	if err := b.SendChannelImage(url, []byte(img), msg, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// 回复频道私信
func (b *QQBot) ReplyPrivateMessage(messageID, userID, content string) (*MessageResponse, error) {
	url := fmt.Sprintf("/dms/%s/messages", userID)

	b.Count++

	msg := MessageToSend{
		Content: content,
		MsgId:   messageID,
		MsgSeq:  b.Count,
	}

	var resp MessageResponse
	if err := b.Send(url, msg, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// 群富媒体
func (b *QQBot) GroupUploadFiles(Type int, groupOpenID, content string) (*GroupMessageFileResponse, error) {
	url := fmt.Sprintf("/v2/groups/%s/files", groupOpenID)
	// 只截取前15个字符
	// if len(content) > 15 {
	// 	fmt.Println("数据:", content[:15])
	// } else {
	// 	fmt.Println("数据:", content)
	// }
	msg := GroupMessageFile{
		Type: Type,
		Srv:  false,
		Data: content,
	}
	var resp GroupMessageFileResponse
	if err := b.Send(url, msg, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// 群私聊富媒体
func (b *QQBot) GroupPrivateUploadFiles(Type int, openID, content string) (*GroupMessageFileResponse, error) {
	url := fmt.Sprintf("/v2/users/%s/files", openID)
	// 只截取前15个字符
	// if len(content) > 15 {
	// 	fmt.Println("数据:", content[:15])
	// } else {
	// 	fmt.Println("数据:", content)
	// }
	msg := GroupMessageFile{
		Type: Type,
		Srv:  false,
		Data: content,
	}
	var resp GroupMessageFileResponse
	if err := b.Send(url, msg, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// eventIDOf 提取可选的事件 ID（最后一个参数，用于 INTERACTION_CREATE 等被动消息的 event_id 字段）
func eventIDOf(eventIDs ...string) string {
	if len(eventIDs) > 0 {
		return eventIDs[0]
	}
	return ""
}

// 回复群聊语音
func (b *QQBot) ReplyGroupVoiceMessage(messageID, groupOpenID, voice string, eventIDs ...string) (*MessageResponse, error) {
	if groupOpenID == "" || voice == "" {
		return nil, fmt.Errorf("groupOpenID或语音内容为空")
	}
	url := fmt.Sprintf("/v2/groups/%s/messages", groupOpenID)
	base64Data := base64.StdEncoding.EncodeToString([]byte(voice))
	voice = base64Data
	media, err := b.GroupUploadFiles(3, groupOpenID, voice)
	if err != nil {
		return nil, err
	}

	b.Count++

	msg := MessageToSend{
		MsgType: 7,
		Media:   media,
		MsgId:   messageID,
		MsgSeq:  b.Count,
		EventId: eventIDOf(eventIDs...),
	}

	var resp MessageResponse
	if err := b.Send(url, msg, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// 回复群聊视频
func (b *QQBot) ReplyGroupVideoMessage(messageID, groupOpenID, video string, eventIDs ...string) (*MessageResponse, error) {
	if groupOpenID == "" || video == "" {
		return nil, fmt.Errorf("groupOpenID或视频内容为空")
	}
	url := fmt.Sprintf("/v2/groups/%s/messages", groupOpenID)
	base64Data := base64.StdEncoding.EncodeToString([]byte(video))
	video = base64Data
	media, err := b.GroupUploadFiles(2, groupOpenID, video)
	if err != nil {
		return nil, err
	}

	b.Count++

	msg := MessageToSend{
		MsgType: 7,
		Media:   media,
		MsgId:   messageID,
		MsgSeq:  b.Count,
		EventId: eventIDOf(eventIDs...),
	}

	var resp MessageResponse
	if err := b.Send(url, msg, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// 回复群markdown
func (b *QQBot) ReplyGroupMarkdownMessage(messageID, groupOpenID string, md *Markdown) (*MessageResponse, error) {
	return b.ReplyGroupMarkdownWithKeyboard(messageID, groupOpenID, md, nil)
}

// 回复群markdown（带键盘）
func (b *QQBot) ReplyGroupMarkdownWithKeyboard(messageID, groupOpenID string, md *Markdown, kb *Keyboard, eventIDs ...string) (*MessageResponse, error) {
	if groupOpenID == "" || md == nil {
		return nil, fmt.Errorf("groupOpenID或markdown为空")
	}
	url := fmt.Sprintf("/v2/groups/%s/messages", groupOpenID)

	b.Count++

	msg := MessageToSend{
		MsgType:  2,
		Markdown: md,
		MsgId:    messageID,
		MsgSeq:   b.Count,
		Keyboard: kb,
		EventId:  eventIDOf(eventIDs...),
	}

	var resp MessageResponse
	if err := b.Send(url, msg, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// 回复群任意markdown
func (b *QQBot) ReplyGroupAnyMarkdownMessage(messageID, groupOpenID, text string) (*MessageResponse, error) {
	return b.ReplyGroupAnyMarkdownWithKeyboard(messageID, groupOpenID, text, nil)
}

// 回复群任意markdown（带键盘）
func (b *QQBot) ReplyGroupAnyMarkdownWithKeyboard(messageID, groupOpenID, text string, kb *Keyboard, eventIDs ...string) (*MessageResponse, error) {
	if groupOpenID == "" || text == "" {
		return nil, fmt.Errorf("groupOpenID或文本为空")
	}
	url := fmt.Sprintf("/v2/groups/%s/messages", groupOpenID)

	b.Count++

	md := &Markdown{Content: text}

	msg := MessageToSend{
		MsgType:  2,
		Markdown: md,
		MsgId:    messageID,
		MsgSeq:   b.Count,
		Keyboard: kb,
		EventId:  eventIDOf(eventIDs...),
	}

	var resp MessageResponse
	if err := b.Send(url, msg, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// 回复私聊任意markdown
func (b *QQBot) ReplyPrivateAnyMarkdownMessage(messageID, openID, text string) (*MessageResponse, error) {
	return b.ReplyPrivateAnyMarkdownWithKeyboard(messageID, openID, text, nil)
}

// 回复私聊任意markdown（带键盘）
func (b *QQBot) ReplyPrivateAnyMarkdownWithKeyboard(messageID, openID, text string, kb *Keyboard) (*MessageResponse, error) {
	if openID == "" || text == "" {
		return nil, fmt.Errorf("openID或文本为空")
	}
	url := fmt.Sprintf("/v2/users/%s/messages", openID)

	b.Count++

	md := &Markdown{Content: text}

	msg := MessageToSend{
		MsgType:  2,
		Markdown: md,
		MsgId:    messageID,
		MsgSeq:   b.Count,
		Keyboard: kb,
	}

	var resp MessageResponse
	if err := b.Send(url, msg, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// 回复私聊markdown
func (b *QQBot) ReplyPrivateMarkdownMessage(messageID, openID string, md *Markdown) (*MessageResponse, error) {
	return b.ReplyPrivateMarkdownWithKeyboard(messageID, openID, md, nil)
}

// 回复私聊markdown（带键盘）
func (b *QQBot) ReplyPrivateMarkdownWithKeyboard(messageID, openID string, md *Markdown, kb *Keyboard) (*MessageResponse, error) {
	if openID == "" || md == nil {
		return nil, fmt.Errorf("openID或markdown为空")
	}
	url := fmt.Sprintf("/v2/users/%s/messages", openID)

	b.Count++

	msg := MessageToSend{
		MsgType:  2,
		Markdown: md,
		MsgId:    messageID,
		MsgSeq:   b.Count,
		Keyboard: kb,
	}

	var resp MessageResponse
	if err := b.Send(url, msg, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// 回复群聊图文
func (b *QQBot) ReplyGroupImgMessage(messageID, groupOpenID, img, content string, eventIDs ...string) (*MessageResponse, error) {
	if groupOpenID == "" || img == "" {
		return nil, fmt.Errorf("groupOpenID或图片内容为空")
	}
	url := fmt.Sprintf("/v2/groups/%s/messages", groupOpenID)
	base64Data := base64.StdEncoding.EncodeToString([]byte(img))
	img = base64Data
	media, err := b.GroupUploadFiles(1, groupOpenID, img)
	if err != nil {
		return nil, err
	}

	b.Count++

	msg := MessageToSend{
		MsgType: 7,
		Media:   media,
		Content: content,
		MsgId:   messageID,
		MsgSeq:  b.Count,
		EventId: eventIDOf(eventIDs...),
	}

	var resp MessageResponse
	if err := b.Send(url, msg, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// 回复群私聊图文
func (b *QQBot) ReplyGroupPrivateImgMessage(messageID, openID, img, content string) (*MessageResponse, error) {
	url := fmt.Sprintf("/v2/users/%s/messages", openID)
	base64Data := base64.StdEncoding.EncodeToString([]byte(img))
	img = base64Data
	media, err := b.GroupPrivateUploadFiles(1, openID, img)
	if err != nil {
		return nil, err
	}

	b.Count++

	msg := MessageToSend{
		MsgType: 7,
		Media:   media,
		Content: content,
		MsgId:   messageID,
		MsgSeq:  b.Count,
	}

	var resp MessageResponse
	if err := b.Send(url, msg, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// 回复群聊
func (b *QQBot) ReplyGroupMessage(messageID, groupOpenID, content string, eventIDs ...string) (*MessageResponse, error) {
	if groupOpenID == "" || content == "" {
		return nil, fmt.Errorf("groupOpenID或消息内容为空")
	}
	url := fmt.Sprintf("/v2/groups/%s/messages", groupOpenID)

	b.Count++

	msg := MessageToSend{
		Content: content,
		MsgId:   messageID,
		MsgSeq:  b.Count,
		EventId: eventIDOf(eventIDs...),
	}

	var resp MessageResponse
	if err := b.Send(url, msg, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// 回复群私聊
func (b *QQBot) ReplyGroupPrivateMessage(messageID, openID, content string) (*MessageResponse, error) {
	if openID == "" || content == "" {
		return nil, fmt.Errorf("openID或消息内容为空")
	}
	url := fmt.Sprintf("/v2/users/%s/messages", openID)

	b.Count++

	msg := MessageToSend{
		Content: content,
		MsgId:   messageID,
		MsgSeq:  b.Count,
	}

	var resp MessageResponse
	if err := b.Send(url, msg, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// 回复群私聊语音
func (b *QQBot) ReplyGroupPrivateVoiceMessage(messageID, openID, voice string) (*MessageResponse, error) {
	if openID == "" || voice == "" {
		return nil, fmt.Errorf("openID或语音内容为空")
	}
	url := fmt.Sprintf("/v2/users/%s/messages", openID)
	base64Data := base64.StdEncoding.EncodeToString([]byte(voice))
	voice = base64Data
	media, err := b.GroupPrivateUploadFiles(3, openID, voice)
	if err != nil {
		return nil, err
	}

	b.Count++

	msg := MessageToSend{
		MsgType: 7,
		Media:   media,
		MsgId:   messageID,
		MsgSeq:  b.Count,
	}

	var resp MessageResponse
	if err := b.Send(url, msg, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// 回复群私聊视频
func (b *QQBot) ReplyGroupPrivateVideoMessage(messageID, openID, video string) (*MessageResponse, error) {
	if openID == "" || video == "" {
		return nil, fmt.Errorf("openID或视频内容为空")
	}
	url := fmt.Sprintf("/v2/users/%s/messages", openID)
	base64Data := base64.StdEncoding.EncodeToString([]byte(video))
	video = base64Data
	media, err := b.GroupPrivateUploadFiles(2, openID, video)
	if err != nil {
		return nil, err
	}

	b.Count++

	msg := MessageToSend{
		MsgType: 7,
		Media:   media,
		MsgId:   messageID,
		MsgSeq:  b.Count,
	}

	var resp MessageResponse
	if err := b.Send(url, msg, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ============= 群信息 ============

// GetGroupInfo 获取群信息
func (b *QQBot) GetGroupInfo(groupOpenID string) (*GroupInfo, error) {
	if groupOpenID == "" {
		return nil, fmt.Errorf("groupOpenID为空")
	}
	url := fmt.Sprintf("/v2/groups/%s/info", groupOpenID)
	var resp GroupInfo
	if err := b.Get(url, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetBotState 获取机器人群内状态
func (b *QQBot) GetBotState(groupOpenID string) (*BotState, error) {
	if groupOpenID == "" {
		return nil, fmt.Errorf("groupOpenID为空")
	}
	url := fmt.Sprintf("/v2/groups/%s/bot_state", groupOpenID)
	var resp BotState
	if err := b.Get(url, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ============= 群禁言管理 ============

// GetMemberMuteStatus 查询群禁言状态
func (b *QQBot) GetMuteStatus(groupOpenID string) (*MuteStatusResponse, error) {
	if groupOpenID == "" {
		return nil, fmt.Errorf("groupOpenID为空")
	}
	url := fmt.Sprintf("/v2/groups/%s/restrict_chat_setting", groupOpenID)
	var resp MuteStatusResponse
	if err := b.Get(url, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SetMemberMute 设置用户禁言，seconds 为 0 表示解除禁言
func (b *QQBot) SetMemberMute(groupOpenID, memberOpenID, seconds string) error {
	if groupOpenID == "" || memberOpenID == "" {
		return fmt.Errorf("groupOpenID或memberOpenID为空")
	}
	op := "add"
	if seconds == "0" {
		op = "del"
	}

	expireAt := ""
	if op != "del" {
		sec, err := strconv.Atoi(seconds)
		if err != nil {
			return fmt.Errorf("禁言秒数格式错误: %w", err)
		}
		expireAt = time.Now().Add(time.Duration(sec) * time.Second).Format(time.RFC3339)
	}

	url := fmt.Sprintf("/v2/groups/%s/restrict_chat_setting", groupOpenID)
	req := &SetMemberMuteRequest{
		Members: []SetMuteMember{{
			Op:           op,
			MemberOpenID: memberOpenID,
			MuteExpireAt: expireAt,
		}},
	}
	return b.Send(url, req, nil)
}

// ============= 入群申请审批 ============

// GetJoinRequests 拉取入群申请列表
func (b *QQBot) GetJoinRequests(groupOpenID, cursor string, limit int) (*JoinRequestListResponse, error) {
	if groupOpenID == "" {
		return nil, fmt.Errorf("groupOpenID为空")
	}
	url := fmt.Sprintf("/v2/groups/%s/join_request_list", groupOpenID)
	if cursor != "" || limit > 0 {
		url += "?"
		if cursor != "" {
			url += fmt.Sprintf("cursor=%s", cursor)
		}
		if limit > 0 {
			if cursor != "" {
				url += "&"
			}
			url += fmt.Sprintf("limit=%d", limit)
		}
	}
	var resp JoinRequestListResponse
	if err := b.Get(url, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ApproveJoinRequest 审批入群请求
func (b *QQBot) ApproveJoinRequest(groupOpenID, memberOpenID, op, joinRequestID, rejectReason string, addToBlacklist bool) error {
	if groupOpenID == "" || memberOpenID == "" {
		return fmt.Errorf("groupOpenID或memberOpenID为空")
	}
	url := fmt.Sprintf("/v2/groups/%s/approval_join_request/%s", groupOpenID, memberOpenID)
	// 入参使用中文：同意/拒绝，映射为 QQ API 要求的英文值
	switch op {
	case "同意", "通过":
		op = "approve"
	case "拒绝":
		op = "decline"
	}
	body := &ApproveJoinRequest{
		Op:                   op,
		JoinRequestID:        joinRequestID,
		RejectReason:         rejectReason,
		AddToMemberBlacklist: addToBlacklist,
	}
	return b.Send(url, body, nil)
}

// ============= 交互事件响应 ============

// ReplyInteraction 回应交互事件（按钮点击等），code=0 表示成功
func (b *QQBot) ReplyInteraction(interactionID string, code int) error {
	if interactionID == "" {
		return fmt.Errorf("interactionID不能为空")
	}
	url := fmt.Sprintf("/interactions/%s", interactionID)
	return b.Put(url, InteractionResponse{Code: code}, nil)
}

// RespondInteraction 回应互动事件（参考: PUT /interactions/{interaction_id}）
// 收到 INTERACTION_CREATE 事件后需调用此接口回应，否则客户端会一直 loading 直到超时。
// interaction_id 从事件的 id 字段获取；同一 interaction_id 只能回应一次，超时后失效。
func (b *QQBot) RespondInteraction(event *InteractionEvent, code InteractionResponseCode) error {
	if event == nil || event.ID == "" {
		return fmt.Errorf("互动事件为空或缺少事件ID")
	}
	return b.ReplyInteraction(event.ID, int(code))
}
