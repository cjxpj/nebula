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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"

	qqbot "github.com/cjxpj/nebula/bot/qqbot"
	qqbot_msg "github.com/cjxpj/nebula/bot/qqbot/msg"
	_ "github.com/cjxpj/nebula/dic" // 触发引擎初始化：注入 dic_api.Api 并注册内置函数
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dic/sandbox"
	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"
	"github.com/cjxpj/nebula/utils"
)

// dllVersion 与 appfiles.Version 对齐；DLL 内直接硬编码，避免引入 appfiles 重依赖。
const dllVersion = "20.6.0"

// ================= 初始化 =================

type initConfig struct {
	BotsRoot string `json:"botsRoot"` // 词库根目录（沙箱工作目录）
	LogDir   string `json:"logDir"`   // 引擎日志目录
	Debug    bool   `json:"debug"`    // 调试开关
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
			// 收紧词库执行边界：限定工作目录、注销高危函数、禁止出网访问内网
			if removed := sandbox.Enable(cf.BotsRoot); len(removed) > 0 {
				debugLog.Infof("[nebula] 已禁用 %d 个高危词库函数", len(removed))
			}
			if cf.LogDir != "" {
				utils.SetLogDir(cf.LogDir)
			}
			// 词库编译磁盘缓存：缓存提到「词库父目录/.dic_cache」
			dto.ServerConfig.DicCache = true
			run.SetDicCacheDirFunc(func(dicPath string) string {
				// 云词库（.../<uid>/web_dic）的编译缓存归到账号级「我的储存」
				// （.../<uid>/database），避免 .dic_cache 混进云词库目录被当作词库列出。
				if accountDir, ok := accountDirOfWebDic(dicPath); ok {
					return filepath.Join(accountDir, "database", ".dic_cache")
				}
				dir := filepath.Dir(dicPath)
				if filepath.Base(dir) == "dic" {
					dir = filepath.Dir(dir)
				}
				return filepath.Join(dir, ".dic_cache")
			})
			// 事件回调：改成写入内部队列，宿主轮询取走
			qqbot.OnRecv = dllOnRecv
			qqbot_msg.OnSend = dllOnSend
			qqbot_msg.OnSendResult = dllOnSendResult
		})
		debugLog.SetDebug(cf.Debug)
		return okEnv(map[string]any{"version": dllVersion})
	})
}

func dllVersionString() string {
	return okEnv(dllVersion)
}

func dllSetDebug(on int) string {
	debugLog.SetDebug(on != 0)
	return okEnv(nil)
}

// ================= 机器人注册表 =================

type botEntry struct {
	id       int64
	appID    string
	secret   string
	name     string
	filePath string
	router   *qqbot_msg.RouterQQBot
}

var (
	regMu    sync.Mutex
	reg      = map[int64]*botEntry{}
	byAppID  = map[string]*botEntry{}
	byRouter = map[*qqbot_msg.RouterQQBot]*botEntry{}
)

func dllBotStart(id int64, appID, secret, name, filePath string) string {
	return guarded(func() string {
		if appID == "" || secret == "" {
			return errEnv("机器人 AppID 或密钥为空")
		}
		if filePath == "" {
			return errEnv("机器人目录为空")
		}
		if err := os.MkdirAll(filepath.Join(filePath, "dic"), 0o755); err != nil {
			return errEnv(err.Error())
		}

		regMu.Lock()
		old := reg[id]
		// 已在运行且配置未变：幂等返回
		if old != nil && old.appID == appID && old.secret == secret && old.name == name && old.filePath == filePath {
			regMu.Unlock()
			return okEnv(nil)
		}
		// 配置变更：摘除旧实例，用新配置重建
		if old != nil {
			delete(reg, id)
			delete(byRouter, old.router)
			delete(byAppID, old.appID)
		}
		router := &qqbot_msg.RouterQQBot{
			Open:     true,
			FilePath: filePath,
			LastMsg:  cache.New(time.Hour, time.Minute),
			API:      qqbot_msg.NewQQBot(appID, secret),
			Remark:   name,
		}
		e := &botEntry{id: id, appID: appID, secret: secret, name: name, filePath: filePath, router: router}
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
		d := dic_dto.NewDic(path, content)
		return okEnv(dic_api.Api.DicRun(d, trigger))
	})
}

