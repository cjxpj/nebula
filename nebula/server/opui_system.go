package dic_server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/cjxpj/nebula/debugLog"
)

// 系统 / 日志 / 更新 / 安全信息 API。
func init() {
	registerOpuiApi(opuiHandleSystemAPI,
		"get_autostart", "set_autostart", "cancel_autostart", "get_server_logs",
		"clear_server_logs", "terminal_input", "get_sys_status", "check_update",
		"online_update", "pause_update", "resume_update", "get_update_status",
		"security_info",
	)
}

// opuiHandleSystemAPI 处理本模块注册的 OPUI API 类型。
func opuiHandleSystemAPI(w http.ResponseWriter, r *http.Request, h *HttpOpUiData) {
	switch h.Type {
	case "get_autostart":
		enabled, err := GetAutoStart()
		if err != nil {
			w.Write([]byte(`{"enabled":false}`))
			return
		}
		jsonResp, _ := json.Marshal(map[string]bool{"enabled": enabled})
		w.Write(jsonResp)
		return

	case "set_autostart":
		if err := SetAutoStart(); err != nil {
			jsonResp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(jsonResp)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "cancel_autostart":
		if err := CancelAutoStart(); err != nil {
			jsonResp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(jsonResp)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "get_server_logs":
		var j struct {
			Limit int `json:"limit"`
			Skip  int `json:"skip"`
		}
		_ = json.Unmarshal(h.Data, &j)
		logs, hasMore := readServerLogs(j.Limit, j.Skip)
		jsonResp, _ := json.Marshal(map[string]any{"logs": logs, "hasMore": hasMore})
		w.Write(jsonResp)
		return

	case "clear_server_logs":
		ClearServerLogs()
		jsonResp, _ := json.Marshal(map[string]any{"ok": true})
		w.Write(jsonResp)
		return

	case "terminal_input":
		var j struct {
			Input string `json:"input"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		input := strings.TrimSpace(j.Input)
		if input == "" {
			http.Error(w, `{"status":"error","error":"输入不能为空"}`, http.StatusBadRequest)
			return
		}
		go runTerminalDic(input)
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "get_sys_status":
		data, err := getSysStatus()
		if err != nil {
			debugLog.Errorf("[OPUI] get_sys_status failed: %v", err)
			http.Error(w, `{"status":"error","error":"collect failed"}`, http.StatusInternalServerError)
			return
		}
		if r, err := json.Marshal(data); err == nil {
			w.Write(r)
		} else {
			http.Error(w, `{"status":"error","error":"marshal failed"}`, http.StatusInternalServerError)
		}
		return

	case "check_update":
		jsonResp, _ := json.Marshal(checkUpdate())
		w.Write(jsonResp)
		return

	case "online_update":
		// 启动/继续在线更新下载（幂等：下载中则忽略，暂停则继续）
		go startOnlineUpdate()
		resp, _ := json.Marshal(map[string]any{"status": "ok", "msg": "开始下载更新", "data": getUpdateStatus()})
		w.Write(resp)
		return

	case "pause_update":
		pauseOnlineUpdate()
		resp, _ := json.Marshal(map[string]any{"status": "ok", "data": getUpdateStatus()})
		w.Write(resp)
		return

	case "resume_update":
		go startOnlineUpdate()
		resp, _ := json.Marshal(map[string]any{"status": "ok", "data": getUpdateStatus()})
		w.Write(resp)
		return

	case "get_update_status":
		resp, _ := json.Marshal(getUpdateStatus())
		w.Write(resp)
		return

	case "security_info":
		info := SecurityInfo{
			ServerStart: serverStartTime.Format("2006-01-02 15:04:05"),
			Uptime:      formatDuration(time.Since(serverStartTime)),
		}

		// 登录事件
		loginEventsMu.Lock()
		info.LoginEvents = make([]LoginEvent, len(loginEvents))
		copy(info.LoginEvents, loginEvents)
		loginEventsMu.Unlock()

		// 在线列表 = OPUI 已连接用户
		rawClients := GetOpuiOnlineClients()
		info.OnlineList = make([]OnlineItem, 0, len(rawClients))
		for _, c := range rawClients {
			item := OnlineItem{}
			if v, ok := c["name"].(string); ok {
				item.Name = v
			}
			if v, ok := c["type"].(string); ok {
				item.Type = v
			}
			if v, ok := c["online"].(bool); ok {
				item.Online = v
			}
			if v, ok := c["detail"].(string); ok {
				item.Detail = v
			}
			info.OnlineList = append(info.OnlineList, item)
		}

		if r, err := json.Marshal(info); err == nil {
			w.Write(r)
		} else {
			http.Error(w, `{"status":"error","error":"marshal failed"}`, http.StatusInternalServerError)
		}
		return

	}
}
