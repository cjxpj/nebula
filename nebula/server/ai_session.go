package dic_server

import (
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// ============== AI 多任务会话与独立记忆 ==============
//
// 每个「任务」是一条独立会话：拥有自己的消息、记忆摘要、系统提示与模型。
// 会话统一持久化到程序私有目录，跨设备/重启后仍可继续，且每个任务的记忆互相隔离。

const (
	// aiSessionsFile AI 任务会话的持久化文件（程序私有目录）
	aiSessionsFile = "private/ai/sessions.json"

	// aiSessionKeepMessages 记忆压缩后保留的最近消息条数
	aiSessionKeepMessages = 20
	// aiSessionCompressThreshold 消息条数超过该值时自动触发记忆压缩
	aiSessionCompressThreshold = 40
	// aiSessionMemoryMaxRunes 任务记忆的最大字符数（超出截断）
	aiSessionMemoryMaxRunes = 4000
	// aiSessionTitleMaxRunes 任务标题最大字符数
	aiSessionTitleMaxRunes = 40
	// aiSessionReasoningMaxRunes 单条消息持久化的思考过程最大字符数（超出截断，避免会话文件过大）
	aiSessionReasoningMaxRunes = 8000
	// aiSessionDraftMinInterval 流式草稿落盘的最小间隔：内存实时更新，磁盘按此间隔节流
	aiSessionDraftMinInterval = 3 * time.Second
)

// AISessionMessage 单条对话消息。
type AISessionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Code    string `json:"code,omitempty"`
	// Reasoning 本轮的思考/工具过程，随消息持久化，刷新页面后可恢复
	Reasoning string `json:"reasoning,omitempty"`
	// Draft 标记该消息正在进行中（由流式增量实时写入），收尾后清除
	Draft bool `json:"draft,omitempty"`
	// Interrupted 标记本轮生成被中断（进程重启、超时、限流、工具轮数超限等），
	// 内容可能不完整；前端据此提示，避免把半截回复当成正常回复
	Interrupted bool  `json:"interrupted,omitempty"`
	Time        int64 `json:"time,omitempty"`
}

// AISession AI 任务（会话）：独立记忆 + 独立配置。
type AISession struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	DicPath     string `json:"dic_path"`
	System      string `json:"system"`
	Model       string `json:"model"`
	ContextMode string `json:"context_mode"` // auto | always | never
	// 任务级工具权限：manual 每次调用都需人工审批 | auto 只读与词库写入放行、其它写改删需审批 | full 全部放行
	PermissionMode string `json:"permission_mode"`
	Memory         string `json:"memory"`
	// 任务级思考/推理模式：inherit 跟随全局 | on 开启 | off 关闭
	ReasoningMode string `json:"reasoning_mode"`
	// 任务级推理强度：low | medium | high，留空跟随全局
	ReasoningEffort string `json:"reasoning_effort"`
	// 任务级推理模型名：留空跟随全局
	ReasoningModel string             `json:"reasoning_model"`
	Messages       []AISessionMessage `json:"messages"`
	CreatedAt      int64              `json:"created_at"`
	UpdatedAt      int64              `json:"updated_at"`
}

// aiSessionStore 会话持久化文件结构。
type aiSessionStore struct {
	Version  int          `json:"version"`
	Sessions []*AISession `json:"sessions"`
}

var (
	aiSessionsMu     sync.Mutex
	aiSessions       = map[string]*AISession{}
	aiSessionsLoaded bool
)

// aiCodeBlockRe 匹配 Markdown 围栏代码块（与前端 extractAICode 行为一致）。
var aiCodeBlockRe = regexp.MustCompile("(?s)```[a-zA-Z0-9_-]*\\n(.*?)```")

// aiClipRunes 按字符数截断文本，超出时追加提示。
func aiClipRunes(s string, max int) string {
	rs := []rune(s)
	if len(rs) <= max {
		return s
	}
	return string(rs[:max]) + "\n…(内容过长已截断)"
}

// aiNormalizeContextMode 归一化上下文附带策略。
func aiNormalizeContextMode(m string) string {
	switch strings.TrimSpace(m) {
	case "always", "never":
		return strings.TrimSpace(m)
	default:
		return "auto"
	}
}

// aiNormalizePermissionMode 归一化任务级工具权限，缺省为人工审批。
func aiNormalizePermissionMode(m string) string {
	switch strings.TrimSpace(m) {
	case "auto", "full":
		return strings.TrimSpace(m)
	default:
		return "manual"
	}
}

// aiNormalizeReasoningMode 归一化任务级思考模式，默认跟随全局。
func aiNormalizeReasoningMode(m string) string {
	switch strings.TrimSpace(m) {
	case "on", "off":
		return strings.TrimSpace(m)
	default:
		return "inherit"
	}
}

