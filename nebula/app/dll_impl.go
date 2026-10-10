//go:build dll

// dll_impl.go 承载 c-shared 构建（-tags dll）下全部导出能力的纯 Go 实现。
// 本文件不 import "C"，只做业务编排；对外的 C ABI 壳子见 main_dll.go。
//
// 设计要点：
//   - 宿主与 DLL 之间只传字符串（JSON），不跨边界传 Go 指针/闭包/http 对象；
//   - 词库执行过程中的事件（收到消息、发送消息、发送完成）不通过 C 回调，
//     而是写入 DLL 内部队列，由宿主轮询 dllPollEvent 取出；
//   - 所有可能执行不可信词库的入口都用 guarded 包裹，把 panic 转成错误信封。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/bot/botdic"
	qqbot "github.com/cjxpj/nebula/bot/qqbot"
	qqbot_msg "github.com/cjxpj/nebula/bot/qqbot/msg"
	"github.com/cjxpj/nebula/debugLog"
	_ "github.com/cjxpj/nebula/dic" // 触发引擎初始化：注入 dic_api.Api 并注册内置函数
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dic/sandbox"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"
	"github.com/cjxpj/nebula/utils"
)

// init 在 DLL 加载时把引擎全局并行度放开到宿主机逻辑核数，
// 使「按账号分配 N 核」真正兑现：账号级并发仍由 accountstat.go 的信号量限制，
// 这里只设整机总闸——全部词库执行（QQ 机器人、网站词库、调试沙箱及 legacy
// RunN/RunCompiled）合计最多用满整机，由 OS 与宿主进程公平抢占。
// 宿主是独立进程，其运行时不受影响。
func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

// ================= 初始化 =================

type initConfig struct {
	BotsRoot string `json:"botsRoot"` // 词库根目录（沙箱工作目录）
	LogDir   string `json:"logDir"`   // 引擎日志目录
	Debug    bool   `json:"debug"`    // 调试开关
	// AccountLayout 账号目录布局（各固定子目录名），由宿主自定义下发；
	// 为空时引擎使用内置默认值（见 utils.DefaultAccountLayout）。
	AccountLayout utils.AccountLayout `json:"accountLayout"`
}

var initOnce sync.Once

func dllInit(cfgJSON string) string {
	return guarded(func() string {
		var cf initConfig
		if strings.TrimSpace(cfgJSON) != "" {
			if err := json.Unmarshal([]byte(cfgJSON), &cf); err != nil {
				return errEnv("初始化配置解析失败: " + err.Error())
			}
		}
		initOnce.Do(func() {
			// 账号目录布局（固定子目录名）由宿主自定义下发，引擎不再硬编码
			utils.SetAccountLayout(cf.AccountLayout)
			// 收紧词库执行边界：限定工作目录、注销高危函数、禁止出网访问内网
			if removed := sandbox.Enable(cf.BotsRoot); len(removed) > 0 {
				debugLog.Infof("[nebula] 已禁用 %d 个高危词库函数", len(removed))
			}
			if cf.LogDir != "" {
				utils.SetLogDir(cf.LogDir)
			}
			// 词库编译磁盘缓存：账号内所有词库统一放到「私有/.dic_cache」（与引擎默认目录一致）
			dto.ServerConfig.DicCache = true
			run.SetDicCacheDirFunc(func(dicPath string) string {
				// 账号内所有词库（机器人词库、网站词库、网站目录等）共用一个缓存目录，
				// 避免缓存散落到各机器人目录，或混进网站词库目录被当作词库列出。
				if accountDir, ok := utils.AccountRootOf(dicPath); ok {
					return filepath.Join(accountDir, utils.PrivateDirName(), ".dic_cache")
				}
				return filepath.Join(filepath.Dir(dicPath), ".dic_cache")
			})
			// 事件回调：改成写入内部队列，宿主轮询取走
			qqbot.OnRecv = dllOnRecv
			// 账号维度包裹一次消息分发：并发限制 + 耗时统计（见 accountstat.go）
			qqbot.OnDispatch = dllOnDispatch
			qqbot_msg.OnSend = dllOnSend
			qqbot_msg.OnSendResult = dllOnSendResult
		})
		debugLog.SetDebug(cf.Debug)
		return okEnv(map[string]any{"version": appfiles.Version})
	})
}

