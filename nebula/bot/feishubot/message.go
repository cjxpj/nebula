package feishubot

import (
	"context"
	"fmt"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

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
		return "", fmt.Errorf("lark api error: code=%d msg=%s", resp.Code, resp.Msg)
	}
	if resp.File == nil {
		return "", fmt.Errorf("lark api error: file is nil")
	}
	return string(resp.RawBody), nil
}

// 群消息
func SendGroupMsg(chatID, text string) (string, error) {
	contentJson, _ := utils.Marshal(MessageText{Text: text})
	content := string(contentJson)
	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType("chat_id").
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType("text").
			Content(content).
			Build()).
		Build()

	resp, err := dto.ServerConfig.FeiShuBot.API.Im.Message.Create(context.Background(), req)
	if err != nil {
		return "", err
	}
	if !resp.Success() {
		return "", fmt.Errorf("lark api error: code=%d msg=%s", resp.Code, resp.Msg)
	}
	// 解引用，空指针则返回空串
	if resp.Data == nil || resp.Data.MessageId == nil {
		return "", nil
	}
	return *resp.Data.MessageId, nil
}
