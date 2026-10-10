package dic_server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/dto"
)

// 全局配置 / AI 配置 API。
func init() {
	registerOpuiApi(opuiHandleConfigAPI,
		"get_global_config", "save_global_config", "get_ai_config", "save_ai_config",
	)
}

// opuiHandleConfigAPI 处理本模块注册的 OPUI API 类型。
func opuiHandleConfigAPI(w http.ResponseWriter, r *http.Request, h *HttpOpUiData) {
	switch h.Type {
	case "get_global_config":
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			jsonResp, _ := json.Marshal(map[string]any{"debug": false, "temp_cleanup_interval": 60, "dic_cache": false})
			w.Write(jsonResp)
			return
		}
		httpSec := cfg.Section("HTTP")
		jsonResp, _ := json.Marshal(map[string]any{
			"debug":                 httpSec.Key("调试").MustBool(false),
			"temp_cleanup_interval": httpSec.Key("临时读写清理周期").MustInt(60),
			"dic_cache":             httpSec.Key("词库编译缓存").MustBool(false),
		})
		w.Write(jsonResp)
		return

	case "save_global_config":
		var j struct {
			Debug               bool `json:"debug"`
			TempCleanupInterval int  `json:"temp_cleanup_interval"`
			DicCache            bool `json:"dic_cache"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			w.Write([]byte(`{"status":"error","error":"读取系统配置失败"}`))
			return
		}
		httpSec := cfg.Section("HTTP")
		httpSec.Key("调试").SetValue(strconv.FormatBool(j.Debug))
		httpSec.Key("临时读写清理周期").SetValue(strconv.Itoa(j.TempCleanupInterval))
		httpSec.Key("词库编译缓存").SetValue(strconv.FormatBool(j.DicCache))
		if err := cfg.Save(); err != nil {
			w.Write([]byte(`{"status":"error","error":"保存系统配置失败"}`))
			return
		}
		dto.ServerConfig.Debug = j.Debug
		dto.ServerConfig.TempCleanupInterval = j.TempCleanupInterval
		dto.ServerConfig.DicCache = j.DicCache
		debugLog.SetDebug(j.Debug)
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "get_ai_config":
		// 读取 AI 对接配置（合并配置的 [AI] 节），供管理面板展示
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			jsonResp, _ := json.Marshal(defaultAIConfigJSON())
			w.Write(jsonResp)
			return
		}
		aiCfg := dto.LoadConfig_ai(cfg.Section("AI"))
		jsonResp, _ := json.Marshal(aiConfigJSON(aiCfg))
		w.Write(jsonResp)
		return

	case "save_ai_config":
		// 保存 AI 模型列表与全局配置到 [AI] 节，并同步运行期内存配置（无需重启）
		var j struct {
			Open           bool    `json:"open"`
			CurrentID      string  `json:"current_id"`
			Timeout        int     `json:"timeout"`
			InlineComplete bool    `json:"inline_complete"`
			ApprovalMode   *string `json:"approval_mode"`
			AutoContinue   *bool   `json:"auto_continue"`
			// 全局默认自动继续次数：0 表示不限制
			AutoContinueMax *int `json:"auto_continue_max"`
			// 全局默认智能体 ID：留空表示由调用方回退出厂默认智能体
			DefaultAgentID *string `json:"default_agent_id"`
			Models         []struct {
				ID              string `json:"id"`
				Name            string `json:"name"`
				BaseURL         string `json:"base_url"`
				APIKey          string `json:"api_key"`
				ClearAPIKey     bool   `json:"clear_api_key"`
				Model           string `json:"model"`
				Vision          bool   `json:"vision"`
				Reasoning       bool   `json:"reasoning"`
				ReasoningEffort string `json:"reasoning_effort"`
				ReasoningModel  string `json:"reasoning_model"`
				ContextLength   int    `json:"context_length"`
				MaxTokens       int    `json:"max_tokens"`
				RateLimit       int    `json:"rate_limit"`
				Concurrency     int    `json:"concurrency"`
			} `json:"models"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		timeout := j.Timeout
		if timeout <= 0 {
			timeout = 60
		}
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			w.Write([]byte(`{"status":"error","error":"读取系统配置失败"}`))
			return
		}
		sec := cfg.Section("AI")
		// 密钥不返回给前端：某模型密钥留空表示保持其已保存的密钥不变，仅 clear_api_key 为 true 时清空
		oldKeys := map[string]string{}
		for _, m := range dto.LoadAIModels(sec) {
			oldKeys[m.ID] = m.APIKey
		}
		models := make([]*dto.AIModelConfig, 0, len(j.Models))
		for i, m := range j.Models {
			id := strings.TrimSpace(m.ID)
			if id == "" {
				id = "m" + strconv.Itoa(i+1)
			}
			apiKey := strings.TrimSpace(m.APIKey)
			if apiKey == "" && !m.ClearAPIKey {
				apiKey = oldKeys[id]
			}
			// 长度限制归一化：未配置时回退默认最佳值，输出长度不超过上下文长度
			contextLength, maxTokens := dto.NormalizeModelLimits(m.ContextLength, m.MaxTokens)
			models = append(models, &dto.AIModelConfig{
				ID:              id,
				Name:            strings.TrimSpace(m.Name),
				BaseURL:         strings.TrimSpace(m.BaseURL),
				APIKey:          apiKey,
				Model:           strings.TrimSpace(m.Model),
				Vision:          m.Vision,
				Reasoning:       m.Reasoning,
				ReasoningEffort: dto.NormalizeReasoningEffort(m.ReasoningEffort),
				ReasoningModel:  strings.TrimSpace(m.ReasoningModel),
				ContextLength:   contextLength,
				MaxTokens:       maxTokens,
				RateLimit:       dto.NormalizeModelRate(m.RateLimit),
				Concurrency:     dto.NormalizeModelConcurrency(m.Concurrency),
			})
		}
		// 当前模型：未指定或已不存在时回退列表首项
		currentID := strings.TrimSpace(j.CurrentID)
		if cur := dto.PickAIModel(models, currentID); cur != nil {
			currentID = cur.ID
		} else {
			currentID = ""
		}
		stored, err := dto.EncodeAIModels(models)
		if err != nil {
			w.Write([]byte(`{"status":"error","error":"密钥加密失败"}`))
			return
		}
		sec.Key("启用").SetValue(strconv.FormatBool(j.Open))
		sec.Key(dto.AIModelsKey).SetValue(stored)
		sec.Key(dto.AICurrentKey).SetValue(currentID)
		sec.Key("超时").SetValue(strconv.Itoa(timeout))
		sec.Key("代码补全").SetValue(strconv.FormatBool(j.InlineComplete))
		// 全局默认审批模式：仅在前端显式下发时更新，未下发时保持既有值（兼容旧版前端）
		if j.ApprovalMode != nil {
			sec.Key(dto.AIApprovalKey).SetValue(dto.NormalizeAIPermissionMode(*j.ApprovalMode))
		}
		// 全局默认自动继续：同样仅在前端显式下发时更新
		if j.AutoContinue != nil {
			sec.Key(dto.AIAutoContinueKey).SetValue(strconv.FormatBool(*j.AutoContinue))
		}
		// 全局默认自动继续次数：0 表示不限制，同样仅在前端显式下发时更新
		if j.AutoContinueMax != nil {
			sec.Key(dto.AIAutoContinueMaxKey).SetValue(strconv.Itoa(dto.NormalizeAIAutoContinueMax(*j.AutoContinueMax)))
		}
		// 全局默认智能体：保存其 ID，留空由调用方回退出厂默认智能体
		if j.DefaultAgentID != nil {
			sec.Key(dto.AIDefaultAgentKey).SetValue(strings.TrimSpace(*j.DefaultAgentID))
		}
		// 清除旧版单模型键，避免与模型列表重复；旧版全局「系统提示」键同样不再使用，一并清除
		for _, k := range []string{"接口地址", "密钥", "模型", "思考模式", "推理强度", "推理模型", "系统提示"} {
			sec.DeleteKey(k)
		}
		if err := cfg.Save(); err != nil {
			w.Write([]byte(`{"status":"error","error":"保存系统配置失败"}`))
			return
		}
		// 由落盘内容重新解析，同步运行期内存配置（密钥保持明文）
		dto.ServerConfig.AI = dto.LoadConfig_ai(sec)
		w.Write([]byte(`{"status":"ok"}`))
		return

	}
}