func dllVersionString() string {
	return okEnv(appfiles.Version)
}

func dllSetDebug(on int) string {
	debugLog.SetDebug(on != 0)
	return okEnv(nil)
}

// ================= 机器人注册表 =================

// botOptions 是机器人上线的可选开关（宿主经 NebulaBotStart 的 cfgJSON 传入）。
// 字段省略时沿用引擎默认值：全量艾特兼容 / 过滤开头斜杠开启，其余关闭。
type botOptions struct {
	AtCompat    bool   `json:"atCompat"`    // 全量消息艾特兼容
	FilterSlash bool   `json:"filterSlash"` // 过滤开头斜杠指令前缀
	Debug       bool   `json:"debug"`       // 调试打印
	Robot       string `json:"robot"`       // 自定义 robot 变量值（union_openid 取不到时使用）
	WsIntents   int    `json:"wsIntents"`   // WebSocket 监听码（0=按公域/私域自动探测）
}

// parseBotOptions 解析宿主下发的开关 JSON；空串或缺省字段沿用引擎默认值。
func parseBotOptions(cfgJSON string) (botOptions, error) {
	// 先填默认值，再由 JSON 覆盖出现的字段
	opts := botOptions{AtCompat: true, FilterSlash: true}
	if strings.TrimSpace(cfgJSON) != "" {
		if err := json.Unmarshal([]byte(cfgJSON), &opts); err != nil {
			return opts, err
		}
	}
	return opts, nil
}

type botEntry struct {
	id       int64
	appID    string
	secret   string
	name     string
	filePath string
	opts     botOptions
	router   *qqbot_msg.RouterQQBot
}

var (
	regMu    sync.Mutex
	reg      = map[int64]*botEntry{}
	byAppID  = map[string]*botEntry{}
	byRouter = map[*qqbot_msg.RouterQQBot]*botEntry{}
)

// dllOnDispatch 在机器人消息分发前按所属账号开始一次「账号级」执行：
// 由 router 反查 botEntry.filePath（.../<uid>/机器人词库/<botid>）→ 账号 uid → 取令牌计时。
// 返回的 done 在分发结束后由 wsDispatch 调用。无法反推账号时返回 nil（不限制、不统计）。
func dllOnDispatch(bot *qqbot_msg.RouterQQBot) func() {
	regMu.Lock()
	e := byRouter[bot]
	var path string
	if e != nil {
		path = e.filePath
	}
	regMu.Unlock()
	if path == "" {
		return nil
	}
	return accountBegin(path)
}

func dllBotStart(id int64, appID, secret, name, filePath, cfgJSON string) string {
	return guarded(func() string {
		if appID == "" || secret == "" {
			return errEnv("机器人 AppID 或密钥为空")
		}
		if filePath == "" {
			return errEnv("机器人目录为空")
		}
		opts, err := parseBotOptions(cfgJSON)
		if err != nil {
			return errEnv("机器人配置解析失败: " + err.Error())
		}
		if err := os.MkdirAll(filepath.Join(filePath, utils.DicDirName()), 0o755); err != nil {
			return errEnv(err.Error())
		}

		regMu.Lock()
		old := reg[id]
		// 已在运行且配置未变：幂等返回
		if old != nil && old.appID == appID && old.secret == secret && old.name == name && old.filePath == filePath && old.opts == opts {
			regMu.Unlock()
			return okEnv(nil)
		}
		// 配置变更：摘除旧实例，用新配置重建
		if old != nil {
			delete(reg, id)
			delete(byRouter, old.router)
			delete(byAppID, old.appID)
		}
		api := qqbot_msg.NewQQBot(appID, secret)
		api.Debug = opts.Debug
		router := &qqbot_msg.RouterQQBot{
			Open:        true,
			FilePath:    filePath,
			LastMsg:     cache.New(time.Hour, time.Minute),
			API:         api,
			Remark:      name,
			AtCompat:    opts.AtCompat,
			FilterSlash: opts.FilterSlash,
			Debug:       opts.Debug,
			Robot:       opts.Robot,
			WsIntents:   opts.WsIntents,
		}
		e := &botEntry{id: id, appID: appID, secret: secret, name: name, filePath: filePath, opts: opts, router: router}
		reg[id] = e
		byRouter[router] = e
		byAppID[appID] = e
		regMu.Unlock()

		if old != nil {
			qqbot_msg.StopWsFunc(old.router)
		}
		qqbot_msg.StartWsFunc(router)
		return okEnv(nil)
	})
}

