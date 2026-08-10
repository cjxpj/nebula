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
	"github.com/cjxpj/nebula/dto"
)

func init() {
	dto.BotFuncsRegistry["QQBot"] = ActiveFuncs
}

// PushContext 存储当前机器人及消息上下文，供 ReplyFuncs 中的回复函数使用
type PushContext struct {
	Bot         *qqbot_msg.RouterQQBot
	MsgID       string
	GroupOpenID string
	ChannelID   string
	UserOpenID  string
}

var (
	pushMu      sync.RWMutex
	currentPush *PushContext
)

// SetPushContext 设置当前上下文
func SetPushContext(ctx *PushContext) {
	pushMu.Lock()
	defer pushMu.Unlock()
	currentPush = ctx
}

// ClearPushContext 清除当前上下文
func ClearPushContext() {
	pushMu.Lock()
	defer pushMu.Unlock()
	currentPush = nil
}

// GetPushContext 获取当前上下文
func GetPushContext() *PushContext {
	pushMu.RLock()
	defer pushMu.RUnlock()
	return currentPush
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

// ========== 群列表/用户列表记录 ==========

var recordMu sync.Mutex

// RecordGroup 记录群号到 bot 目录下的 groups.json
func RecordGroup(bot *qqbot_msg.RouterQQBot, groupOpenID string) {
	if bot == nil || groupOpenID == "" {
		return
	}
	p := filepath.Join(bot.FilePath, "groups.json")
	recordMu.Lock()
	defer recordMu.Unlock()
	groups := loadStringSet(p)
	if groups[groupOpenID] {
		return // 已存在，跳过写入
	}
	groups[groupOpenID] = true
	saveStringSet(p, groups)
}

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

// GetRecordedGroups 获取已记录的群列表
func GetRecordedGroups(bot *qqbot_msg.RouterQQBot) []string {
	if bot == nil {
		return nil
	}
	groups := loadStringSet(filepath.Join(bot.FilePath, "groups.json"))
	result := make([]string, 0, len(groups))
	for g := range groups {
		result = append(result, g)
	}
	sort.Strings(result)
	return result
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
	"群发": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			bot := getBotByIndex(d.Inputs.Int(1))
			if bot == nil {
				return "QQBot未启用或未配置", nil
			}
			rMsg := d.Inputs.String(2)
			if rMsg == "" {
				return "内容为空", nil
			}
			rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
			groups := GetRecordedGroups(bot)
			if len(groups) == 0 {
				return "没有已记录的群", nil
			}
			var success, fail int
			for _, g := range groups {
				if _, err := bot.API.ReplyGroupMessage("", g, rMsg); err != nil {
					fail++
				} else {
					success++
				}
			}
			return fmt.Sprintf("群发完成: 成功%d 失败%d", success, fail), nil
		},
	},
	"群发MD": {
		L: "2..",
		Fn: func(d *dto.DicInputs) (any, error) {
			bot := getBotByIndex(d.Inputs.Int(1))
			if bot == nil {
				return "QQBot未启用或未配置", nil
			}
			groups := GetRecordedGroups(bot)
			if len(groups) == 0 {
				return "没有已记录的群", nil
			}

			// 提取键盘参数（从索引3扫描"按钮"，账号序号占索引1）
			l := d.Inputs.Len()
			contentEnd := l
			var kb *qqbot_msg.Keyboard
			for i := 3; i <= l; i++ {
				if d.Inputs.String(i) == "按钮" {
					if i+1 <= l {
						kb = parseTextButtons(d, i+1, l)
					}
					contentEnd = i - 1
					break
				}
			}
			pLen := contentEnd - 1 // 内容参数个数（去掉账号序号）

			var success, fail int
			if pLen == 1 {
				// 简单MD文本发送
				text := d.Inputs.String(2)
				for _, g := range groups {
					if _, err := bot.API.ReplyGroupAnyMarkdownWithKeyboard("", g, text, kb); err != nil {
						fail++
					} else {
						success++
					}
				}
			} else {
				// CustomTemplateId + key=value 参数对
				list := d.Inputs.StringAfterList(3)
				n := len(list)
				if n > pLen-1 {
					n = pLen - 1
				}
				if n%2 != 0 {
					n--
				}
				params := make([]*qqbot_msg.MarkdownParams, 0, n/2)
				for i := 0; i < n; i += 2 {
					params = append(params, formatMDPair(list[i], list[i+1]))
				}
				md := &qqbot_msg.Markdown{
					CustomTemplateId: d.Inputs.String(2),
					Params:           params,
				}
				for _, g := range groups {
					if _, err := bot.API.ReplyGroupMarkdownWithKeyboard("", g, md, kb); err != nil {
						fail++
					} else {
						success++
					}
				}
			}
			return fmt.Sprintf("群发MD完成: 成功%d 失败%d", success, fail), nil
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
		L: "4|5",
		Fn: func(d *dto.DicInputs) (any, error) {
			bot := getBotByIndex(d.Inputs.Int(1))
			if bot == nil {
				return "", nil
			}
			groupOpenID := d.Inputs.String(2)
			memberOpenID := d.Inputs.String(3)
			op := d.Inputs.String(4)
			joinRequestID := d.Inputs.String(5)
			if err := bot.API.ApproveJoinRequest(groupOpenID, memberOpenID, op, joinRequestID, "", false); err != nil {
				debugLog.Infof("[QQBot] 入群审批失败: %v", err)
			}
			return "", nil
		},
	},
}

