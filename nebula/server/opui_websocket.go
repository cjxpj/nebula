package dic_server

import (
	"encoding/json"
	"net/http"

	"github.com/cjxpj/nebula/dto"
)

type HttpOpUiWebSocketItem struct {
	Addr     string `json:"addr"`
	Cors     bool   `json:"cors"`
	Open     bool   `json:"open"`
	Closable bool   `json:"closable"`
	// ServerAddr 所属（承载）服务器地址：WebSocket 由核心服务器响应，无核心服务器时为空
	ServerAddr string `json:"server_addr"`
}

// WebSocket 管理 API。
func init() {
	registerOpuiApi(opuiHandleWebSocketAPI,
		"get_websocket", "close_websocket",
	)
}

// opuiHandleWebSocketAPI 处理本模块注册的 OPUI API 类型。
func opuiHandleWebSocketAPI(w http.ResponseWriter, r *http.Request, h *HttpOpUiData) {
	switch h.Type {
	case "get_websocket":
		list := dto.ServerConfig.WsListSnapshot()
		coreAddr := dto.FuncServers.CoreAddr()
		items := make([]HttpOpUiWebSocketItem, 0, len(list)+1)
		for _, ws := range list {
			items = append(items, HttpOpUiWebSocketItem{
				Addr:       ws.Addr,
				Cors:       ws.Cors,
				Open:       ws.Open,
				Closable:   true,
				ServerAddr: coreAddr,
			})
		}
		// OPUI 本身也是一个 WebSocket 服务，纳入监听列表（但不可关闭，关闭等于关闭面板自身）
		if opui := dto.ServerConfig.OPUI; opui != nil {
			items = append(items, HttpOpUiWebSocketItem{
				Addr:       opui.Addr,
				Cors:       opui.Cors,
				Open:       true,
				Closable:   false,
				ServerAddr: coreAddr,
			})
		}
		r, _ := json.Marshal(map[string]any{"list": items})
		w.Write(r)
		return

	case "close_websocket":
		var j struct {
			Addr string `json:"addr"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if j.Addr == "" {
			http.Error(w, `{"status":"error","error":"addr is empty"}`, http.StatusBadRequest)
			return
		}
		// OPUI 是管理面板自身的 WebSocket，不可通过面板关闭
		if dto.ServerConfig.OPUI != nil && j.Addr == dto.ServerConfig.OPUI.Addr {
			http.Error(w, `{"status":"error","error":"opui cannot be closed"}`, http.StatusBadRequest)
			return
		}
		dto.ServerConfig.RemoveWs(j.Addr)
		w.Write([]byte(`{"status":"ok"}`))
		return

	}
}
