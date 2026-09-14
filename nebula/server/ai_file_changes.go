package dic_server

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/cjxpj/nebula/utils"
)

// ============== AI 文件改动记录（逐行标注 + 按任务回撤） ==============
//
// AI 通过工具改动磁盘文件时，本模块在落盘前抓取「改动前快照」，落盘后登记一条改动记录：
//   1. 编辑器据此逐行标注「本任务内被 AI 改过的行」；
//   2. 任务面板据此列出「当前任务的待确认改动文件」；
//   3. 用户可把某个文件一键回撤（驳回）到本任务首次改动之前的内容；
//   4. 用户确认改动无误（保存）后，该记录移出待确认列表，不再标注、不可回撤。
// 记录以「任务 + 当前路径」为粒度聚合：首次改动时定格回撤目标（是否存在 + 内容），后续改动只累加次数。

const (
	// aiFileChangesFile 文件改动记录的持久化文件（程序私有目录）
	aiFileChangesFile = "private/ai/file_changes.json"
	// aiFileChangeMaxEntries 记录总条数上限，超出裁剪最旧记录，避免长期使用后无限膨胀
	aiFileChangeMaxEntries = 2000
	// aiFileChangeMaxSnapBytes 单文件快照最大字节数，超出则放弃快照（无法回撤内容，仍列出改动）
	aiFileChangeMaxSnapBytes = 4 << 20
	// aiFileChangeMaxDiffLines 参与逐行标注比对的单侧最大行数，超出放弃比对（避免大文件上做无意义的逐行匹配）
	aiFileChangeMaxDiffLines = 5000
)

// AIFileChange AI 在本任务内对某个文件的改动。
type AIFileChange struct {
	SessionID string `json:"session_id"`
	Path      string `json:"path"`
	Action    string `json:"action"` // 最近一次动作：write | save | delete | rename | move
	// OrigExists 本任务首次改动该路径前，文件是否存在
	OrigExists bool `json:"orig_exists"`
	// OrigContent 本任务首次改动前的内容（文本文件），回撤目标
	OrigContent string `json:"orig_content,omitempty"`
	// NoSnapshot 改动前的文件是二进制或体积过大，未保存快照：可列出但无法回撤内容
	NoSnapshot bool  `json:"no_snapshot,omitempty"`
	Edits      int   `json:"edits"`
	Time       int64 `json:"time"`
}

// aiFileChangeStore 记录持久化文件结构。
type aiFileChangeStore struct {
	Version int             `json:"version"`
	Changes []*AIFileChange `json:"changes"`
}

var (
	aiFileChangesMu     sync.Mutex
	aiFileChanges       = map[string]*AIFileChange{} // key: sessionID + "\x00" + path
	aiFileChangesLoaded bool
)

// aiFileChangeKey 记录键：任务 + 路径。
func aiFileChangeKey(sessionID, path string) string {
	return sessionID + "\x00" + path
}

// aiFileChangesEnsureLoadedLocked 首次访问时从私有目录加载记录；调用方需持有 aiFileChangesMu。
func aiFileChangesEnsureLoadedLocked() {
	if aiFileChangesLoaded {
		return
	}
	aiFileChangesLoaded = true
	aiFileChanges = map[string]*AIFileChange{}
	data, err := utils.NewFileQueue(aiFileChangesFile).ReadFromFile()
	if err != nil || strings.TrimSpace(data) == "" {
		return
	}
	store := aiFileChangeStore{}
	if json.Unmarshal([]byte(data), &store) != nil {
		return
	}
	for _, c := range store.Changes {
		if c == nil || c.SessionID == "" || c.Path == "" {
			continue
		}
		aiFileChanges[aiFileChangeKey(c.SessionID, c.Path)] = c
	}
}

// aiFileChangesSaveLocked 把记录写回私有目录（按时间倒序，超上限裁剪最旧）；调用方需持有 aiFileChangesMu。
func aiFileChangesSaveLocked() {
	list := make([]*AIFileChange, 0, len(aiFileChanges))
	for _, c := range aiFileChanges {
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Time > list[j].Time })
	if len(list) > aiFileChangeMaxEntries {
		for _, c := range list[aiFileChangeMaxEntries:] {
			delete(aiFileChanges, aiFileChangeKey(c.SessionID, c.Path))
		}
		list = list[:aiFileChangeMaxEntries]
	}
	data, err := json.Marshal(aiFileChangeStore{Version: 1, Changes: list})
	if err != nil {
		return
	}
	utils.NewFileQueue(aiFileChangesFile).WriteToFile(string(data))
}