func dllBotStop(id int64) string {
	return guarded(func() string {
		regMu.Lock()
		e := reg[id]
		if e != nil {
			delete(reg, id)
			delete(byRouter, e.router)
			delete(byAppID, e.appID)
		}
		regMu.Unlock()
		if e != nil {
			qqbot_msg.StopWsFunc(e.router)
			// 清除该机器人的外部词库注册，避免机器人 ID 复用时残留
			botdic.SetExtraDics(e.filePath, nil)
		}
		return okEnv(nil)
	})
}

func dllBotStopAll() string {
	return guarded(func() string {
		regMu.Lock()
		list := make([]*botEntry, 0, len(reg))
		for _, e := range reg {
			list = append(list, e)
		}
		reg = map[int64]*botEntry{}
		byAppID = map[string]*botEntry{}
		byRouter = map[*qqbot_msg.RouterQQBot]*botEntry{}
		regMu.Unlock()
		for _, e := range list {
			qqbot_msg.StopWsFunc(e.router)
			botdic.SetExtraDics(e.filePath, nil)
		}
		return okEnv(nil)
	})
}

func dllBotOnline(id int64) string {
	regMu.Lock()
	e := reg[id]
	regMu.Unlock()
	if e == nil {
		return okEnv(false)
	}
	e.router.WsMutex.Lock()
	defer e.router.WsMutex.Unlock()
	return okEnv(e.router.WsConn != nil && e.router.WsSessionID != "")
}

func dllBotRunning(id int64) string {
	regMu.Lock()
	_, ok := reg[id]
	regMu.Unlock()
	return okEnv(ok)
}

// extraDicReq 是宿主下发的「外部词库」条目：
// Virtual 为账号布局内的绝对虚拟路径（用于账号上下文推断），Real 为可读的真实绝对路径。
type extraDicReq struct {
	Virtual string `json:"virtual"`
	Real    string `json:"real"`
}

// dllBotSetExtraDics 为已启动的机器人设置外部词库（商城/云词库等，不落盘到账号目录）。
// 机器人重启后需由宿主重新下发。
func dllBotSetExtraDics(id int64, reqJSON string) string {
	return guarded(func() string {
		regMu.Lock()
		e := reg[id]
		regMu.Unlock()
		if e == nil {
			return errEnv("机器人未启动")
		}
		list := []extraDicReq{}
		if strings.TrimSpace(reqJSON) != "" {
			if err := json.Unmarshal([]byte(reqJSON), &list); err != nil {
				return errEnv("外部词库配置解析失败: " + err.Error())
			}
		}
		dics := make([]botdic.ExtraDic, 0, len(list))
		for _, d := range list {
			if d.Virtual == "" || d.Real == "" {
				continue
			}
			dics = append(dics, botdic.ExtraDic{Virtual: d.Virtual, Real: d.Real})
		}
		botdic.SetExtraDics(e.filePath, dics)
		return okEnv(len(dics))
	})
}

func dllBotRemove(id int64, filePath string) string {
	return guarded(func() string {
		regMu.Lock()
		e := reg[id]
		if e != nil {
			delete(reg, id)
			delete(byRouter, e.router)
			delete(byAppID, e.appID)
			if filePath == "" {
				filePath = e.filePath
			}
		}
		regMu.Unlock()
		if e != nil {
			qqbot_msg.StopWsFunc(e.router)
		}
		// 释放该机器人的线程变量，避免机器人 ID 被复用时继承旧变量
		if filePath != "" {
			dto.ReleaseBotThreadVars(filePath)
			botdic.SetExtraDics(filePath, nil)
		}
		return okEnv(nil)
	})
}

