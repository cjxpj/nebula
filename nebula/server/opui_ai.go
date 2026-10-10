package dic_server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dic_funcs "github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
)

// ============== AI 对接（OpenAI 兼容，默认 DeepSeek） ==============

// aiDicSystemPrompt AI 词库开发内置系统提示词（智能体未配置系统提示时使用）。
// 只保留身份定位与事实约束：语法结构与工具能力已拆成内置技能（见 ai_skill.go），
// 「词库语法结构」作为常驻技能每轮直接给出正文，「词库工具能力」仍由「可用技能」清单按需 read_skill 读取。
const aiDicSystemPrompt = `你是 Nebula 词库（.n / .wn 文件）开发助手。回答必须以本次对话实际提供的信息为准，不得臆造。

写或修改词库代码前，先按下方【常驻技能：词库语法结构】的要求重新读取相关语法文档并逐条复核，严格按文档原文书写，不要凭记忆或前几轮留下的印象拼代码；需要读取 / 写入文件、编译或运行词库时，先读取「词库工具能力」技能，按其中的调用通道与词条边界约定执行。

书写纪律（硬性）：一律按语法原样书写，禁止为了对齐、美观或分段插入多余的空行与空格——正文行与行紧贴、块结构（如果>/循环>/匹配>/遍历>/文本>/函数> 等）内部不得出现空行，%变量%、$函数 参数$、键:值 之间只保留语法要求的单个空格，行首 / 行尾不要补空格，[...] 表达式内不写空格（写 [1000*%秒%]，不写 [1000 * %秒%]）；行首缩进不必自己对，保存时会按 .n 块结构 / .wn HTML 结构自动格式化（.wn 的 <?n ... ?> 内联块与 <script type="nebula"> 脚本块正文同样会被自动排版，行首缩进在解析前会被裁掉；确实要保留行首空白的行用 //@关闭缩进 与 //@启用缩进 夹起来）。唯一必须保留的空行是词条边界：任何情况下触发词都独占一行，且其上方必须有一个空行与头部或上一条词条分隔（漏掉这个空行，触发词会被并入上一条或头部、完全失效，这不属于多余空行；文件第一个词条就是触发词时，文件开头同样要留一个空行；.wn 没有触发词，不适用这条）。

事实约束：
1. 当前词库的函数、变量、类、触发词与语法，一律以用户提供的「当前词库代码 / 编译诊断」为准；未出现在其中、也未出现在下方【可用内置函数】清单中的名称，视为不存在；
2. 只能调用下方【可用内置函数】清单里列出的全局函数，禁止编造或用品名相近的顶替；清单没有所需能力时如实说「没有该内置函数」；
3. 清单只含全局函数，不含实例方法的名称与签名：实例（$创建画布$、$new 类名$ 得到）的方法必须写 $变量.方法 参数$（如 $变量.设置颜色 ...$），不得写成全局函数形式；实例方法的参数个数不在清单中，禁止臆测，需以文档或示例为准，拿不准就说「不确定」；
4. 若本次未附带「当前词库代码」，而问题又取决于具体词库内容，应先说明缺少代码并请用户提供，或只作一般性讲解并标注不确定，禁止凭空猜测词库内容；
5. 无法确定时如实说明，不要编造。
输出要求：
1. 只输出与词库开发相关的内容，简洁直接；
2. 需要给出代码时使用 Markdown 围栏代码块包裹，代码必须符合 Nebula 词库语法；
3. 若用户提供了编译错误或警告信息，优先逐条修复，并简要说明每处修改的原因。`

// aiCompleteSystemPrompt AI 内联补全内置系统提示词：要求只输出待插入的代码片段。
const aiCompleteSystemPrompt = `你是 Nebula 词库（.n 文件）代码补全引擎。根据用户给出的上下文，在【光标处】续写最符合语境的词库代码。
要求：
1. 只输出需要插入到【光标处】的代码，禁止输出任何解释、注释说明或 Markdown 代码围栏；
2. 保持与上下文一致的语言风格与缩进；
3. 新增词条时，触发词独占一行且其上方留一个空行（空行是词条的硬边界，漏了触发词会被并入上一条），触发词默认写 Main（词库调试默认按 Main 运行），除非上下文已明确其它触发词；
4. 若无法确定合适的补全内容，输出空字符串。`

// aiBuiltinFuncsHeader 内置函数清单的说明头（完整清单用）。
const aiBuiltinFuncsHeader = "【可用内置函数】词库中可直接调用以下内置函数，调用格式为 $函数名 参数...$；" +
	"名称后括号内为该函数允许的参数个数（如 1|2 表示 1 或 2 个参数，2.. 表示 2 个及以上，0 表示无参数），未标注括号时为 1 个参数。\n"

// aiBuiltinFuncsHeaderUsed 按需注入（仅当前代码已调用函数）时的说明头。
const aiBuiltinFuncsHeaderUsed = "【当前代码已调用的内置函数】调用格式为 $函数名 参数...$；" +
	"名称后括号内为该函数允许的参数个数，未标注括号时为 1 个参数。\n"

// aiBuiltinFuncsEntry 缓存的函数清单文本及其对应的注册表版本号。
type aiBuiltinFuncsEntry struct {
	rev  int64
	text string
}

// aiBuiltinFuncsCache 缓存函数清单文本：注册表未变更时直接复用，避免每次请求重建 300+ 函数列表，
// 同时保持 system 提示词稳定，便于上游接口做前缀缓存。
var aiBuiltinFuncsCache atomic.Value // aiBuiltinFuncsEntry

// aiFormatFuncNames 将函数名格式化为「名称(参数规则)」文本；无规则时仅输出名称。
func aiFormatFuncNames(names []string) string {
	parts := make([]string, 0, len(names))
	for _, n := range names {
		if rule, ok := dto.GetFuncRule(n); ok && rule != "" {
			parts = append(parts, n+"("+rule+")")
			continue
		}
		parts = append(parts, n)
	}
	return strings.Join(parts, " ")
}

// aiBuiltinFuncsText 汇总当前已注册的全部内置函数（名称 + 参数个数规则），
// 供 AI 对话时了解可用函数。ListFuncs 读取运行期注册表，新注册（含云工具注入）的函数会实时出现。
func aiBuiltinFuncsText() string {
	infos := dic_funcs.ListFuncs()
	if len(infos) == 0 {
		return ""
	}
	names := make([]string, 0, len(infos))
	for _, info := range infos {
		names = append(names, info.Name)
	}
	return aiBuiltinFuncsHeader + aiFormatFuncNames(names)
}

// aiBuiltinFuncsTextCached 返回带缓存的内置函数完整清单。
func aiBuiltinFuncsTextCached() string {
	rev := dic_funcs.Revision()
	if v, ok := aiBuiltinFuncsCache.Load().(aiBuiltinFuncsEntry); ok && v.rev == rev {
		return v.text
	}
	text := aiBuiltinFuncsText()
	aiBuiltinFuncsCache.Store(aiBuiltinFuncsEntry{rev: rev, text: text})
	return text
}

// aiBuiltinFuncsTextUsed 仅汇总 code 中实际调用的内置函数，用于高频的内联补全按需注入，
// 避免每次补全都下发完整函数清单造成 token 与延迟浪费。
func aiBuiltinFuncsTextUsed(code string) string {
	names := aiReferencedFuncs(code)
	if len(names) == 0 {
		return ""
	}
	return aiBuiltinFuncsHeaderUsed + aiFormatFuncNames(names)
}

// aiReferencedFuncs 扫描词库代码，按出现顺序提取其中已注册的内置函数名（形如 $函数名 参数$）。
func aiReferencedFuncs(code string) []string {
	if code == "" {
		return nil
	}
	seen := make(map[string]struct{})
	names := make([]string, 0)
	for i := 0; i < len(code); i++ {
		if code[i] != '$' {
			continue
		}
		j := i + 1
		for j < len(code) && !strings.ContainsRune("$ \t\r\n", rune(code[j])) {
			j++
		}
		name := code[i+1 : j]
		if _, ok := seen[name]; !ok {
			if _, registered := dto.GetFuncRule(name); registered {
				seen[name] = struct{}{}
				names = append(names, name)
			}
		}
		i = j
	}
	return names
}

