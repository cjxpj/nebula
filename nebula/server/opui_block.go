package dic_server

import (
	"encoding/json"
	"net/http"
	"time"

	dic_funcs "github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
)

func loadBlockDefaults() map[string]any {
	def := map[string]any{}
	// 默认打开的词库：取「词库调试」的默认词库（如 private/debug.n），供无打开记录时初始化
	def["defaultDic"] = defaultDebugDic()
	cfg, err := dto.LoadConfigFile()
	if err != nil {
		return def
	}
	sec := cfg.Section("积木编程")
	if v := sec.Key("当前词库").String(); v != "" {
		def["dic"] = v
	}
	if v := sec.Key("打开的标签").String(); v != "" {
		// 标签列表整体 JSON 编码存储（.n 路径数组），保持单行值
		var tabs []string
		if err := json.Unmarshal([]byte(v), &tabs); err == nil && tabs != nil {
			def["tabs"] = tabs
		}
	}
	return def
}

// blockViewTable 积木编程「视图记录」在全局数据库中的表名。
// key 为 .n 词库路径，data 为该词库的画布视图 JSON（滚动位置、缩放比例、积木排列坐标）。
// 每个词库单独一条记录，随全局数据库持久化，可在「清理」页统一管理。
const blockViewTable = "block_view"

// blockViewSave 保存某个词库的积木视图记录（写入全局数据库）
func blockViewSave(dicPath string, view json.RawMessage) error {
	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return err
	}
	if err := dic_funcs.EnsureFsTable(db, blockViewTable); err != nil {
		return err
	}
	_, err = db.Exec(`
		INSERT INTO "block_view" (key, data, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			data = excluded.data,
			updated_at = excluded.updated_at
	`, dicPath, []byte(view), time.Now().Unix())
	return err
}

// blockViewGet 读取某个词库的积木视图记录（不存在返回 nil）
func blockViewGet(dicPath string) json.RawMessage {
	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return nil
	}
	if err := dic_funcs.EnsureFsTable(db, blockViewTable); err != nil {
		return nil
	}
	var data []byte
	if err := db.QueryRow(`SELECT data FROM "block_view" WHERE key=?`, dicPath).Scan(&data); err != nil {
		return nil
	}
	return json.RawMessage(data)
}

// blockViewList 列出全部积木视图记录（供「清理」页展示并逐条清理）
func blockViewList() []map[string]any {
	items := make([]map[string]any, 0)
	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return items
	}
	if err := dic_funcs.EnsureFsTable(db, blockViewTable); err != nil {
		return items
	}
	rows, err := db.Query(`SELECT key, length(data), updated_at FROM "block_view" ORDER BY updated_at DESC`)
	if err != nil {
		return items
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var size, updatedAt int64
		if err := rows.Scan(&key, &size, &updatedAt); err != nil {
			continue
		}
		items = append(items, map[string]any{
			"path":      key,
			"size":      size,
			"updatedAt": updatedAt,
		})
	}
	return items
}

// blockViewRemove 删除某个词库的积木视图记录
func blockViewRemove(dicPath string) error {
	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return err
	}
	if err := dic_funcs.EnsureFsTable(db, blockViewTable); err != nil {
		return err
	}
	_, err = db.Exec(`DELETE FROM "block_view" WHERE key=?`, dicPath)
	return err
}

// blockViewClear 清空全部积木视图记录
func blockViewClear() error {
	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return err
	}
	if err := dic_funcs.EnsureFsTable(db, blockViewTable); err != nil {
		return err
	}
	_, err = db.Exec(`DELETE FROM "block_view"`)
	return err
}

// loadDicDebugDefaults 读取合并配置中 [词库调试] 节的配置（运行配置的唯一存储位置）

// 积木配置 / 视图 API。
func init() {
	registerOpuiApi(opuiHandleBlockAPI,
		"get_block_config", "save_block_config", "get_block_view", "save_block_view",
		"list_block_view", "remove_block_view", "clear_block_view",
	)
}

// opuiHandleBlockAPI 处理本模块注册的 OPUI API 类型。
func opuiHandleBlockAPI(w http.ResponseWriter, r *http.Request, h *HttpOpUiData) {
	switch h.Type {
	case "get_block_config":
		// 读取积木编程页的打开记录（合并配置的 [积木编程] 节）
		jsonResp, _ := json.Marshal(loadBlockDefaults())
		w.Write(jsonResp)
		return

	case "save_block_config":
		// 保存积木编程页打开的词库标签到合并配置的 [积木编程] 节
		var cfg map[string]any
		if err := json.Unmarshal(h.Data, &cfg); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		cfgFile, err := dto.LoadConfigFile()
		if err != nil {
			http.Error(w, `{"status":"error","error":"读取配置文件失败"}`, http.StatusInternalServerError)
			return
		}
		sec := cfgFile.Section("积木编程")
		if v, ok := cfg["dic"].(string); ok {
			sec.Key("当前词库").SetValue(v)
		}
		if tabs, ok := cfg["tabs"].([]any); ok {
			items := make([]string, 0, len(tabs))
			for _, it := range tabs {
				p, ok := it.(string)
				if !ok || p == "" {
					continue
				}
				items = append(items, p)
			}
			// 词库标签列表整体 JSON 编码存储（.n 路径数组），保持单行值
			if b, err := json.Marshal(items); err == nil {
				sec.Key("打开的标签").SetValue(string(b))
			}
		}
		if err := cfgFile.Save(); err != nil {
			http.Error(w, `{"status":"error","error":"写入配置文件失败: `+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
		w.Write(jsonResp)
		return

	case "get_block_view":
		// 读取某个 .n 词库的积木视图记录（画布滚动位置、缩放比例、积木排列坐标）
		var j struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		resp := map[string]any{"status": "ok"}
		if j.Path != "" {
			if v := blockViewGet(j.Path); v != nil {
				resp["view"] = v
			}
		}
		jsonResp, _ := json.Marshal(resp)
		w.Write(jsonResp)
		return

	case "save_block_view":
		// 保存某个 .n 词库的积木视图记录（写入全局数据库，可在「清理」页管理）
		var j struct {
			Path string          `json:"path"`
			View json.RawMessage `json:"view"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if j.Path == "" || len(j.View) == 0 {
			http.Error(w, `{"status":"error","error":"参数不完整"}`, http.StatusBadRequest)
			return
		}
		if err := blockViewSave(j.Path, j.View); err != nil {
			jsonResp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(jsonResp)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "list_block_view":
		// 积木视图记录列表：供「清理」页展示并逐条清理
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok", "items": blockViewList()})
		w.Write(jsonResp)
		return

	case "remove_block_view":
		// 清理单个词库的积木视图记录（下次打开按积木默认位置展示）
		var j struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(h.Data, &j)
		if err := blockViewRemove(j.Path); err != nil {
			jsonResp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(jsonResp)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "clear_block_view":
		// 清空全部积木视图记录
		if err := blockViewClear(); err != nil {
			jsonResp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(jsonResp)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	}
}