// botAPI 返回可用发送实例：在线复用运行实例，离线用传入凭据临时创建。
func botAPI(id int64, appID, secret string) (*qqbot_msg.QQBot, error) {
	regMu.Lock()
	e := reg[id]
	regMu.Unlock()
	if e != nil {
		return e.router.API, nil
	}
	if appID == "" || secret == "" {
		return nil, fmt.Errorf("机器人 AppID 或密钥为空")
	}
	return qqbot_msg.NewQQBot(appID, secret), nil
}

func dllRecall(id int64, appID, secret, scene, target, msgID string) string {
	return guarded(func() string {
		if strings.TrimSpace(msgID) == "" {
			return errEnv("该消息缺少消息 ID，无法撤回")
		}
		api, err := botAPI(id, appID, secret)
		if err != nil {
			return errEnv(err.Error())
		}
		switch scene {
		case "group":
			if target == "" {
				return errEnv("缺少群标识")
			}
			if err := api.RecallGroupMessage(target, msgID); err != nil {
				return errEnv(err.Error())
			}
		case "c2c":
			if target == "" {
				return errEnv("缺少用户标识")
			}
			if err := api.RecallPrivateMessage(target, msgID); err != nil {
				return errEnv(err.Error())
			}
		default:
			return errEnv("该场景不支持撤回")
		}
		return okEnv(nil)
	})
}

func dllReply(id int64, appID, secret, scene, target, msgID, content string) string {
	return guarded(func() string {
		content = strings.TrimSpace(content)
		if content == "" {
			return errEnv("消息内容不能为空")
		}
		if target == "" {
			return errEnv("缺少会话标识")
		}
		api, err := botAPI(id, appID, secret)
		if err != nil {
			return errEnv(err.Error())
		}
		switch scene {
		case "group":
			_, err = api.ReplyGroupMessage(msgID, target, content)
		case "c2c":
			_, err = api.ReplyGroupPrivateMessage(msgID, target, content)
		case "channel":
			_, err = api.ReplyChannelMessage(msgID, target, content)
		case "dm":
			_, err = api.ReplyPrivateMessage(msgID, target, content)
		default:
			return errEnv("未知会话场景")
		}
		if err != nil {
			return errEnv(err.Error())
		}
		return okEnv(nil)
	})
}

// ================= 词库执行 =================

func dllDicRun(path, content, trigger string) string {
	return guarded(func() string {
		if done := accountBegin(path); done != nil {
			defer done()
		}
		d := dic_dto.NewDic(path, content)
		return okEnv(dic_api.Api.DicRun(d, trigger))
	})
}

func dllWebDicRun(path, content string) string {
	return guarded(func() string {
		if done := accountBegin(path); done != nil {
			defer done()
		}
		wd := dic_dto.NewWebDic(path, content)
		return okEnv(dic_api.Api.WebDicRun(wd))
	})
}

type dicInfoResult struct {
	Name  string `json:"name"`
	Desc  string `json:"desc"`
	Price int64  `json:"price"`
}

func dllDicReadInfo(path, content string) string {
	return guarded(func() string {
		info, err := dic_api.Api.DicReadInfo(dic_dto.NewDic(path, content))
		if err != nil {
			return errEnv(err.Error())
		}
		return okEnv(dicInfoResult{Name: info.Name, Desc: info.Desc, Price: info.Price})
	})
}

func dllSandboxScan(content string) string {
	return guarded(func() string {
		return okEnv(sandbox.Scan(content))
	})
}

// ================= 网站词库公开访问 =================

type cloudReq struct {
	FilePath string          `json:"filePath"`
	Ext      string          `json:"ext"` // .n / .wn
	Content  string          `json:"content"`
	WebRoot  string          `json:"webRoot"`
	Trigger  string          `json:"trigger"` // .n 的触发词，空则 Main
	Access   json.RawMessage `json:"access"`
	Body     string          `json:"body"`
}