// aiToolCall 模型返回的工具（函数）调用请求（OpenAI 兼容格式）。
type aiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// aiChatMessage 单条对话消息（OpenAI 兼容格式）。
// 工具调用链中：assistant 消息可携带 ToolCalls；tool 消息通过 ToolCallID 回填执行结果。
// Content 为 any：普通消息是字符串；视觉能力开启时需携带图片的消息为多模态分段数组
// （text / image_url），直接序列化即可被上游识别。
type aiChatMessage struct {
	Role       string       `json:"role"`
	Content    any          `json:"content"`
	ToolCalls  []aiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
	Name       string       `json:"name,omitempty"`
}

// aiConfigJSON 将 AI 配置转为前端 JSON 结构。
// 密钥已加密存储在配置文件中，不下发明文，每个模型仅告知是否已配置密钥。
func aiConfigJSON(c *dto.AIConfig) map[string]any {
	if c == nil {
		return defaultAIConfigJSON()
	}
	models := make([]map[string]any, 0, len(c.Models))
	for _, m := range c.Models {
		if m == nil {
			continue
		}
		models = append(models, map[string]any{
			"id":               m.ID,
			"name":             m.Name,
			"base_url":         m.BaseURL,
			"api_key_set":      strings.TrimSpace(m.APIKey) != "",
			"model":            m.Model,
			"vision":           m.Vision,
			"reasoning":        m.Reasoning,
			"reasoning_effort": m.ReasoningEffort,
			"reasoning_model":  m.ReasoningModel,
			"context_length":   m.ContextLength,
			"max_tokens":       m.MaxTokens,
			"rate_limit":       m.RateLimit,
			"concurrency":      m.Concurrency,
		})
	}
	return map[string]any{
		"open":              c.Open,
		"current_id":        c.CurrentID,
		"approval_mode":     c.ApprovalMode,
		"auto_continue":     c.AutoContinue,
		"auto_continue_max": c.AutoContinueMax,
		"default_agent_id":  c.DefaultAgentID,
		"models":            models,
		"timeout":           c.Timeout,
		"inline_complete":   c.InlineComplete,
		"vision":            c.Vision,
		// 兼容既有调用方（如词库调试的模型候选）：附带当前模型的解析结果
		"base_url":         c.BaseURL,
		"model":            c.Model,
		"reasoning":        c.Reasoning,
		"reasoning_effort": c.ReasoningEffort,
		"reasoning_model":  c.ReasoningModel,
		"context_length":   c.ContextLength,
		"max_tokens":       c.MaxTokens,
	}
}

// defaultAIConfigJSON 返回 AI 配置的默认值（配置文件缺失时使用）。
func defaultAIConfigJSON() map[string]any {
	return map[string]any{
		"open":              false,
		"current_id":        "",
		"approval_mode":     dto.DefaultAIPermissionMode,
		"auto_continue":     true,
		"auto_continue_max": dto.DefaultAIAutoContinueMax,
		"default_agent_id":  "",
		"models":            []map[string]any{},
		"timeout":           60,
		"inline_complete":   true,
		"vision":            false,
		"base_url":          "",
		"model":             dto.DefaultAIModel,
		"reasoning":         false,
		"reasoning_effort":  "",
		"reasoning_model":   "",
		"context_length":    dto.DefaultAIContextLength,
		"max_tokens":        dto.DefaultAIMaxTokens,
	}
}

// aiResolveConfig 返回当前 AI 配置：优先取运行期内存值，缺失时回退读取配置文件。
func aiResolveConfig() *dto.AIConfig {
	if c := dto.ServerConfig.AI; c != nil {
		return c
	}
	cfg, err := dto.LoadConfigFile()
	if err != nil {
		return &dto.AIConfig{Model: dto.DefaultAIModel, Timeout: 60, InlineComplete: true, ApprovalMode: dto.DefaultAIPermissionMode, AutoContinue: true, AutoContinueMax: dto.DefaultAIAutoContinueMax}
	}
	return dto.LoadConfig_ai(cfg.Section("AI"))
}

// aiCheckReady 校验 AI 配置是否可用于发起请求；forComplete 为 true 时额外要求开启内联补全。
func aiCheckReady(c *dto.AIConfig, forComplete bool) error {
	if c == nil || !c.Open {
		return errors.New("AI 未启用，请先在「基础配置 → AI」中开启")
	}
	if strings.TrimSpace(c.BaseURL) == "" {
		return errors.New("未配置 AI 接口地址，请先在「基础配置 → AI」中填写")
	}
	if forComplete && !c.InlineComplete {
		return errors.New("未开启代码补全")
	}
	return nil
}

// aiChatOnce 调用 OpenAI 兼容的 chat/completions 接口，返回首个候选的文本内容。
func aiChatOnce(c *dto.AIConfig, model string, messages []aiChatMessage) (string, error) {
	content, _, err := aiChatOnceReasoning(c, model, messages, "")
	return content, err
}

// aiChatOnceReasoning 调用 chat/completions 接口，返回首个候选的文本内容与思维链（不启用工具）。
func aiChatOnceReasoning(c *dto.AIConfig, model string, messages []aiChatMessage, reasoningEffort string) (string, string, error) {
	content, reasoning, _, _, err := aiChatOnceTools(c, model, messages, reasoningEffort, nil, "", "")
	return content, reasoning, err
}

// ============== AI 上游限流/过载自动重试 ==============
//
// 服务商高峰期常返回 HTTP 429 或「该模型当前访问量过大，请您稍后再试」等瞬时错误，
// 这类失败稍后重试即可恢复，不应直接抛给用户。退避等待期间会经 WS 推送重试状态，
// 避免前端看起来像卡死。

const (
	// aiRetryMaxAttempts 一般瞬时故障（网关抖动、网络断流）的最大尝试次数（含首次请求）
	aiRetryMaxAttempts = 5
	// aiRetryRateLimitMaxAttempts 模型过载 / 账户限流（如「该模型当前访问量过大，请您稍后再试」）
	// 的最大尝试次数：上游高峰通常持续一两分钟，只重试几次会在十几秒内就被判失败，
	// 这里放宽次数以便熬过高峰。
	aiRetryRateLimitMaxAttempts = 8
	// aiRetryStallMaxAttempts 上游静默超时（连接建着却不吐任何数据）的最大尝试次数：
	// 每次尝试最坏要等满一个空闲超时窗口（默认 3 分钟），只安排一次重试——重试能救回
	// 上游偶发断流，又不至于为了一个持续过载的上游让用户等上十几分钟。
	aiRetryStallMaxAttempts = 2
	// aiRetryBaseDelay 一般瞬时故障（网关抖动、过载）首次重试前的等待时长，之后按 2 倍退避
	aiRetryBaseDelay = time.Second
	// aiRetryRateLimitBaseDelay 账户级限流（按分钟窗口计费）首次重试前的等待时长，
	// 这类错误需要更长的冷却时间，退避序列为 3s→6s→12s→24s→30s…
	aiRetryRateLimitBaseDelay = 3 * time.Second
	// aiRetryMaxDelay 单次退避等待的上限：翻倍退避到很长时截断，避免前端长时间无进展
	aiRetryMaxDelay = 30 * time.Second
)

// aiRetryableError 标记可重试的上游错误（限流、过载、网关抖动等）。
type aiRetryableError struct{ msg string }

func (e *aiRetryableError) Error() string { return e.msg }

// aiStallError 标记「上游静默超时」：请求已发出（响应头也可能已返回），但上游长时间
// 一个字节都不吐，被 aiStreamIdleTimeout 的看门狗掐断。它与限流/网络故障同属
// 「换一条新请求通常就能恢复」的类型，但每次尝试最坏要等满一个空闲超时窗口，
// 因此单独成一类，只安排一次重试（见 aiRetryStallMaxAttempts）。
type aiStallError struct{ msg string }

func (e *aiStallError) Error() string { return e.msg }

// aiNewStallError 构造「上游静默超时」错误。
func aiNewStallError(msg string) error { return &aiStallError{msg: msg} }