// aiNormalizeSession 补齐会话的缺省字段。
func aiNormalizeSession(s *AISession) {
	s.Title = strings.TrimSpace(s.Title)
	if s.Title == "" {
		s.Title = "新任务"
	}
	s.ContextMode = aiNormalizeContextMode(s.ContextMode)
	s.PermissionMode = aiNormalizePermissionMode(s.PermissionMode)
	s.ReasoningMode = aiNormalizeReasoningMode(s.ReasoningMode)
	s.ReasoningEffort = dto.NormalizeReasoningEffort(s.ReasoningEffort)
	s.ReasoningModel = strings.TrimSpace(s.ReasoningModel)
	if s.Messages == nil {
		s.Messages = []AISessionMessage{}
	}
	interrupted := 0
	for i := range s.Messages {
		// 草稿只在生成过程中有效：能走到加载说明上一轮生成已结束（进程重启/被中断），
		// 转为「被中断」标记让前端明确提示，避免半截回复被当作正常回复静默展示
		if s.Messages[i].Draft {
			s.Messages[i].Interrupted = true
			interrupted++
		}
		s.Messages[i].Draft = false
	}
	if interrupted > 0 {
		debugLog.Warnf("[AI 会话] 发现 %d 条未收尾的生成草稿，已标记为中断: session_id=%s title=%s",
			interrupted, s.ID, s.Title)
	}
}

// aiEffectiveReasoning 计算实际生效的推理设置：
// 任务级 mode 覆盖全局开关，推理强度与推理模型未设置时回退全局。
// 开启但既未指定推理模型也未指定强度时，默认使用中等强度，确保开关实际生效。
func aiEffectiveReasoning(cfg *dto.AIConfig, mode, effort, model string) (enabled bool, outEffort, outModel string) {
	globalOn := cfg != nil && cfg.Reasoning
	switch aiNormalizeReasoningMode(mode) {
	case "on":
		enabled = true
	case "off":
		enabled = false
	default:
		enabled = globalOn
	}
	outEffort = dto.NormalizeReasoningEffort(effort)
	outModel = strings.TrimSpace(model)
	if cfg != nil {
		if outEffort == "" {
			outEffort = dto.NormalizeReasoningEffort(cfg.ReasoningEffort)
		}
		if outModel == "" {
			outModel = strings.TrimSpace(cfg.ReasoningModel)
		}
	}
	if !enabled {
		return false, "", ""
	}
	if outEffort == "" && outModel == "" {
		outEffort = "medium"
	}
	return true, outEffort, outModel
}

// aiSessionTitle 生成任务标题：优先显式标题，其次取首条消息，最后回退默认值。
func aiSessionTitle(title, firstMsg string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = strings.TrimSpace(firstMsg)
	}
	title = strings.Join(strings.Fields(title), " ")
	if title == "" {
		return "新任务"
	}
	rs := []rune(title)
	if len(rs) > aiSessionTitleMaxRunes {
		title = string(rs[:aiSessionTitleMaxRunes]) + "…"
	}
	return title
}

// newAISessionID 生成会话 ID（时间戳 + 随机数，避免并发冲突）。
func newAISessionID() string {
	var b [6]byte
	_, _ = crand.Read(b[:])
	return fmt.Sprintf("ai%d%s", time.Now().UnixMilli(), hex.EncodeToString(b[:]))
}

// ensureAISessionsLoadedLocked 首次访问时从私有目录加载会话；调用方需持有 aiSessionsMu。
func ensureAISessionsLoadedLocked() {
	if aiSessionsLoaded {
		return
	}
	aiSessionsLoaded = true
	aiSessions = map[string]*AISession{}
	data, err := utils.NewFileQueue(aiSessionsFile).ReadFromFile()
	if err != nil || strings.TrimSpace(data) == "" {
		return
	}
	store := aiSessionStore{}
	if json.Unmarshal([]byte(data), &store) != nil {
		return
	}
	for _, s := range store.Sessions {
		if s == nil || strings.TrimSpace(s.ID) == "" {
			continue
		}
		aiNormalizeSession(s)
		aiSessions[s.ID] = s
	}
}

// saveAISessionsLocked 将会话写入私有目录（按更新时间倒序）；调用方需持有 aiSessionsMu。
func saveAISessionsLocked() {
	utils.NewFileQueue(aiSessionsFile).WriteToFile(aiSessionsJSONLocked())
}

// aiSessionsJSONLocked 序列化全部会话（按更新时间倒序）；调用方需持有 aiSessionsMu。
func aiSessionsJSONLocked() string {
	list := make([]*AISession, 0, len(aiSessions))
	for _, s := range aiSessions {
		list = append(list, s)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].UpdatedAt > list[j].UpdatedAt })
	data, _ := json.Marshal(aiSessionStore{Version: 1, Sessions: list})
	return string(data)
}

// aiSessionMetaJSON 会话元信息（列表用，不含消息正文以减小体积）。
type aiSessionMetaJSON struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	DicPath      string `json:"dic_path"`
	Model        string `json:"model"`
	ContextMode  string `json:"context_mode"`
	MessageCount int    `json:"message_count"`
	HasMemory    bool   `json:"has_memory"`
	UpdatedAt    int64  `json:"updated_at"`
}

