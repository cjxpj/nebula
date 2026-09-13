package dic_server

import (
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// ============== AI 智能体（可复用的任务预设） ==============
//
// 一个「智能体」是一套完整的任务预设：名称、系统提示、模型、上下文策略、
// 工具权限与推理设置。每个智能体绑定一个独立任务（会话），在词库调试中
// 切换智能体即切换到该智能体对应的任务，配置互相隔离。

const (
	// aiAgentsFile 智能体的持久化文件（程序私有目录，与 sessions.json 解耦）
	aiAgentsFile = "private/ai/agents.json"
	// aiAgentNameMaxRunes 智能体名称最大字符数
	aiAgentNameMaxRunes = 40
	// aiAgentSystemMaxRunes 智能体系统提示最大字符数
	aiAgentSystemMaxRunes = 20000
)

// AIAgent 智能体：可复用的任务预设配置。
type AIAgent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// 预设系统提示：切换到该智能体时套用到其任务上
	System string `json:"system"`
	Model  string `json:"model"`
	// 预设上下文附带策略：auto | always | never
	ContextMode string `json:"context_mode"`
	// 预设工具权限：manual | auto | full
	PermissionMode string `json:"permission_mode"`
	// 预设思考/推理设置：inherit | on | off
	ReasoningMode   string `json:"reasoning_mode"`
	ReasoningEffort string `json:"reasoning_effort"`
	ReasoningModel  string `json:"reasoning_model"`
	// SessionID 该智能体绑定的独立任务（首次切换时自动创建并绑定）
	SessionID string `json:"session_id"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// aiAgentStore 智能体持久化文件结构。
type aiAgentStore struct {
	Version int        `json:"version"`
	Agents  []*AIAgent `json:"agents"`
}

var (
	aiAgentsMu     sync.Mutex
	aiAgents       = map[string]*AIAgent{}
	aiAgentsLoaded bool
)

// newAIAgentID 生成智能体 ID（时间戳 + 随机数，避免并发冲突）。
func newAIAgentID() string {
	var b [6]byte
	_, _ = crand.Read(b[:])
	return fmt.Sprintf("ag%d%s", time.Now().UnixMilli(), hex.EncodeToString(b[:]))
}

// aiNormalizeAgent 补齐并规范化智能体字段。
func aiNormalizeAgent(a *AIAgent) {
	name := strings.TrimSpace(a.Name)
	if name == "" {
		name = "未命名智能体"
	}
	if rs := []rune(name); len(rs) > aiAgentNameMaxRunes {
		name = string(rs[:aiAgentNameMaxRunes]) + "…"
	}
	a.Name = name
	a.System = aiClipRunes(strings.TrimSpace(a.System), aiAgentSystemMaxRunes)
	a.Model = strings.TrimSpace(a.Model)
	a.ContextMode = aiNormalizeContextMode(a.ContextMode)
	a.PermissionMode = aiNormalizePermissionMode(a.PermissionMode)
	a.ReasoningMode = aiNormalizeReasoningMode(a.ReasoningMode)
	a.ReasoningEffort = dto.NormalizeReasoningEffort(a.ReasoningEffort)
	a.ReasoningModel = strings.TrimSpace(a.ReasoningModel)
	a.SessionID = strings.TrimSpace(a.SessionID)
}

// ensureAIAgentsLoadedLocked 首次访问时从私有目录加载智能体；调用方需持有 aiAgentsMu。
func ensureAIAgentsLoadedLocked() {
	if aiAgentsLoaded {
		return
	}
	aiAgentsLoaded = true
	aiAgents = map[string]*AIAgent{}
	data, err := utils.NewFileQueue(aiAgentsFile).ReadFromFile()
	if err != nil || strings.TrimSpace(data) == "" {
		return
	}
	store := aiAgentStore{}
	if json.Unmarshal([]byte(data), &store) != nil {
		return
	}
	for _, a := range store.Agents {
		if a == nil || strings.TrimSpace(a.ID) == "" {
			continue
		}
		aiNormalizeAgent(a)
		aiAgents[a.ID] = a
	}
}

// saveAIAgentsLocked 把智能体写入私有目录（按更新时间倒序）；调用方需持有 aiAgentsMu。
func saveAIAgentsLocked() {
	list := make([]*AIAgent, 0, len(aiAgents))
	for _, a := range aiAgents {
		list = append(list, a)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].UpdatedAt > list[j].UpdatedAt })
	data, _ := json.Marshal(aiAgentStore{Version: 1, Agents: list})
	utils.NewFileQueue(aiAgentsFile).WriteToFile(string(data))
}

// aiAgentCopy 返回智能体的副本，避免调用方在锁外读取到并发修改。
func aiAgentCopy(a *AIAgent) *AIAgent {
	if a == nil {
		return nil
	}
	cp := *a
	return &cp
}

// aiSwitchAgent 切换到指定智能体对应的独立任务：
// 首次切换时新建任务并绑定，之后每次切换把智能体预设套用到该任务上。
func aiSwitchAgent(id, dicPath string) (*AIAgent, *AISession, error) {
	aiAgentsMu.Lock()
	ensureAIAgentsLoadedLocked()
	agent := aiAgents[id]
	if agent == nil {
		aiAgentsMu.Unlock()
		return nil, nil, fmt.Errorf("智能体不存在")
	}
	aiNormalizeAgent(agent)

	aiSessionsMu.Lock()
	ensureAISessionsLoadedLocked()
	sess := aiSessions[agent.SessionID]
	if sess == nil {
		now := time.Now().Unix()
		sess = &AISession{
			ID:        newAISessionID(),
			Title:     aiSessionTitle(agent.Name, ""),
			DicPath:   strings.TrimSpace(dicPath),
			Messages:  []AISessionMessage{},
			CreatedAt: now,
			UpdatedAt: now,
		}
		aiSessions[sess.ID] = sess
		agent.SessionID = sess.ID
	}
	// 套用智能体预设：每次切换都以智能体配置为准，保证编辑智能体后立即生效
	sess.System = agent.System
	sess.Model = agent.Model
	sess.ContextMode = agent.ContextMode
	sess.PermissionMode = agent.PermissionMode
	sess.ReasoningMode = agent.ReasoningMode
	sess.ReasoningEffort = agent.ReasoningEffort
	sess.ReasoningModel = agent.ReasoningModel
	if dp := strings.TrimSpace(dicPath); dp != "" {
		sess.DicPath = dp
	}
	sess.UpdatedAt = time.Now().Unix()
	saveAISessionsLocked()
	snapshot := *sess
	snapshot.Messages = append([]AISessionMessage(nil), sess.Messages...)
	agentSnap := aiAgentCopy(agent)
	saveAIAgentsLocked()
	aiSessionsMu.Unlock()
	aiAgentsMu.Unlock()
	return agentSnap, &snapshot, nil
}

// ---------- 接口处理 ----------

// aiAgentHandle 处理 AI 智能体相关的 OPUI 请求。
func aiAgentHandle(w http.ResponseWriter, h *HttpOpUiData) {
	switch h.Type {
	case "list_ai_agents":
		aiAgentsMu.Lock()
		ensureAIAgentsLoadedLocked()
		list := make([]*AIAgent, 0, len(aiAgents))
		for _, a := range aiAgents {
			list = append(list, aiAgentCopy(a))
		}
		aiAgentsMu.Unlock()
		sort.Slice(list, func(i, j int) bool { return list[i].UpdatedAt > list[j].UpdatedAt })
		aiWriteJSON(w, map[string]any{"status": "ok", "list": list})
		return

	case "save_ai_agent":
		var j struct {
			ID              string `json:"id"`
			Name            string `json:"name"`
			System          string `json:"system"`
			Model           string `json:"model"`
			ContextMode     string `json:"context_mode"`
			PermissionMode  string `json:"permission_mode"`
			ReasoningMode   string `json:"reasoning_mode"`
			ReasoningEffort string `json:"reasoning_effort"`
			ReasoningModel  string `json:"reasoning_model"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		aiAgentsMu.Lock()
		ensureAIAgentsLoadedLocked()
		now := time.Now().Unix()
		agent := aiAgents[strings.TrimSpace(j.ID)]
		if agent == nil {
			agent = &AIAgent{ID: newAIAgentID(), CreatedAt: now}
			aiAgents[agent.ID] = agent
		}
		agent.Name = j.Name
		agent.System = j.System
		agent.Model = j.Model
		agent.ContextMode = j.ContextMode
		agent.PermissionMode = j.PermissionMode
		agent.ReasoningMode = j.ReasoningMode
		agent.ReasoningEffort = j.ReasoningEffort
		agent.ReasoningModel = j.ReasoningModel
		agent.UpdatedAt = now
		aiNormalizeAgent(agent)
		saveAIAgentsLocked()
		snap := aiAgentCopy(agent)
		aiAgentsMu.Unlock()
		aiWriteJSON(w, map[string]any{"status": "ok", "agent": snap})
		return

	case "delete_ai_agent":
		var j struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		aiAgentsMu.Lock()
		ensureAIAgentsLoadedLocked()
		delete(aiAgents, j.ID)
		saveAIAgentsLocked()
		aiAgentsMu.Unlock()
		aiWriteJSON(w, map[string]any{"status": "ok"})
		return

	case "switch_ai_agent":
		var j struct {
			ID      string `json:"id"`
			DicPath string `json:"dic_path"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		agent, sess, err := aiSwitchAgent(strings.TrimSpace(j.ID), j.DicPath)
		if err != nil {
			aiWriteError(w, err.Error())
			return
		}
		aiWriteJSON(w, map[string]any{"status": "ok", "agent": agent, "session": sess})
		return

	default:
		http.Error(w, `{"status":"error","error":"invalid type"}`, http.StatusBadRequest)
		return
	}
}