// aiRetryableByMessage 判断错误文案是否属于可重试的限流/过载类错误。
func aiRetryableByMessage(msg string) bool {
	lower := strings.ToLower(msg)
	for _, kw := range []string{
		"访问量过大", "稍后再试", "请稍后重试", "请求过于频繁", "限流",
		"速率限制", "请求频率", "频率限制", "配额已满",
		"rate limit", "too many requests", "overloaded", "temporarily unavailable", "server busy",
	} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// aiRetryRateLimitByMessage 判断错误文案是否属于账户/接口级限流与模型过载（按分钟窗口类），
// 这类错误需要比网关抖动更长的冷却时间和更多次尝试，重试时采用更大的退避基数。
func aiRetryRateLimitByMessage(msg string) bool {
	lower := strings.ToLower(msg)
	for _, kw := range []string{
		"速率限制", "请求频率", "频率限制", "配额已满", "限流", "请求过于频繁",
		// 智谱等厂商高峰期返回的「该模型当前访问量过大，请您稍后再试」属于模型过载，
		// 同属「等一会儿就好」的类型，按限流处理才能拿到足够长的退避与重试次数
		"访问量过大", "稍后再试", "请稍后重试", "繁忙", "过载",
		"rate limit", "too many requests", "overloaded",
	} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// aiRetryableByStatus 判断 HTTP 状态码是否属于可重试的瞬时故障。
func aiRetryableByStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests, http.StatusInternalServerError,
		http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// aiRetryableByNetError 判断错误是否属于网络层瞬时故障（连不上、连接被重置、读超时、
// 连接中途断开等）。这类失败多由链路抖动或对端瞬时不可用引起，稍后重试通常即可恢复，
// 不应直接判为生成失败——否则一次建连失败就会把前面等待的时间全部白费。
func aiRetryableByNetError(err error) bool {
	if err == nil {
		return false
	}
	// 上下文被取消（用户点了「终止」）不属于网络故障，不应重试
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// 传输层超时（Dial / Read / TLS 握手超时）
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	lower := strings.ToLower(err.Error())
	for _, kw := range []string{
		// Windows（wsarecv / wsasend）与类 Unix 的常见瞬时故障描述
		"connection attempt failed", "connection reset", "connection refused",
		"connection aborted", "connection timed out", "connection closed",
		"broken pipe", "wsarecv", "wsasend",
		"read tcp", "write tcp", "dial tcp", "i/o timeout", "tls handshake timeout",
		"network is unreachable", "no such host", "eof",
	} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// aiNewRetryableError 构造可重试错误。
func aiNewRetryableError(msg string) error { return &aiRetryableError{msg: msg} }

// aiRetryable 报告错误是否应触发自动重试。
func aiRetryable(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := errors.AsType[*aiRetryableError](err); ok {
		return true
	}
	// 上游静默超时：连接建着却不吐数据，重发一条新请求通常就能恢复
	if _, ok := errors.AsType[*aiStallError](err); ok {
		return true
	}
	return aiRetryableByMessage(err.Error()) || aiRetryableByNetError(err)
}

// ============== AI 每模型频率 / 并发限制 ==============
//
// 同一账户下不同模型往往有各自的调用配额（每分钟请求数、最大并发数），
// 并发过高或短时间请求过密会被上游限流甚至封禁。这里按「模型配置项」维护一套限制：
// 频率限制把相邻两次请求的发起时刻按最小间隔错开，并发限制约束同时进行中的请求数。
// 频率上限可留空（0 表示不限制）；并发上限默认 1，即同一模型串行请求，可按模型上调。
// 保存配置后立即生效。

// aiModelLimiter 单个模型配置项对应的频率 / 并发限制状态。
type aiModelLimiter struct {
	mu sync.Mutex
	// rateLimit 每分钟最大请求数（0 表示不限制），interval 为其换算出的相邻请求最小间隔
	rateLimit int
	interval  time.Duration
	// nextStart 下一次允许发起请求的最早时刻
	nextStart time.Time
	// concLimit 最大并发请求数（0 表示不限制），concSem 为并发名额
	concLimit int
	concSem   chan struct{}
}

// configure 按最新配置调整限制；数值未变化时保持现状，不打断正在等待或进行中的请求。
func (l *aiModelLimiter) configure(rateLimit, concurrency int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if rateLimit != l.rateLimit {
		l.rateLimit = rateLimit
		l.nextStart = time.Time{}
		if rateLimit > 0 {
			l.interval = time.Minute / time.Duration(rateLimit)
		} else {
			l.interval = 0
		}
	}
	if concurrency != l.concLimit {
		l.concLimit = concurrency
		if concurrency > 0 {
			l.concSem = make(chan struct{}, concurrency)
		} else {
			l.concSem = nil
		}
	}
}

// acquire 申请一次请求配额：先按频率上限等到允许发起的时刻，再占用一个并发名额。
// 返回的释放函数须在请求结束后调用，用于归还并发名额。
// 等待超过 1 秒、或并发名额已满时经 WS 告知前端，避免界面看起来像卡死（streamID 为空时提示自动忽略）。
func (l *aiModelLimiter) acquire(streamID string) func() {
	l.mu.Lock()
	var wait time.Duration
	if l.interval > 0 {
		now := time.Now()
		start := now
		if l.nextStart.After(now) {
			start = l.nextStart
			wait = start.Sub(now)
		}
		l.nextStart = start.Add(l.interval)
	}
	sem := l.concSem
	l.mu.Unlock()

	if wait > 0 {
		if wait >= time.Second {
			aiStreamNotify(streamID, "ai_stream_delta", map[string]any{
				"kind":    "wait",
				"reason":  "模型频率限制",
				"seconds": int((wait + time.Second - 1) / time.Second),
			})
		}
		time.Sleep(wait)
	}
	if sem == nil {
		return func() {}
	}
	select {
	case sem <- struct{}{}:
	default:
		// 并发名额已满：先告知前端正在排队，再等待名额释放
		aiStreamNotify(streamID, "ai_stream_delta", map[string]any{
			"kind":   "wait",
			"reason": "模型并发已满",
		})
		sem <- struct{}{}
	}
	return func() { <-sem }
}

// aiModelLimiters 按模型配置项 ID 维护的限流器表
var (
	aiModelLimitersMu sync.Mutex
	aiModelLimiters   = map[string]*aiModelLimiter{}
)

// aiModelLimiterFor 取（必要时创建）指定模型配置项对应的限流器，并按最新配置调整。
func aiModelLimiterFor(id string, rateLimit, concurrency int) *aiModelLimiter {
	aiModelLimitersMu.Lock()
	defer aiModelLimitersMu.Unlock()
	l := aiModelLimiters[id]
	if l == nil {
		l = &aiModelLimiter{}
		aiModelLimiters[id] = l
	}
	l.configure(rateLimit, concurrency)
	return l
}

// aiModelLimiterOf 解析本次请求实际所用模型对应的限流器：
// 按模型名匹配模型列表中的某项（任务可单独指定模型，含推理模型），未匹配时回退当前选中模型；
// 没有任何模型配置时返回 nil，表示不做限制。
func aiModelLimiterOf(c *dto.AIConfig, model string) *aiModelLimiter {
	if c == nil {
		return nil
	}
	m := dto.PickAIModelByName(c.Models, model)
	if m == nil {
		m = dto.PickAIModel(c.Models, c.CurrentID)
	}
	if m == nil || m.ID == "" {
		return nil
	}
	return aiModelLimiterFor(m.ID, m.RateLimit, m.Concurrency)
}

// aiRetryBackoff 按「可重试则指数退避重试」的策略执行一次上游请求，最多尝试
// aiRetryMaxAttempts 次（模型过载 / 账户限流放宽到 aiRetryRateLimitMaxAttempts 次）。
// 限流/过载、以及网络层瞬时故障（连不上、连接被重置、读超时、连接中途断开等）都属于
// 稍后重试即可恢复的类型，交给这里统一退避重试；其它错误（参数错误、鉴权失败等）
// 立即返回，不做无谓等待。
// 退避等待期间会经 WS 推送重试状态，避免前端看起来像卡死；用户点「终止」时等待立即结束。
func aiRetryBackoff(streamID string, fn func() error) error {
	var lastErr error
	delay := aiRetryBaseDelay
	attempts := aiRetryMaxAttempts
	// 父上下文用于打断退避等待：否则用户点了「终止」还要把当前这轮退避睡满才停下
	ctx := aiStreamParentCtx(streamID)
	for attempt := 1; attempt <= attempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err
		if !aiRetryable(err) {
			break
		}
		// 「该模型当前访问量过大，请您稍后再试」这类模型过载要等上游高峰过去，
		// 按限流处理：更长的冷却时间（3s 起）+ 更多次尝试，而不是 1s 起、5 次就放弃
		rateLimited := aiRetryRateLimitByMessage(err.Error())
		if rateLimited {
			attempts = aiRetryRateLimitMaxAttempts
			if attempt == 1 {
				delay = aiRetryRateLimitBaseDelay
			}
		}
		// 上游静默超时：每次尝试最坏要等满一个空闲超时窗口，只再试一次
		stalled := false
		if _, ok := errors.AsType[*aiStallError](err); ok {
			stalled = true
			attempts = aiRetryStallMaxAttempts
		}
		if attempt >= attempts {
			break
		}
		// 让前端知道正处于退避等待中，而不是卡死（streamID 为空时该调用自动忽略）。
		// 这里只给出秒数与次数，由前端自行倒计时，文案才能逐秒跳动。
		reason := "上游繁忙"
		switch {
		case rateLimited:
			reason = "触发限流"
		case stalled:
			reason = "上游无响应"
		case aiRetryableByNetError(err):
			reason = "网络异常"
		}
		aiStreamNotify(streamID, "ai_stream_delta", map[string]any{
			"kind":    "retry",
			"reason":  reason,
			"attempt": attempt,
			"seconds": int(delay / time.Second),
		})
		if !aiRetrySleep(ctx, delay) {
			// 等待期间用户终止了本次生成：按「用户终止」返回，不记为失败
			return errAIStreamCancelled
		}
		delay *= 2
		if delay > aiRetryMaxDelay {
			delay = aiRetryMaxDelay
		}
	}
	return lastErr
}

// aiRetrySleep 可被「终止」打断的退避等待：返回 false 表示等待期间用户取消了本次生成。
func aiRetrySleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 || ctx == nil {
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// aiChatOnceTools 在 aiChatOnceToolsOnce 基础上，对限流/过载与网络层瞬时故障做自动退避重试。
// 重试期间前一次尝试已流出的思考/正文会作废（重试请求会重新生成），因此失败时统一返回空内容。
func aiChatOnceTools(c *dto.AIConfig, model string, messages []aiChatMessage, reasoningEffort string, tools []map[string]any, toolChoice, streamID string) (string, string, []aiToolCall, string, error) {
	limiter := aiModelLimiterOf(c, model)
	var content, reasoning, finishReason string
	var calls []aiToolCall
	err := aiRetryBackoff(streamID, func() error {
		release := func() {}
		if limiter != nil {
			release = limiter.acquire(streamID)
		}
		defer release()
		var e error
		content, reasoning, calls, finishReason, e = aiChatOnceToolsOnce(c, model, messages, reasoningEffort, tools, toolChoice, streamID)
		return e
	})
	if err != nil {
		return "", "", nil, "", err
	}
	return content, reasoning, calls, finishReason, nil
}

// ============== 智谱 GLM 专有请求参数 ==============

// aiIsZhipuProvider 判断本次请求是否发往智谱 GLM：接口地址为 bigmodel.cn 开放平台，或模型名为 glm-* 系列。
// 智谱专有的 thinking / tool_stream 参数只对它下发，避免其他 OpenAI 兼容服务商收到未知字段。
func aiIsZhipuProvider(baseURL, model string) bool {
	if strings.Contains(strings.ToLower(baseURL), "bigmodel.cn") {
		return true
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "glm-")
}

// aiGLMVersion 解析 glm-* 模型名的主/次版本号（glm-5.3-flash → 5,3；glm-4.5 → 4,5；glm-5v-turbo → 5,0）。
// 非 glm-* 或无法解析时 ok 为 false。
func aiGLMVersion(model string) (major, minor int, ok bool) {
	m := strings.ToLower(strings.TrimSpace(model))
	if !strings.HasPrefix(m, "glm-") {
		return 0, 0, false
	}
	m = strings.TrimPrefix(m, "glm-")
	if n, _ := fmt.Sscanf(m, "%d.%d", &major, &minor); n == 2 {
		return major, minor, true
	}
	if n, _ := fmt.Sscanf(m, "%d", &major); n == 1 {
		return major, 0, true
	}
	return 0, 0, false
}

// aiGLMThinkingModel 判断智谱模型是否支持 thinking 字段：官方文档为 GLM-4.5 及以上版本。
// 更早的模型不下发该字段，避免未知参数导致整轮请求失败。
func aiGLMThinkingModel(model string) bool {
	major, minor, ok := aiGLMVersion(model)
	if !ok {
		return false
	}
	return major > 4 || (major == 4 && minor >= 5)
}

// aiGLMForcesThinking 判断智谱模型是否强制开启思考、不接受 thinking.type=disabled：官方文档明确
// GLM-5.3 / GLM-5.3-FLASH 传 disabled 会报错，GLM-4.7 / GLM-4.5V 亦为强制思考模型。
// 这些模型即使把「思考模式」关掉也只能保持开启，故不下发 disabled，以免整轮请求失败。
func aiGLMForcesThinking(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	for _, prefix := range []string{"glm-5.3", "glm-4.7", "glm-4.5v"} {
		if strings.HasPrefix(m, prefix) {
			return true
		}
	}
	// GA 之后的更高主版本按强制思考处理（无法在旧版本上关闭）
	if major, _, ok := aiGLMVersion(m); ok && major >= 6 {
		return true
	}
	return false
}

// aiGLMSupportsToolStream 判断智谱模型是否支持 tool_stream（工具调用参数随流逐步返回）。
// 官方文档（工具流式输出）列出的支持范围为 GLM-4.6 及以上的「最新模型」，
// 更早的 GLM-4.5 及以下不下发，避免未知参数影响其工具调用。
func aiGLMSupportsToolStream(model string) bool {
	major, minor, ok := aiGLMVersion(model)
	if !ok {
		return false
	}
	return major > 4 || (major == 4 && minor >= 6)
}

// aiApplyZhipuParams 为智谱 GLM 补充专有请求参数（其他服务商直接返回，不受影响）：
//   - thinking：GLM 的思考默认开启，不显式下发 disabled 时配置里的「关闭思考」对智谱形同虚设；
//     仅对支持该字段的 GLM-4.5+ 下发，强制思考的模型（GLM-5.3 等）保持默认不下发。
//   - tool_stream：与 stream 同用，工具调用参数由一次性返回改为随流逐步返回，缩短等待；
//     解析侧已按 index 归并 arguments（见 aiChatOnceToolsOnce），无需额外改动。
//
// effort 为空表示本次未开启思考模式。
func aiApplyZhipuParams(payload map[string]any, baseURL, model, effort string, hasTools bool) {
	if !aiIsZhipuProvider(baseURL, model) {
		return
	}
	if aiGLMThinkingModel(model) {
		switch {
		case effort != "":
			payload["thinking"] = map[string]any{"type": "enabled"}
		case !aiGLMForcesThinking(model):
			payload["thinking"] = map[string]any{"type": "disabled"}
		}
	}
	if hasTools && aiGLMSupportsToolStream(model) {
		payload["tool_stream"] = true
	}
}

// ============== DeepSeek 专有工具调用参数 ==============
//
// 依据 https://api-docs.deepseek.com/zh-cn/guides/tool_calls：
// 思考模式自 DeepSeek-V3.2 起原生支持工具调用（reasoning_content 与 tool_calls 同流返回，
// 现有流式解析已覆盖，无需额外改动）；strict 模式为 Beta 功能，要求 base_url 指向
// https://api.deepseek.com/beta，且一次请求里所有 function 都必须声明 strict=true，
// 参数 JSON Schema 还需满足 strict 约束（见 aiApplyDeepSeekStrictTools）。
// 非 beta 的 DeepSeek 接口保持标准 OpenAI 工具格式，额外下发 strict 会被按非法字段拒绝。

// aiDeepSeekBetaURL 判断 baseURL 是否指向 DeepSeek Beta 接口（strict 模式要求 https://api.deepseek.com/beta）。
func aiDeepSeekBetaURL(baseURL string) bool {
	if !strings.Contains(strings.ToLower(baseURL), "api.deepseek.com") {
		return false
	}
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Path == "" {
		return false
	}
	for _, seg := range strings.Split(strings.Trim(u.Path, "/"), "/") {
		if seg == "beta" {
			return true
		}
	}
	return false
}

// aiApplyDeepSeekStrictTools 当 baseURL 指向 DeepSeek Beta 接口时，把工具清单改写为 strict 模式要求的格式：
// function 增加 strict=true；参数对象的全部属性列入 required，并补 additionalProperties=false。
// strict 模式还要求 schema 不使用 minLength / maxLength / minItems / maxItems 等不支持的关键字，
// 现有工具定义只用 object / string / boolean / integer / array 与 description，均在支持范围内，无需裁剪。
// 非 beta 接口原样返回，避免给其他服务商下发 strict 字段。
func aiApplyDeepSeekStrictTools(baseURL string, tools []map[string]any) []map[string]any {
	if !aiDeepSeekBetaURL(baseURL) {
		return tools
	}
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		fn, ok := t["function"].(map[string]any)
		if !ok {
			out = append(out, t)
			continue
		}
		clone := make(map[string]any, len(fn)+1)
		maps.Copy(clone, fn)
		clone["strict"] = true
		if params, ok := clone["parameters"].(map[string]any); ok {
			pc := make(map[string]any, len(params)+1)
			maps.Copy(pc, params)
			if props, ok := pc["properties"].(map[string]any); ok && len(props) > 0 {
				required := make([]any, 0, len(props))
				for k := range props {
					required = append(required, k)
				}
				sort.Slice(required, func(i, j int) bool {
					return required[i].(string) < required[j].(string)
				})
				pc["required"] = required
			}
			pc["additionalProperties"] = false
			clone["parameters"] = pc
		}
		out = append(out, map[string]any{"type": "function", "function": clone})
	}
	return out
}

// aiChatOnceToolsOnce 以流式（SSE）方式调用 OpenAI 兼容的 chat/completions 接口（单次尝试），
// 边解析上游增量边推送思维链（ai_stream_delta，kind=reasoning），最后聚合返回首个候选的
// 文本内容、思维链与工具调用请求。tools 非空时随请求下发工具清单；
// 模型请求调用工具时正文通常为空、tool_calls 非空。
// reasoningEffort 非空时附加 reasoning_effort 参数（适配 OpenAI o 系列等），并解析增量中的
// reasoning_content / reasoning 字段作为思维链；未开启思考模式时思维链为空。
// toolChoice 为空时不下发 tool_choice 字段（部分服务商不支持该字段）。
// streamID 非空时，思维链增量即时经 WS 推送给前端，实现逐字显示。
// 返回值中的 finishReason 为上游给出的结束原因（如 "stop" / "length" / "tool_calls"），
// 供上层判断本轮答复是否为「被长度截断的半截话」。
func aiChatOnceToolsOnce(c *dto.AIConfig, model string, messages []aiChatMessage, reasoningEffort string, tools []map[string]any, toolChoice, streamID string) (string, string, []aiToolCall, string, error) {
	// 接口地址可只填到版本号（如 https://api.deepseek.com/v1），也可直接填完整的 chat/completions 地址
	endpoint := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if endpoint == "" {
		return "", "", nil, "", errors.New("未配置 AI 接口地址")
	}
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}
	if model == "" {
		model = c.Model
	}
	if model == "" {
		model = dto.DefaultAIModel
	}

	// 统一走流式：思维链才能边产生边推送；工具调用分片在解析时按 index 归并
	payload := map[string]any{
		"model":    model,
		"messages": messages,
		"stream":   true,
	}
	// 输出长度上限：取当前模型配置（未配置时为默认最佳值）
	if c.MaxTokens > 0 {
		payload["max_tokens"] = c.MaxTokens
	}
	effort := dto.NormalizeReasoningEffort(reasoningEffort)
	if effort != "" {
		payload["reasoning_effort"] = effort
	}
	if len(tools) > 0 {
		// DeepSeek Beta 接口（base_url 含 /beta）要求工具清单走 strict 模式（见 aiApplyDeepSeekStrictTools），
		// 其余服务商保持标准 OpenAI 工具格式、原样下发
		payload["tools"] = aiApplyDeepSeekStrictTools(c.BaseURL, tools)
		if toolChoice != "" {
			payload["tool_choice"] = toolChoice
		}
	}
	// 智谱 GLM 专有参数：thinking（让「思考模式」开关真正生效）与 tool_stream（工具参数随流返回）
	aiApplyZhipuParams(payload, c.BaseURL, model, effort, len(tools) > 0)
	body, err := json.Marshal(payload)
	if err != nil {
		return "", "", nil, "", err
	}

	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 60
	}
	// 建连与「首个字节」阶段同样按空闲超时兜底（见 aiStreamIdleTimeout）：
	// 上游在长上下文 / 长思考时可能数十秒才回响应头，若按配置超时掐断，会被误判成
	// 「AI 接口请求失败: Post ...: context canceled」这类中断（并非用户终止，也非网络故障）。
	// 父上下文来自流式取消注册表：用户点击「终止」时随之取消本次请求。
	parent := aiStreamParentCtx(streamID)
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	idleTimeout := aiStreamIdleTimeout(timeout)
	watchdog := time.AfterFunc(idleTimeout, cancel)
	defer watchdog.Stop()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", "", nil, "", fmt.Errorf("AI 接口地址不合法: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if parent.Err() != nil {
			return "", "", nil, "", errAIStreamCancelled
		}
		// 取消源自本函数看门狗（父上下文未取消）：上游迟迟未响应，属超时而非网络故障。
		// 标记为「上游静默」以便退避重试——这类超时换一条新请求通常就能恢复
		if ctx.Err() != nil {
			return "", "", nil, "", aiNewStallError(fmt.Sprintf("AI 接口建连超时：超过 %v 未收到上游响应", idleTimeout))
		}
		// 用 %w 保留底层错误链：网络层瞬时故障（连不上、连接被重置、读超时等）由此被
		// aiRetryableByNetError 识别，交给 aiRetryBackoff 自动退避重试，而不是直接判失败
		return "", "", nil, "", fmt.Errorf("AI 接口请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 非 200 时上游返回的是 JSON 错误体，尽量解析出可读原因
		msg := fmt.Sprintf("AI 接口返回 HTTP %d", resp.StatusCode)
		var out struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.NewDecoder(resp.Body).Decode(&out) == nil && out.Error != nil && strings.TrimSpace(out.Error.Message) != "" {
			msg = strings.TrimSpace(out.Error.Message)
		}
		if aiRetryableByStatus(resp.StatusCode) || aiRetryableByMessage(msg) {
			return "", "", nil, "", aiNewRetryableError(msg)
		}
		return "", "", nil, "", errors.New(msg)
	}

	var content, reasoning strings.Builder
	var calls []aiToolCall
	// finishReason 记录上游给出的结束原因，最后一片增量里的 finish_reason 为准
	var finishReason string

	// 正文里可能混入文本形式的工具调用（<tool_call>…</tool_call>，见 aiParseTextToolCalls）：
	// 这类片段不应当出现在答复正文里，流式推送时改路由到思考区。
	var callText aiToolCallTextRouter

	// 已建立连接：看门狗改按空闲超时续期，只要上游持续产出就不中断
	watchdog.Reset(idleTimeout)
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		watchdog.Reset(idleTimeout)
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		chunk := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if chunk == "" {
			continue
		}
		if chunk == "[DONE]" {
			break
		}
		var part struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
					Reasoning        string `json:"reasoning"`
					Text             string `json:"text"`
					ToolCalls        []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				// finish_reason 用于区分正常收尾与「输出被长度截断」：
				// 截断时模型往往只输出了「我先…然后…」式计划前言就中断，必须识别出来。
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(chunk), &part); err != nil {
			continue // 跳过无法解析的心跳/注释行
		}
		if part.Error != nil && strings.TrimSpace(part.Error.Message) != "" {
			msg := strings.TrimSpace(part.Error.Message)
			if aiRetryableByMessage(msg) {
				return content.String(), reasoning.String(), nil, finishReason, aiNewRetryableError(msg)
			}
			return content.String(), reasoning.String(), nil, finishReason, errors.New(msg)
		}
		if len(part.Choices) == 0 {
			continue
		}
		d := part.Choices[0].Delta
		// 结束原因可能随任意一片增量下发，取最后一次非空值
		if fr := part.Choices[0].FinishReason; fr != "" {
			finishReason = fr
		}
		// 思维链增量：累积并即时推送，前端据此逐字渲染
		if t := d.ReasoningContent; t != "" {
			reasoning.WriteString(t)
			aiStreamNotify(streamID, "ai_stream_delta", map[string]any{"kind": "reasoning", "text": t})
		} else if t := d.Reasoning; t != "" {
			reasoning.WriteString(t)
			aiStreamNotify(streamID, "ai_stream_delta", map[string]any{"kind": "reasoning", "text": t})
		}
		// 正文增量即时推送，前端据此逐字渲染；若本轮最终转为工具调用，
		// 泄漏的过渡文本会在收尾时被 ai_stream_end 的最终答复整体覆盖。
		// 其中文本形式的工具调用片段改走思考区，避免正文出现裸 JSON。
		if t := d.Content; t != "" {
			content.WriteString(t)
			aiPushRoutedDelta(streamID, &callText, &reasoning, t)
		} else if t := d.Text; t != "" {
			content.WriteString(t)
			aiPushRoutedDelta(streamID, &callText, &reasoning, t)
		}
		// 工具调用分片：id/name/arguments 可能分散在多个增量中，按 index 归并
		for _, tc := range d.ToolCalls {
			for len(calls) <= tc.Index {
				calls = append(calls, aiToolCall{})
			}
			if tc.ID != "" {
				calls[tc.Index].ID = tc.ID
			}
			if tc.Type != "" {
				calls[tc.Index].Type = tc.Type
			}
			calls[tc.Index].Function.Name += tc.Function.Name
			calls[tc.Index].Function.Arguments += tc.Function.Arguments
		}
	}
	// 流结束：吐出路由器滞留的尾部文本（可能是被拆散、未闭合的调用片段）
	if body, thought := callText.aiRouteFlush(); body != "" || thought != "" {
		if body != "" {
			aiStreamNotify(streamID, "ai_stream_delta", map[string]any{"kind": "content", "text": body})
		}
		if thought != "" {
			reasoning.WriteString(thought)
			aiStreamNotify(streamID, "ai_stream_delta", map[string]any{"kind": "reasoning", "text": thought})
		}
	}
	if err := scanner.Err(); err != nil {
		if parent.Err() != nil {
			return content.String(), reasoning.String(), calls, finishReason, errAIStreamCancelled
		}
		if ctx.Err() != nil {
			return content.String(), reasoning.String(), calls, finishReason,
				aiNewStallError(fmt.Sprintf("AI 流式读取超时：超过 %v 未收到上游数据", idleTimeout))
		}
		return content.String(), reasoning.String(), calls, finishReason, fmt.Errorf("AI 流式读取失败: %v", err)
	}
	// 剔除空槽，兼容部分服务商 index 不连续的情况
	filtered := calls[:0]
	for _, call := range calls {
		if call.Function.Name != "" || call.ID != "" {
			filtered = append(filtered, call)
		}
	}
	return strings.TrimSpace(content.String()), strings.TrimSpace(reasoning.String()), filtered, finishReason, nil
}

