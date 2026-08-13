package secludedbot

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cjxpj/nebula/debugLog"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
	"github.com/gorilla/websocket"
)

// 全局连接和锁（简单单连接模型：一次只维护一个 WebSocket 连接）
var (
	mu         sync.Mutex
	conn       *websocket.Conn
	seqCounter int64
	connected  bool
	pendingRsp sync.Map // seq -> chan *rawPacketHeader
	stopping   atomic.Bool
	started    atomic.Bool
)

// IsConnected 只读获取连接状态
func IsConnected() bool {
	mu.Lock()
	defer mu.Unlock()
	return connected && conn != nil
}

// sendRaw 发送一条 JSON 消息（线程安全）
func sendRaw(v any) error {
	mu.Lock()
	defer mu.Unlock()
	if conn == nil {
		return fmt.Errorf("secluded websocket not connected")
	}
	return conn.WriteJSON(v)
}

// sendRawNoWait 发送一条 JSON 消息，失败时仅记录日志
func sendRawNoWait(v any) {
	if err := sendRaw(v); err != nil {
		debugLog.Infof("[secluded] send failed: %v", err)
	}
}

// nextSeq 获取下一个包序号
func nextSeq() int64 {
	mu.Lock()
	defer mu.Unlock()
	seqCounter++
	return seqCounter
}

// connectAndLogin 连接到 Secluded：HTTP Header Bearer Token 鉴权，服务端下发 Sync 确认
func connectAndLogin(wsUrl, token string) error {
	mu.Lock()
	if conn != nil {
		_ = conn.Close()
		conn = nil
	}
	connected = false
	mu.Unlock()

	// HTTP Header 鉴权
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+token)

	c, _, err := websocket.DefaultDialer.Dial(wsUrl, headers)
	if err != nil {
		return fmt.Errorf("dial secluded %s: %w", wsUrl, err)
	}

	// Dial 成功后检查是否已被 Stop，避免覆盖 Stop 效果
	if stopping.Load() {
		c.Close()
		return fmt.Errorf("stopped")
	}

	mu.Lock()
	conn = c
	mu.Unlock()

	// 鉴权成功后，服务端下发 Sync 包
	c.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, raw, err := c.ReadMessage()
	c.SetReadDeadline(time.Time{})
	if err != nil {
		_ = c.Close()
		return fmt.Errorf("read Sync: %w", err)
	}

	syncResp := &struct {
		Cmd  string `json:"cmd"`
		Data struct {
			Time     int64   `json:"time"`
			Version  string  `json:"version"`
			Platform string  `json:"platform"`
			List     []int64 `json:"list"`
		} `json:"data"`
	}{}
	if err := json.Unmarshal(raw, syncResp); err != nil {
		_ = c.Close()
		return fmt.Errorf("parse Sync: %w", err)
	}
	if syncResp.Cmd != "Sync" {
		_ = c.Close()
		return fmt.Errorf("expected Sync, got %s", syncResp.Cmd)
	}

	if len(syncResp.Data.List) > 0 && dto.ServerConfig.SecludedBot != nil {
		dto.ServerConfig.SecludedBot.Account = fmt.Sprintf("%d", syncResp.Data.List[0])
	}

	dbgLog("[secluded] 已连接: version=%s, platform=%s, accounts=%v",
		syncResp.Data.Version, syncResp.Data.Platform, syncResp.Data.List)

	mu.Lock()
	connected = true
	mu.Unlock()

	return nil
}

// readLoop 持续读取消息并派发处理
func readLoop(onMessage func(raw []byte, header *rawPacketHeader)) {
	for {
		if !IsConnected() || stopping.Load() {
			return
		}
		mu.Lock()
		c := conn
		mu.Unlock()
		if c == nil {
			return
		}
		_, raw, err := c.ReadMessage()
		if err != nil {
			mu.Lock()
			connected = false
			mu.Unlock()
			debugLog.Infof("[secluded] secluded read error: %v, will reconnect", err)
			return
		}
		header := &rawPacketHeader{}
		_ = json.Unmarshal(raw, header)
		onMessage(raw, header)
	}
}

// rawPacketHeader 仅用来解析包头部
type rawPacketHeader struct {
	Seq     int64           `json:"seq"`
	Cmd     string          `json:"cmd"`
	Rsp     bool            `json:"rsp"`
	Data    json.RawMessage `json:"data"`
	RawData []byte          `json:"-"`
}

// sendAndWait 发送请求并等待响应（超时10秒）
func sendAndWait(v any, seq int64) (*rawPacketHeader, error) {
	// 创建接收响应的 channel
	ch := make(chan *rawPacketHeader, 1)
	pendingRsp.Store(seq, ch)
	defer pendingRsp.Delete(seq)

	// 发送请求
	if err := sendRaw(v); err != nil {
		return nil, fmt.Errorf("send failed: %w", err)
	}

	// 等待响应（超时10秒）
	select {
	case rsp := <-ch:
		return rsp, nil
	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("wait response timeout (10s)")
	}
}

// handleResponse 处理响应包，发送到对应的等待channel
func handleResponse(header *rawPacketHeader) {
	if header == nil {
		return
	}
	if val, ok := pendingRsp.LoadAndDelete(header.Seq); ok {
		if ch, ok := val.(chan *rawPacketHeader); ok {
			select {
			case ch <- header:
			default:
			}
		}
	}
}