// aiSessionMetaOf 由会话生成列表元信息。
func aiSessionMetaOf(s *AISession) aiSessionMetaJSON {
	return aiSessionMetaJSON{
		ID:           s.ID,
		Title:        s.Title,
		DicPath:      s.DicPath,
		Model:        s.Model,
		ContextMode:  s.ContextMode,
		MessageCount: len(s.Messages),
		HasMemory:    strings.TrimSpace(s.Memory) != "",
		UpdatedAt:    s.UpdatedAt,
	}
}

// ---------- 流式草稿：让「生成中」的内容在页面刷新后仍可恢复 ----------
//
// 生成过程中思考链与正文以增量方式推送给前端；这些增量同时写入会话末尾的「草稿」AI 消息，
// 于是刷新页面时（后端仍在生成）get_ai_session 即可返回已产生的部分内容，
// 前端据此恢复「生成中」视图并继续轮询，直到草稿收尾。

var (
	aiStreamDraftMu       sync.Mutex
	aiStreamDraftSessions = map[string]string{}
	aiSessionDraftSavedAt time.Time
)

// aiStreamDraftBegin 登记某个流式请求所属的任务，使其增量可写入该任务的草稿消息。
func aiStreamDraftBegin(streamID, sessionID string) {
	if strings.TrimSpace(streamID) == "" || strings.TrimSpace(sessionID) == "" {
		return
	}
	aiStreamDraftMu.Lock()
	aiStreamDraftSessions[streamID] = sessionID
	aiStreamDraftMu.Unlock()
}

// aiStreamDraftEnd 注销流式草稿登记。
func aiStreamDraftEnd(streamID string) {
	if strings.TrimSpace(streamID) == "" {
		return
	}
	aiStreamDraftMu.Lock()
	delete(aiStreamDraftSessions, streamID)
	aiStreamDraftMu.Unlock()
}

// aiStreamSessionOf 返回某流所属的任务 id（未登记时为空），供按任务取消在途生成使用。
func aiStreamSessionOf(streamID string) string {
	if strings.TrimSpace(streamID) == "" {
		return ""
	}
	aiStreamDraftMu.Lock()
	defer aiStreamDraftMu.Unlock()
	return aiStreamDraftSessions[streamID]
}

// aiSessionAppendDraftDelta 把一条流式增量追加到对应任务的草稿消息上。
func aiSessionAppendDraftDelta(streamID, kind, text string) {
	if text == "" {
		return
	}
	aiStreamDraftMu.Lock()
	sessionID := aiStreamDraftSessions[streamID]
	aiStreamDraftMu.Unlock()
	if sessionID == "" {
		return
	}
	aiSessionsMu.Lock()
	defer aiSessionsMu.Unlock()
	ensureAISessionsLoadedLocked()
	sess := aiSessions[sessionID]
	if sess == nil {
		return
	}
	draft := aiSessionDraftLocked(sess)
	if kind == "reasoning" {
		draft.Reasoning += text
	} else {
		draft.Content += text
	}
	sess.UpdatedAt = time.Now().Unix()
	aiSaveDraftThrottledLocked()
}

// aiSessionDraftLocked 取（必要时新建）会话末尾的草稿消息；调用方需持有 aiSessionsMu。
func aiSessionDraftLocked(sess *AISession) *AISessionMessage {
	if n := len(sess.Messages); n > 0 && sess.Messages[n-1].Draft {
		return &sess.Messages[n-1]
	}
	sess.Messages = append(sess.Messages, AISessionMessage{Role: "assistant", Draft: true, Time: time.Now().Unix()})
	return &sess.Messages[len(sess.Messages)-1]
}

// aiSaveDraftThrottledLocked 按固定间隔落盘草稿；调用方需持有 aiSessionsMu。
func aiSaveDraftThrottledLocked() {
	if time.Since(aiSessionDraftSavedAt) < aiSessionDraftMinInterval {
		return
	}
	aiSessionDraftSavedAt = time.Now()
	saveAISessionsLocked()
}