// aiStreamNotify 经 WS 广播一条 AI 流式事件（无 id，走前端 onPush 推送通道）。
// streamID 为空时不做任何事；调用方需保证 streamID 在前端可唯一匹配当前消息。
func aiStreamNotify(streamID, eventType string, data map[string]any) {
	if streamID == "" {
		return
	}
	if data == nil {
		data = map[string]any{}
	}
	data["id"] = streamID
	// 带上所属任务 id：后端自行接续的流（后台自动继续）在前端没有对应的本地请求，
	// 前端据此判断该流是否属于当前打开的任务，从而接管展示
	if _, ok := data["session_id"]; !ok {
		if sid := aiStreamSessionOf(streamID); sid != "" {
			data["session_id"] = sid
		}
	}
	// 思考/正文增量同步写入会话草稿，页面刷新后仍可恢复已产生的部分
	if eventType == "ai_stream_delta" {
		kind, _ := data["kind"].(string)
		if kind == "reasoning" || kind == "content" {
			if text, _ := data["text"].(string); text != "" {
				aiSessionAppendDraftDelta(streamID, kind, text)
			}
		}
	}
	msg, err := json.Marshal(map[string]any{"type": eventType, "data": data})
	if err != nil {
		return
	}
	broadcastOpuiNotify(msg)
}