// ========== ReplyFuncs：回复发送（依赖 PushContext，用于 Bot 消息处理） ==========
var ReplyFuncs = map[string]dto.DicFunc{

	"发送文本": {
		L: "1|2",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext()
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", fmt.Errorf("QQBot上下文未初始化")
			}
			go func() {
				rMsg := strings.ReplaceAll(d.Inputs.String(1), "\\r", "\n")
				if d.Inputs.LenOk(1) {
					if rMsg != "" {
						ctx.Bot.API.ReplyGroupMessage(ctx.MsgID, ctx.GroupOpenID, "\n"+rMsg)
					}
				} else {
					ctx.Bot.API.ReplyGroupImgMessage(ctx.MsgID, ctx.GroupOpenID, d.Inputs.String(2), rMsg)
				}
			}()
			return "", nil
		},
	},
	"发送MD": {
		L: "1..",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext()
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", fmt.Errorf("QQBot上下文未初始化")
			}
			pLen, kb := popMDKeyboard(d)

			// 简单MD文本发送
			if pLen == 1 {
				ctx.Bot.API.ReplyGroupAnyMarkdownWithKeyboard(ctx.MsgID, ctx.GroupOpenID, d.Inputs.String(1), kb)
				return "", nil
			}
			// CustomTemplateId + key=value 参数对
			list := d.Inputs.StringAfterList(2)
			n := len(list)
			if n > pLen-1 {
				n = pLen - 1
			}
			if n%2 != 0 {
				n--
			}
			params := make([]*qqbot_msg.MarkdownParams, 0, n/2)
			for i := 0; i < n; i += 2 {
				params = append(params, formatMDPair(list[i], list[i+1]))
			}
			ctx.Bot.API.ReplyGroupMarkdownWithKeyboard(ctx.MsgID, ctx.GroupOpenID, &qqbot_msg.Markdown{
				CustomTemplateId: d.Inputs.String(1),
				Params:           params,
			}, kb)
			return "", nil
		},
	},
	"发送视频": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext()
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", fmt.Errorf("QQBot上下文未初始化")
			}
			go func() {
				ctx.Bot.API.ReplyGroupVideoMessage(ctx.MsgID, ctx.GroupOpenID, d.Inputs.String(1))
			}()
			return "", nil
		},
	},
	"发送语音": {
		L: "1",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext()
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", fmt.Errorf("QQBot上下文未初始化")
			}
			go func() {
				ctx.Bot.API.ReplyGroupVoiceMessage(ctx.MsgID, ctx.GroupOpenID, d.Inputs.String(1))
			}()
			return "", nil
		},
	},
	"禁": {
		L: "2|3",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext()
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
			ctx := GetPushContext()
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
		L: "2|3",
		Fn: func(d *dto.DicInputs) (any, error) {
			ctx := GetPushContext()
			if ctx == nil || ctx.Bot == nil || ctx.Bot.API == nil {
				return "", nil
			}
			var groupOpenID, memberOpenID, op string
			if d.Inputs.Len() == 3 {
				groupOpenID = d.Inputs.String(1)
				memberOpenID = d.Inputs.String(2)
				op = d.Inputs.String(3)
			} else {
				groupOpenID = ctx.GroupOpenID
				memberOpenID = d.Inputs.String(1)
				op = d.Inputs.String(2)
			}
			if groupOpenID == "" {
				return "", nil
			}
			if err := ctx.Bot.API.ApproveJoinRequest(groupOpenID, memberOpenID, op, "", "", false); err != nil {
				debugLog.Infof("[QQBot] 入群审批失败: %v", err)
			}
			return "", nil
		},
	},
}