// aiFileChangeRecord 登记一次文件改动：首次见到该路径时定格回撤目标，之后只累加次数。
func aiFileChangeRecord(sessionID, path, action string, origExists bool, origContent string, noSnapshot bool) {
	if sessionID == "" || path == "" {
		return
	}
	aiFileChangesMu.Lock()
	defer aiFileChangesMu.Unlock()
	aiFileChangesEnsureLoadedLocked()
	key := aiFileChangeKey(sessionID, path)
	if c := aiFileChanges[key]; c != nil {
		c.Action = action
		c.Edits++
		c.Time = time.Now().Unix()
		aiFileChangesSaveLocked()
		return
	}
	aiFileChanges[key] = &AIFileChange{
		SessionID:   sessionID,
		Path:        path,
		Action:      action,
		OrigExists:  origExists,
		OrigContent: origContent,
		NoSnapshot:  noSnapshot,
		Edits:       1,
		Time:        time.Now().Unix(),
	}
	aiFileChangesSaveLocked()
}

// aiFileChangeList 返回某任务的全部文件改动（按时间倒序）。
func aiFileChangeList(sessionID string) []*AIFileChange {
	aiFileChangesMu.Lock()
	defer aiFileChangesMu.Unlock()
	aiFileChangesEnsureLoadedLocked()
	list := make([]*AIFileChange, 0, 8)
	for _, c := range aiFileChanges {
		if c.SessionID == sessionID {
			list = append(list, c)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Time > list[j].Time })
	return list
}

// aiFileChangeOf 取某任务下某个路径的改动记录（不存在返回 nil）。
func aiFileChangeOf(sessionID, path string) *AIFileChange {
	aiFileChangesMu.Lock()
	defer aiFileChangesMu.Unlock()
	aiFileChangesEnsureLoadedLocked()
	return aiFileChanges[aiFileChangeKey(sessionID, path)]
}

// aiFileChangeRemoveSession 删除某任务的全部改动记录（任务被删除时调用）。
func aiFileChangeRemoveSession(sessionID string) {
	aiFileChangesMu.Lock()
	defer aiFileChangesMu.Unlock()
	aiFileChangesEnsureLoadedLocked()
	changed := false
	for k, c := range aiFileChanges {
		if c.SessionID == sessionID {
			delete(aiFileChanges, k)
			changed = true
		}
	}
	if changed {
		aiFileChangesSaveLocked()
	}
}

// aiFileChangeDrop 删除单条改动记录（回撤成功后调用）。
func aiFileChangeDrop(sessionID, path string) {
	aiFileChangesMu.Lock()
	defer aiFileChangesMu.Unlock()
	aiFileChangesEnsureLoadedLocked()
	key := aiFileChangeKey(sessionID, path)
	if _, ok := aiFileChanges[key]; ok {
		delete(aiFileChanges, key)
		aiFileChangesSaveLocked()
	}
}

// aiFileChangeConfirm 确认 AI 改动无误：接受该改动并把记录移出「待确认」列表，不改动磁盘内容。
// path 为空表示确认该任务下全部待确认改动；返回确认的条数。
// 确认后该文件的回撤基线一并清除：后续再被 AI 改动时，以本次确认后的内容为新基线。
func aiFileChangeConfirm(sessionID, path string) int {
	if sessionID == "" {
		return 0
	}
	aiFileChangesMu.Lock()
	defer aiFileChangesMu.Unlock()
	aiFileChangesEnsureLoadedLocked()
	confirmed := 0
	if path != "" {
		key := aiFileChangeKey(sessionID, path)
		if _, ok := aiFileChanges[key]; ok {
			delete(aiFileChanges, key)
			confirmed = 1
		}
	} else {
		for k, c := range aiFileChanges {
			if c.SessionID == sessionID {
				delete(aiFileChanges, k)
				confirmed++
			}
		}
	}
	if confirmed > 0 {
		aiFileChangesSaveLocked()
	}
	return confirmed
}

// aiFileAbsPath 把应用目录内的相对路径解析为绝对路径。
func aiFileAbsPath(rel string) string {
	return filepath.Join(opuiAppDir(), filepath.FromSlash(rel))
}

