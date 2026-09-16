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

	"github.com/cjxpj/nebula/appfiles"
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
	Content string `json:"content"`
	// Builtin 标记该技能为应用内置技能：随程序分发、不可编辑或删除，仅接口返回时使用（不落盘）
	Builtin   bool  `json:"builtin,omitempty"`
	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
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
// 内置技能优先：同名时以随程序分发的内置技能为准，避免被用户技能遮蔽。
func aiSkillFindByName(name string) (*AISkill, bool) {
	if s, ok := aiBuiltinSkillFindByName(name); ok {
		return s, true
	}
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
// builtinNames 为当前任务所属智能体声明可用的内置技能名；用户自建技能始终全部列出。
// 常驻技能（见 aiBuiltinPinnedSkills）的正文已直接注入系统提示，这里不再列为「按需读取」。
// 无技能时返回空串（不注入任何内容）。
func aiSkillsPromptText(builtinNames []string) string {
	list := make([]*AISkill, 0, len(builtinNames))
	pinned := make([]string, 0, len(aiBuiltinPinnedSkills))
	for _, name := range builtinNames {
		if b, ok := aiBuiltinSkillFindByName(name); ok {
			if aiBuiltinPinnedSkills[b.Name] {
				pinned = append(pinned, b.Name)
				continue
			}
			list = append(list, b)
		}
	}
	list = append(list, aiSkillList()...)
	if len(list) == 0 && len(pinned) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("【可用技能】\n")
	sb.WriteString("以下是本次任务可用的技能（含应用内置技能）。当任务与某个技能的描述相符时，先调用 read_skill 读取该技能全文，再严格按其中的步骤与约定执行；与描述不符的技能无需读取。\n")
	if len(pinned) > 0 {
		sb.WriteString("（")
		sb.WriteString(strings.Join(pinned, "、"))
		sb.WriteString(" 为常驻技能，正文已直接给出，无需调用 read_skill。）\n")
	}
	for _, s := range list {
		sb.WriteString("- ")
		sb.WriteString(s.Name)
		if s.Builtin {
			sb.WriteString("（内置）")
		}
		if d := strings.TrimSpace(s.Description); d != "" {
			sb.WriteString("：")
			sb.WriteString(d)
		}
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// ============== 应用内置技能 ==============
//
// 「工具能力」这类过去常驻在系统提示里的内容，改为内置技能按需读取：
// 只有任务所属智能体（见 ai_agent.go 的 AIAgent）声明的内置技能才进入「可用技能」清单，
// 模型判断任务与描述相符时再调用 read_skill 读取正文，避免每轮把全部内容塞进上下文。
// 内置技能随程序分发，不可编辑或删除（read_skill 始终可读取，不受智能体声明限制）。
// 例外是「词库语法结构」这类硬约束：它被定为常驻技能（见下方「常驻技能」段），正文每轮直接注入。

// 内置技能名称（read_skill 与「可用技能」清单共用）。
const (
	// aiBuiltinSkillIDPrefix 内置技能快照的 ID 前缀（用户技能 ID 形如 sk<时间戳><随机>，不会冲突）
	aiBuiltinSkillIDPrefix = "builtin:"

	// 按词库类型划分的三类技能（与内置智能体一一对应）
	aiBuiltinSkillWeb = "网页词库开发"
	aiBuiltinSkillBot = "机器人词库开发"
	aiBuiltinSkillAPI = "API词库开发"
	// 三类词库通用的技能
	aiBuiltinSkillSceneDev = "词库场景开发"
	aiBuiltinSkillTools    = "词库工具能力"
	aiBuiltinSkillSyntax   = "词库语法结构"
)

// 内置技能正文不写死在 Go 代码里，而是作为 embed 资源随程序分发：
// appfiles/static/skills/N-<技能名>.md，一个技能一篇 md，头部 front-matter 给出 name 与 description。
// 上面的名称常量供内置智能体声明技能用，必须与对应 md 的 name 保持一致。

// aiBuiltinSkillDir 内置技能资源目录（相对 embed 资源根）
const aiBuiltinSkillDir = "skills"

// aiBuiltinSkill 一条内置技能定义。
type aiBuiltinSkill struct {
	Name        string
	Description string
	Content     string
}

var (
	aiBuiltinSkillsOnce sync.Once
	// aiBuiltinSkills 全部内置技能：切片顺序（md 文件名序号）即「可用技能」清单中的展示顺序
	aiBuiltinSkills []aiBuiltinSkill
)

// aiBuiltinSkillList 返回全部内置技能（进程内只解析一次）。
func aiBuiltinSkillList() []aiBuiltinSkill {
	aiBuiltinSkillsOnce.Do(func() {
		files, err := appfiles.ListFiles(aiBuiltinSkillDir)
		if err != nil {
			return
		}
		for _, f := range files {
			if !strings.HasSuffix(strings.ToLower(f), ".md") {
				continue
			}
			data, err := appfiles.GetFile(f)
			if err != nil {
				continue
			}
			name, desc, body := aiParseSkillFile(string(data))
			if name == "" || body == "" {
				continue
			}
			aiBuiltinSkills = append(aiBuiltinSkills, aiBuiltinSkill{Name: name, Description: desc, Content: body})
		}
	})
	return aiBuiltinSkills
}

// aiParseSkillFile 解析技能资源文件：头部 front-matter（--- 包裹的 name / description）+ 正文。
// 缺 front-matter 或缺 name 时返回空名称，由调用方跳过该文件。
func aiParseSkillFile(text string) (name, desc, body string) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", "", ""
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
		key, value, ok := strings.Cut(lines[i], ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "name":
			name = strings.TrimSpace(value)
		case "description":
			desc = strings.TrimSpace(value)
		}
	}
	if end < 0 {
		return "", "", ""
	}
	return name, desc, strings.TrimSpace(strings.Join(lines[end+1:], "\n"))
}

// aiBuiltinSkillFindByName 按名称查找内置技能（忽略首尾空白与大小写）。
func aiBuiltinSkillFindByName(name string) (*AISkill, bool) {
	key := strings.TrimSpace(name)
	if key == "" {
		return nil, false
	}
	list := aiBuiltinSkillList()
	for i := range list {
		if strings.EqualFold(list[i].Name, key) {
			return aiBuiltinSkillSnapshot(&list[i]), true
		}
	}
	return nil, false
}

// aiBuiltinSkillSnapshot 把内置技能定义转成技能快照（供读技能、技能清单与接口返回共用）。
func aiBuiltinSkillSnapshot(b *aiBuiltinSkill) *AISkill {
	return &AISkill{
		ID:          aiBuiltinSkillIDPrefix + b.Name,
		Name:        b.Name,
		Description: b.Description,
		Content:     b.Content,
		Builtin:     true,
	}
}

// aiAllSkillList 内置技能 + 用户技能（内置在前），供技能管理界面展示。
func aiAllSkillList() []*AISkill {
	list := aiBuiltinSkillList()
	out := make([]*AISkill, 0, len(list))
	for i := range list {
		out = append(out, aiBuiltinSkillSnapshot(&list[i]))
	}
	return append(out, aiSkillList()...)
}

// ---------- 常驻技能 ----------
//
// 语法这类硬约束内容，只在「可用技能」清单里给个名字是不够的：模型读过的文档只存在于对话历史里，
// 多轮之后会被上下文压缩概括掉，于是开始凭印象写错语法。因此把这类技能定为「常驻技能」：
// 正文（连同其指向的易错点文档）每轮随系统提示直接给出，不进对话历史，压缩也就碰不到它。

// aiBuiltinPinnedSkills 常驻技能集合：值为 true 表示正文每轮注入系统提示。
var aiBuiltinPinnedSkills = map[string]bool{
	aiBuiltinSkillSyntax: true,
}

// aiPinnedSyntaxDoc 随「词库语法结构」一起常驻的易错点文档：
// 规则类内容里最短、也最常被写错的一篇，全文常驻的代价很小。
const aiPinnedSyntaxDoc = "docs/0-词库语法/08-高频易错点.md"

// aiPinnedSkillsText 生成常驻技能的正文段，只包含该智能体已声明的常驻技能；
// 未声明常驻技能的智能体（不写词库的场景）返回空串，不占用上下文。
func aiPinnedSkillsText(builtinNames []string) string {
	var sb strings.Builder
	for _, name := range builtinNames {
		b, ok := aiBuiltinSkillFindByName(name)
		if !ok || !aiBuiltinPinnedSkills[b.Name] {
			continue
		}
		body := strings.TrimSpace(b.Content)
		if body == "" {
			continue
		}
		if b.Name == aiBuiltinSkillSyntax {
			if doc, err := dicDocFind(aiPinnedSyntaxDoc); err == nil {
				if extra := strings.TrimSpace(doc.content); extra != "" {
					body += "\n\n" + extra
				}
			}
		}
		sb.WriteString("【常驻技能：")
		sb.WriteString(b.Name)
		sb.WriteString("（每轮直接给出，不随对话压缩丢失，写代码前必须据此复核）】\n")
		sb.WriteString(body)
		sb.WriteString("\n\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// ---------- 接口处理 ----------

// aiSkillHandle 处理 AI 技能相关的 OPUI 请求。
func aiSkillHandle(w http.ResponseWriter, h *HttpOpUiData) {
	switch h.Type {
	case "list_ai_skills":
		// 内置技能在前（builtin=true，前端应禁止编辑 / 删除），用户技能在后
		aiWriteJSON(w, map[string]any{"status": "ok", "list": aiAllSkillList()})
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
		// 内置技能不可修改：既不能按内置 ID 保存，也不能另存一个同名技能（同名会遮蔽内置技能）
		if strings.HasPrefix(strings.TrimSpace(j.ID), aiBuiltinSkillIDPrefix) {
			aiWriteError(w, "应用内置技能不可修改")
			return
		}
		if _, ok := aiBuiltinSkillFindByName(j.Name); ok {
			aiWriteError(w, "「"+strings.TrimSpace(j.Name)+"」是应用内置技能名称，请换一个名称")
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
