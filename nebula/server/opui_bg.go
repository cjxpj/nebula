package dic_server

import (
	"encoding/json"
	"net/http"
	"time"

	dic_funcs "github.com/cjxpj/nebula/dic/funcs"
)

type HttpOpUiConfig_bg struct {
	Light HttpOpUiConfig_bgItem `json:"light"` // 亮色主题背景
	Dark  HttpOpUiConfig_bgItem `json:"dark"`  // 暗色主题背景
}

type HttpOpUiConfig_bgItem struct {
	Type  string `json:"type"`  // "url" | "local" | ""
	Data  string `json:"data"`  // URL 或 base64
	Color string `json:"color"` // 无背景图时的自定义背景色（如 #f5f5f5），留空表示默认
}

// 面板背景 API。
func init() {
	registerOpuiApi(opuiHandleBgAPI,
		"get_bg", "save_bg",
	)
}

// opuiHandleBgAPI 处理本模块注册的 OPUI API 类型。
func opuiHandleBgAPI(w http.ResponseWriter, r *http.Request, h *HttpOpUiData) {
	switch h.Type {
	case "get_bg":
		db, err := dic_funcs.GetGlobalDB()
		if err != nil {
			w.Write([]byte(`{"light":{"type":"","data":""},"dark":{"type":"","data":""}}`))
			return
		}
		if e := dic_funcs.EnsureFsTable(db, "opui_bg"); e != nil {
			w.Write([]byte(`{"light":{"type":"","data":""},"dark":{"type":"","data":""}}`))
			return
		}
		var data string
		err = db.QueryRow(`SELECT data FROM "opui_bg" WHERE key='bg'`).Scan(&data)
		if err != nil {
			w.Write([]byte(`{"light":{"type":"","data":""},"dark":{"type":"","data":""}}`))
			return
		}

		bgData := HttpOpUiConfig_bg{}
		// 新格式：{"light":{...},"dark":{...}}
		if err := json.Unmarshal([]byte(data), &bgData); err != nil ||
			(bgData.Light.Type == "" && bgData.Light.Data == "" && bgData.Light.Color == "" &&
				bgData.Dark.Type == "" && bgData.Dark.Data == "" && bgData.Dark.Color == "") {
			// 旧格式：{"type":..,"data":..} 或纯文本，迁移为亮暗共用同一背景
			var old HttpOpUiConfig_bgItem
			if e := json.Unmarshal([]byte(data), &old); e != nil {
				// 纯文本旧格式，需同时读取 type 列
				var bgType string
				_ = db.QueryRow(`SELECT type FROM "opui_bg" WHERE key='bg'`).Scan(&bgType)
				old = HttpOpUiConfig_bgItem{Type: bgType, Data: data}
			}
			bgData.Light = old
			bgData.Dark = old
		}
		r, _ := json.Marshal(bgData)
		w.Write(r)
		return

	case "save_bg":
		var j HttpOpUiConfig_bg
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		db, err := dic_funcs.GetGlobalDB()
		if err != nil {
			http.Error(w, `{"status":"error","error":"db not ready"}`, http.StatusInternalServerError)
			return
		}
		if e := dic_funcs.EnsureFsTable(db, "opui_bg"); e != nil {
			http.Error(w, `{"status":"error","error":"db init failed"}`, http.StatusInternalServerError)
			return
		}
		bgJson, _ := json.Marshal(j)
		now := time.Now().Unix()
		// 尝试新表结构（无 type 列），失败则回退旧表结构
		_, err = db.Exec(`
				INSERT INTO "opui_bg" (key, data, updated_at)
				VALUES ('bg', ?, ?)
				ON CONFLICT(key) DO UPDATE SET
					data = excluded.data,
					updated_at = excluded.updated_at
			`, string(bgJson), now)
		if err != nil {
			_, err = db.Exec(`
					INSERT INTO "opui_bg" (key, type, data, updated_at)
					VALUES ('bg', ?, ?, ?)
					ON CONFLICT(key) DO UPDATE SET
						type = excluded.type,
						data = excluded.data,
						updated_at = excluded.updated_at
				`, "", string(bgJson), now)
		}
		if err != nil {
			http.Error(w, `{"status":"error","error":"save failed"}`, http.StatusInternalServerError)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	}
}
