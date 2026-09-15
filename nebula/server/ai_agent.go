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
// 一个「智能体」是一套完整的任务预设：名称、系统提示、工作提示词、
// 可用的内置技能、模型、上下文策略与推理设置。任务（会话）各自选择一个
// 智能体，同一个智能体可被多个任务共用；在任务里切换智能体即把该套预设套用到本任务上，
// 任务的对话与记忆保持不变。各类词库场景（网页 / 机器人 / API）由不同的智能体分别负责。

const (
	// aiAgentsFile 智能体的持久化文件（程序私有目录，与 sessions.json 解耦）
	aiAgentsFile = "private/ai/agents.json"
	// aiBuiltinAgentIDPrefix 内置智能体的 ID 前缀（用户自建智能体 ID 形如 ag<时间戳><随机>，不会冲突）
	aiBuiltinAgentIDPrefix = "builtin:"
	// aiAgentNameMaxRunes 智能体名称最大字符数
	aiAgentNameMaxRunes = 40
	// aiAgentSystemMaxRunes 智能体系统提示最大字符数
	aiAgentSystemMaxRunes = 20000
	// aiAgentPromptMaxRunes 智能体工作提示词最大字符数（随系统提示注入，需精简）
	aiAgentPromptMaxRunes = 8000
)

// AIAgent 智能体：可复用的任务预设配置。
type AIAgent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// 预设系统提示：切换到该智能体时套用到其任务上
	System string `json:"system"`
	// 工作提示词：声明该智能体负责的场景与工作方式，随系统提示注入
	Prompt string `json:"prompt"`
	// 该智能体进入「可用技能」清单的内置技能名称（用户自建技能始终全部可用，无需在此声明）
	Skills []string `json:"skills"`
	Model  string   `json:"model"`
	// 预设上下文附带策略：auto | always | never
	ContextMode string `json:"context_mode"`
	// 预设思考/推理设置：inherit | on | off
	ReasoningMode   string `json:"reasoning_mode"`
	ReasoningEffort string `json:"reasoning_effort"`
	ReasoningModel  string `json:"reasoning_model"`
	// Builtin 标记该智能体为应用内置（出厂预置）：不可删除，仅可编辑。由 ID 前缀推导，不单独落盘
	Builtin   bool  `json:"builtin,omitempty"`
	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
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

// aiBuiltinAgents 出厂内置的智能体：按词库类型划分，各负责一类词库场景，
// 并声明该场景可用的内置技能（与 aiBuiltinSkills 对应）。
// 已存在的同名项保留用户改动，只补齐缺失项；因此内置智能体始终可编辑，但不可删除。
var aiBuiltinAgents = []*AIAgent{
	{
		ID:   "builtin:web",
		Name: "网页词库",
		Prompt: "【当前智能体：网页词库】\n" +
			"本智能体只负责网页词库 .wn（WebNebula）：产出与保存的文件一律以 .wn 结尾，禁止生成机器人 / API 词库那种 .n 文件；\n" +
			".wn 没有触发词概念——文件里不写触发词、不写 Main、不用 %参数N%，页面里的脚本块自上而下依次执行；\n" +
			"注意 <script type=\"nebula\"> 是服务端 Nebula 脚本、不是 JavaScript：块里禁止写 JS（document / console / let / function / 箭头函数等），写了也不会执行；需要浏览器端 JS 就另写不带 type 的普通 <script>，不要把 JS 包进 type=\"nebula\" 块；\n" +
			"动手前先调用 read_skill 读取「" + aiBuiltinSkillWeb + "」技能，按其执行模型与模板渲染规则编写；脚本块内部按「" + aiBuiltinSkillSyntax + "」技能的语法书写，读写文件与运行前读取「" + aiBuiltinSkillTools + "」技能；\n" +
			"改完用 save_dic 保存 .wn，再用 run_web_dic 运行验证渲染结果（脚本块已执行、模板变量已替换），不要只凭保存成功就汇报完成；\n" +
			"用户要求修改词库内容时必须真正调用写入工具完成，不要只输出代码让用户手动粘贴。",
		Skills: []string{aiBuiltinSkillWeb, aiBuiltinSkillSceneDev, aiBuiltinSkillTools, aiBuiltinSkillSyntax},
	},
	{
		ID:   "builtin:bot",
		Name: "机器人词库",
		Prompt: "【当前智能体：机器人词库】\n" +
			"本智能体负责编写机器人 .n 词库（放在机器人账号路径的 dic/ 目录，靠用户发消息触发）：动手前先调用 read_skill 读取「" + aiBuiltinSkillBot + "」技能，按其约定写菜单（逐个列出各功能的触发指令）、触发词与正文；\n" +
			"写代码前读取「" + aiBuiltinSkillSyntax + "」技能，读写文件与运行前读取「" + aiBuiltinSkillTools + "」技能；\n" +
			"功能与娱乐类词条一律不写 Main（机器人按触发词匹配消息，Main 永远不会被消息触发）；用户要求修改词库内容时必须真正调用写入工具完成。",
		Skills: []string{aiBuiltinSkillBot, aiBuiltinSkillSceneDev, aiBuiltinSkillTools, aiBuiltinSkillSyntax},
	},
	{
		ID:   "builtin:api",
		Name: "API词库",
		Prompt: "【当前智能体：API词库】\n" +
			"本智能体负责编写被 HTTP 访问的 .n 接口词库（放在网站根目录下，由 system/router.n 以 Main 为触发词执行）：动手前先调用 read_skill 读取「" + aiBuiltinSkillAPI + "」技能，按其约定处理请求数据与响应；\n" +
			"写代码前读取「" + aiBuiltinSkillSyntax + "」技能，读写文件与运行前读取「" + aiBuiltinSkillTools + "」技能；\n" +
			"本场景恰恰必须写 Main 触发词（与机器人功能词库相反）；用户要求修改词库内容时必须真正调用写入工具完成。",
		Skills: []string{aiBuiltinSkillAPI, aiBuiltinSkillSceneDev, aiBuiltinSkillTools, aiBuiltinSkillSyntax},
	},
}