// aiSessionFinishDraft 收尾草稿：成功时以最终内容为准，失败时保留已产生的部分内容；
// 失败（超时/限流/工具轮数超限等）会给消息打上「被中断」标记，用户主动终止不算中断；
// 内容与思考都没有时移除草稿消息，避免留下空回复。
func aiSessionFinishDraft(sessionID, content, reasoning string, err error) {
	aiSessionsMu.Lock()
	defer aiSessionsMu.Unlock()
	ensureAISessionsLoadedLocked()
	sess := aiSessions[sessionID]
	if sess == nil {
		return
	}
	failed := err != nil
	hasDraft := len(sess.Messages) > 0 && sess.Messages[len(sess.Messages)-1].Draft
	if failed && !hasDraft {
		return
	}
	if !hasDraft {
		sess.Messages = append(sess.Messages, AISessionMessage{Role: "assistant", Time: time.Now().Unix()})
	}
	msg := &sess.Messages[len(sess.Messages)-1]
	if failed {
		msg.Content = strings.TrimSpace(msg.Content)
		msg.Reasoning = aiClipRunes(msg.Reasoning, aiSessionReasoningMaxRunes)
	} else {
		msg.Content = strings.TrimSpace(content)
		msg.Reasoning = aiClipRunes(strings.TrimSpace(reasoning), aiSessionReasoningMaxRunes)
	}
	if msg.Content == "" && msg.Reasoning == "" {
		sess.Messages = sess.Messages[:len(sess.Messages)-1]
		sess.UpdatedAt = time.Now().Unix()
		saveAISessionsLocked()
		return
	}
	msg.Code = aiExtractCode(msg.Content)
	msg.Draft = false
	// 失败保留的部分内容不完整：打标记让前端提示；用户主动终止是有意为之，不打标记
	msg.Interrupted = failed && !errors.Is(err, errAIStreamCancelled)
	sess.UpdatedAt = time.Now().Unix()
	saveAISessionsLocked()
}

// aiClassifyStreamInterrupt 归类本轮生成未完成的原因，便于日志检索。
func aiClassifyStreamInterrupt(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, errAIStreamCancelled) {
		return "用户终止"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "超时"):
		return "超时"
	case strings.Contains(msg, "轮数"):
		return "工具轮数超限"
	case aiRetryable(err):
		return "限流/过载"
	case strings.Contains(msg, "未返回内容"):
		return "上游未返回内容"
	default:
		return "生成失败"
	}
}

// aiLogStreamInterrupt 把本轮生成未完成的原因写入日志（database/log）：
// 记录 stream_id、session_id、已产生的正文/思考规模与耗时，便于定位「生成到一半就断了」。
func aiLogStreamInterrupt(streamID, sessionID, content, reasoning string, startedAt time.Time, err error) {
	detail := fmt.Sprintf("[AI 生成] %s stream_id=%s session_id=%s 正文=%d字 思考=%d字 耗时=%v 原因：%v",
		aiClassifyStreamInterrupt(err), streamID, sessionID,
		len([]rune(content)), len([]rune(reasoning)),
		time.Since(startedAt).Truncate(time.Millisecond), err)
	if errors.Is(err, errAIStreamCancelled) {
		debugLog.Infof("%s", detail)
		return
	}
	debugLog.Warnf("%s", detail)
}

// aiWriteJSON 输出 JSON 响应。
func aiWriteJSON(w http.ResponseWriter, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"status":"error","error":"响应编码失败"}`, http.StatusInternalServerError)
		return
	}
	w.Write(b)
}

// aiWriteError 输出错误响应（HTTP 200 + status=error，供前端统一提示）。
func aiWriteError(w http.ResponseWriter, msg string) {
	aiWriteJSON(w, map[string]string{"status": "error", "error": msg})
}

// aiExtractCode 从 AI 回复中提取首个代码块。
func aiExtractCode(text string) string {
	if m := aiCodeBlockRe.FindStringSubmatch(text); len(m) > 1 {
		return strings.TrimRight(m[1], "\n \t\r")
	}
	return ""
}

// aiSummarizeMemory 调用 AI 把较早的对话归纳为任务记忆。
func aiSummarizeMemory(cfg *dto.AIConfig, model, memory string, old []AISessionMessage) (string, error) {
	var sb strings.Builder
	if m := strings.TrimSpace(memory); m != "" {
		sb.WriteString("已有的任务记忆（请在此基础上补充更新，不要丢失关键信息）：\n")
		sb.WriteString(m)
		sb.WriteString("\n\n")
	}
	sb.WriteString("需要归纳的新增对话记录：\n")
	for _, m := range old {
		role := "用户"
		if m.Role == "assistant" {
			role = "AI"
		}
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		sb.WriteString(role)
		sb.WriteString("：")
		sb.WriteString(aiClipRunes(content, 2000))
		sb.WriteString("\n")
	}
	const system = `你是对话记忆压缩助手。请把用户提供的对话记录归纳为简洁、结构化的「任务记忆」，供后续对话持续参考。
