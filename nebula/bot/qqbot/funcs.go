package qqbot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	qqbot_msg "github.com/cjxpj/nebula/bot/qqbot/msg"
	"github.com/cjxpj/nebula/debugLog"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
)

func init() {
	dto.BotFuncsRegistry["QQBot"] = ActiveFuncs
}

// PushContext 存储当前机器人及消息上下文，供 ReplyFuncs 中的回复函数使用
type PushContext struct {
	Bot             *qqbot_msg.RouterQQBot
	MsgID           string
	EventID         string
	GroupOpenID     string
	ChannelID       string
	UserOpenID      string
	PrivateUserID   string // 非空时使用私信发送（机器人退群等场景）
	InteractionCode int    // 按钮交互事件返回码，默认 0 成功
}

// pushCtxKey 是 PushContext 存入词库全局变量（Val.G）的键名。
// 存到词库实例而不是全局单例：多条消息可并发处理，互不串扰上下文。
const pushCtxKey = "_qqbot_pushctx_"

// SetPushContext 将消息上下文挂到当前词库实例上，供 #引入=QQBot 的回复函数使用
func SetPushContext(dic *dic_dto.Dic, ctx *PushContext) {
	if dic == nil || dic.Val == nil || dic.Val.G == nil || ctx == nil {
		return
	}
	dic.Val.G.Set(pushCtxKey, ctx)
}

// GetPushContext 从当前执行的词库变量中获取上下文（嵌套运行共享同一 G，均可取到）
func GetPushContext(d *dto.DicInputs) *PushContext {
	if d == nil || d.V == nil || d.V.G == nil {
		return nil
	}
	ctx, _ := d.V.G.Get(pushCtxKey).(*PushContext)
	return ctx
}

// ConsumeEventID 原子读取并清除 eventID，确保每次 INTERACTION_CREATE 的 event_id 只被使用一次
func ConsumeEventID(d *dto.DicInputs) string {
	if ctx := GetPushContext(d); ctx != nil {
		eid := ctx.EventID
		ctx.EventID = ""
		return eid
	}
	return ""
}