// aiDefaultAgentID 返回默认智能体的 ID：优先取全局配置「默认智能体」，未配置时回退出厂默认
// 智能体（网页词库）。只读配置、不持锁，供不持有 aiAgentsMu 的调用方（会话归一化）使用；
// 配置指向的智能体若已被删除，由 aiEnsureSessionAgents 在持锁状态下改落到实际存在的默认智能体。
func aiDefaultAgentID() string {
	if c := dto.ServerConfig.AI; c != nil {
		if id := strings.TrimSpace(c.DefaultAgentID); id != "" {
			return id
		}
	}
	return aiBuiltinAgents[0].ID
}

// aiDefaultAgent 返回当前默认智能体的副本：未选择智能体的任务以它为兜底配置。
func aiDefaultAgent() *AIAgent {
	aiAgentsMu.Lock()
	defer aiAgentsMu.Unlock()
	return aiDefaultAgentLocked()
}

// aiDefaultAgentLocked 与 aiDefaultAgent 同义，要求调用方已持有 aiAgentsMu：
// 默认智能体不存在（未配置或已被删除）时回退出厂默认智能体。
func aiDefaultAgentLocked() *AIAgent {
	ensureAIAgentsLoadedLocked()
	if a := aiAgents[aiDefaultAgentID()]; a != nil {
		return aiAgentCopy(a)
	}
	return aiAgentCopy(aiBuiltinAgents[0])
}

// aiAgentByID 按 id 返回智能体副本（任务的「工作提示词 + 可用内置技能」以它为准，因此编辑智能体后立即对其任务生效）；
// id 为空或智能体已不存在时返回默认智能体，保证任务始终有一套可用预设。
func aiAgentByID(id string) *AIAgent {
	key := strings.TrimSpace(id)
	if key == "" {
		return aiDefaultAgent()
	}
	aiAgentsMu.Lock()
	ensureAIAgentsLoadedLocked()
	hit := aiAgentCopy(aiAgents[key])
	aiAgentsMu.Unlock()
	if hit == nil {
		return aiDefaultAgent()
	}
	return hit
}