要求：
1. 只输出记忆正文，不要任何前言、解释或 Markdown 代码围栏；
2. 保留关键结论、已确定的方案、待办事项、涉及的文件与函数名、用户偏好与约束；
3. 合并重复信息，删除寒暄与无关内容；
4. 使用中文，条目式呈现，控制在 800 字以内。`
	return aiChatOnce(cfg, model, []aiChatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: sb.String()},
	})
}

// compressAISessionByID 将指定会话较早的消息压缩为任务记忆，仅保留最近若干条。
// AI 调用在锁外进行，避免长时间占用会话锁；应用结果前校验会话未被并发修改。
func compressAISessionByID(id string) (bool, error) {
	aiSessionsMu.Lock()
	ensureAISessionsLoadedLocked()
	sess := aiSessions[id]
	if sess == nil || len(sess.Messages) <= aiSessionKeepMessages {
		aiSessionsMu.Unlock()
		return false, nil
	}
	total := len(sess.Messages)
	cut := total - aiSessionKeepMessages
	old := append([]AISessionMessage(nil), sess.Messages[:cut]...)
	memory := sess.Memory
	model := sess.Model
	aiSessionsMu.Unlock()

	cfg := aiResolveConfig()
	if err := aiCheckReady(cfg, false); err != nil {
		return false, err
	}
	summary, err := aiSummarizeMemory(cfg, model, memory, old)
	if err != nil {
		return false, err
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return false, nil
	}

	aiSessionsMu.Lock()
	defer aiSessionsMu.Unlock()
	sess = aiSessions[id]
	if sess == nil || len(sess.Messages) != total {
		return false, nil // 会话已被并发修改，放弃本次压缩结果
	}
	sess.Memory = aiClipRunes(summary, aiSessionMemoryMaxRunes)
	sess.Messages = append([]AISessionMessage(nil), sess.Messages[cut:]...)
	sess.UpdatedAt = time.Now().Unix()
	saveAISessionsLocked()
	return true, nil
}

// ---------- 接口处理 ----------

// aiSessionHandle 处理 AI 任务会话相关的 OPUI 请求。
func aiSessionHandle(w http.ResponseWriter, h *HttpOpUiData) {
	switch h.Type {
	case "cancel_ai_chat":
		// 终止在途生成：按 stream_id 精确取消，未命中时回退按 session_id 查找（页面刷新后的恢复场景）
		var j struct {
			StreamID  string `json:"stream_id"`
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		aiWriteJSON(w, map[string]any{"status": "ok", "cancelled": aiCancelStream(j.StreamID, j.SessionID)})
		return

	case "list_ai_sessions":
		aiSessionsMu.Lock()
		ensureAISessionsLoadedLocked()
		list := make([]aiSessionMetaJSON, 0, len(aiSessions))
		for _, s := range aiSessions {
			list = append(list, aiSessionMetaOf(s))
		}
		aiSessionsMu.Unlock()
		sort.Slice(list, func(i, j int) bool { return list[i].UpdatedAt > list[j].UpdatedAt })
		aiWriteJSON(w, map[string]any{"status": "ok", "list": list})
		return

	case "get_ai_session":
		var j struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		aiSessionsMu.Lock()
		ensureAISessionsLoadedLocked()
		sess := aiSessions[j.ID]
		var snapshot *AISession
		if sess != nil {
			// 生成中会有并发的草稿增量写入，这里在锁内复制一份，避免序列化时与其竞争
			cp := *sess
			cp.Messages = append([]AISessionMessage(nil), sess.Messages...)
			snapshot = &cp
		}
		aiSessionsMu.Unlock()
		if snapshot == nil {
			aiWriteError(w, "任务不存在")
			return
		}
		aiWriteJSON(w, map[string]any{"status": "ok", "session": snapshot})
		return

	case "create_ai_session":
		var j struct {
			Title   string `json:"title"`
			DicPath string `json:"dic_path"`
			System  string `json:"system"`
			Model   string `json:"model"`
		}
		_ = json.Unmarshal(h.Data, &j)
		now := time.Now().Unix()
		sess := &AISession{
			ID:             newAISessionID(),
			Title:          aiSessionTitle(j.Title, ""),
			DicPath:        strings.TrimSpace(j.DicPath),
			System:         strings.TrimSpace(j.System),
			Model:          strings.TrimSpace(j.Model),
			ContextMode:    "auto",
			PermissionMode: "manual",
			ReasoningMode:  "inherit",
			Messages:       []AISessionMessage{},
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		aiSessionsMu.Lock()
		ensureAISessionsLoadedLocked()
		aiSessions[sess.ID] = sess
		saveAISessionsLocked()
		aiSessionsMu.Unlock()
		aiWriteJSON(w, map[string]any{"status": "ok", "session": sess})
		return

	case "save_ai_session":
		var j struct {
			ID              string  `json:"id"`
			Title           *string `json:"title"`
			DicPath         *string `json:"dic_path"`
			System          *string `json:"system"`
			Model           *string `json:"model"`
			ContextMode     *string `json:"context_mode"`
			Memory          *string `json:"memory"`
			PermissionMode  *string `json:"permission_mode"`
			ReasoningMode   *string `json:"reasoning_mode"`
			ReasoningEffort *string `json:"reasoning_effort"`
			ReasoningModel  *string `json:"reasoning_model"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		aiSessionsMu.Lock()
		ensureAISessionsLoadedLocked()
		sess := aiSessions[j.ID]
		if sess == nil {
			aiSessionsMu.Unlock()
			aiWriteError(w, "任务不存在")
			return
		}
		if j.Title != nil {
			sess.Title = aiSessionTitle(*j.Title, "")
		}
		if j.DicPath != nil {
			sess.DicPath = strings.TrimSpace(*j.DicPath)
		}
		if j.System != nil {
			sess.System = strings.TrimSpace(*j.System)
		}
		if j.Model != nil {
			sess.Model = strings.TrimSpace(*j.Model)
		}
		if j.ContextMode != nil {
			sess.ContextMode = aiNormalizeContextMode(*j.ContextMode)
		}
		if j.Memory != nil {
			sess.Memory = aiClipRunes(strings.TrimSpace(*j.Memory), aiSessionMemoryMaxRunes)
		}
		if j.PermissionMode != nil {
			sess.PermissionMode = aiNormalizePermissionMode(*j.PermissionMode)
		}
		if j.ReasoningMode != nil {
			sess.ReasoningMode = aiNormalizeReasoningMode(*j.ReasoningMode)
		}
		if j.ReasoningEffort != nil {
			sess.ReasoningEffort = dto.NormalizeReasoningEffort(*j.ReasoningEffort)
		}
		if j.ReasoningModel != nil {
			sess.ReasoningModel = strings.TrimSpace(*j.ReasoningModel)
		}
		sess.UpdatedAt = time.Now().Unix()
		saveAISessionsLocked()
		aiSessionsMu.Unlock()
		aiWriteJSON(w, map[string]any{"status": "ok"})
		return

	case "delete_ai_session":
		var j struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		aiSessionsMu.Lock()
		ensureAISessionsLoadedLocked()
		delete(aiSessions, j.ID)
		saveAISessionsLocked()
		aiSessionsMu.Unlock()
		aiWriteJSON(w, map[string]any{"status": "ok"})
		return

	case "clear_ai_session":
		// 清空会话消息与记忆，保留任务本身
		var j struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		aiSessionsMu.Lock()
		ensureAISessionsLoadedLocked()
		if sess := aiSessions[j.ID]; sess != nil {
			sess.Messages = []AISessionMessage{}
			sess.Memory = ""
			sess.UpdatedAt = time.Now().Unix()
			saveAISessionsLocked()
		}
		aiSessionsMu.Unlock()
		aiWriteJSON(w, map[string]any{"status": "ok"})
		return

	case "list_ai_approvals":
		// 返回该任务下仍在等待用户审批的工具调用（刷新页面后可恢复内联审批卡片）
		var j struct {
			SessionID string `json:"session_id"`
		}
		_ = json.Unmarshal(h.Data, &j)
		aiWriteJSON(w, map[string]any{"status": "ok", "list": aiApprovalPendingList(strings.TrimSpace(j.SessionID))})
		return

	case "truncate_ai_messages":
		// 截断到指定条数（用于消息重试/编辑重发）；保留任务记忆
		var j struct {
			ID   string `json:"id"`
			Keep int    `json:"keep"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		aiSessionsMu.Lock()
		ensureAISessionsLoadedLocked()
		sess := aiSessions[j.ID]
		if sess == nil {
			aiSessionsMu.Unlock()
			aiWriteError(w, "任务不存在")
			return
		}
		keep := j.Keep
		if keep < 0 {
			keep = 0
		}
		if keep < len(sess.Messages) {
			sess.Messages = append([]AISessionMessage(nil), sess.Messages[:keep]...)
		}
		sess.UpdatedAt = time.Now().Unix()
		saveAISessionsLocked()
		aiSessionsMu.Unlock()
		aiWriteJSON(w, map[string]any{"status": "ok"})
		return

	case "compress_ai_session":
		// 手动触发记忆压缩
		var j struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		compressed, err := compressAISessionByID(j.ID)
		if err != nil {
			aiWriteError(w, err.Error())
			return
		}
		aiSessionsMu.Lock()
		ensureAISessionsLoadedLocked()
		sess := aiSessions[j.ID]
		var memory string
		if sess != nil {
			memory = sess.Memory
		}
		aiSessionsMu.Unlock()
		aiWriteJSON(w, map[string]any{"status": "ok", "compressed": compressed, "memory": memory})
		return

	case "export_ai_sessions":
		aiSessionsMu.Lock()
		ensureAISessionsLoadedLocked()
		data := aiSessionsJSONLocked()
		aiSessionsMu.Unlock()
		aiWriteJSON(w, map[string]any{"status": "ok", "data": data})
		return

	case "import_ai_sessions":
		var j struct {
			Data string `json:"data"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		incoming, err := aiParseSessions(j.Data)
		if err != nil {
			aiWriteError(w, err.Error())
			return
		}
		now := time.Now().Unix()
		aiSessionsMu.Lock()
		ensureAISessionsLoadedLocked()
		count := 0
		for _, s := range incoming {
			if s == nil {
				continue
			}
			// 统一分配新 ID，避免与现有任务冲突或覆盖
			s.ID = newAISessionID()
			aiNormalizeSession(s)
			if s.CreatedAt == 0 {
				s.CreatedAt = now
			}
			if s.UpdatedAt == 0 {
				s.UpdatedAt = s.CreatedAt
			}
			for i := range s.Messages {
				if s.Messages[i].Role != "assistant" {
					s.Messages[i].Role = "user"
				}
			}
			aiSessions[s.ID] = s
			count++
		}
		saveAISessionsLocked()
		aiSessionsMu.Unlock()
		aiWriteJSON(w, map[string]any{"status": "ok", "count": count})
		return

	default:
		http.Error(w, `{"status":"error","error":"invalid type"}`, http.StatusBadRequest)
		return
	}
}

