package dic

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
	yunhubottool "github.com/cjxpj/nebula/yunhuBotTool"
)

// 群消息处理
func (s *ServeRouter) YunHuBOTGroupRun(payload *yunhubottool.Payload) {
	// 词库
	BotDic := utils.NewFileQueue(s.YunHuBot.FilePath)
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
	dic := NewDic(s.YunHuBot.FilePath, FileData).
		SetGlobal_v(dto.NewVal().
			Set("来源", "群聊").
			Set("昵称", nick).
			Set("群号", groupID).
			Set("QQ", userID))

	rMsg := dic.Run(content)
	rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
	if rMsg != "" {
		fmt.Println(rMsg)
		if err := s.YunHuBot.SendText(groupID, "group", rMsg); err != nil {
			fmt.Println(err)
		}
	}
}

func (s *ServeRouter) YunHuBotRun(w http.ResponseWriter, r *http.Request) {
	httpBody, err := io.ReadAll(r.Body)
	if err != nil {
		w.Write([]byte("Bot Post"))
		return
	}

	payload := &yunhubottool.Payload{}
	if err = json.Unmarshal(httpBody, payload); err != nil {
		w.Write([]byte("Bot ErrorData"))
		return
	}
	fmt.Println(string(httpBody))
	switch payload.Header.EventType {
	case "message.receive.normal":
		s.YunHuBOTGroupRun(payload)
		w.Write([]byte("Bot Message"))

	default:
		fmt.Println("QQBot消息类型未支持")
		return
	}

}
