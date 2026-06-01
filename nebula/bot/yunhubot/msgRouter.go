package yunhubot

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// 群消息处理
func yunHuBOTGroupRun(payload *Payload) {
	// 词库
	BotDic := utils.NewFileQueue(dto.ServerConfig.YunHuBot.FilePath)
	FileData, err := BotDic.ReadFromFile()
	if err != nil {
		utils.Error("读取机器人词库出错")
		return
	}

	ev := payload.Event
	sendPlayer := ev.Sender
	msgData := ev.Message

	// 取出需要的数据
	groupID := ev.Chat.ChatID         // 群号
	userID := sendPlayer.SenderID     // QQ
	nick := sendPlayer.SenderNickname // 昵称
	content := msgData.Content.Text   // 消息内容

	// 回复消息
	dic := dic_dto.NewDic(dto.ServerConfig.YunHuBot.FilePath, FileData).
		SetGlobal_v(dto.NewVal().
			Set("来源", "群聊").
			Set("昵称", nick).
			Set("群号", groupID).
			Set("QQ", userID))

	rMsg := dic_api.Api.DicRun(dic, content)
	rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
	if rMsg != "" {
		debugLog.Infof("%v", rMsg)
		if err := SendText(groupID, "group", rMsg); err != nil {
			debugLog.Infof("%v", err)
		}
	}
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