// aiParseSessions 解析导入数据，兼容「{version,sessions:[...]}」与裸数组两种格式。
func aiParseSessions(data string) ([]*AISession, error) {
	data = strings.TrimSpace(data)
	if data == "" {
		return nil, fmt.Errorf("导入内容为空")
	}
	var store aiSessionStore
	if json.Unmarshal([]byte(data), &store) == nil && len(store.Sessions) > 0 {
		return store.Sessions, nil
	}
	var list []*AISession
	if err := json.Unmarshal([]byte(data), &list); err != nil {
		return nil, fmt.Errorf("导入内容不是有效的任务数据")
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("导入内容中没有任务")
	}
	return list, nil
}

// aiChatHandle 处理 AI 对话请求。
//
// 携带 session_id 时进入「任务模式」：消息与记忆由后端持久化，每轮自动附带任务记忆，
// 消息过多时异步压缩为记忆摘要；未携带 session_id 时保持原有的无状态单轮调用（如连接测试）。
func aiChatHandle(w http.ResponseWriter, h *HttpOpUiData) {
	var j struct {
		SessionID string          `json:"session_id"`
		Message   string          `json:"message"`
		Messages  []aiChatMessage `json:"messages"`
		System    string          `json:"system"`
		Model     string          `json:"model"`
		Context   string          `json:"context"`
		Title     string          `json:"title"`
		DicPath   string          `json:"dic_path"`
		StreamID  string          `json:"stream_id"`
	}
	if err := json.Unmarshal(h.Data, &j); err != nil {
		http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	aiCfg := aiResolveConfig()
	if err := aiCheckReady(aiCfg, false); err != nil {
		aiWriteError(w, err.Error())
		return
	}

	if strings.TrimSpace(j.SessionID) == "" {
		aiChatStateless(w, aiCfg, j.Messages, j.System, j.Model)
		return
	}
	aiChatWithSession(w, aiCfg, j.SessionID, j.Message, j.Title, j.DicPath, j.Context, j.System, j.Model, j.StreamID)
}

// aiChatStateless 无状态单轮对话（兼容连接测试等调用）。
func aiChatStateless(w http.ResponseWriter, aiCfg *dto.AIConfig, messages []aiChatMessage, system, model string) {
	if len(messages) == 0 {
		http.Error(w, `{"status":"error","error":"消息不能为空"}`, http.StatusBadRequest)
		return
	}
	system = strings.TrimSpace(system)
	if system == "" {
		system = aiCfg.SystemPrompt
	}
	if system == "" {
		system = aiDicSystemPrompt
	}
	if f := aiBuiltinFuncsTextCached(); f != "" {
		system += "\n\n" + f
	}
	msgs := make([]aiChatMessage, 0, len(messages)+1)
	msgs = append(msgs, aiChatMessage{Role: "system", Content: system})
	msgs = append(msgs, messages...)
	content, err := aiChatOnce(aiCfg, model, msgs)
	if err != nil {
		aiWriteError(w, err.Error())
		return
	}
	aiWriteJSON(w, map[string]string{"status": "ok", "content": content})
}

// aiChatWithSession 基于任务的对话：读取历史与记忆、追加消息、持久化并按需压缩。
// streamID 非空时以流式方式请求上游，并经由 WS 推送思考/正文增量（多轮思考实时呈现）。
func aiChatWithSession(w http.ResponseWriter, aiCfg *dto.AIConfig, sessionID, message, title, dicPath, context, system, model, streamID string) {
	aiSessionsMu.Lock()
	ensureAISessionsLoadedLocked()
	sess := aiSessions[sessionID]
	if sess == nil {
		// 前端通常已先行创建；此处兜底新建，保证不丢消息
		now := time.Now().Unix()
		sess = &AISession{ID: sessionID, ContextMode: "auto", Messages: []AISessionMessage{}, CreatedAt: now, UpdatedAt: now}
		aiSessions[sess.ID] = sess
	}
	if t := strings.TrimSpace(message); t != "" {
		sess.Messages = append(sess.Messages, AISessionMessage{Role: "user", Content: t, Time: time.Now().Unix()})
		// 首次发送消息时用消息内容作为任务标题
		if sess.Title == "" || sess.Title == "新任务" {
			sess.Title = aiSessionTitle(title, t)
		}
	}
	if dp := strings.TrimSpace(dicPath); dp != "" && sess.DicPath == "" {
		sess.DicPath = dp
	}
	sess.UpdatedAt = time.Now().Unix()
	history := append([]AISessionMessage(nil), sess.Messages...)
	memory := sess.Memory
	baseSystem := strings.TrimSpace(sess.System)
	sessModel := sess.Model
	reasoningMode := sess.ReasoningMode
	reasoningEffort := sess.ReasoningEffort
	reasoningModel := sess.ReasoningModel
	permissionMode := aiNormalizePermissionMode(sess.PermissionMode)
	saveAISessionsLocked()
	aiSessionsMu.Unlock()

	if len(history) == 0 {
		aiWriteError(w, "消息不能为空")
		return
	}
	if history[len(history)-1].Role != "user" {
		aiWriteError(w, "请先发送一条消息")
		return
	}

	if baseSystem == "" {
		baseSystem = strings.TrimSpace(system)
	}
	if baseSystem == "" {
		baseSystem = aiCfg.SystemPrompt
	}
	if baseSystem == "" {
		baseSystem = aiDicSystemPrompt
	}
	if m := strings.TrimSpace(memory); m != "" {
		baseSystem += "\n\n【任务记忆（较早对话的摘要，供持续参考）】\n" + m
	}
	if f := aiBuiltinFuncsTextCached(); f != "" {
		baseSystem += "\n\n" + f
	}
	// 词库调试 AI 具备文件/词库工具能力：补充工具使用与「改盘后同步编辑器」的约定
	baseSystem += aiDicToolsPrompt
	if dp := strings.TrimSpace(sess.DicPath); dp != "" {
		baseSystem += "\n\n当前任务关联的词库文件：" + dp
	}

	if sessModel == "" {
		sessModel = model
	}

	// 思考/推理模式：任务级设置优先，未设置时跟随全局；开启且指定了推理模型时切换模型
	reasonEnabled, reasonEffort, reasonModel := aiEffectiveReasoning(aiCfg, reasoningMode, reasoningEffort, reasoningModel)
	if reasonEnabled && reasonModel != "" {
		sessModel = reasonModel
	}

	msgs := make([]aiChatMessage, 0, len(history)+2)
	msgs = append(msgs, aiChatMessage{Role: "system", Content: baseSystem})
	// 当前词库实时信息由前端「按需附带」，仅本次请求生效，不写入历史
	if ctx := strings.TrimSpace(context); ctx != "" {
		msgs = append(msgs, aiChatMessage{Role: "system", Content: "以下是当前词库的实时信息，供本次回答参考：\n\n" + ctx})
	}
	for _, m := range history {
		role := m.Role
		if role != "assistant" {
			role = "user"
		}
		msgs = append(msgs, aiChatMessage{Role: role, Content: m.Content})
	}

	if streamID != "" {
		// 登记可取消上下文：用户点击「终止」时据此打断上游请求
		aiRegisterStreamCancel(streamID)
		defer aiUnregisterStreamCancel(streamID)
		// 生成过程中的增量会同步写入任务草稿，刷新页面后可恢复已产生的思考与正文
		aiStreamDraftBegin(streamID, sessionID)
		defer aiStreamDraftEnd(streamID)
		aiStreamNotify(streamID, "ai_stream_start", nil)
	}
	chatStartedAt := time.Now()
	content, reasoning, err := aiChatWithTools(aiCfg, sessModel, msgs, reasonEffort, streamID, permissionMode, sessionID)
	// 无论成功或失败都收尾草稿：失败时保留已产生的部分内容，避免刷新后整轮消失
	aiSessionFinishDraft(sessionID, content, reasoning, err)
	if err != nil {
		// 中断/超时/限流/终止的原因落盘 database/log，便于事后定位「生成到一半就断了」
		aiLogStreamInterrupt(streamID, sessionID, content, reasoning, chatStartedAt, err)
	}
	terminated := errors.Is(err, errAIStreamCancelled)
	if err != nil && !terminated {
		// 用户消息与已产生的部分内容均已落盘，便于直接重试
		aiStreamNotify(streamID, "ai_stream_error", map[string]any{"error": err.Error()})
		aiWriteError(w, err.Error())
		return
	}

	aiSessionsMu.Lock()
	ensureAISessionsLoadedLocked()
	sess = aiSessions[sessionID]
	if sess == nil {
		aiSessionsMu.Unlock()
		aiWriteError(w, "任务不存在")
		return
	}
	snapshot := *sess
	snapshot.Messages = append([]AISessionMessage(nil), sess.Messages...)
	needCompress := len(sess.Messages) > aiSessionCompressThreshold
	aiSessionsMu.Unlock()

	aiWriteJSON(w, map[string]any{
		"status":     "ok",
		"terminated": terminated,
		"content":    content,
		"code":       aiExtractCode(content),
		"reasoning":  reasoning,
		"session_id": snapshot.ID,
		"session":    &snapshot,
	})

	// 用户主动终止：不再推送收尾事件（前端已按终止态收尾），也无需压缩历史
	if terminated {
		return
	}

	aiStreamNotify(streamID, "ai_stream_end", map[string]any{
		"content":   content,
		"code":      aiExtractCode(content),
		"reasoning": reasoning,
	})

	// 消息过多时异步压缩，避免阻塞本次对话响应
	if needCompress {
		go func(id string) { _, _ = compressAISessionByID(id) }(snapshot.ID)
	}
}
