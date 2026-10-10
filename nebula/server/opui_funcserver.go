package dic_server

import (
	"encoding/json"
	"net/http"

	"github.com/cjxpj/nebula/dto"
)

// 函数服务器 API。
func init() {
	registerOpuiApi(opuiHandleFuncServerAPI,
		"get_func_servers", "save_func_server", "close_func_server",
	)
}

// opuiHandleFuncServerAPI 处理本模块注册的 OPUI API 类型。
func opuiHandleFuncServerAPI(w http.ResponseWriter, r *http.Request, h *HttpOpUiData) {
	switch h.Type {
	case "get_func_servers":
		res, _ := json.Marshal(map[string]any{"servers": dto.FuncServers.Snapshot()})
		w.Write(res)
		return

	case "save_func_server":
		var req struct {
			Addr string `json:"addr"`
			dto.FuncServerUpdate
		}
		if err := json.Unmarshal(h.Data, &req); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		info := dto.FuncServers.Get(req.Addr)
		if info == nil || info.Update == nil {
			w.Write([]byte(`{"status":"error","error":"服务器不存在"}`))
			return
		}
		if err := info.Update(req.FuncServerUpdate); err != nil {
			w.Write([]byte(`{"status":"error","error":` + strconvQuote(err.Error()) + `}`))
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "close_func_server":
		var req struct {
			Addr string `json:"addr"`
		}
		if err := json.Unmarshal(h.Data, &req); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		info := dto.FuncServers.Get(req.Addr)
		if info == nil || info.Close == nil {
			w.Write([]byte(`{"status":"error","error":"服务器不存在"}`))
			return
		}
		info.Close()
		w.Write([]byte(`{"status":"ok"}`))
		return

	}
}
