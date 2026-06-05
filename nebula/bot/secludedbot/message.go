package secludedbot

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cjxpj/nebula/debugLog"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// startupOnce 确保启动回调在整个客户端生命周期中只触发一次
var startupOnce sync.Once

var reMsgData = regexp.MustCompile(`±(atMsg|at|img)=([^±]+)±|([^±]+)`)

// parseMixedTextResult 解析结果
type parseMixedTextResult struct {
	Items []map[string]string
	Reply string // 引用回复的消息ID
}

// parseMixedText 把一段带 ±at=xxx± / ±img=url± 的混合文本
// 解析成 []map[string]string 的切片和引用回复ID
func parseMixedText(input string) parseMixedTextResult {
	var items []map[string]string
	var reply string
	for _, m := range reMsgData.FindAllStringSubmatch(input, -1) {
		switch {
		case m[1] == "at":
			items = append(items, map[string]string{"AtUin": m[2]})
		case m[1] == "img":
			items = append(items, map[string]string{"Img": m[2]})
		case m[1] == "atMsg":
			reply = m[2]
		case m[3] != "":
			items = append(items, map[string]string{"Text": m[3]})
		}
	}
	return parseMixedTextResult{Items: items, Reply: reply}
}

// Start 启动 Secluded 插件（WebSocket 客户端）
// 会在后台 goroutine 中持续维护连接，收到消息通过词库处理后回复
func Start(wsUrl, token string) {
	go func() {
		firstAttempt := true
		for {
			if dto.ServerConfig.SecludedBot == nil || !dto.ServerConfig.SecludedBot.Open {
				return
			}
			if err := connectAndLogin(wsUrl, token); err != nil {
				if firstAttempt {
					debugLog.Infof("[secluded] Secluded 插件启动，将连接到: %s", wsUrl)
					firstAttempt = false
				}
				debugLog.Infof("[secluded] 连接失败: %v，5秒后重试...", err)
				time.Sleep(5 * time.Second)
				continue
			}
			debugLog.Infof("[secluded] 已成功连接到 Secluded，等待消息...")

			// 启动触发每个词库只会触发一次并且伴随客户端，不会因为断开重新连接重新触发
			startupOnce.Do(func() {
				triggerStartupCallback()
			})

			readLoop(func(raw []byte, header *rawPacketHeader) {
				handleMessage(raw, header)
			})
			debugLog.Infof("[secluded] 连接已断开")
		}
	}()
}

// handleMessage 处理一条来自 Secluded 的消息
func handleMessage(_ []byte, header *rawPacketHeader) {
	switch header.Cmd {
	case "PushOicqMsg":
		// data 是一个数组，通常第一个元素是消息元信息，第二个元素是 {Text:"xxx"}
		// 但我们这里简单地：合并 Text 字段，用 dic 跑
		elems, err := parsePushOicqData(header.Data)
		if err != nil {
			debugLog.Infof("[secluded] parse PushOicqMsg: %v", err)
			return
		}
		dispatchPush(elems, header.Data)
	case "Response":
		// 响应包，发送到等待的 channel
		handleResponse(header)
	default:
		// 其它包直接忽略
	}
}

// pushElem 是 PushOicqMsg 的 data 数组中一个元素的通用表示
type pushElem struct {
	Account   string `json:"Account"`
	Group     string `json:"Group"`
	Friend    string `json:"Friend"`
	Temp      string `json:"Temp"`
	GroupId   string `json:"GroupId"`
	Uin       string `json:"Uin"`
	MsgId     string `json:"MsgId"`
	GroupName string `json:"GroupName"`
	OpName    string `json:"OpName"`
	UinName   string `json:"UinName"`
	Text      string `json:"Text"`
	Img       string `json:"Img"`
	AtUin     string `json:"AtUin"`
	AtName    string `json:"AtName"`
	Uid       string `json:"Uid"`
	All       string `json:"All"`
	Time      string `json:"Time"`
	People    string `json:"People"`
	Op        string `json:"Op"`
	Url       string `json:"Url"`
}