func dllCloudServe(reqJSON string) string {
	return guarded(func() string {
		var req cloudReq
		if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
			return errEnv("请求解析失败: " + err.Error())
		}
		if done := accountBegin(req.FilePath); done != nil {
			defer done()
		}
		return okEnv(dic_api.Api.WebHTTPRun(&dic_dto.WebHTTPRequest{
			Path:    req.FilePath,
			Ext:     req.Ext,
			Content: req.Content,
			WebRoot: req.WebRoot,
			Trigger: req.Trigger,
			Access:  string(req.Access),
			Body:    req.Body,
		}))
	})
}

// ================= 事件队列 =================

const evQueueMax = 10000

var (
	evMu    sync.Mutex
	evQueue []string
)

// dllEvent 是 DLL 向外发出的已归一化事件；宿主据此落库/推送。
type dllEvent struct {
	Kind    string `json:"kind"`              // recv / send / sendResult
	BotID   int64  `json:"botId"`             //
	MsgKind string `json:"msgKind,omitempty"` // 群聊@ / 群聊 / 文本 / markdown ...
	Content string `json:"content,omitempty"` //
	MsgID   string `json:"msgId,omitempty"`   //
	Group   string `json:"group,omitempty"`   //
	User    string `json:"user,omitempty"`    //
	Scene   string `json:"scene,omitempty"`   //
	Target  string `json:"target,omitempty"`  //
}

func pushEvent(v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		return
	}
	evMu.Lock()
	if len(evQueue) >= evQueueMax {
		// 丢弃最旧事件：用 copy 前移而非 reslice，避免底层数组长期持有已出队字符串
		copy(evQueue, evQueue[1:])
		evQueue = evQueue[:len(evQueue)-1]
	}
	evQueue = append(evQueue, string(raw))
	evMu.Unlock()
}

// dllPollEvent 取出一个事件（无事件时返回空串）。
func dllPollEvent() string {
	evMu.Lock()
	if len(evQueue) == 0 {
		evMu.Unlock()
		return ""
	}
	ev := evQueue[0]
	evQueue = evQueue[1:]
	if len(evQueue) == 0 {
		evQueue = nil
	}
	evMu.Unlock()
	return ev
}

// dllOnRecv 引擎收到事件时回调，仅记录真正的用户消息。
func dllOnRecv(r *qqbot_msg.RouterQQBot, eventType string, data json.RawMessage) {
	kind, ok := recvKinds[eventType]
	if !ok {
		return
	}
	regMu.Lock()
	e := byRouter[r]
	regMu.Unlock()
	if e == nil {
		return
	}
	ev := dllEvent{Kind: "recv", BotID: e.id, MsgKind: kind}
	if strings.HasPrefix(eventType, "GROUP_") || strings.HasPrefix(eventType, "C2C_") {
		var g qqbot_msg.GroupMessageEvent
		if err := json.Unmarshal(data, &g); err != nil {
			return
		}
		ev.Content = g.Content
		if strings.TrimSpace(ev.Content) == "" && len(g.Attachments) > 0 {
			ev.Content = "[富媒体]"
		}
		ev.MsgID = g.ID
		if g.GroupOpenID != "" {
			ev.Group = g.GroupOpenID
		} else {
			ev.Group = g.GroupID
		}
		ev.User = firstNonEmpty(g.Author.MemberOpenID, g.Author.UserOpenID, g.Author.Username)
		if strings.HasPrefix(eventType, "C2C_") {
			ev.Scene, ev.Target = "c2c", ev.User
		} else {
			ev.Scene, ev.Target = "group", ev.Group
		}
	} else {
		var g qqbot_msg.GuildMessageEvent
		if err := json.Unmarshal(data, &g); err != nil {
			return
		}
		ev.Content = g.Content
		ev.MsgID = g.ID
		ev.Group = g.ChannelID
		ev.User = firstNonEmpty(g.Author.Username, g.Author.ID)
		if eventType == "DIRECT_MESSAGE_CREATE" {
			ev.Scene, ev.Target = "dm", g.GuildID
		} else {
			ev.Scene, ev.Target = "channel", g.ChannelID
		}
	}
	pushEvent(ev)
}

