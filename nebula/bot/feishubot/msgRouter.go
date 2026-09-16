package feishubot

import (
	"encoding/json"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/cjxpj/nebula/debugLog"

	feishubot_msg "github.com/cjxpj/nebula/bot/feishubot/msg"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// isAdmin 判断用户是否在主人（管理员）列表中，是则返回匹配到的条目，否则返回 "null"
func isAdmin(userID string) string {
	adminList, err := utils.NewFileQueue(path.Join(dto.ServerConfig.FeiShuBot.FilePath, "admin.txt")).ReadFromFile()
	if err != nil {
		return "null"
	}
	for s := range strings.SplitSeq(adminList, ",") {
		if id := strings.TrimSpace(s); id != "" && userID == id {
			return id
		}
	}
	return "null"
}

// runDic 遍历词库目录执行词库，reply 为回复函数
func runDic(valData *dto.Val, content string, reply func(string)) {
	botDicList, err := utils.NewFileQueue(path.Join(dto.ServerConfig.FeiShuBot.FilePath, "dic")).GetFileList()
	if err != nil {
		return
	}

	for _, v := range botDicList {
		if !strings.HasSuffix(v, ".n") {
			continue
		}
		go func() {
			dicPath := path.Join(dto.ServerConfig.FeiShuBot.FilePath, "dic", v)
			FileData, err := utils.NewFileQueue(dicPath).ReadFromFile()
			if err != nil {
				return
			}

			dic := dic_dto.NewDic(dicPath, FileData).
				SetGlobal_v(valData)

			// 延迟回复
			dic.SetFunc("调用", dto.DicFunc{
				L: "2..",
				Fn: func(d *dto.DicInputs) (any, error) {
					go func() {
						qqVal := dic.NewDicVal()
						sleepTime := d.Inputs.Int(1)
						time.Sleep(time.Duration(sleepTime) * time.Millisecond)
						rMsg := dic_api.Api.DicRunPrivateVal(dic, d.Inputs.StringAfter(2), qqVal)
						if rMsg != "" {
							reply(strings.ReplaceAll(rMsg, "\\r", "\n"))
						}
					}()
					return "", nil
				}})

			dic.AddFuncs(Funcs)

			rMsg := dic_api.Api.DicRun(dic, content)
			if rMsg != "" {
				reply(strings.ReplaceAll(rMsg, "\\r", "\n"))
			}
		}()
	}
}

func groupMsg(m *feishubot_msg.ImMessageReceiveV1) {
	// 用户
	userID := m.Event.Sender.SenderID.OpenID
	// 群号
	groupID := m.Event.Message.ChatID
	// 消息ID
	msgID := m.Event.Message.MessageID

	valData := dto.NewVal().
		Set("来源", "群聊").
		Set("群号", groupID).
		Set("QQ", userID).
		Set("MsgId", msgID).
		Set("MessageID", msgID).
		Set("主人", isAdmin(userID))

	runDic(valData, extractText(m.Event.Message.Content), func(rMsg string) {
		if _, err := SendGroupMsg(groupID, rMsg); err != nil {
			debugLog.Infof("%v", err)
		}
	})
}

func p2pMsg(m *feishubot_msg.ImMessageReceiveV1) {
	// 用户
	userID := m.Event.Sender.SenderID.OpenID
	// 消息ID
	msgID := m.Event.Message.MessageID

	valData := dto.NewVal().
		Set("来源", "私聊").
		Set("群号", "0").
		Set("QQ", userID).
		Set("MsgId", msgID).
		Set("MessageID", msgID).
		Set("主人", isAdmin(userID))

	runDic(valData, extractText(m.Event.Message.Content), func(rMsg string) {
		if _, err := SendPrivateMsg(userID, rMsg); err != nil {
			debugLog.Infof("%v", err)
		}
	})
}

// BotMessage 统一入口：只负责路由层逻辑
func BotMessage(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		debugLog.Info("read body failed:", err)
		return
	}
	defer r.Body.Close()

	debugLog.Infof("%v", string(body))

	plain, err := decryptIfNeeded(body)
	if err != nil {
		debugLog.Info("decrypt failed:", err)
		http.Error(w, "decrypt failed", http.StatusBadRequest)
		return
	}

	// 处理 URL 验证
	var verify feishubot_msg.SlackURLVerification
	if err := json.Unmarshal(plain, &verify); err == nil && verify.Type == "url_verification" {
		if !checkToken(verify.Token) {
			debugLog.Info("飞书验证令牌不匹配")
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"challenge": verify.Challenge})
		return
	}

	// 处理事件
	var ev feishubot_msg.ImMessageReceiveV1
	if err := json.Unmarshal(plain, &ev); err != nil {
		debugLog.Info("parse failed:", err)
		return
	}
	if !checkToken(ev.Header.Token) {
		debugLog.Info("飞书验证令牌不匹配")
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	switch ev.Event.Message.ChatType {
	case "group":
		groupMsg(&ev)
	case "p2p":
		p2pMsg(&ev)
	}
}