// parsePushOicqData 解析 PushOicqMsg 的 data 数组
// 协议规定 data 是 []map[string]string，这里通用解析
func parsePushOicqData(data json.RawMessage) ([]pushElem, error) {
	// 直接按 []map[string]string 解析，再逐个合并成 pushElem
	var rawMaps []map[string]string
	if err := json.Unmarshal(data, &rawMaps); err != nil {
		// 也可能是单个对象
		var one map[string]string
		if err2 := json.Unmarshal(data, &one); err2 != nil {
			return nil, err
		}
		rawMaps = []map[string]string{one}
	}

	res := make([]pushElem, 0, len(rawMaps))
	for _, m := range rawMaps {
		b, _ := json.Marshal(m)
		e := pushElem{}
		_ = json.Unmarshal(b, &e)
		res = append(res, e)
	}
	return res, nil
}

// dispatchPush 运行词库并回写消息
func dispatchPush(elems []pushElem, rawData json.RawMessage) {
	if len(elems) == 0 {
		return
	}

	// 从第一个元素里取元信息；从第二个元素里取 Text
	meta := elems[0]

	// 过滤系统消息（没有来源类型的是系统通知）
	if meta.Group == "" && meta.Friend == "" && meta.Temp == "" {
		return
	}
	content := ""
	// 收集图片URL列表
	var imgUrls []string
	if len(elems) > 1 {
		for _, e := range elems[1:] {
			if e.Text != "" {
				content += e.Text
			}
			if e.Img != "" && e.Url != "" {
				content += "[图片]"
				imgUrls = append(imgUrls, e.Url)
			}
		}
	}
	// 全体禁言事件
	isInternalEvent := false
	if content == "" && meta.All == "All" && meta.Time != "" {
		isInternalEvent = true
		if meta.Time == "0" {
			content = "全体禁言关闭"
		} else {
			content = "全体禁言开启"
		}
	}
	// 成员禁言/解禁事件
	if content == "" && meta.People == "People" && meta.Time != "" {
		isInternalEvent = true
		content = "成员禁言 " + meta.Time
	}
	// 如果第一段就包含 Text（某些实现），也取
	if content == "" && meta.Text != "" {
		content = meta.Text
	}

	// 来源类型
	sourceType := "群聊"
	switch {
	case meta.Friend != "":
		sourceType = "私聊"
		content = "#私聊#" + content
	case meta.Temp != "":
		sourceType = "临时"
		content = "#临时#" + content
	}

	// 群号 / QQ
	targetId := meta.GroupId
	if targetId == "" {
		targetId = meta.Uin
	}
	userId := meta.Uin
	nick := meta.OpName
	if nick == "" {
		nick = meta.UinName
	}

	// 遍历 dic/*.n 词库
	botDicPath := utils.NewFileQueue(dto.ServerConfig.SecludedBot.FilePath + "/dic")
	botDicList, err := botDicPath.GetFileList()
	if err != nil {
		debugLog.Infof("[secluded] get dic list failed: %v", err)
		return
	}

	uid := meta.Uid

	// 主人列表
	isAdmin := "null"
	adminPath := dto.ServerConfig.SecludedBot.FilePath + "/admin.txt"
	if adminList, err := utils.NewFileQueue(adminPath).ReadFromFile(); err == nil {
		for s := range strings.SplitSeq(adminList, ",") {
			if userId == strings.TrimSpace(s) || uid == strings.TrimSpace(s) {
				isAdmin = strings.TrimSpace(s)
				break
			}
		}
	}

	// 群白名单
	if sourceType == "群聊" {
		groupPath := dto.ServerConfig.SecludedBot.FilePath + "/groups.txt"
		if groupList, err := utils.NewFileQueue(groupPath).ReadFromFile(); err == nil && groupList != "" {
			if strings.TrimSpace(groupList) == "all" {
				goto skipGroupCheck
			}
			allowed := false
			for s := range strings.SplitSeq(groupList, ",") {
				if targetId == strings.TrimSpace(s) {
					allowed = true
					break
				}
			}
			if !allowed {
				return
			}
		}
	}
skipGroupCheck:

	// 提取 AT 信息
	var atUins, atNames []string
	for _, e := range elems[1:] {
		if e.AtUin != "" {
			atUins = append(atUins, e.AtUin)
			atNames = append(atNames, e.AtName)
		}
	}

	valData := dto.NewVal().
		Set("来源", sourceType).
		Set("群号", targetId).
		Set("QQ", userId).
		Set("qq", userId).
		Set("uid", uid).
		Set("昵称", nick).
		Set("主人", isAdmin).
		Set("MsgId", meta.MsgId).
		Set("Op", meta.Op).
		Set("robot", meta.Account).
		Set("Robot", meta.Account).
		Set("data", string(rawData))

	for i, uin := range atUins {
		valData.Set(fmt.Sprintf("AT%d", i), uin)
	}
	for i, name := range atNames {
		valData.Set(fmt.Sprintf("AtName%d", i), name)
	}

	for _, v := range botDicList {
		if !strings.HasSuffix(v, ".n") {
			continue
		}
		// 保存闭包变量，避免 goroutine 中引用过期
		dicFile := v
		msgMeta := meta
		msgContent := content
		msgIsInternal := isInternalEvent
		msgImgUrls := imgUrls
		msgValData := valData

		go func() {
			dicPath := dto.ServerConfig.SecludedBot.FilePath + "/dic/" + dicFile
			fileData, err := utils.NewFileQueue(dicPath).ReadFromFile()
			if err != nil {
				return
			}

			dic := dic_dto.NewDic(dicPath, fileData).
				SetGlobal_v(msgValData)

			// 设置当前上下文（供词库函数使用）
			pushContext.current = &msgMeta
			pushContext.text = msgContent

			dic.AddFuncs(Funcs)

			dic.SetFunc("调用", dto.DicFunc{
				L: "2..",
				Fn: func(d *dto.DicInputs) (any, error) {
					go func() {
						qqVal := dto.NewDicVal()
						sleepTime := d.Inputs.Int(1)
						time.Sleep(time.Duration(sleepTime) * time.Millisecond)

						rMsg := dic_api.Api.DicRunPrivateVal(dic, d.Inputs.StringAfter(2), qqVal)
						rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")

						if rMsg != "" {
							if err := ReplyText(msgMeta, rMsg); err != nil {
								debugLog.Infof("[secluded] 调用回复失败: %v", err)
							}
						}
					}()
					return "", nil
				}})

			dic.SetFunc("IMG", dto.DicFunc{
				L: "0|1",
				Fn: func(d *dto.DicInputs) (any, error) {
					if d.Inputs.Len() == 0 {
						if len(msgImgUrls) == 0 {
							return "[]", nil
						}
						data, _ := json.Marshal(msgImgUrls)
						return string(data), nil
					}
					index := d.Inputs.Int(1)
					if index <= 0 || index > len(msgImgUrls) {
						return "null", nil
					}
					return msgImgUrls[index-1], nil
				},
			})

			var rMsg string
			if msgIsInternal {
				rMsg = dic_api.Api.DicRunPrivate(dic, msgContent)
			} else {
				rMsg = dic_api.Api.DicRun(dic, msgContent)
			}
			rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
			if rMsg != "" {
				debugLog.Infof("[secluded] %v", rMsg)
				if err := ReplyText(msgMeta, rMsg); err != nil {
					debugLog.Infof("[secluded] reply failed: %v", err)
				}
			}

			// 清空上下文
			pushContext.current = nil
			pushContext.text = ""
		}()
	}
}