// dllOnSend 引擎发送消息时回调，仅记录真正的消息发送。
func dllOnSend(b *qqbot_msg.QQBot, path string, body any) {
	if b == nil {
		return
	}
	regMu.Lock()
	e := byAppID[b.AppId]
	regMu.Unlock()
	if e == nil {
		return
	}
	ev := dllEvent{Kind: "send", BotID: e.id}
	switch v := body.(type) {
	case qqbot_msg.MessageToSend:
		ev.MsgKind, ev.Content = sendKindContent(v)
	case *qqbot_msg.MessageToSend:
		if v == nil {
			return
		}
		ev.MsgKind, ev.Content = sendKindContent(*v)
	case qqbot_msg.ChannelSend:
		ev.MsgKind, ev.Content = "文本", v.Content
	case *qqbot_msg.ChannelSend:
		if v == nil {
			return
		}
		ev.MsgKind, ev.Content = "文本", v.Content
	default:
		return // 上传资源、群设置、菜单等内部请求，不记日志
	}
	scene, target := sendScene(path)
	ev.Scene, ev.Target = scene, target
	if scene == "group" || scene == "channel" {
		ev.Group = target
	} else {
		ev.User = target
	}
	pushEvent(ev)
}

// dllOnSendResult 发送成功后回调，把响应里的消息 ID 回传给宿主用于回填发送日志。
func dllOnSendResult(b *qqbot_msg.QQBot, path string, resp any) {
	if b == nil {
		return
	}
	msgID := messageResponseID(resp)
	if msgID == "" {
		return
	}
	regMu.Lock()
	e := byAppID[b.AppId]
	regMu.Unlock()
	if e == nil {
		return
	}
	pushEvent(dllEvent{Kind: "sendResult", BotID: e.id, MsgID: msgID})
}

// ================= 分类辅助 =================

// recvKinds 记录的事件类型 → 展示用分类
var recvKinds = map[string]string{
	"GROUP_AT_MESSAGE_CREATE": "群聊@",
	"GROUP_MESSAGE_CREATE":    "群聊",
	"C2C_MESSAGE_CREATE":      "私聊",
	"MESSAGE_CREATE":          "频道",
	"AT_MESSAGE_CREATE":       "频道@",
	"DIRECT_MESSAGE_CREATE":   "频道私信",
}

// sendKinds 消息类型 → 展示用分类
var sendKinds = map[int]string{
	0: "文本",
	2: "markdown",
	3: "ark",
	4: "embed",
	7: "富媒体",
}

func sendKindContent(m qqbot_msg.MessageToSend) (string, string) {
	kind := sendKinds[m.MsgType]
	if kind == "" {
		kind = "类型" + strconv.Itoa(m.MsgType)
	}
	content := m.Content
	if content == "" && m.Markdown != nil {
		content = m.Markdown.Content
	}
	return kind, content
}

// sendScene 从发送路径解析场景与目标。
func sendScene(path string) (scene, target string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i := 0; i+1 < len(parts); i++ {
		switch parts[i] {
		case "groups":
			return "group", parts[i+1]
		case "users":
			return "c2c", parts[i+1]
		case "channels":
			return "channel", parts[i+1]
		case "dms":
			return "dm", parts[i+1]
		}
	}
	return "", ""
}

// 平台账号目录布局（固定子目录名）由宿主自定义下发，相关反推逻辑见 utils.AccountRootOf。

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// messageResponseID 从发送响应对象中提取消息 ID
func messageResponseID(resp any) string {
	switch v := resp.(type) {
	case *qqbot_msg.MessageResponse:
		if v != nil {
			return v.ID
		}
	case qqbot_msg.MessageResponse:
		return v.ID
	}
	return ""
}

// ================= 信封与兜底 =================

func okEnv(data any) string {
	b, err := json.Marshal(map[string]any{"ok": true, "data": data})
	if err != nil {
		return errEnv("结果序列化失败: " + err.Error())
	}
	return string(b)
}

func errEnv(msg string) string {
	b, _ := json.Marshal(map[string]any{"ok": false, "error": msg})
	return string(b)
}

// guarded 执行不可信操作并把 panic 转成错误信封。
func guarded(fn func() string) (out string) {
	defer func() {
		if r := recover(); r != nil {
			out = errEnv(fmt.Sprint("内部异常: ", r))
		}
	}()
	return fn()
}
