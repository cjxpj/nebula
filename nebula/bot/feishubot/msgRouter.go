package feishubot

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	feishubot_msg "github.com/cjxpj/nebula/bot/feishubot/msg"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

func groupMsg(m *feishubot_msg.ImMessageReceiveV1) {
	botDicPath := utils.NewFileQueue(dto.ServerConfig.NapCatBot.FilePath + "/dic")
	botDicList, err := botDicPath.GetFileList()
	if err != nil {
		return
	}

	// 取出需要的数据
	// QQ
	userID := m.Event.Sender.SenderID.OpenID
	// 群号
	groupID := m.Event.Message.ChatID
	// 消息ID
	msgID := m.Event.Message.MessageID

	isAdmin := "null" // 是否是管理员
	// 主人列表
	if adminList, err := utils.NewFileQueue(dto.ServerConfig.NapCatBot.FilePath + "/admin.txt").ReadFromFile(); err == nil {
		for s := range strings.SplitSeq(adminList, ",") {
			id := strings.TrimSpace(s)
			if userID == id {
				isAdmin = s
				break
			}
		}
	}

	content := extractText(m.Event.Message.Content)

	valData := dto.NewVal().
		Set("来源", "群聊").
		Set("群号", groupID).
		Set("QQ", userID).
		Set("MsgId", msgID).
		Set("MessageID", msgID).
		Set("主人", isAdmin)

	for _, v := range botDicList {
		go func() {
			FileData, err := utils.NewFileQueue(dto.ServerConfig.FeiShuBot.FilePath + "/dic/" + v).ReadFromFile()
			if err != nil {
				return
			}

			// 回复消息
			dic := dic_dto.NewDic(dto.ServerConfig.FeiShuBot.FilePath, FileData).
				SetGlobal_v(valData)

			dic.SetFunc("调用", dto.DicFunc{
				L: "2..",
				Fn: func(d *dto.DicInputs) (any, error) {
					go func() {
						qqVal := dic.NewDicVal()
						sleepTime := d.Inputs.Int(1)
						time.Sleep(time.Duration(sleepTime) * time.Millisecond)
						rMsg := dic_api.Api.DicRunPrivateVal(dic, d.Inputs.StringAfter(2), qqVal)
						if rMsg != "" {
							rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
							SendGroupMsg(groupID, rMsg)
						}
					}()
					return "", nil
				}})

			dic.AddFuncs(Funcs)

			rMsg := dic_api.Api.DicRun(dic, content)
			if rMsg != "" {
				rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
				fmt.Println(rMsg)
				_, err := SendGroupMsg(groupID, rMsg)
				if err != nil {
					fmt.Println(err)
				}
			}
		}()
	}
}

func p2pMsg(m *feishubot_msg.ImMessageReceiveV1) {
	fmt.Println("私聊", m)
}

// BotMessage 统一入口：只负责路由层逻辑
func BotMessage(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Println("read body failed:", err)
		return
	}
	defer r.Body.Close()

	fmt.Println(string(body))

	// 处理验证码
	plain, err := parseAndDecrypt(body)
	if err == nil {
		// URL 验证
		if plain.Type == "url_verification" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"challenge": plain.Challenge})
			return
		}
	}

	// 处理事件
	ev, err := parseGroupMessage(body)
	if err != nil {
		log.Println("parse failed:", err)
		return
	}
	switch ev.Event.Message.ChatType {
	case "group":
		groupMsg(ev)
	case "p2p":
		p2pMsg(ev)
	}
}