// pushContext 给词库函数使用（例如 群单发 / 私聊 等无需指定 qq 时使用当前上下文）
var pushContext struct {
	current *pushElem
	text    string
}

// ReplyText 用 SendOicqMsg 回复一条文本消息
func ReplyText(meta pushElem, text string) error {
	if text == "" {
		return nil
	}

	parseResult := parseMixedText(text)
	if len(parseResult.Items) == 0 && parseResult.Reply == "" {
		return nil
	}

	seq := nextSeq()
	header := map[string]string{
		"Account": meta.Account,
		"MsgId":   meta.MsgId,
	}
	switch {
	case meta.Friend != "":
		header["Friend"] = meta.Friend
		header["Uin"] = meta.Uin
	case meta.Temp != "":
		header["Temp"] = meta.Temp
		header["GroupId"] = meta.GroupId
		header["Uin"] = meta.Uin
	default:
		header["Group"] = "Group"
		header["GroupId"] = meta.GroupId
	}

	// 只有在明确使用 ±atMsg=xxx± 时才添加 Reply 字段（引用回复）
	if parseResult.Reply != "" {
		header["Reply"] = parseResult.Reply
	}

	data := []any{header}
	for _, item := range parseResult.Items {
		data = append(data, item)
	}

	packet := map[string]any{
		"seq":  seq,
		"cmd":  "SendOicqMsg",
		"rsp":  true,
		"data": data,
	}
	return sendRaw(packet)
}

