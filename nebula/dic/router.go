//go:build !js

package dic

import (
	"net/http"
	"time"

	"github.com/cjxpj/nebula/debugLog"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
	"github.com/gorilla/websocket"
)

// handleWsServer 处理单个 WebSocket 监听路径
func handleWsServer(w http.ResponseWriter, r *http.Request, ws *dto.ServerRouterWebSocket) {
	if ws == nil || ws.Conn == nil {
		return
	}
	conn, err := ws.Conn.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	path := r.URL.Path
	getType := r.Method
	queryParams := r.URL.Query()
	ip := utils.GetClientIP(r)

	// 心跳机制：防止中间代理/防火墙断开空闲连接
	const (
		pongWait   = 60 * time.Second
		pingPeriod = (pongWait * 9) / 10
		writeWait  = 10 * time.Second
	)
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for range ticker.C {
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if e := conn.WriteMessage(websocket.PingMessage, nil); e != nil {
				return
			}
		}
	}()

	responseData := dto.HTTPRequestInfo{
		Path:        path,
		Type:        getType,
		QueryParams: queryParams,
		Headers:     r.Header,
		IP:          ip,
		Host:        r.Host,
	}

	// 将数据转换为JSON格式
	responseJSON, err := json.Marshal(responseData)
	if err != nil {
		utils.Error("访问数据异常")
		return
	}

	dicPath := ws.FilePath
	if dicPath == "" {
		dicPath = "private/websocket/server.n"
	}

	// 运行词库
	if dic, err := dic_dto.NewDicFile(dicPath); err == nil {
		dic.Val.G.Set("访问数据", string(responseJSON))
		dic.SetFunc("断开连接", dto.DicFunc{
			L: "0",
			Fn: func(d *dto.DicInputs) (any, error) {
				conn.Close()
				return "", nil
			}})
		dic.Val.G.SetRaw("_WS连接_", conn)
		resData := dic_api.Api.DicRunPrivate(dic, "连接成功")
		if resData != "" {
			if err := conn.WriteMessage(websocket.TextMessage, []byte(resData)); err != nil {
				debugLog.Infof("发送消息时出错: %v", err)
			}
		}
	}

	go func() {
		messageTypeMap := map[int]string{
			websocket.TextMessage:   "文本消息",
			websocket.BinaryMessage: "二进制消息",
		}

		// 读取来自 WebSocket 服务器的消息
		for {
			Tstr := ""
			wsClose := false
			messageType, message, readMsgErr := conn.ReadMessage()
			if readMsgErr != nil {
				// 判断是否是正常关闭
				if websocket.IsUnexpectedCloseError(readMsgErr, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					debugLog.Infof("读取消息时出错: %v", readMsgErr)
					Tstr = "断开连接"
					wsClose = true
				} else {
					Tstr = "断开连接"
					wsClose = true
				}
				conn.Close()
			} else {
				Tstr = string(message)
			}
			typeName, ok := messageTypeMap[messageType]
			if !ok {
				typeName = "未知消息"
			}

			d, err := dic_dto.NewDicFile(dicPath)
			if err != nil {
				debugLog.Infof("读取文件时出错: %v", err)
				conn.Close() // 关闭连接
				break
			}
			d.Val.G.SetRaw("_WS连接_", conn)
			d.Val.G.Set("访问数据", string(responseJSON))
			d.SetFunc("断开连接", dto.DicFunc{
				L: "0",
				Fn: func(d *dto.DicInputs) (any, error) {
					conn.Close()
					return "", nil
				}})
			d.Val.P.Set("类型", typeName)
			rStr := ""
			if wsClose {
				rStr = dic_api.Api.DicRunPrivate(d, Tstr)
			} else {
				rStr = dic_api.Api.DicRun(d, Tstr)
			}
			// 拦截并处理错误
			if readMsgErr != nil {
				if rStr != "" {
					debugLog.Infof("%v", rStr)
					break
				}
			}
			if rStr != "" {
				if wsClose {
					debugLog.Infof("%v", rStr)
				} else if err := conn.WriteMessage(websocket.TextMessage, []byte(rStr)); err != nil {
					debugLog.Infof("发送消息时出错: %v", err)
				}
			}
		}
	}()
}
