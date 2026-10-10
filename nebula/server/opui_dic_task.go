package dic_server

import (
	"encoding/json"
	"net/http"
	"strconv"

	dic_funcs "github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
)

// 定时任务 / 词库配置 API。
func init() {
	registerOpuiApi(opuiHandleDicTaskAPI,
		"get_dic_tasks", "add_dic_task", "del_dic_task", "get_dic_config",
		"save_dic_config",
	)
}

// opuiHandleDicTaskAPI 处理本模块注册的 OPUI API 类型。
func opuiHandleDicTaskAPI(w http.ResponseWriter, r *http.Request, h *HttpOpUiData) {
	switch h.Type {
	case "get_dic_tasks":
		// 返回定时任务（分页），避免任务过多时一次性返回全部
		var j struct {
			Limit int `json:"limit"`
			Skip  int `json:"skip"`
		}
		_ = json.Unmarshal(h.Data, &j)
		if j.Limit <= 0 {
			j.Limit = 50
		}
		if j.Skip < 0 {
			j.Skip = 0
		}
		all := dic_funcs.ListScheduledTasks()
		total := len(all)
		start := min(j.Skip, total)
		end := min(j.Skip+j.Limit, total)
		jsonResp, _ := json.Marshal(map[string]any{
			"list":    all[start:end],
			"total":   total,
			"hasMore": end < total,
		})
		w.Write(jsonResp)
		return

	case "add_dic_task":
		var j struct {
			DicPath    string `json:"dic_path"`
			Trigger    string `json:"trigger"`
			Interval   string `json:"interval"`
			Once       bool   `json:"once"`
			RunAtStart bool   `json:"run_at_start"`
			// 指针用于区分「未传」与「显式 false」，未传时默认异步执行
			Async *bool `json:"async"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		// 词库路径留空时使用合并配置中 [词库调试] 默认词库
		if j.DicPath == "" {
			if p, ok := loadDicDebugDefaults()["path"].(string); ok {
				j.DicPath = p
			}
		}
		if !checkDicPath(j.DicPath) {
			http.Error(w, `{"status":"error","error":"词库路径不合法"}`, http.StatusBadRequest)
			return
		}
		async := true
		if j.Async != nil {
			async = *j.Async
		}
		id, err := dic_funcs.AddScheduledTask(j.DicPath, j.Trigger, j.Interval, j.Once, j.RunAtStart, async)
		if err != nil {
			http.Error(w, `{"status":"error","error":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok", "id": id})
		w.Write(jsonResp)
		return

	case "del_dic_task":
		var j struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if err := dic_funcs.DelScheduledTask(j.ID); err != nil {
			http.Error(w, `{"status":"error","error":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
		w.Write(jsonResp)
		return

	case "get_dic_config":
		// 读取词库调试运行配置（合并配置的 [词库调试] 节）
		jsonResp, _ := json.Marshal(loadDicDebugDefaults())
		w.Write(jsonResp)
		return

	case "save_dic_config":
		// 保存词库调试运行配置到合并配置的 [词库调试] 节
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
		sec := cfgFile.Section("词库调试")
		if v, ok := cfg["path"].(string); ok && v != "" {
			sec.Key("默认词库").SetValue(v)
		}
		if tabs, ok := cfg["tabs"].([]any); ok {
			var items []string
			for _, it := range tabs {
				if s, ok := it.(string); ok && s != "" {
					items = append(items, s)
				}
			}
			// 路径列表整体 JSON 编码存储，保持 ini 单行值
			if b, err := json.Marshal(items); err == nil {
				sec.Key("打开的标签").SetValue(string(b))
			}
		}
		if v, ok := cfg["trigger"].(string); ok {
			sec.Key("触发文本").SetValue(v)
		}
		if v, ok := cfg["timeout"].(float64); ok {
			sec.Key("超时").SetValue(strconv.Itoa(int(v)))
		}
		if v, ok := cfg["historyMax"].(float64); ok && v > 0 {
			sec.Key("历史记录数量").SetValue(strconv.Itoa(int(v)))
		}
		if v, ok := cfg["saveRun"].(bool); ok {
			sec.Key("保存运行").SetValue(strconv.FormatBool(v))
		}
		if v, ok := cfg["autoSave"].(bool); ok {
			sec.Key("实时保存").SetValue(strconv.FormatBool(v))
		}
		if v, ok := cfg["autoFormat"].(bool); ok {
			sec.Key("保存自动格式化").SetValue(strconv.FormatBool(v))
		}
		if g, ok := cfg["g"].([]any); ok {
			var items []string
			for _, it := range g {
				if s, ok := it.(string); ok {
					items = append(items, s)
				}
			}
			// 值可含任意换行：整体 JSON 编码存储（ini 值保持单行，避免按行拆分时被截断）
			if b, err := json.Marshal(items); err == nil {
				sec.Key("全局变量").SetValue(string(b))
			}
		}
		if err := cfgFile.Save(); err != nil {
			http.Error(w, `{"status":"error","error":"写入配置文件失败: `+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
		w.Write(jsonResp)
		return

	}
}