// aiReadTextSnapshot 读取文件内容作为文本快照；不存在返回 (false, "", false)，
// 二进制或超大返回 (true, "", true)（存在但无法快照）。
func aiReadTextSnapshot(rel string) (exists bool, content string, noSnap bool) {
	full := aiFileAbsPath(rel)
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		return false, "", false
	}
	if info.Size() > aiFileChangeMaxSnapBytes {
		return true, "", true
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return true, "", true
	}
	if !utf8.Valid(data) || strings.IndexByte(string(data), 0) >= 0 {
		return true, "", true
	}
	return true, string(data), false
}

// aiRestoreFile 把内容写回应用目录内的相对路径（必要时创建上级目录）。
func aiRestoreFile(rel, content string) error {
	full := aiFileAbsPath(rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, []byte(content), 0o644)
}

// aiFileChangeRevert 把某个文件回撤到本任务首次改动前的内容。
// 首次改动前不存在（AI 新建）时，回撤即删除该文件。
func aiFileChangeRevert(sessionID, path string) error {
	c := aiFileChangeOf(sessionID, path)
	if c == nil {
		return errors.New("该文件没有可回撤的 AI 改动记录")
	}
	if c.NoSnapshot {
		return errors.New("该文件改动前的内容未保存快照（二进制或体积过大），无法回撤")
	}
	if c.OrigExists {
		if err := aiRestoreFile(path, c.OrigContent); err != nil {
			return err
		}
	} else {
		if err := os.RemoveAll(aiFileAbsPath(path)); err != nil {
			return err
		}
	}
	aiFileChangeDrop(sessionID, path)
	return nil
}

// aiFileChangesRevertAll 回撤某任务的全部文件改动，返回成功数与失败原因。
func aiFileChangesRevertAll(sessionID string) (int, []string) {
	list := aiFileChangeList(sessionID)
	done := 0
	fails := make([]string, 0)
	for _, c := range list {
		if err := aiFileChangeRevert(sessionID, c.Path); err != nil {
			fails = append(fails, c.Path+"："+err.Error())
			continue
		}
		done++
	}
	return done, fails
}

// aiChangedLines 计算新内容中「相对旧内容新增或改动」的行号（1 起）。
// 采用逐行贪心匹配：按顺序尽量把新行匹配到旧行，匹配不上的即为改动行。
// 对词库这类「整段重写」的场景足够准确，实现简单且无需引入 diff 依赖。
func aiChangedLines(oldText, newText string) []int {
	if oldText == newText {
		return nil
	}
	oldLines := strings.Split(oldText, "\n")
	newLines := strings.Split(newText, "\n")
	if len(oldLines) > aiFileChangeMaxDiffLines || len(newLines) > aiFileChangeMaxDiffLines {
		return nil
	}
	// 旧内容行号索引（同一文本可能出现多行，按出现顺序记录）
	index := make(map[string][]int, len(oldLines))
	for i, l := range oldLines {
		index[l] = append(index[l], i)
	}
	cursor := make(map[string]int, len(oldLines))
	last := -1
	changed := make([]int, 0)
	for i, l := range newLines {
		list := index[l]
		p := cursor[l]
		for p < len(list) && list[p] <= last {
			p++
		}
		if p < len(list) {
			last = list[p]
			cursor[l] = p + 1
			continue
		}
		changed = append(changed, i+1)
	}
	return changed
}

// ---------- 工具执行前后的快照与登记 ----------

// aiFileTarget 工具执行前捕获的一个目标文件。
type aiFileTarget struct {
	Path    string // 改动前路径
	NewPath string // 重命名 / 移动后的新路径（其它工具为空）
	Exists  bool
	Content string
	NoSnap  bool
}

// aiFileToolTargets 解析工具参数，返回该工具会改动的文件目标；非写工具返回 nil。
func aiFileToolTargets(toolName, argsJSON string) []aiFileTarget {
	switch toolName {
	case "write_file", "save_dic", "delete_file":
		var a struct {
			Path string `json:"path"`
		}
		if aiToolDecode(argsJSON, &a) != nil {
			return nil
		}
		p := strings.TrimSpace(a.Path)
		if p == "" {
			return nil
		}
		return []aiFileTarget{{Path: p}}
	case "rename_file":
		var a struct {
			Path    string `json:"path"`
			NewName string `json:"new_name"`
		}
		if aiToolDecode(argsJSON, &a) != nil {
			return nil
		}
		p := strings.TrimSpace(a.Path)
		name := strings.TrimSpace(a.NewName)
		if p == "" || name == "" {
			return nil
		}
		newPath := filepath.ToSlash(filepath.Join(filepath.Dir(filepath.FromSlash(p)), name))
		return []aiFileTarget{{Path: p, NewPath: newPath}}
	case "move_file":
		var a struct {
			Paths  []string `json:"paths"`
			Target string   `json:"target"`
		}
		if aiToolDecode(argsJSON, &a) != nil {
			return nil
		}
		target := strings.TrimSpace(a.Target)
		targets := make([]aiFileTarget, 0, len(a.Paths))
		for _, p := range a.Paths {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			targets = append(targets, aiFileTarget{
				Path:    p,
				NewPath: filepath.ToSlash(filepath.Join(target, filepath.Base(filepath.FromSlash(p)))),
			})
		}
		return targets
	default:
		return nil
	}
}