// SendText 通用发送（不带 reply）；由词库函数调用
func SendText(targetType, targetId, text string) error {
	if text == "" {
		return nil
	}
	if dto.ServerConfig.SecludedBot == nil {
		return fmt.Errorf("secluded bot not configured")
	}

	account := dto.ServerConfig.SecludedBot.Account
	if account == "" {
		if pushContext.current != nil && pushContext.current.Account != "" {
			account = pushContext.current.Account
		}
	}
	if account == "" {
		return fmt.Errorf("secluded bot account not set")
	}
	return SendTextWithAccount(targetType, targetId, text, account)
}

// SendTextWithAccount 通用发送（带指定 account）；由词库函数调用
func SendTextWithAccount(targetType, targetId, text, account string) error {
	if text == "" {
		return nil
	}
	if account == "" {
		return fmt.Errorf("secluded bot account not set")
	}

	parseResult := parseMixedText(text)
	if len(parseResult.Items) == 0 && parseResult.Reply == "" {
		return nil
	}

	seq := nextSeq()
	header := map[string]string{
		"Account": account,
	}
	switch targetType {
	case "friend":
		header["Friend"] = "Friend"
		header["Uin"] = targetId
	case "temp":
		header["Temp"] = "Temp"
		header["Uin"] = targetId
	default:
		header["Group"] = "Group"
		header["GroupId"] = targetId
	}

	// 如果用户指定了 ±atMsg=xxx±，添加 Reply 字段
	if parseResult.Reply != "" {
		header["Reply"] = parseResult.Reply
	}

	data := []any{header}
	for _, item := range parseResult.Items {
		data = append(data, item)
	}

	packet := map[string]any{
		"seq":  seq,
		"cmd":  "SendOicqMsg",
		"rsp":  true,
		"data": data,
	}
	return sendRaw(packet)
}

// Log 向 Secluded 控制台输出日志（PrintD/PrintE/PrintI/PrintS/PrintW）
func Log(level, text string) error {
	cmd := "PrintI"
	switch level {
	case "d", "D", "debug":
		cmd = "PrintD"
	case "e", "E", "error":
		cmd = "PrintE"
	case "s", "S", "success":
		cmd = "PrintS"
	case "w", "W", "warn":
		cmd = "PrintW"
	}
	packet := map[string]any{
		"seq":  nextSeq(),
		"cmd":  cmd,
		"rsp":  false,
		"data": text,
	}
	return sendRaw(packet)
}