func dllWebDicRun(path, content string) string {
	return guarded(func() string {
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

// ================= 云词库公开访问 =================

type cloudReq struct {
	Method     string              `json:"method"`
	URLPath    string              `json:"urlPath"` // 请求 URI（含 query）
	FilePath   string              `json:"filePath"`
	Ext        string              `json:"ext"` // .n / .wn
	Content    string              `json:"content"`
	Host       string              `json:"host"`
	RemoteAddr string              `json:"remoteAddr"`
	Headers    map[string][]string `json:"headers"`
	Body       string              `json:"body"`
}

type cloudResp struct {
	Status  int         `json:"status"`
	Headers http.Header `json:"headers"`
	Body    string      `json:"body"`
}

// capWriter 捕获词库设置的响应头/状态码，替代真实 http.ResponseWriter 跨 DLL 传递。
type capWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newCapWriter() *capWriter {
	return &capWriter{header: make(http.Header), status: http.StatusOK}
}

func (c *capWriter) Header() http.Header         { return c.header }
func (c *capWriter) Write(b []byte) (int, error) { return c.body.Write(b) }
func (c *capWriter) WriteHeader(code int)        { c.status = code }

func dllCloudServe(reqJSON string) string {
	return guarded(func() string {
		var req cloudReq
		if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
			return errEnv("请求解析失败: " + err.Error())
		}
		target := req.URLPath
		if target == "" {
			target = "/"
		}
		hreq, err := http.NewRequest(req.Method, target, io.NopCloser(strings.NewReader(req.Body)))
		if err != nil {
			return errEnv("请求重建失败: " + err.Error())
		}
		if req.Host != "" {
			hreq.Host = req.Host
		}
		if req.RemoteAddr != "" {
			hreq.RemoteAddr = req.RemoteAddr
		}
		for k, vs := range req.Headers {
			for _, v := range vs {
				hreq.Header.Add(k, v)
			}
		}

		info := cloudRequestInfo(hreq, req.Body)
		cw := newCapWriter()
		globalV := dto.NewVal().
			Set("响应状态", "200").
			Set("输出头部", "{}").
			Set("COOKIE", "[]").
			Set("网站根目录", ".").
			SetRaw("_请求数据_", hreq).
			SetRaw("_响应数据_", cw)
		if raw, err := json.Marshal(info); err == nil {
			globalV.Set("访问数据", string(raw))
		}
		funcs := cloudHTTPFuncs(cw, info)

		var out string
		if req.Ext == ".wn" {
			wd := dic_dto.NewWebDic(req.FilePath, req.Content)
			wd.SetGlobal_v(globalV)
			wd.MyFunc = funcs
			out = dic_api.Api.WebDicRun(wd)
		} else {
			// 编译不可信词库：panic 已由 guarded 兜底
			dic := dic_dto.NewDic(req.FilePath, req.Content)
			dic.SetGlobal_v(globalV).AddFuncs(funcs)
			out = dic_api.Api.DicRun(dic, "Main")
		}
		applyCloudOutput(cw, globalV, out)
		return okEnv(cloudResp{Status: cw.status, Headers: cw.header, Body: out})
	})
}

// cloudRequestInfo 构建一次公开访问的请求信息（与引擎网页词库的访问数据保持一致）。
func cloudRequestInfo(r *http.Request, rawBody string) *dto.HTTPRequestInfo {
	info := &dto.HTTPRequestInfo{
		Path:        r.URL.Path,
		Type:        r.Method,
		QueryParams: r.URL.Query(),
		Headers:     r.Header,
		IP:          utils.GetClientIP(r),
		Host:        r.Host,
	}
	if r.Method != http.MethodPost {
		return info
	}
	if strings.TrimSpace(rawBody) != "" {
		var bodyMap map[string]any
		if err := json.Unmarshal([]byte(rawBody), &bodyMap); err == nil {
			info.Post = bodyMap
		} else {
			info.Post = rawBody
		}
	} else {
		_ = r.ParseForm()
		info.Post = r.PostForm
	}
	return info
}

// applyCloudOutput 按网页词库规范写出响应：应用词库设置的输出头部/COOKIE/响应状态。
func applyCloudOutput(w *capWriter, globalV *dto.Val, runData string) {
	if sendHeader, _ := globalV.Get("输出头部").(string); sendHeader != "" && sendHeader != "{}" {
		var headerMap map[string]string
		if err := json.Unmarshal([]byte(sendHeader), &headerMap); err == nil {
			for k, v := range headerMap {
				w.header.Set(k, v)
			}
		}
	}
	if sendCookie, _ := globalV.Get("COOKIE").(string); sendCookie != "" && sendCookie != "[]" {
		var cookies []*dto.SetCookie
		if err := json.Unmarshal([]byte(sendCookie), &cookies); err == nil {
			for _, c := range cookies {
				if c == nil {
					continue
				}
				http.SetCookie(w, &http.Cookie{
					Name:     c.Name,
					Value:    c.Value,
					Path:     c.Path,
					HttpOnly: c.HttpOnly,
					MaxAge:   c.MaxAge,
				})
			}
		}
	}
	status := http.StatusOK
	if v, ok := globalV.Get("响应状态").(string); ok {
		if n, err := strconv.Atoi(v); err == nil {
			status = n
		}
	}
	w.status = status
	w.header.Set("Content-Length", strconv.Itoa(len(runData)))
}

// cloudHTTPFuncs 构造公开访问注入的内置函数：设置头部 / GET / POST。
func cloudHTTPFuncs(w http.ResponseWriter, info *dto.HTTPRequestInfo) map[string]dto.DicFunc {
	return map[string]dto.DicFunc{
		"设置头部": {L: "2", Fn: func(d *dto.DicInputs) (any, error) {
			if k, ok := d.Inputs.Get(1).(string); ok {
				if v, ok := d.Inputs.Get(2).(string); ok {
					w.Header().Set(k, v)
					return "", nil
				}
				return "参数错误2", nil
			}
			return "参数错误1", nil
		}},
		"GET": {L: "1|2", Fn: func(d *dto.DicInputs) (any, error) {
			if k, ok := d.Inputs.Get(1).(string); ok {
				if v := info.QueryParams.Get(k); v != "" {
					return v, nil
				}
				if v, ok := d.Inputs.Get(2).(string); ok {
					return v, nil
				}
			}
			return "", nil
		}},
		"POST": {L: "1|2", Fn: func(d *dto.DicInputs) (any, error) {
			key, ok := d.Inputs.Get(1).(string)
			if !ok {
				return "参数必须是字符串", nil
			}
			switch post := info.Post.(type) {
			case url.Values:
				if v := post.Get(key); v != "" {
					return v, nil
				}
			case map[string]any:
				if v, exists := post[key]; exists {
					switch n := v.(type) {
					case int:
						return strconv.FormatInt(int64(n), 10), nil
					case int64:
						return strconv.FormatInt(n, 10), nil
					case float64:
						return strconv.FormatFloat(n, 'f', -1, 64), nil
					default:
						return fmt.Sprint(v), nil
					}
				}
			case map[string][]string:
				if arr, exists := post[key]; exists && len(arr) > 0 {
					return arr[0], nil
				}
			}
			if d.Inputs.LenOk(2) {
				if def, ok := d.Inputs.Get(2).(string); ok {
					return def, nil
				}
			}
			return "", nil
		}},
	}
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

// accountDirOfWebDic 若词库位于账号云词库目录（.../<uid>/web_dic[/...]）之下，
// 返回账号目录 .../<uid>，否则返回 false。用于把云词库编译缓存归到账号级存储目录。
func accountDirOfWebDic(dicPath string) (string, bool) {
	dir := filepath.Clean(dicPath)
	for {
		if filepath.Base(dir) == "web_dic" {
			return filepath.Dir(dir), true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

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