// aiFileToolCapture 逐个抓取目标的改动前内容。
func aiFileToolCapture(targets []aiFileTarget) {
	for i := range targets {
		targets[i].Content = targetContentSet(&targets[i])
	}
}

// targetContentSet 读取目标文件改动前的内容并写回目标结构。
func targetContentSet(t *aiFileTarget) string {
	exists, content, noSnap := aiReadTextSnapshot(t.Path)
	t.Exists = exists
	t.NoSnap = noSnap
	return content
}

// aiFileToolRecord 工具执行后登记改动：按落盘后的实际状态判断是否真的发生改动，
// 只登记确有变化的路径（工具失败时状态不变，自动跳过）。
// 返回本次被改动的路径列表（供推送逐行标注）。
func aiFileToolRecord(sessionID, toolName string, targets []aiFileTarget) []string {
	changed := make([]string, 0, len(targets))
	for i := range targets {
		t := &targets[i]
		switch toolName {
		case "write_file", "save_dic":
			exists, newContent, _ := aiReadTextSnapshot(t.Path)
			if !exists {
				continue // 写入失败或文件未落地
			}
			if t.Exists && t.Content == newContent {
				continue // 内容未变化（工具失败），不登记
			}
			aiFileChangeRecord(sessionID, t.Path, aiFileActionOf(toolName), t.Exists, t.Content, t.NoSnap)
			changed = append(changed, t.Path)
			aiNotifyFileDiff(sessionID, t.Path, newContent)
		case "delete_file":
			if _, err := os.Stat(aiFileAbsPath(t.Path)); err == nil {
				continue // 仍存在，说明删除失败
			}
			aiFileChangeRecord(sessionID, t.Path, "delete", t.Exists, t.Content, t.NoSnap)
			changed = append(changed, t.Path)
		case "rename_file", "move_file":
			if t.NewPath == "" {
				continue
			}
			// 旧路径应已消失、新路径应已存在，否则视为失败跳过
			if _, err := os.Stat(aiFileAbsPath(t.Path)); err == nil {
				continue
			}
			if _, err := os.Stat(aiFileAbsPath(t.NewPath)); err != nil {
				continue
			}
			action := aiFileActionOf(toolName)
			aiFileChangeRecord(sessionID, t.Path, action, t.Exists, t.Content, t.NoSnap)
			aiFileChangeRecord(sessionID, t.NewPath, action, false, "", false)
			changed = append(changed, t.Path, t.NewPath)
		}
	}
	return changed
}

// aiFileActionOf 工具名映射为记录中的动作名。
func aiFileActionOf(toolName string) string {
	switch toolName {
	case "save_dic":
		return "save"
	case "write_file":
		return "write"
	case "delete_file":
		return "delete"
	case "rename_file":
		return "rename"
	case "move_file":
		return "move"
	default:
		return toolName
	}
}

// aiNotifyFileDiff 推送「本任务内该文件被 AI 改动的行」，供编辑器逐行标注。
// 以任务首次改动前的快照为基线，与刚落盘的内容比对，得到累计改动行；
// 基线不存在（AI 新建）时全部行视为改动；行数为空则推送空列表以清除前端标注。
func aiNotifyFileDiff(sessionID, path, newContent string) {
	c := aiFileChangeOf(sessionID, path)
	if c == nil || c.NoSnapshot {
		return
	}
	base := ""
	if c.OrigExists {
		base = c.OrigContent
	}
	lines := aiChangedLines(base, newContent)
	if lines == nil {
		lines = []int{}
	}
	msg, err := json.Marshal(map[string]any{"type": "ai_file_diff", "data": map[string]any{
		"session_id":    sessionID,
		"path":          path,
		"changed_lines": lines,
	}})
	if err != nil {
		return
	}
	broadcastOpuiNotify(msg)
}