// aiSeedMissingBuiltinAgentsLocked 补齐缺失的出厂内置智能体；调用方需持有 aiAgentsMu。
// 内置项随程序出厂且被各词库场景依赖，用户误删或旧文件里没有时在此补回；已存在的保留用户改动，不覆盖。
func aiSeedMissingBuiltinAgentsLocked() {
	now := time.Now().Unix()
	changed := false
	for i, b := range aiBuiltinAgents {
		if _, ok := aiAgents[b.ID]; ok {
			continue
		}
		a := aiAgentCopy(b)
		// 时间戳递减，保证列表中按出厂顺序展示（列表按更新时间倒序）
		a.CreatedAt, a.UpdatedAt = now-int64(i), now-int64(i)
		aiNormalizeAgent(a)
		aiAgents[a.ID] = a
		changed = true
	}
	if changed {
		saveAIAgentsLocked()
	}
}

// newAIAgentID 生成智能体 ID（时间戳 + 随机数，避免并发冲突）。
func newAIAgentID() string {
	var b [6]byte
	_, _ = crand.Read(b[:])
	return fmt.Sprintf("ag%d%s", time.Now().UnixMilli(), hex.EncodeToString(b[:]))
}

// aiNormalizeAgentSkills 过滤智能体声明的内置技能清单：只保留确实存在的内置技能名（去重、保持声明顺序）。
func aiNormalizeAgentSkills(names []string) []string {
	skills := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, n := range names {
		b, ok := aiBuiltinSkillFindByName(n)
		if !ok || seen[b.Name] {
			continue
		}
		seen[b.Name] = true
		skills = append(skills, b.Name)
	}
	return skills
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
	a.Prompt = aiClipRunes(strings.TrimSpace(a.Prompt), aiAgentPromptMaxRunes)
	a.Skills = aiNormalizeAgentSkills(a.Skills)
	a.Model = strings.TrimSpace(a.Model)
	a.ContextMode = aiNormalizeContextMode(a.ContextMode)
	a.ReasoningMode = aiNormalizeReasoningMode(a.ReasoningMode)
	a.ReasoningEffort = dto.NormalizeReasoningEffort(a.ReasoningEffort)
	a.ReasoningModel = strings.TrimSpace(a.ReasoningModel)
	// 内置标记由 ID 前缀推导：用户自建智能体 ID 形如 ag<时间戳><随机>，不会与内置前缀冲突
	a.Builtin = strings.HasPrefix(strings.TrimSpace(a.ID), aiBuiltinAgentIDPrefix)
}