// getBotByIndex 按排序后的账号列表取指定索引的 QQBot（index: 0=第一个, 1=第二个...），不存在返回 nil
func getBotByIndex(index int) *qqbot_msg.RouterQQBot {
	if len(dto.ServerConfig.QQBots) == 0 {
		return nil
	}
	keys := make([]string, 0, len(dto.ServerConfig.QQBots))
	for k, bot := range dto.ServerConfig.QQBots {
		if bot != nil && bot.Open && bot.API != nil {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	if index < 0 || index >= len(keys) {
		return nil
	}
	return dto.ServerConfig.QQBots[keys[index]]
}

// formatMDPair 格式化 MD 模板键值对，绕过渲染限制
func formatMDPair(key, val string) *qqbot_msg.MarkdownParams {
	val = strings.ReplaceAll(val, "\n", "\r")
	val = mdRe.ReplaceAllString(val, "[\r\n$1]($2)")
	val = mdReAt.ReplaceAllString(val, "<$1\r\n$2>")
	val = strings.ReplaceAll(val, "```", "'''")
	if strings.HasPrefix(val, "#") {
		val = " " + val
	}
	return &qqbot_msg.MarkdownParams{
		Key:    key,
		Values: strings.Split(val, "\r\n"),
	}
}

// getBotKey 从 QQBots map 反查 bot 对应的 key，找不到返回空字符串
func getBotKey(bot *qqbot_msg.RouterQQBot) string {
	for k, v := range dto.ServerConfig.QQBots {
		if v == bot {
			return k
		}
	}
	return ""
}

// getSortedBotKeys 返回所有已启用账号的排序后的 key 列表
func getSortedBotKeys() []string {
	if len(dto.ServerConfig.QQBots) == 0 {
		return nil
	}
	keys := make([]string, 0, len(dto.ServerConfig.QQBots))
	for k, bot := range dto.ServerConfig.QQBots {
		if bot != nil && bot.Open && bot.API != nil {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// ========== 用户列表记录 ==========

var recordMu sync.Mutex

// RecordUser 记录用户 ID+昵称到 bot 目录下的 users.json
func RecordUser(bot *qqbot_msg.RouterQQBot, userID, username string) {
	if bot == nil || userID == "" {
		return
	}
	p := filepath.Join(bot.FilePath, "users.json")
	key := userID
	if username != "" {
		key = userID + "\t" + username
	}
	recordMu.Lock()
	defer recordMu.Unlock()
	users := loadStringSet(p)
	if users[key] {
		return // 已存在，跳过写入
	}
	users[key] = true
	saveStringSet(p, users)
}

func loadStringSet(path string) map[string]bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return make(map[string]bool)
	}
	result := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			result[line] = true
		}
	}
	return result
}

func saveStringSet(path string, m map[string]bool) {
	lines := make([]string, 0, len(m))
	for k := range m {
		lines = append(lines, k)
	}
	sort.Strings(lines)
	os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// ========== ActiveFuncs：主动发送（#引入=@QQBot 注入），第一个参数为账号序号（0=第一个） ==========
var ActiveFuncs = map[string]dto.DicFunc{
	"获取账号": {
		L: "0|1",
		Fn: func(d *dto.DicInputs) (any, error) {
			keys := getSortedBotKeys()
			// 第一个参数留空 → 返回全部账号列表 JSON
			if d.Inputs.Len() == 0 || d.Inputs.String(1) == "" {
				list := make([]map[string]string, 0, len(keys))
				for _, k := range keys {
					bot := dto.ServerConfig.QQBots[k]
					list = append(list, map[string]string{
						"name":  k,
						"appid": bot.API.AppId,
						"path":  bot.Addr,
					})
				}
				data, _ := json.Marshal(list)
				return string(data), nil
			}
			// 第一个参数为序号（正整数，0=第一个）
			index := d.Inputs.Int(1)
			bot := getBotByIndex(index)
			if bot == nil {
				return "null", nil
			}
			info := map[string]string{
				"name":  getBotKey(bot),
				"appid": bot.API.AppId,
				"path":  bot.Addr,
			}
			data, _ := json.Marshal(info)
			return string(data), nil
		},
	},
	"搜索账号": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			keyword := d.Inputs.String(1)
			if keyword == "" {
				return `[]`, nil
			}
			keyword = strings.ToLower(keyword)
			result := make([]map[string]string, 0)
			for k, bot := range dto.ServerConfig.QQBots {
				if bot == nil || !bot.Open || bot.API == nil {
					continue
				}
				if strings.Contains(strings.ToLower(k), keyword) ||
					strings.Contains(strings.ToLower(bot.Remark), keyword) ||
					strings.Contains(strings.ToLower(bot.API.AppId), keyword) {
					result = append(result, map[string]string{
						"name":   k,
						"appid":  bot.API.AppId,
						"path":   bot.Addr,
						"remark": bot.Remark,
					})
				}
			}
			data, _ := json.Marshal(result)
			return string(data), nil
		},
	},
	"群单发": {
		L: "3",
		Fn: func(d *dto.DicInputs) (any, error) {
			bot := getBotByIndex(d.Inputs.Int(1))
			if bot == nil {
				return "QQBot未启用或未配置", nil
			}
			groupOpenID := d.Inputs.String(2)
			rMsg := d.Inputs.String(3)
			if rMsg == "" {
				return "内容为空", nil
			}
			rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
			if _, err := bot.API.ReplyGroupMessage("", groupOpenID, rMsg); err != nil {
				return "发送失败: " + err.Error(), nil
			}
			return "发送成功", nil
		},
	},
	"群单发图": {
		L: "4",
		Fn: func(d *dto.DicInputs) (any, error) {
			bot := getBotByIndex(d.Inputs.Int(1))
			if bot == nil {
				return "QQBot未启用或未配置", nil
			}
			if _, err := bot.API.ReplyGroupImgMessage("", d.Inputs.String(2), d.Inputs.String(3), d.Inputs.String(4)); err != nil {
				return "发送失败: " + err.Error(), nil
			}
			return "发送成功", nil
		},
	},
	"群单发MD": {
		L: "3|4",
		Fn: func(d *dto.DicInputs) (any, error) {
			bot := getBotByIndex(d.Inputs.Int(1))
			if bot == nil {
				return "QQBot未启用或未配置", nil
			}
			kb := qqbot_msg.ParseKeyboardJSON(d.Inputs.String(4))
			if _, err := bot.API.ReplyGroupAnyMarkdownWithKeyboard("", d.Inputs.String(2), d.Inputs.String(3), kb); err != nil {
				return "发送失败: " + err.Error(), nil
			}
			return "发送成功", nil
		},
	},
	"群单发语音": {
		L: "3",
		Fn: func(d *dto.DicInputs) (any, error) {
			bot := getBotByIndex(d.Inputs.Int(1))
			if bot == nil {
				return "QQBot未启用或未配置", nil
			}
			if _, err := bot.API.ReplyGroupVoiceMessage("", d.Inputs.String(2), d.Inputs.String(3)); err != nil {
				return "发送失败: " + err.Error(), nil
			}
			return "发送成功", nil
		},
	},
	"群单发视频": {
		L: "3",
		Fn: func(d *dto.DicInputs) (any, error) {
			bot := getBotByIndex(d.Inputs.Int(1))
			if bot == nil {
				return "QQBot未启用或未配置", nil
			}
			if _, err := bot.API.ReplyGroupVideoMessage("", d.Inputs.String(2), d.Inputs.String(3)); err != nil {
				return "发送失败: " + err.Error(), nil
			}
			return "发送成功", nil
		},
	},
	"私聊": {
		L: "3",
		Fn: func(d *dto.DicInputs) (any, error) {
			bot := getBotByIndex(d.Inputs.Int(1))
			if bot == nil {
				return "QQBot未启用或未配置", nil
			}
			rMsg := d.Inputs.String(3)
			if rMsg == "" {
				return "内容为空", nil
			}
			rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
			if _, err := bot.API.ReplyGroupPrivateMessage("", d.Inputs.String(2), rMsg); err != nil {
				return "发送失败: " + err.Error(), nil
			}
			return "发送成功", nil
		},
	},
	"私聊图": {
		L: "4",
		Fn: func(d *dto.DicInputs) (any, error) {
			bot := getBotByIndex(d.Inputs.Int(1))
			if bot == nil {
				return "QQBot未启用或未配置", nil
			}
			if _, err := bot.API.ReplyGroupPrivateImgMessage("", d.Inputs.String(2), d.Inputs.String(3), d.Inputs.String(4)); err != nil {
				return "发送失败: " + err.Error(), nil
			}
			return "发送成功", nil
		},
	},
	"禁": {
		L: "4",
		Fn: func(d *dto.DicInputs) (any, error) {
			bot := getBotByIndex(d.Inputs.Int(1))
			if bot == nil {
				return "", nil
			}
			groupOpenID := d.Inputs.String(2)
			memberOpenID := d.Inputs.String(3)
			seconds := d.Inputs.String(4)
			if err := bot.API.SetMemberMute(groupOpenID, memberOpenID, seconds); err != nil {
				debugLog.Infof("[QQBot] 禁言失败: %v", err)
			}
			return "", nil
		},
	},
	"群信息": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			bot := getBotByIndex(d.Inputs.Int(1))
			if bot == nil {
				return "", nil
			}
			groupOpenID := d.Inputs.String(2)
			info, err := bot.API.GetGroupInfo(groupOpenID)
			if err != nil {
				debugLog.Infof("[QQBot] 获取群信息失败: %v", err)
				return "", nil
			}
			data, _ := json.Marshal(info)
			return string(data), nil
		},
	},
	"入群审批": {
		L: "4|5|6",
		Fn: func(d *dto.DicInputs) (any, error) {
			bot := getBotByIndex(d.Inputs.Int(1))
			if bot == nil {
				return "", nil
			}
			groupOpenID := d.Inputs.String(2)
			memberOpenID := d.Inputs.String(3)
			op := d.Inputs.String(4)
			joinRequestID := d.Inputs.String(5)
			rejectReason := d.Inputs.String(6)
			if err := bot.API.ApproveJoinRequest(groupOpenID, memberOpenID, op, joinRequestID, rejectReason); err != nil {
				debugLog.Infof("[QQBot] 入群审批失败: %v", err)
			}
			return "", nil
		},
	},
	"撤回": {
		L: "3",
		Fn: func(d *dto.DicInputs) (any, error) {
			bot := getBotByIndex(d.Inputs.Int(1))
			if bot == nil {
				return "", nil
			}
			if err := bot.API.RecallGroupMessage(d.Inputs.String(2), d.Inputs.String(3)); err != nil {
				debugLog.Infof("[QQBot] 撤回消息失败: %v", err)
			}
			return "", nil
		},
	},
	"撤回私聊": {
		L: "3",
		Fn: func(d *dto.DicInputs) (any, error) {
			bot := getBotByIndex(d.Inputs.Int(1))
			if bot == nil {
				return "", nil
			}
			if err := bot.API.RecallPrivateMessage(d.Inputs.String(2), d.Inputs.String(3)); err != nil {
				debugLog.Infof("[QQBot] 撤回私聊消息失败: %v", err)
			}
			return "", nil
		},
	},
}