// aiPushRoutedDelta 推送一段上游正文增量：普通文本走 kind=content 进答复正文，
// 文本形式的工具调用片段（<tool_call>…</tool_call>）改走 kind=reasoning 计入思考过程，
// 既避免正文出现裸 JSON，也保证 ai_stream_end 覆盖思考区后这些片段不会丢失。
func aiPushRoutedDelta(streamID string, router *aiToolCallTextRouter, reasoning *strings.Builder, text string) {
	body, thought := router.aiRouteFeed(text)
	if thought != "" {
		reasoning.WriteString(thought)
		aiStreamNotify(streamID, "ai_stream_delta", map[string]any{"kind": "reasoning", "text": thought})
	}
	if body != "" {
		aiStreamNotify(streamID, "ai_stream_delta", map[string]any{"kind": "content", "text": body})
	}
}

// aiStreamIdleTimeout 流式读取阶段的空闲超时：只要上游持续产出数据就不中断，
// 仅当连续这段时间没有收到任何新数据才终止请求。取「配置超时」与 3 分钟中的较大值，
// 避免长思维链生成被总时长上限（默认 60 秒）误杀。
func aiStreamIdleTimeout(timeoutSec int) time.Duration {
	return max(time.Duration(timeoutSec)*time.Second, 3*time.Minute)
}

