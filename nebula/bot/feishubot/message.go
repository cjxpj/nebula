package feishubot

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// larkErr 统一转换飞书接口的错误信息
func larkErr(code int, msg string) error {
	return fmt.Errorf("lark api error: code=%d msg=%s", code, msg)
}

// sendMessage 发送消息（群/私聊）通用实现，返回消息ID
func sendMessage(receiveIdType, receiveId, msgType, content string) (string, error) {
	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(receiveIdType).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(receiveId).
			MsgType(msgType).
			Content(content).
			Build()).
		Build()

	resp, err := dto.ServerConfig.FeiShuBot.API.Im.Message.Create(context.Background(), req)
	if err != nil {
		return "", err
	}
	if !resp.Success() {
		return "", larkErr(resp.Code, resp.Msg)
	}
	// 解引用，空指针则返回空串
	if resp.Data == nil || resp.Data.MessageId == nil {
		return "", nil
	}
	return *resp.Data.MessageId, nil
}

// 获取图片
func GetImageMsg(msgId, fileKey string) (string, error) {
	req := larkim.NewGetMessageResourceReqBuilder().
		MessageId(msgId).
		FileKey(fileKey).
		Type("image").
		Build()
	resp, err := dto.ServerConfig.FeiShuBot.API.Im.V1.MessageResource.Get(context.Background(), req)
	if err != nil {
		return "", err
	}
	if !resp.Success() {
		return "", larkErr(resp.Code, resp.Msg)
	}
	if resp.File == nil {
		return "", fmt.Errorf("lark api error: file is nil")
	}
	return string(resp.RawBody), nil
}

// 群消息
func SendGroupMsg(chatID, text string) (string, error) {
	contentJson, _ := utils.Marshal(MessageText{Text: text})
	return sendMessage("chat_id", chatID, "text", string(contentJson))
}

// 私聊消息
func SendPrivateMsg(openID, text string) (string, error) {
	contentJson, _ := utils.Marshal(MessageText{Text: text})
	return sendMessage("open_id", openID, "text", string(contentJson))
}

// 群图片
func SendGroupImg(chatID, imageKey string) (string, error) {
	contentJson, _ := utils.Marshal(MessageImg{ImageKey: imageKey})
	return sendMessage("chat_id", chatID, "image", string(contentJson))
}

// 私聊图片
func SendPrivateImg(openID, imageKey string) (string, error) {
	contentJson, _ := utils.Marshal(MessageImg{ImageKey: imageKey})
	return sendMessage("open_id", openID, "image", string(contentJson))
}

// 回复消息
func ReplyMsg(msgID, text string) (string, error) {
	contentJson, _ := utils.Marshal(MessageText{Text: text})
	req := larkim.NewReplyMessageReqBuilder().
		MessageId(msgID).
		Body(larkim.NewReplyMessageReqBodyBuilder().
			MsgType("text").
			Content(string(contentJson)).
			Build()).
		Build()

	resp, err := dto.ServerConfig.FeiShuBot.API.Im.Message.Reply(context.Background(), req)
	if err != nil {
		return "", err
	}
	if !resp.Success() {
		return "", larkErr(resp.Code, resp.Msg)
	}
	if resp.Data == nil || resp.Data.MessageId == nil {
		return "", nil
	}
	return *resp.Data.MessageId, nil
}

// 撤回消息
func RecallMsg(msgID string) error {
	req := larkim.NewDeleteMessageReqBuilder().MessageId(msgID).Build()
	resp, err := dto.ServerConfig.FeiShuBot.API.Im.Message.Delete(context.Background(), req)
	if err != nil {
		return err
	}
	if !resp.Success() {
		return larkErr(resp.Code, resp.Msg)
	}
	return nil
}

// 表情回复，返回 reaction ID
func AddReaction(msgID, emojiType string) (string, error) {
	req := larkim.NewCreateMessageReactionReqBuilder().
		MessageId(msgID).
		Body(larkim.NewCreateMessageReactionReqBodyBuilder().
			ReactionType(&larkim.Emoji{EmojiType: &emojiType}).
			Build()).
		Build()

	resp, err := dto.ServerConfig.FeiShuBot.API.Im.V1.MessageReaction.Create(context.Background(), req)
	if err != nil {
		return "", err
	}
	if !resp.Success() {
		return "", larkErr(resp.Code, resp.Msg)
	}
	if resp.Data == nil || resp.Data.ReactionId == nil {
		return "", nil
	}
	return *resp.Data.ReactionId, nil
}

// 上传图片，返回 image_key
func UploadImage(data []byte) (string, error) {
	req := larkim.NewCreateImageReqBuilder().
		Body(larkim.NewCreateImageReqBodyBuilder().
			ImageType("message").
			Image(bytes.NewReader(data)).
			Build()).
		Build()

	resp, err := dto.ServerConfig.FeiShuBot.API.Im.V1.Image.Create(context.Background(), req)
	if err != nil {
		return "", err
	}
	if !resp.Success() {
		return "", larkErr(resp.Code, resp.Msg)
	}
	if resp.Data == nil || resp.Data.ImageKey == nil {
		return "", fmt.Errorf("lark api error: image_key is nil")
	}
	return *resp.Data.ImageKey, nil
}

// 获取群信息
func GetChatInfo(chatID string) (string, error) {
	req := larkim.NewGetChatReqBuilder().ChatId(chatID).Build()
	resp, err := dto.ServerConfig.FeiShuBot.API.Im.V1.Chat.Get(context.Background(), req)
	if err != nil {
		return "", err
	}
	if !resp.Success() {
		return "", larkErr(resp.Code, resp.Msg)
	}
	return string(resp.RawBody), nil
}

// 获取群成员列表
func GetChatMemberList(chatID string) (string, error) {
	req := larkim.NewGetChatMembersReqBuilder().
		ChatId(chatID).
		MemberIdType("open_id").
		PageSize(100).
		Build()
	resp, err := dto.ServerConfig.FeiShuBot.API.Im.V1.ChatMembers.Get(context.Background(), req)
	if err != nil {
		return "", err
	}
	if !resp.Success() {
		return "", larkErr(resp.Code, resp.Msg)
	}
	return string(resp.RawBody), nil
}

// 图片数据解析：支持 http(s) 链接、data URL、纯 base64、本地文件路径
func parseImgData(data string) ([]byte, error) {
	data = strings.TrimSpace(data)
	if data == "" {
		return nil, fmt.Errorf("图片数据为空")
	}
	switch {
	case strings.HasPrefix(data, "http://"), strings.HasPrefix(data, "https://"):
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(data)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("图片下载失败: %s", resp.Status)
		}
		return io.ReadAll(resp.Body)
	case strings.HasPrefix(data, "data:"):
		// data:image/png;base64,xxx
		_, raw, ok := strings.Cut(strings.TrimPrefix(data, "data:"), ",")
		if !ok {
			return nil, fmt.Errorf("无效的 data URL")
		}
		return base64.StdEncoding.DecodeString(raw)
	case strings.HasPrefix(data, "base64,"):
		return base64.StdEncoding.DecodeString(strings.TrimPrefix(data, "base64,"))
	}

	// 本地文件
	if fileData, err := os.ReadFile(data); err == nil {
		return fileData, nil
	}
	// 兜底按 base64 处理
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("图片数据格式不支持: %w", err)
	}
	return raw, nil
}