// ========== ReplyFuncs：Bot 消息处理时自动注入（回复发送 + 菜单/面板操作，无需 #引入=） ==========
var ReplyFuncs = map[string]dto.DicFunc{

	"发送文本": {
		L: "1|2",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext(d)
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", fmt.Errorf("QQBot上下文未初始化")
			}
			go func() {
				rMsg := strings.ReplaceAll(d.Inputs.String(1), "\\r", "\n")
				if d.Inputs.LenOk(1) {
					if rMsg != "" {
						if ctx.PrivateUserID != "" {
							ctx.Bot.API.ReplyGroupPrivateMessage(ctx.MsgID, ctx.PrivateUserID, "\n"+rMsg)
						} else {
							ctx.Bot.API.ReplyGroupMessage(ctx.MsgID, ctx.GroupOpenID, "\n"+rMsg, ConsumeEventID(d))
						}
					}
				} else {
					if ctx.PrivateUserID != "" {
						ctx.Bot.API.ReplyGroupPrivateImgMessage(ctx.MsgID, ctx.PrivateUserID, d.Inputs.String(2), rMsg)
					} else {
						ctx.Bot.API.ReplyGroupImgMessage(ctx.MsgID, ctx.GroupOpenID, d.Inputs.String(2), rMsg, ConsumeEventID(d))
					}
				}
			}()
			return "", nil
		},
	},
	"发送MD": {
		L: "1..",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext(d)
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", fmt.Errorf("QQBot上下文未初始化")
			}
			pLen, kb := popMDKeyboard(d)

			// 简单MD文本发送
			if pLen == 1 || (pLen-1)%2 != 0 {
				if ctx.PrivateUserID != "" {
					ctx.Bot.API.ReplyPrivateAnyMarkdownWithKeyboard(ctx.MsgID, ctx.PrivateUserID, d.Inputs.String(1), kb)
				} else {
					ctx.Bot.API.ReplyGroupAnyMarkdownWithKeyboard(ctx.MsgID, ctx.GroupOpenID, d.Inputs.String(1), kb, ConsumeEventID(d))
				}
				return "", nil
			}
			// CustomTemplateId + key=value 参数对
			params := make([]*qqbot_msg.MarkdownParams, 0, (pLen-1)/2)
			for i := 2; i <= pLen; i += 2 {
				params = append(params, formatMDPair(d.Inputs.String(i), d.Inputs.String(i+1)))
			}
			if ctx.PrivateUserID != "" {
				ctx.Bot.API.ReplyPrivateMarkdownWithKeyboard(ctx.MsgID, ctx.PrivateUserID, &qqbot_msg.Markdown{
					CustomTemplateId: d.Inputs.String(1),
					Params:           params,
				}, kb)
			} else {
				ctx.Bot.API.ReplyGroupMarkdownWithKeyboard(ctx.MsgID, ctx.GroupOpenID, &qqbot_msg.Markdown{
					CustomTemplateId: d.Inputs.String(1),
					Params:           params,
				}, kb, ConsumeEventID(d))
			}
			return "", nil
		},
	},
	"发送视频": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext(d)
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", fmt.Errorf("QQBot上下文未初始化")
			}
			go func() {
				if ctx.PrivateUserID != "" {
					ctx.Bot.API.ReplyGroupPrivateVideoMessage(ctx.MsgID, ctx.PrivateUserID, d.Inputs.String(1))
				} else {
					ctx.Bot.API.ReplyGroupVideoMessage(ctx.MsgID, ctx.GroupOpenID, d.Inputs.String(1), ConsumeEventID(d))
				}
			}()
			return "", nil
		},
	},
	"发送语音": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext(d)
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", fmt.Errorf("QQBot上下文未初始化")
			}
			go func() {
				if ctx.PrivateUserID != "" {
					ctx.Bot.API.ReplyGroupPrivateVoiceMessage(ctx.MsgID, ctx.PrivateUserID, d.Inputs.String(1))
				} else {
					ctx.Bot.API.ReplyGroupVoiceMessage(ctx.MsgID, ctx.GroupOpenID, d.Inputs.String(1), ConsumeEventID(d))
				}
			}()
			return "", nil
		},
	},
	"禁": {
		L: "2|3",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext(d)
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", nil
			}
			var groupOpenID, memberOpenID, seconds string
			if d.Inputs.Len() == 3 {
				groupOpenID = d.Inputs.String(1)
				memberOpenID = d.Inputs.String(2)
				seconds = d.Inputs.String(3)
			} else {
				groupOpenID = ctx.GroupOpenID
				memberOpenID = d.Inputs.String(1)
				seconds = d.Inputs.String(2)
			}
			if groupOpenID == "" {
				return "", nil
			}
			if err := ctx.Bot.API.SetMemberMute(groupOpenID, memberOpenID, seconds); err != nil {
				debugLog.Infof("[QQBot] 禁言失败: %v", err)
			}
			return "", nil
		},
	},
	"获取群信息": {
		L: "0|1",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext(d)
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", nil
			}
			groupOpenID := d.Inputs.String(1)
			if groupOpenID == "" {
				groupOpenID = ctx.GroupOpenID
			}
			if groupOpenID == "" {
				return "", nil
			}
			info, err := ctx.Bot.API.GetGroupInfo(groupOpenID)
			if err != nil {
				debugLog.Infof("[QQBot] 获取群信息失败: %v", err)
				return "", nil
			}
			data, _ := json.Marshal(info)
			return string(data), nil
		},
	},
	"入群审批": {
		L: "3|4|5",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext(d)
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", nil
			}
			groupOpenID := d.Inputs.String(1)
			memberOpenID := d.Inputs.String(2)
			op := d.Inputs.String(3)
			joinRequestID := d.Inputs.String(4)
			rejectReason := d.Inputs.String(5)
			if groupOpenID == "" {
				return "", nil
			}
			if err := ctx.Bot.API.ApproveJoinRequest(groupOpenID, memberOpenID, op, joinRequestID, rejectReason); err != nil {
				debugLog.Infof("[QQBot] 入群审批失败: %v", err)
			}
			return "", nil
		},
	},
	"撤回": {
		L: "1|2",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext(d)
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", nil
			}
			var groupOpenID, messageID string
			if d.Inputs.Len() == 2 {
				groupOpenID = d.Inputs.String(1)
				messageID = d.Inputs.String(2)
			} else {
				groupOpenID = ctx.GroupOpenID
				messageID = d.Inputs.String(1)
			}
			if groupOpenID == "" || messageID == "" {
				return "", nil
			}
			if err := ctx.Bot.API.RecallGroupMessage(groupOpenID, messageID); err != nil {
				debugLog.Infof("[QQBot] 撤回消息失败: %v", err)
			}
			return "", nil
		},
	},
	"撤回私聊": {
		L: "1|2",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext(d)
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", nil
			}
			var userOpenID, messageID string
			if d.Inputs.Len() == 2 {
				userOpenID = d.Inputs.String(1)
				messageID = d.Inputs.String(2)
			} else {
				userOpenID = ctx.UserOpenID
				if userOpenID == "" {
					userOpenID = ctx.PrivateUserID
				}
				messageID = d.Inputs.String(1)
			}
			if userOpenID == "" || messageID == "" {
				return "", nil
			}
			if err := ctx.Bot.API.RecallPrivateMessage(userOpenID, messageID); err != nil {
				debugLog.Infof("[QQBot] 撤回私聊消息失败: %v", err)
			}
			return "", nil
		},
	},
	"获取菜单": {
		L: "0",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext(d)
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", nil
			}
			resp, err := ctx.Bot.API.GetMenu()
			if err != nil {
				return "获取失败: " + err.Error(), nil
			}
			data, _ := json.Marshal(resp)
			return string(data), nil
		},
	},
	"设置菜单": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext(d)
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", nil
			}
			menu, err := qqbot_msg.ParseMenuJSON(d.Inputs.String(1))
			if err != nil {
				return "菜单JSON解析失败: " + err.Error(), nil
			}
			resp, err := ctx.Bot.API.SetMenu(menu)
			if err != nil {
				return "设置失败: " + err.Error(), nil
			}
			data, _ := json.Marshal(resp)
			return string(data), nil
		},
	},
	"面板列表": {
		L: "1|2|3",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext(d)
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", nil
			}
			resp, err := ctx.Bot.API.GetPanels(d.Inputs.String(1), d.Inputs.String(2), d.Inputs.Int(3))
			if err != nil {
				return "获取失败: " + err.Error(), nil
			}
			data, _ := json.Marshal(resp)
			return string(data), nil
		},
	},
	"创建面板": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext(d)
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", nil
			}
			var req qqbot_msg.CreatePanelRequest
			if err := json.Unmarshal([]byte(d.Inputs.String(1)), &req); err != nil {
				return "面板JSON解析失败: " + err.Error(), nil
			}
			resp, err := ctx.Bot.API.CreatePanel(&req)
			if err != nil {
				return "创建失败: " + err.Error(), nil
			}
			data, _ := json.Marshal(resp)
			return string(data), nil
		},
	},
	"面板详情": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext(d)
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", nil
			}
			resp, err := ctx.Bot.API.GetPanel(d.Inputs.String(1))
			if err != nil {
				return "获取失败: " + err.Error(), nil
			}
			data, _ := json.Marshal(resp)
			return string(data), nil
		},
	},
	"修改面板": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext(d)
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", nil
			}
			panel, err := qqbot_msg.ParsePanelJSON(d.Inputs.String(2))
			if err != nil {
				return "面板JSON解析失败: " + err.Error(), nil
			}
			resp, err := ctx.Bot.API.UpdatePanel(d.Inputs.String(1), &qqbot_msg.UpdatePanelRequest{Panel: panel})
			if err != nil {
				return "修改失败: " + err.Error(), nil
			}
			data, _ := json.Marshal(resp)
			return string(data), nil
		},
	},
	"删除面板": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext(d)
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", nil
			}
			if err := ctx.Bot.API.DeletePanel(d.Inputs.String(1)); err != nil {
				return "删除失败: " + err.Error(), nil
			}
			return "删除成功", nil
		},
	},
	"面板关联": {
		L: "3",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext(d)
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", nil
			}
			op := d.Inputs.String(2)
			switch op {
			case "添加", "增加":
				op = "add"
			case "删除", "移除":
				op = "del"
			}
			var req qqbot_msg.UpdatePanelTargetRequest
			if err := json.Unmarshal([]byte(d.Inputs.String(3)), &req); err != nil {
				return "关联JSON解析失败: " + err.Error(), nil
			}
			req.Op = op
			if err := ctx.Bot.API.UpdatePanelTarget(d.Inputs.String(1), &req); err != nil {
				return "操作失败: " + err.Error(), nil
			}
			return "操作成功", nil
		},
	},
}