// ---- AI 流式请求取消注册表：支持用户在生成中途「终止」 ----
//
// 每个流式请求（streamID 非空）在上游调用前登记一个可取消上下文，该流内的所有上游请求
// （工具多轮、降级纯对话、审批等待）都从它派生；用户点击「终止」时统一取消，请求随即结束，
// 已产生的思考与正文由调用方收尾保留。
var (
	aiStreamCancelMu sync.Mutex
	aiStreamCancels  = map[string]*aiStreamCancelEntry{}
)

type aiStreamCancelEntry struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// errAIStreamCancelled 标记「用户主动终止」，与超时、上游错误区分开。
var errAIStreamCancelled = errors.New("用户已终止本次生成")

// aiRegisterStreamCancel 为某流登记取消函数并返回其上下文；streamID 为空时返回独立背景上下文。
func aiRegisterStreamCancel(streamID string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	if strings.TrimSpace(streamID) == "" {
		return ctx, cancel
	}
	aiStreamCancelMu.Lock()
	if old := aiStreamCancels[streamID]; old != nil {
		old.cancel()
	}
	aiStreamCancels[streamID] = &aiStreamCancelEntry{ctx: ctx, cancel: cancel}
	aiStreamCancelMu.Unlock()
	return ctx, cancel
}

// aiUnregisterStreamCancel 注销某流的取消函数并释放其上下文（请求收尾时调用）。
func aiUnregisterStreamCancel(streamID string) {
	if strings.TrimSpace(streamID) == "" {
		return
	}
	aiStreamCancelMu.Lock()
	if e := aiStreamCancels[streamID]; e != nil {
		e.cancel()
		delete(aiStreamCancels, streamID)
	}
	aiStreamCancelMu.Unlock()
}

// aiStreamParentCtx 返回某流已登记的父上下文；未登记（如无 streamID）时返回背景上下文。
func aiStreamParentCtx(streamID string) context.Context {
	if strings.TrimSpace(streamID) == "" {
		return context.Background()
	}
	aiStreamCancelMu.Lock()
	defer aiStreamCancelMu.Unlock()
	if e := aiStreamCancels[streamID]; e != nil {
		return e.ctx
	}
	return context.Background()
}

