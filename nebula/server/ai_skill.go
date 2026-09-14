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

	"github.com/cjxpj/nebula/utils"
)

// ============== AI 技能（全局一份，按需读取） ==============
//
// 一个「技能」是一份可复用的工作指引：名称 + 触发描述 + 正文。
// 系统提示中只注入技能清单（名称 + 描述），模型判断任务与某技能描述相符时，
// 再调用 read_skill 工具读取该技能正文，避免把所有技能全文都塞进每轮上下文。
// 技能全局共享（所有任务可用），持久化于程序私有目录。

const (
	// aiSkillsFile 技能的持久化文件（程序私有目录，与 agents.json / sessions.json 解耦）
	aiSkillsFile = "private/ai/skills.json"
	// aiSkillNameMaxRunes 技能名称最大字符数
	aiSkillNameMaxRunes = 40
	// aiSkillDescMaxRunes 技能描述最大字符数（注入系统提示，需精简）
	aiSkillDescMaxRunes = 500
	// aiSkillContentMaxRunes 技能正文最大字符数（超出截断，避免单文件过大）
	aiSkillContentMaxRunes = 40000
)

// AISkill 技能：名称 + 触发描述 + 正文。
type AISkill struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Description 技能的适用场景描述：随系统提示注入，供模型判断是否需要读取本技能
	Description string `json:"description"`
	// Content 技能正文（步骤与约定）：仅在模型调用 read_skill 时返回
	Content   string `json:"content"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// aiSkillStore 技能持久化文件结构。
type aiSkillStore struct {
	Version int        `json:"version"`
	Skills  []*AISkill `json:"skills"`
}

var (
	aiSkillsMu     sync.Mutex
	aiSkills       = map[string]*AISkill{}
	aiSkillsLoaded bool
)

// newAISkillID 生成技能 ID（时间戳 + 随机数，避免并发冲突）。
func newAISkillID() string {
	var b [6]byte
	_, _ = crand.Read(b[:])
	return fmt.Sprintf("sk%d%s", time.Now().UnixMilli(), hex.EncodeToString(b[:]))
}

// aiNormalizeSkill 补齐并规范化技能字段。
func aiNormalizeSkill(s *AISkill) {
	name := strings.TrimSpace(s.Name)
	if name == "" {
		name = "未命名技能"
	}
	if rs := []rune(name); len(rs) > aiSkillNameMaxRunes {
		name = string(rs[:aiSkillNameMaxRunes]) + "…"
	}
	s.Name = name
	s.Description = aiClipRunes(strings.TrimSpace(s.Description), aiSkillDescMaxRunes)
	s.Content = aiClipRunes(strings.TrimSpace(s.Content), aiSkillContentMaxRunes)
}

// ensureAISkillsLoadedLocked 首次访问时从私有目录加载技能；调用方需持有 aiSkillsMu。
func ensureAISkillsLoadedLocked() {
	if aiSkillsLoaded {
		return
	}
	aiSkillsLoaded = true
	aiSkills = map[string]*AISkill{}
	data, err := utils.NewFileQueue(aiSkillsFile).ReadFromFile()
	if err != nil || strings.TrimSpace(data) == "" {
		return
	}
	store := aiSkillStore{}
	if json.Unmarshal([]byte(data), &store) != nil {
		return
	}
	for _, s := range store.Skills {
		if s == nil || strings.TrimSpace(s.ID) == "" {
			continue
		}
		aiNormalizeSkill(s)
		aiSkills[s.ID] = s
	}
}

// saveAISkillsLocked 把技能写入私有目录（按更新时间倒序）；调用方需持有 aiSkillsMu。
func saveAISkillsLocked() {
	list := make([]*AISkill, 0, len(aiSkills))
	for _, s := range aiSkills {
		list = append(list, s)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].UpdatedAt > list[j].UpdatedAt })
	data, _ := json.Marshal(aiSkillStore{Version: 1, Skills: list})
	utils.NewFileQueue(aiSkillsFile).WriteToFile(string(data))
}

// aiSkillCopy 返回技能的副本，避免调用方在锁外读取到并发修改。
func aiSkillCopy(s *AISkill) *AISkill {
	if s == nil {
		return nil
	}
	cp := *s
	return &cp
}

// aiSkillList 返回全部技能（按更新时间倒序）的副本快照。
func aiSkillList() []*AISkill {
	aiSkillsMu.Lock()
	ensureAISkillsLoadedLocked()
	list := make([]*AISkill, 0, len(aiSkills))
	for _, s := range aiSkills {
		list = append(list, aiSkillCopy(s))
	}
	aiSkillsMu.Unlock()
	sort.Slice(list, func(i, j int) bool { return list[i].UpdatedAt > list[j].UpdatedAt })
	return list
}

// aiSkillFindByName 按名称查找技能（忽略首尾空白与大小写）。
func aiSkillFindByName(name string) (*AISkill, bool) {
	key := strings.TrimSpace(name)
	if key == "" {
		return nil, false
	}
	aiSkillsMu.Lock()
	ensureAISkillsLoadedLocked()
	var hit *AISkill
	for _, s := range aiSkills {
		if strings.EqualFold(strings.TrimSpace(s.Name), key) {
			hit = aiSkillCopy(s)
			break
		}
	}
	aiSkillsMu.Unlock()
	return hit, hit != nil
}

// aiSkillsPromptText 生成注入系统提示的「技能清单」：仅名称 + 描述，提示模型按需调用 read_skill。
// 无技能时返回空串（不注入任何内容）。
func aiSkillsPromptText() string {
	list := aiSkillList()
	if len(list) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("【可用技能】\n")
	sb.WriteString("以下是用户为你配置的技能。当任务与某个技能的描述相符时，先调用 read_skill 读取该技能全文，再严格按其中的步骤与约定执行；与描述不符的技能无需读取。\n")
	for _, s := range list {
		sb.WriteString("- ")
		sb.WriteString(s.Name)
		if d := strings.TrimSpace(s.Description); d != "" {
			sb.WriteString("：")
			sb.WriteString(d)
		}
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// ---------- 接口处理 ----------

// aiSkillHandle 处理 AI 技能相关的 OPUI 请求。
func aiSkillHandle(w http.ResponseWriter, h *HttpOpUiData) {
	switch h.Type {
	case "list_ai_skills":
		aiWriteJSON(w, map[string]any{"status": "ok", "list": aiSkillList()})
		return

	case "save_ai_skill":
		var j struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
			Content     string `json:"content"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(j.Name) == "" {
			aiWriteError(w, "请填写技能名称")
			return
		}
		aiSkillsMu.Lock()
		ensureAISkillsLoadedLocked()
		now := time.Now().Unix()
		skill := aiSkills[strings.TrimSpace(j.ID)]
		if skill == nil {
			skill = &AISkill{ID: newAISkillID(), CreatedAt: now}
			aiSkills[skill.ID] = skill
		}
		skill.Name = j.Name
		skill.Description = j.Description
		skill.Content = j.Content
		skill.UpdatedAt = now
		aiNormalizeSkill(skill)
		saveAISkillsLocked()
		snap := aiSkillCopy(skill)
		aiSkillsMu.Unlock()
		aiWriteJSON(w, map[string]any{"status": "ok", "skill": snap})
		return

	case "delete_ai_skill":
		var j struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		aiSkillsMu.Lock()
		ensureAISkillsLoadedLocked()
		delete(aiSkills, strings.TrimSpace(j.ID))
		saveAISkillsLocked()
		aiSkillsMu.Unlock()
		aiWriteJSON(w, map[string]any{"status": "ok"})
		return

	default:
		http.Error(w, `{"status":"error","error":"invalid type"}`, http.StatusBadRequest)
		return
	}
}