// Stop 停止 Secluded 连接
func Stop() {
	stopping.Store(true)
	mu.Lock()
	defer mu.Unlock()
	if conn != nil {
		_ = conn.Close()
		conn = nil
	}
	connected = false
}

// 启动时自动获取的群列表缓存
var (
	startupGroupListMu sync.Mutex
	startupGroupList   string
)

// fetchGroupListOnStartup 连接成功后自动获取群列表并缓存
// 注意：必须在 readLoop 运行后调用，否则收不到响应
func fetchGroupListOnStartup() {
	account := getCurrentAccount()
	if account == "" {
		return
	}
	seq := nextSeq()
	packet := map[string]any{
		"seq": seq,
		"cmd": "SendOicqMsg",
		"rsp": true,
		"data": []any{map[string]string{
			"Account":      account,
			"GroupListGet": "GroupListGet",
			"GroupId":      "0",
		}},
	}
	rsp, err := sendAndWait(packet, seq)
	if err != nil {
		debugLog.Infof("[secluded] 启动获取群列表失败: %v", err)
		return
	}
	startupGroupListMu.Lock()
	startupGroupList = string(rsp.Data)
	startupGroupListMu.Unlock()
	dbgLog("[secluded] 启动获取群列表成功: %s", startupGroupList)
}

// getStartupGroupList 返回启动时缓存的群列表，未获取到返回空字符串
func getStartupGroupList() string {
	startupGroupListMu.Lock()
	defer startupGroupListMu.Unlock()
	return startupGroupList
}

// triggerStartupCallback 连接成功后触发词库回调
func triggerStartupCallback() {
	if dto.ServerConfig.SecludedBot == nil || dto.ServerConfig.SecludedBot.FilePath == "" {
		return
	}

	// 遍历 dic/*.n 词库
	botDicPath := utils.NewFileQueue(dto.ServerConfig.SecludedBot.FilePath + "/dic")
	botDicList, err := botDicPath.GetFileList()
	if err != nil {
		debugLog.Infof("[secluded] get dic list for startup callback failed: %v", err)
		return
	}

	for _, v := range botDicList {
		if !strings.HasSuffix(v, ".n") {
			continue
		}
		dicFile := v
		go func() {
			dicPath := dto.ServerConfig.SecludedBot.FilePath + "/dic/" + dicFile
			fileData, err := utils.NewFileQueue(dicPath).ReadFromFile()
			if err != nil {
				return
			}

			dic := dic_dto.NewDic(dicPath, fileData)

			dic.AddFuncs(Funcs)
			dic.SetFunc("调用", dto.DicFunc{
				L: "2..",
				Fn: func(d *dto.DicInputs) (any, error) {
					qqVal := dto.NewDicVal()
					sleepTime := d.Inputs.Int(1)
					if sleepTime > 0 {
						time.Sleep(time.Duration(sleepTime) * time.Millisecond)
					}

					dic.AddFuncs(Funcs)

					rMsg := dic_api.Api.DicRunPrivateVal(dic, d.Inputs.StringAfter(2), qqVal)
					return strings.ReplaceAll(rMsg, "\\r", "\n"), nil
				}})

			// debugLog.Infof("[secluded] 启动触发: %s", dicPath)

			// 触发 [系统]启动
			rMsg := dic_api.Api.DicRunEvent(dic, "系统", "启动")

			// debugLog.Infof("[secluded] 返回: %s", rMsg)

			rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
			if rMsg != "" {
				fmt.Println(rMsg)
			}
		}()
	}
}

// triggerDisconnectCallback 连接断开后触发词库回调
func triggerDisconnectCallback() {
	if dto.ServerConfig.SecludedBot == nil || dto.ServerConfig.SecludedBot.FilePath == "" {
		return
	}

	botDicPath := utils.NewFileQueue(dto.ServerConfig.SecludedBot.FilePath + "/dic")
	botDicList, err := botDicPath.GetFileList()
	if err != nil {
		debugLog.Infof("[secluded] get dic list for disconnect callback failed: %v", err)
		return
	}

	for _, v := range botDicList {
		if !strings.HasSuffix(v, ".n") {
			continue
		}
		dicFile := v
		go func() {
			dicPath := dto.ServerConfig.SecludedBot.FilePath + "/dic/" + dicFile
			fileData, err := utils.NewFileQueue(dicPath).ReadFromFile()
			if err != nil {
				return
			}

			dic := dic_dto.NewDic(dicPath, fileData)
			dic.AddFuncs(Funcs)
			dic.SetFunc("调用", dto.DicFunc{
				L: "2..",
				Fn: func(d *dto.DicInputs) (any, error) {
					qqVal := dto.NewDicVal()
					sleepTime := d.Inputs.Int(1)
					if sleepTime > 0 {
						time.Sleep(time.Duration(sleepTime) * time.Millisecond)
					}
					dic.AddFuncs(Funcs)
					rMsg := dic_api.Api.DicRunPrivateVal(dic, d.Inputs.StringAfter(2), qqVal)
					return strings.ReplaceAll(rMsg, "\\r", "\n"), nil
				}})

			rMsg := dic_api.Api.DicRunPrivate(dic, "断开连接")
			rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
			if rMsg != "" {
				fmt.Println(rMsg)
			}
		}()
	}
}
