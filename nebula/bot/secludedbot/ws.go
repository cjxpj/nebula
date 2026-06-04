package secludedbot

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
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
)

// isConnected 只读获取连接状态
func isConnected() bool {
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

// nextSeq 获取下一个包序号
func nextSeq() int64 {
	mu.Lock()
	defer mu.Unlock()
	seqCounter++
	return seqCounter
}

// connectAndLogin 连接到 Secluded 并发送上线包，等待 Response 确认
func connectAndLogin(wsUrl, token string) error {
	mu.Lock()
	// 先关闭旧连接
	if conn != nil {
		_ = conn.Close()
		conn = nil
	}
	connected = false
	mu.Unlock()

	c, _, err := websocket.DefaultDialer.Dial(wsUrl, nil)
	if err != nil {
		return fmt.Errorf("dial secluded %s: %w", wsUrl, err)
	}

	mu.Lock()
	conn = c
	mu.Unlock()

	// 发送上线包
	seq := nextSeq()
	loginPacket := map[string]any{
		"seq": seq,
		"cmd": "SyncOicq",
		"rsp": true,
		"data": map[string]string{
			"pid":   "nebula.secluded.plugin",
			"name":  "nebula-secluded",
			"token": token,
		},
	}
	if err := sendRaw(loginPacket); err != nil {
		_ = c.Close()
		return fmt.Errorf("send SyncOicq: %w", err)
	}

	// 读取一个应答包
	c.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, raw, err := c.ReadMessage()
	c.SetReadDeadline(time.Time{})
	if err != nil {
		_ = c.Close()
		return fmt.Errorf("read SyncOicq response: %w", err)
	}

	packet := &struct {
		Seq  int64           `json:"seq"`
		Cmd  string          `json:"cmd"`
		Data json.RawMessage `json:"data"`
	}{}
	if err := json.Unmarshal(raw, packet); err != nil {
		_ = c.Close()
		return fmt.Errorf("parse login response: %w", err)
	}
	if packet.Cmd != "Response" {
		_ = c.Close()
		return fmt.Errorf("unexpected login response cmd: %s", packet.Cmd)
	}

	resp := &struct {
		Status  bool   `json:"status"`
		Account string `json:"account"`
	}{}
	_ = json.Unmarshal(packet.Data, resp)
	if !resp.Status {
		_ = c.Close()
		return fmt.Errorf("secluded token rejected")
	}

	// 保存机器人账户
	if resp.Account != "" && dto.ServerConfig.SecludedBot != nil {
		dto.ServerConfig.SecludedBot.Account = resp.Account
	}

	mu.Lock()
	connected = true
	mu.Unlock()

	return nil
}

// readLoop 持续读取消息并派发处理
func readLoop(onMessage func(raw []byte, header *rawPacketHeader)) {
	for {
		if !isConnected() {
			time.Sleep(2 * time.Second)
			continue
		}
		mu.Lock()
		c := conn
		mu.Unlock()
		if c == nil {
			time.Sleep(2 * time.Second)
			continue
		}
		_, raw, err := c.ReadMessage()
		if err != nil {
			mu.Lock()
			connected = false
			mu.Unlock()
			debugLog.Infof("[secluded] secluded read error: %v, will reconnect", err)
			time.Sleep(3 * time.Second)
			continue
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
	mu.Lock()
	defer mu.Unlock()
	if conn != nil {
		_ = conn.Close()
		conn = nil
	}
	connected = false
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

			// 触发 [内部]启动成功
			rMsg := dic_api.Api.DicRunPrivate(dic, "启动成功")

			// debugLog.Infof("[secluded] 返回: %s", rMsg)

			rMsg = strings.ReplaceAll(rMsg, "\\r", "\n")
			if rMsg != "" {
				fmt.Println(rMsg)
			}
		}()
	}
}