// aiCancelStream 取消某流的在途生成并返回是否命中；streamID 未命中时按 sessionID 回退查找
// （页面刷新后前端只剩任务 id，拿不到流 id）。
func aiCancelStream(streamID, sessionID string) bool {
	aiStreamCancelMu.Lock()
	defer aiStreamCancelMu.Unlock()
	if id := strings.TrimSpace(streamID); id != "" {
		if e := aiStreamCancels[id]; e != nil {
			e.cancel()
			return true
		}
	}
	if sid := strings.TrimSpace(sessionID); sid != "" {
		for id, e := range aiStreamCancels {
			if aiStreamSessionOf(id) == sid {
				e.cancel()
				return true
			}
		}
	}
	return false
}

// aiChatStreamReasoning 流式对话（无工具）并兜底「输出被长度截断」：
// 上游因长度上限中断（finish_reason=length）时，把已生成的半截正文作为 assistant 消息回灌，
// 提示模型紧接上文续写，最多 aiToolMaxContinues 轮，避免用户看到「话说到一半突然没了」。
func aiChatStreamReasoning(c *dto.AIConfig, model string, messages []aiChatMessage, reasoningEffort, streamID string) (string, string, error) {
	limiter := aiModelLimiterOf(c, model)
	work := append([]aiChatMessage(nil), messages...)
	var contentAll, reasoningAll strings.Builder
	// 续写提示写入思考区，让用户知道正文为何被拆成多段
	appendReasoning := func(text string) {
		if text == "" {
			return
		}
		reasoningAll.WriteString(text)
		aiStreamNotify(streamID, "ai_stream_delta", map[string]any{"kind": "reasoning", "text": text})
	}
	for i := 0; i <= aiToolMaxContinues; i++ {
		// 续写属新的一次上游请求，同样受该模型的频率 / 并发限制约束；
		// 限流/过载与网络层瞬时故障在这里退避重试，避免一次抖动就丢掉整轮答复
		var content, reasoning, finishReason string
		err := aiRetryBackoff(streamID, func() error {
			release := func() {}
			if limiter != nil {
				release = limiter.acquire(streamID)
			}
			defer release()
			var e error
			content, reasoning, finishReason, e = aiChatStreamReasoningOnce(c, model, work, reasoningEffort, streamID)
			return e
		})
		// 思维链已由单次请求在流式解析时逐片推送，此处仅累积，避免重复推送
		if r := strings.TrimSpace(reasoning); r != "" {
			reasoningAll.WriteString(r)
			reasoningAll.WriteByte('\n')
		}
		if err != nil {
			if contentAll.Len() == 0 {
				return content, reasoningAll.String(), err
			}
			contentAll.WriteString(content)
			return contentAll.String(), reasoningAll.String(), err
		}
		trimmed := strings.TrimSpace(content)
		// 正常收尾：拼接此前续写的片段一并返回，前端在 ai_stream_end 用返回值整体覆盖正文
		if finishReason != "length" || trimmed == "" {
			contentAll.WriteString(content)
			// 上游偶尔返回空正文（例如输出预算被思维链吃满）：给出可读提示，避免前端出现空白气泡
			if contentAll.Len() == 0 {
				return "（AI 未返回内容，请重试或换个说法再问一次。）", reasoningAll.String(), nil
			}
			return contentAll.String(), reasoningAll.String(), nil
		}
		contentAll.WriteString(content)
		if i < aiToolMaxContinues {
			// 被长度截断且仍有续写额度：回灌半截正文，请模型紧接上文继续
			appendReasoning("\n⚠ 输出达到上游长度上限被截断，已自动续写\n")
			work = append(work, aiChatMessage{Role: "assistant", Content: content})
			work = append(work, aiChatMessage{
				Role: "user",
				Content: "你上一条回复因长度上限被截断了。请紧接上文继续输出剩余内容：" +
					"不要重复已经写过的部分，不要重新开头或重新总结。",
			})
			continue
		}
		// 续写额度用尽仍被截断：明确告知，避免用户误以为答复已经完整
		contentAll.WriteString("\n\n（提示：本次输出多次触达模型长度上限，内容可能仍不完整。可回复「继续」让我接着补全。）")
		return contentAll.String(), reasoningAll.String(), nil
	}
	return contentAll.String(), reasoningAll.String(), nil
}

// aiChatStreamReasoningOnce 以流式方式调用 OpenAI 兼容的 chat/completions 接口（单次尝试），
// 边解析上游 SSE 增量边经 WS 推送（ai_stream_delta），并返回累计的正文与思维链。
// 不同服务商的思维链字段不一：优先 delta.reasoning_content，回退 delta.reasoning。
// 返回值中的 finishReason 为上游给出的结束原因（"stop" / "length" 等），供上层判断是否被长度截断。
func aiChatStreamReasoningOnce(c *dto.AIConfig, model string, messages []aiChatMessage, reasoningEffort, streamID string) (string, string, string, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if endpoint == "" {
		return "", "", "", errors.New("未配置 AI 接口地址")
	}
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}
	if model == "" {
		model = c.Model
	}
	if model == "" {
		model = dto.DefaultAIModel
	}

	payload := map[string]any{
		"model":    model,
		"messages": messages,
		"stream":   true,
	}
	// 输出长度上限：取当前模型配置（未配置时为默认最佳值）
	if c.MaxTokens > 0 {
		payload["max_tokens"] = c.MaxTokens
	}
	effort := dto.NormalizeReasoningEffort(reasoningEffort)
	if effort != "" {
		payload["reasoning_effort"] = effort
	}
	// 智谱 GLM 专有参数：thinking（让「思考模式」开关真正生效；本路径不下发工具，无 tool_stream）
	aiApplyZhipuParams(payload, c.BaseURL, model, effort, false)
	body, err := json.Marshal(payload)
	if err != nil {
		return "", "", "", err
	}

	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 60
	}
	// 建连与「首个字节」阶段同样按空闲超时兜底（见 aiStreamIdleTimeout），
	// 避免长上下文 / 长思考下响应头迟到被误判为「context canceled」中断。
	// 父上下文来自流式取消注册表：用户点击「终止」时随之取消本次请求。
	parent := aiStreamParentCtx(streamID)
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	idleTimeout := aiStreamIdleTimeout(timeout)
	watchdog := time.AfterFunc(idleTimeout, cancel)
	defer watchdog.Stop()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", "", "", fmt.Errorf("AI 接口地址不合法: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if parent.Err() != nil {
			return "", "", "", errAIStreamCancelled
		}
		// 取消源自本函数看门狗（父上下文未取消）：上游迟迟未响应，属超时而非网络故障。
		// 标记为「上游静默」以便退避重试——这类超时换一条新请求通常就能恢复
		if ctx.Err() != nil {
			return "", "", "", aiNewStallError(fmt.Sprintf("AI 接口建连超时：超过 %v 未收到上游响应", idleTimeout))
		}
		// 同 aiChatOnceToolsOnce：保留错误链，让网络层瞬时故障可被识别并重试
		return "", "", "", fmt.Errorf("AI 接口请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 尽量解析上游返回的错误信息，便于前端提示
		var out struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.NewDecoder(resp.Body).Decode(&out) == nil && out.Error != nil && strings.TrimSpace(out.Error.Message) != "" {
			return "", "", "", errors.New(out.Error.Message)
		}
		return "", "", "", fmt.Errorf("AI 接口返回 HTTP %d", resp.StatusCode)
	}

	var content, reasoning strings.Builder
	// finishReason 记录上游给出的结束原因（"length" 表示输出被长度上限截断）
	var finishReason string
	// 已建立连接：看门狗改按空闲超时续期，只要上游持续产出就不中断
	watchdog.Reset(idleTimeout)
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		watchdog.Reset(idleTimeout)
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		chunk := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if chunk == "" || chunk == "[DONE]" {
			if chunk == "[DONE]" {
				break
			}
			continue
		}
		var part struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
					Reasoning        string `json:"reasoning"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(chunk), &part); err != nil {
			continue // 跳过无法解析的心跳/注释行
		}
		if part.Error != nil && strings.TrimSpace(part.Error.Message) != "" {
			return content.String(), reasoning.String(), finishReason, errors.New(part.Error.Message)
		}
		if len(part.Choices) == 0 {
			continue
		}
		d := part.Choices[0].Delta
		// 结束原因可能随任意一片增量下发，取最后一次非空值
		if fr := part.Choices[0].FinishReason; fr != "" {
			finishReason = fr
		}
		if t := d.ReasoningContent; t != "" {
			reasoning.WriteString(t)
			aiStreamNotify(streamID, "ai_stream_delta", map[string]any{"kind": "reasoning", "text": t})
		} else if t := d.Reasoning; t != "" {
			reasoning.WriteString(t)
			aiStreamNotify(streamID, "ai_stream_delta", map[string]any{"kind": "reasoning", "text": t})
		}
		if t := d.Content; t != "" {
			content.WriteString(t)
			aiStreamNotify(streamID, "ai_stream_delta", map[string]any{"kind": "content", "text": t})
		}
	}
	if err := scanner.Err(); err != nil {
		if parent.Err() != nil {
			return content.String(), reasoning.String(), finishReason, errAIStreamCancelled
		}
		if ctx.Err() != nil {
			return content.String(), reasoning.String(), finishReason,
				aiNewStallError(fmt.Sprintf("AI 流式读取超时：超过 %v 未收到上游数据", idleTimeout))
		}
		return content.String(), reasoning.String(), finishReason, fmt.Errorf("AI 流式读取失败: %v", err)
	}
	return strings.TrimSpace(content.String()), strings.TrimSpace(reasoning.String()), finishReason, nil
}

// aiBuildCompletePrompt 依据光标前后的词库代码拼装补全提示词。
// 仅截取靠近光标的片段以控制上下文长度；上下文过短（无有效内容）时返回空串跳过本次补全。
func aiBuildCompletePrompt(path, prefix, suffix string) string {
	prefix = aiTailLines(prefix, 120)
	suffix = aiHeadLines(suffix, 30)
	if strings.TrimSpace(prefix) == "" && strings.TrimSpace(suffix) == "" {
		return ""
	}
	name := filepath.ToSlash(strings.TrimSpace(path))
	if name == "" {
		name = "词库文件"
	}
	var sb strings.Builder
	sb.WriteString("词库文件：")
	sb.WriteString(name)
	sb.WriteString("\n\n")
	if prefix != "" {
		sb.WriteString("光标之前的代码：\n```\n")
		sb.WriteString(prefix)
		sb.WriteString("\n```\n\n")
	}
	if suffix != "" {
		sb.WriteString("光标之后的代码：\n```\n")
		sb.WriteString(suffix)
		sb.WriteString("\n```\n\n")
	}
	sb.WriteString("请只输出应插入到【光标处】的代码。")
	return sb.String()
}

