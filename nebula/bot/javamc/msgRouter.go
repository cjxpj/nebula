package javamc

import (
	"fmt"
	"net/http"

	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
	"github.com/gorilla/websocket"
)

func BotMessage(w http.ResponseWriter, r *http.Request) {
	queryParams := r.URL.Query()

	ip := utils.GetClientIP(r)
	// 检查是否为 WebSocket 升级请求
	conn, err := dto.ServerConfig.Ws.Conn.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	responseData := dto.HTTPRequestInfo{
		Path:        r.URL.Path,
		Type:        r.Method,
		QueryParams: queryParams,
		Headers:     r.Header,
		IP:          ip,
		Host:        r.Host,
	}

	// 将数据转换为JSON格式
	responseJSON, _ := utils.Json.Marshal(responseData)
	if dic, err := dic_dto.RunDic("private/websocket/server.n"); err == nil {
		dic.Val.G.Set("访问数据", string(responseJSON))
		dic.SetFunc("断开连接", dto.DicFunc{
			L: "0",
			Fn: func(d *dto.DicInputs) (any, error) {
				conn.Close()
				return "", nil
			}})
		resData := dic_api.Api.DicRunPrivate(dic, "连接成功")
		if resData != "" {
			if err := conn.WriteMessage(websocket.TextMessage, []byte(resData)); err != nil {
				fmt.Println("发送消息时出错:", err)
			}
		}
	}

}
