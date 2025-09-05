package api

import (
	"encoding/base64"
	"fmt"
)

// 回复子频道消息
func (b *QQBot) ReplyChannelMessage(messageID, channelID, content string) (*MessageResponse, error) {
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

// 回复群聊语音
func (b *QQBot) ReplyGroupVoiceMessage(messageID, groupOpenID, voice string) (*MessageResponse, error) {
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
	}

	var resp MessageResponse
	if err := b.Send(url, msg, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// 回复群聊视频
func (b *QQBot) ReplyGroupVideoMessage(messageID, groupOpenID, video string) (*MessageResponse, error) {
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
	}

	var resp MessageResponse
	if err := b.Send(url, msg, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// 回复群聊图文
func (b *QQBot) ReplyGroupImgMessage(messageID, groupOpenID, img, content string) (*MessageResponse, error) {
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
	}
	return &resp, nil
}

// 回复群聊
func (b *QQBot) ReplyGroupMessage(messageID, groupOpenID, content string) (*MessageResponse, error) {
	url := fmt.Sprintf("/v2/groups/%s/messages", groupOpenID)

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

// 回复群私聊
func (b *QQBot) ReplyGroupPrivateMessage(messageID, openID, content string) (*MessageResponse, error) {
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