// ensureAIAgentsLoadedLocked 首次访问时从私有目录加载智能体；调用方需持有 aiAgentsMu。
func ensureAIAgentsLoadedLocked() {
	if aiAgentsLoaded {
		return
	}
	aiAgentsLoaded = true
	aiAgents = map[string]*AIAgent{}
	data, err := utils.NewFileQueue(aiAgentsFile).ReadFromFile()
	if err == nil && strings.TrimSpace(data) != "" {
		store := aiAgentStore{}
		if json.Unmarshal([]byte(data), &store) == nil {
			for _, a := range store.Agents {
				if a == nil || strings.TrimSpace(a.ID) == "" {
					continue
				}
				aiNormalizeAgent(a)
				aiAgents[a.ID] = a
			}
		}
	}
	// 内置智能体始终补齐：它们随程序出厂，被各词库场景依赖，不可删除
	aiSeedMissingBuiltinAgentsLocked()
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

// aiApplyAgentPreset 把智能体预设套用到任务上（任务里切换智能体与新建任务共用）。
// 只覆盖预设类字段，任务的对话、记忆与标题保持不动。
func aiApplyAgentPreset(sess *AISession, agent *AIAgent) {
	if sess == nil || agent == nil {
		return
	}
	sess.System = agent.System
	sess.Model = agent.Model
	// 智能体未指定模型时落为全局默认模型名（任务不再保留「跟随服务端默认」的空值语义）
	if sess.Model == "" {
		sess.Model = aiDefaultModelName()
	}
	sess.ContextMode = agent.ContextMode
	sess.ReasoningMode = agent.ReasoningMode
	sess.ReasoningEffort = agent.ReasoningEffort
	sess.ReasoningModel = agent.ReasoningModel
}

// aiSetSessionAgent 给指定任务指定智能体：把该智能体的预设套用到任务上（系统提示、模型、
// 上下文策略、推理设置），任务的对话与记忆保持不动；一个智能体可被多个任务共用。
// dicPath 非空时同时把任务关联的词库更新为用户此刻编辑的文件。
func aiSetSessionAgent(sessionID, agentID, dicPath string) (*AIAgent, *AISession, error) {
	aiAgentsMu.Lock()
	ensureAIAgentsLoadedLocked()
	agent := aiAgents[strings.TrimSpace(agentID)]
	if agent == nil {
		aiAgentsMu.Unlock()
		return nil, nil, fmt.Errorf("智能体不存在")
	}
	aiNormalizeAgent(agent)

	aiSessionsMu.Lock()
	ensureAISessionsLoadedLocked()
	sess := aiSessions[strings.TrimSpace(sessionID)]
	if sess == nil {
		aiSessionsMu.Unlock()
		aiAgentsMu.Unlock()
		return nil, nil, fmt.Errorf("任务不存在")
	}
	sess.AgentID = agent.ID
	aiApplyAgentPreset(sess, agent)
	if dp := strings.TrimSpace(dicPath); dp != "" {
		sess.DicPath = dp
	}
	sess.UpdatedAt = time.Now().Unix()
	saveAISessionsLocked()
	snapshot := *sess
	snapshot.Messages = append([]AISessionMessage(nil), sess.Messages...)
	agentSnap := aiAgentCopy(agent)
	aiSessionsMu.Unlock()
	aiAgentsMu.Unlock()
	return agentSnap, &snapshot, nil
}

// aiEnsureSessionAgents 补齐任务的智能体归属：未选用智能体（历史任务）或所选智能体已被删除的
// 任务，一律落为默认智能体。任务因此始终有一个明确的智能体，「切换智能体」处也就始终显示
// 真实选用的智能体名，而不是空值兜底文案；已有归属的任务不受影响。
// 锁顺序与其它入口保持一致：先 aiAgentsMu 再 aiSessionsMu。
func aiEnsureSessionAgents() {
	aiAgentsMu.Lock()
	ensureAIAgentsLoadedLocked()
	defaultID := aiDefaultAgentLocked().ID
	aiSessionsMu.Lock()
	ensureAISessionsLoadedLocked()
	changed := false
	for _, s := range aiSessions {
		if id := strings.TrimSpace(s.AgentID); id != "" {
			if _, ok := aiAgents[id]; ok {
				continue
			}
		}
		s.AgentID = defaultID
		changed = true
	}
	if changed {
		saveAISessionsLocked()
	}
	aiSessionsMu.Unlock()
	aiAgentsMu.Unlock()
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
		// 同时下发默认智能体 id：列表按更新时间排序，客户端无法自行判断哪个是默认，
		// 有了它就能把「缺省」直接指定成具体的默认智能体（新建任务预选、名称回显）
		aiWriteJSON(w, map[string]any{"status": "ok", "list": list, "default_id": aiDefaultAgent().ID})
		return

	case "save_ai_agent":
		var j struct {
			ID              string   `json:"id"`
			Name            string   `json:"name"`
			System          string   `json:"system"`
			Prompt          string   `json:"prompt"`
			Skills          []string `json:"skills"`
			Model           string   `json:"model"`
			ContextMode     string   `json:"context_mode"`
			ReasoningMode   string   `json:"reasoning_mode"`
			ReasoningEffort string   `json:"reasoning_effort"`
			ReasoningModel  string   `json:"reasoning_model"`
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
		agent.Prompt = j.Prompt
		agent.Skills = j.Skills
		agent.Model = j.Model
		agent.ContextMode = j.ContextMode
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
		// 内置智能体随程序出厂、且各类词库场景依赖它兜底，只允许编辑，不允许删除
		if strings.HasPrefix(strings.TrimSpace(j.ID), aiBuiltinAgentIDPrefix) {
			aiWriteError(w, "内置智能体不可删除")
			return
		}
		aiAgentsMu.Lock()
		ensureAIAgentsLoadedLocked()
		delete(aiAgents, j.ID)
		saveAIAgentsLocked()
		aiAgentsMu.Unlock()
		aiWriteJSON(w, map[string]any{"status": "ok"})
		return

	case "set_session_agent":
		// 给指定任务指定智能体：套用其预设到该任务（对话与记忆保持不动）
		var j struct {
			ID        string `json:"id"`
			SessionID string `json:"session_id"`
			DicPath   string `json:"dic_path"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		agent, sess, err := aiSetSessionAgent(j.SessionID, j.ID, j.DicPath)
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
