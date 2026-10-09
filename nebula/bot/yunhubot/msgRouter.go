package yunhubot

import (
	"encoding/json"
	"io"
	"net/http"

	botdic "github.com/cjxpj/nebula/bot/botdic"
	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/dto"
)

// 群消息处理
func yunHuBOTGroupRun(payload *Payload) {
	ev := payload.Event
	sendPlayer := ev.Sender
	msgData := ev.Message

	// 取出需要的数据
	groupID := ev.Chat.ChatID         // 群号
	userID := sendPlayer.SenderID     // QQ
	nick := sendPlayer.SenderNickname // 昵称
	content := msgData.Content.Text   // 消息内容

	// 回复消息
	botdic.Run{
		FilePath:   dto.ServerConfig.YunHuBot.FilePath,
		SingleFile: true,
		Val: dto.NewVal().
			Set("来源", "群聊").
			Set("昵称", nick).
			Set("群号", groupID).
			Set("QQ", userID),
		Trigger: content,
		Deliver: func(rMsg string, _ *dto.DicVal) {
			debugLog.Infof("%v", rMsg)
			if err := SendText(groupID, "group", rMsg); err != nil {
				debugLog.Infof("%v", err)
			}
		},
	}.Exec()
}

func BotMessage(w http.ResponseWriter, r *http.Request) {
	httpBody, err := io.ReadAll(r.Body)
	if err != nil {
		w.Write([]byte("Bot Post"))
		return
	}

	payload := &Payload{}
	if err = json.Unmarshal(httpBody, payload); err != nil {
		w.Write([]byte("Bot ErrorData"))
		return
	}
	debugLog.Infof("%v", string(httpBody))
	switch payload.Header.EventType {
	case "message.receive.normal":
		yunHuBOTGroupRun(payload)
		w.Write([]byte("Bot Message"))

	default:
		debugLog.Infof("Bot消息类型未支持")
		return
	}

}