// aiTailLines 返回文本末尾的至多 n 行。
func aiTailLines(s string, n int) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// aiHeadLines 返回文本开头的至多 n 行。
func aiHeadLines(s string, n int) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

// aiCleanComplete 清洗补全结果：去除模型可能附带的代码围栏，避免虚影插入多余内容。
func aiCleanComplete(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// 去掉开头/结尾的 Markdown 代码围栏
	if strings.HasPrefix(s, "```") {
		if i := strings.Index(s, "\n"); i >= 0 {
			s = s[i+1:]
		} else {
			s = ""
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
		s = strings.TrimSpace(s)
	}
	// 模型有时会把整段上下文回显，出现光标占位标记时只取其后的内容
	for _, marker := range []string{"<|光标|>", "【光标处】", "[光标]"} {
		if i := strings.LastIndex(s, marker); i >= 0 {
			s = strings.TrimSpace(s[i+len(marker):])
		}
	}
	return s
}

// AI 对话 / 会话 / 补全 API。
func init() {
	registerOpuiApi(opuiHandleAIAPI,
		"ai_chat", "list_ai_sessions", "get_ai_session", "create_ai_session",
		"save_ai_session", "delete_ai_session", "clear_ai_session", "truncate_ai_messages",
		"compress_ai_session", "export_ai_sessions", "import_ai_sessions", "cancel_ai_chat",
		"list_ai_file_changes", "revert_ai_file_changes", "confirm_ai_file_changes", "ai_upload_image",
		"get_ai_memory", "save_ai_memory", "get_ai_task_layout", "save_ai_task_layout",
		"list_ai_agents", "save_ai_agent", "delete_ai_agent", "set_session_agent",
		"list_ai_skills", "save_ai_skill", "delete_ai_skill", "ai_complete",
	)
}

// opuiHandleAIAPI 处理本模块注册的 OPUI API 类型。
func opuiHandleAIAPI(w http.ResponseWriter, r *http.Request, h *HttpOpUiData) {
	switch h.Type {
	case "ai_chat":
		// AI 对话：词库编辑时与 AI 协作开发（关联词库代码、结合编译报错/警告）
		// 携带 session_id 时进入「任务模式」（独立记忆 + 持久化 + 自动压缩）
		aiChatHandle(w, h)
		return

	case "list_ai_sessions", "get_ai_session", "create_ai_session", "save_ai_session",
		"delete_ai_session", "clear_ai_session", "truncate_ai_messages",
		"compress_ai_session", "export_ai_sessions", "import_ai_sessions", "cancel_ai_chat",
		"list_ai_file_changes", "revert_ai_file_changes", "confirm_ai_file_changes",
		"ai_upload_image", "get_ai_memory", "save_ai_memory",
		"get_ai_task_layout", "save_ai_task_layout":
		// AI 多任务会话管理：任务的增删改查、记忆压缩与导入导出、终止在途生成、
		// 「本任务改动过的文件」列表、确认无误与回撤（驳回）、输入框图片的上传落盘，
		// 以及全局「项目记忆」的读取与保存（所有任务共享一份）、
		// 任务列表抽屉布局（分组与手工排序）的读取与保存
		aiSessionHandle(w, h)
		return

	case "list_ai_agents", "save_ai_agent", "delete_ai_agent", "set_session_agent":
		// AI 智能体管理：预设的增删改查，以及「给某个任务指定智能体」
		aiAgentHandle(w, h)
		return

	case "list_ai_skills", "save_ai_skill", "delete_ai_skill":
		// AI 技能管理：技能（名称 + 描述 + 正文）的增删改查。各类词库场景由不同智能体负责，
		// 智能体声明自己可用的内置技能；系统提示只注入「可用技能清单」，模型按需 read_skill 读取正文
		aiSkillHandle(w, h)
		return

	case "ai_complete":
		// AI 内联补全：根据光标前后代码续写，供编辑器以虚影（ghost text）形式提示
		var j struct {
			Path   string `json:"path"`
			Prefix string `json:"prefix"`
			Suffix string `json:"suffix"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		aiCfg := aiResolveConfig()
		// 补全由用户显式触发（Alt+\）：未就绪时带上原因返回，供前端提示，避免看起来"快捷键没反应"
		if err := aiCheckReady(aiCfg, true); err != nil {
			b, _ := json.Marshal(map[string]string{"status": "disabled", "error": err.Error()})
			w.Write(b)
			return
		}
		prompt := aiBuildCompletePrompt(j.Path, j.Prefix, j.Suffix)
		if prompt == "" {
			w.Write([]byte(`{"status":"ok","content":""}`))
			return
		}
		// 内联补全为高频请求：仅注入光标附近代码实际调用的内置函数，避免每次下发完整函数清单
		completeSystem := aiCompleteSystemPrompt
		if f := aiBuiltinFuncsTextUsed(j.Prefix + "\n" + j.Suffix); f != "" {
			completeSystem += "\n\n" + f
		}
		content, err := aiChatOnce(aiCfg, "", []aiChatMessage{
			{Role: "system", Content: completeSystem},
			{Role: "user", Content: prompt},
		})
		if err != nil {
			// 显式触发时把失败原因回传前端提示；不再静默，便于用户排查
			b, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(b)
			return
		}
		jsonResp, _ := json.Marshal(map[string]string{"status": "ok", "content": aiCleanComplete(content)})
		w.Write(jsonResp)
		return

	}
}
