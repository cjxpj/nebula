package dto

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/cjxpj/nebula/appfiles"
	feishubot_msg "github.com/cjxpj/nebula/bot/feishubot/msg"
	napcatbot_dto "github.com/cjxpj/nebula/bot/napcatbot/dto"
	qqbot_msg "github.com/cjxpj/nebula/bot/qqbot/msg"
	secludedbot_dto "github.com/cjxpj/nebula/bot/secludedbot/dto"
	yunhubot_dto "github.com/cjxpj/nebula/bot/yunhubot/dto"
	"github.com/cjxpj/nebula/utils"
	"github.com/gorilla/websocket"
)

// ==============Server================

// 默认路由词库文件与默认网站根目录
const (
	DefaultRouterFile = "private/system/router.n"
	DefaultWebRoot    = "public"
)

// WS连接
type ServerRouterWebSocket struct {
	// 是否开启
	Open bool
	// 地址
	Addr string
	// 跨域
	Cors bool
	// 词库路径
	FilePath string
	// 连接
	Conn *websocket.Upgrader
}

type OPUI struct {
	// 地址
	Addr string
	// 跨域开关
	Cors bool
}

// CloudTool 内置云工具服务端配置（config.yaml [云工具服务端] 节）。
type CloudTool struct {
	// 是否开启
	Open bool
	// 访问路径（含前导 /，如 /cloudtool）
	Addr string
	// 是否允许任意账号注册（关闭时仅白名单账号可登录/注册）
	AllowRegister bool
	// 账号白名单（用户名 -> true）
	Whitelist map[string]bool
	// 云工具词库目录（相对 private/，如 cloudtool）
	DicDir string
	// 断开自动注销时长（秒）：连接断开时本次在线时长低于该值即注销，0 表示关闭
	LogoutSec int
	// 调试打印
	Debug bool
	// 词库商城开关：开启后客户端可浏览、购买并下载词库（余额购买）
	ShopOpen bool
	// 图床开关：开启后客户端可上传图片并获取外链 URL
	ImageHost bool
	// 词库上传审核：开启后上传的词库需审核通过才上架，默认开启
	ShopReview bool
	// 图床上传审核：开启后上传的图片需审核通过才可访问外链，默认开启
	ImageReview bool
}

// AIConfig AI 对接配置（config.yaml [AI] 节）：OpenAI 兼容接口，支持多模型列表。
// BaseURL/APIKey/Model/Reasoning 等运行期字段为「当前选中模型」的解析结果，供发起 AI 请求的代码直接使用；
// Models 为完整模型列表，CurrentID 为选中项标识（列表为空时为空串）。
type AIConfig struct {
	// 是否启用
	Open bool
	// 当前模型的接口地址（OpenAI 兼容 BaseURL，如 https://api.deepseek.com/v1）
	BaseURL string
	// 当前模型的接口密钥（明文，仅存内存）
	APIKey string
	// 当前模型名，默认 deepseek-flash
	Model string
	// 系统提示词，留空时使用内置词库开发提示词
	SystemPrompt string
	// 请求超时（秒）
	Timeout int
	// 是否启用词库编辑器内联补全
	InlineComplete bool
	// 当前模型是否默认开启思考/推理模式（任务可单独覆盖）
	Reasoning bool
	// 当前模型的推理强度：low | medium | high | max，留空时由接口按模型默认处理
	ReasoningEffort string
	// 当前模型的推理模型名，如 deepseek-reasoner；填写后开启思考模式时改用此模型
	ReasoningModel string
	// 当前选中模型的 ID
	CurrentID string
	// 模型列表（每项为独立完整配置）
	Models []*AIModelConfig
}

// AIModelConfig 模型列表中的单个模型配置：每项可独立对接不同服务商，
// 分别保存接口地址、密钥、模型名与思考模式设置。
type AIModelConfig struct {
	// 唯一标识（列表内不重复，用于选中与密钥保留匹配）
	ID string `json:"id"`
	// 展示名称
	Name string `json:"name"`
	// 接口地址（OpenAI 兼容 BaseURL）
	BaseURL string `json:"base_url"`
	// 接口密钥：配置文件中为密文（enc: 前缀），内存中为明文
	APIKey string `json:"api_key"`
	// 模型名
	Model string `json:"model"`
	// 是否开启思考/推理模式
	Reasoning bool `json:"reasoning"`
	// 推理强度：low | medium | high | max
	ReasoningEffort string `json:"reasoning_effort"`
	// 推理模型名
	ReasoningModel string `json:"reasoning_model"`
}

// DefaultAIModel AI 默认模型名（未配置模型时使用）。
const DefaultAIModel = "deepseek-flash"

// AI 模型列表在 [AI] 节中的存储键：模型列表为 JSON 数组字符串，当前模型为选中项 ID。
const (
	AIModelsKey  = "模型列表"
	AICurrentKey = "当前模型"
)

// aiKeyPrefix AI 密钥在配置文件中的密文前缀：用于识别密文，并兼容历史明文值。
const aiKeyPrefix = "enc:"

// EncryptAIKey 加密 AI 接口密钥，用于写入配置文件，避免密钥明文落盘；明文为空时返回空串。
func EncryptAIKey(plain string) (string, error) {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return "", nil
	}
	enc, err := utils.Encrypt(plain, appfiles.Key)
	if err != nil {
		return "", err
	}
	return aiKeyPrefix + enc, nil
}

// DecryptAIKey 解密配置文件中的 AI 接口密钥。
// 不带前缀的值为历史明文，原样返回（下次保存时自动加密为密文）；带前缀但解密失败时返回空串，避免把密文当密钥使用。
func DecryptAIKey(stored string) string {
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return ""
	}
	if !strings.HasPrefix(stored, aiKeyPrefix) {
		return stored
	}
	plain, err := utils.Decrypt(strings.TrimPrefix(stored, aiKeyPrefix), appfiles.Key)
	if err != nil {
		return ""
	}
	return plain
}

// LoadConfig_ai 从配置节读取 AI 配置：读取模型列表与全局设置，并解析当前选中模型填充运行期字段。
// 模型名留空时回退默认模型；密钥字段以密文存储，此处解密后再提供给调用方。
func LoadConfig_ai(sec *ConfigSection) *AIConfig {
	models := LoadAIModels(sec)
	cur := PickAIModel(models, sec.Key(AICurrentKey).String())

	cfg := &AIConfig{
		Open:           sec.Key("启用").MustBool(false),
		SystemPrompt:   strings.TrimSpace(sec.Key("系统提示").String()),
		Timeout:        sec.Key("超时").MustInt(60),
		InlineComplete: sec.Key("代码补全").MustBool(true),
		Models:         models,
	}
	if cur != nil {
		cfg.CurrentID = cur.ID
		cfg.BaseURL = cur.BaseURL
		cfg.APIKey = cur.APIKey
		cfg.Model = cur.Model
		cfg.Reasoning = cur.Reasoning
		cfg.ReasoningEffort = cur.ReasoningEffort
		cfg.ReasoningModel = cur.ReasoningModel
	}
	if cfg.Model == "" {
		cfg.Model = DefaultAIModel
	}
	return cfg
}

// PickAIModel 按 ID 从模型列表中查找；ID 为空或未命中时返回首项，列表为空返回 nil。
func PickAIModel(models []*AIModelConfig, id string) *AIModelConfig {
	if len(models) == 0 {
		return nil
	}
	id = strings.TrimSpace(id)
	if id != "" {
		for _, m := range models {
			if m != nil && m.ID == id {
				return m
			}
		}
	}
	for _, m := range models {
		if m != nil {
			return m
		}
	}
	return nil
}

// LoadAIModels 读取 [AI] 节中的模型列表（JSON 数组），密钥字段解密为明文返回。
// 兼容旧配置：无「模型列表」键时，用旧的单模型键生成一项。
func LoadAIModels(sec *ConfigSection) []*AIModelConfig {
	var models []*AIModelConfig
	if raw := strings.TrimSpace(sec.Key(AIModelsKey).String()); raw != "" {
		_ = json.Unmarshal([]byte(raw), &models)
	}
	if len(models) == 0 {
		if legacy := legacyAIModel(sec); legacy != nil {
			models = []*AIModelConfig{legacy}
		}
	}

	out := make([]*AIModelConfig, 0, len(models))
	for i, m := range models {
		if m == nil {
			continue
		}
		m.ID = strings.TrimSpace(m.ID)
		if m.ID == "" {
			m.ID = fmt.Sprintf("m%d", i+1)
		}
		m.Name = strings.TrimSpace(m.Name)
		m.BaseURL = strings.TrimSpace(m.BaseURL)
		m.APIKey = DecryptAIKey(m.APIKey)
		m.Model = strings.TrimSpace(m.Model)
		m.ReasoningEffort = NormalizeReasoningEffort(m.ReasoningEffort)
		m.ReasoningModel = strings.TrimSpace(m.ReasoningModel)
		out = append(out, m)
	}
	return out
}

// EncodeAIModels 将模型列表序列化为 JSON 字符串供写入配置文件，密钥统一加密；列表为空时返回 "[]"。
func EncodeAIModels(models []*AIModelConfig) (string, error) {
	encrypted := make([]*AIModelConfig, 0, len(models))
	for _, m := range models {
		if m == nil {
			continue
		}
		c := *m
		stored, err := EncryptAIKey(c.APIKey)
		if err != nil {
			return "", err
		}
		c.APIKey = stored
		encrypted = append(encrypted, &c)
	}
	out, err := json.Marshal(encrypted)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// legacyAIModel 用旧版单模型键（接口地址/密钥/模型/思考模式/推理强度/推理模型）构造一项模型配置；
// 未配置接口地址时返回 nil（视为无模型），用于旧配置的平滑迁移。
func legacyAIModel(sec *ConfigSection) *AIModelConfig {
	baseURL := strings.TrimSpace(sec.Key("接口地址").String())
	if baseURL == "" {
		return nil
	}
	model := strings.TrimSpace(sec.Key("模型").String())
	if model == "" {
		model = DefaultAIModel
	}
	return &AIModelConfig{
		ID:              "default",
		Name:            "默认模型",
		BaseURL:         baseURL,
		APIKey:          DecryptAIKey(sec.Key("密钥").String()),
		Model:           model,
		Reasoning:       sec.Key("思考模式").MustBool(false),
		ReasoningEffort: NormalizeReasoningEffort(sec.Key("推理强度").String()),
		ReasoningModel:  strings.TrimSpace(sec.Key("推理模型").String()),
	}
}

// NormalizeReasoningEffort 归一化推理强度，仅接受 low/medium/high/max，其余（含留空）返回空串。
// low/medium/high 适配 OpenAI 等，low/high/max 适配 DeepSeek、Kimi K3 等。
func NormalizeReasoningEffort(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "low", "medium", "high", "max":
		return v
	default:
		return ""
	}
}

type ServerConfigInfo struct {
	// Debug 全局调试开关：控制打印词库缓存等调试信息
	Debug bool
	// TempCleanupInterval 全局临时读写清理周期（秒）
	TempCleanupInterval int
	// DicCache 是否启用词库编译磁盘缓存（private/.dic_cache），默认关闭
	DicCache bool
	// OPUI
	OPUI *OPUI
	// AI 对接（DeepSeek 等 OpenAI 兼容接口）
	AI *AIConfig
	// 内置云工具
	CloudTool *CloudTool
	// 正在监听的WS列表
	WsList map[string]*ServerRouterWebSocket
	// WS列表锁
	WsListMu sync.Mutex
	// QQBot地址（多开支持，key为INI section名如"QQ"、"QQ2"等）
	QQBots map[string]*qqbot_msg.RouterQQBot
	// YunHuBot地址
	YunHuBot *yunhubot_dto.RouterYunHuBot
	// NapCatBot地址
	NapCatBot *napcatbot_dto.RouterNapCatBot
	FeiShuBot *feishubot_msg.RouterFeishubot
	// SecludedBot 对接
	SecludedBot *secludedbot_dto.RouterSecludedBot
	// Ngrok地址
	Ngrok *NgrokConfig
	// Ngrok 隧道监听器（运行时启停用）
	NgrokListener net.Listener
	// Ngrok 取消上下文（运行时启停用）
	NgrokCancel context.CancelFunc
}

// AddWs 添加或更新一个正在监听的 WS 服务
func (s *ServerConfigInfo) AddWs(ws *ServerRouterWebSocket) {
	if ws == nil {
		return
	}
	s.WsListMu.Lock()
	defer s.WsListMu.Unlock()
	if s.WsList == nil {
		s.WsList = make(map[string]*ServerRouterWebSocket)
	}
	s.WsList[ws.Addr] = ws
}

// RemoveWs 移除指定地址的 WS 服务
func (s *ServerConfigInfo) RemoveWs(addr string) {
	s.WsListMu.Lock()
	defer s.WsListMu.Unlock()
	if s.WsList != nil {
		delete(s.WsList, addr)
	}
}

// WsListSnapshot 返回当前监听中的 WS 服务快照（按地址排序）
func (s *ServerConfigInfo) WsListSnapshot() []*ServerRouterWebSocket {
	s.WsListMu.Lock()
	defer s.WsListMu.Unlock()
	list := make([]*ServerRouterWebSocket, 0, len(s.WsList))
	for _, ws := range s.WsList {
		list = append(list, ws)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Addr < list[j].Addr })
	return list
}

type NgrokConfig struct {
	// 地址
	Addr string
	// Token
	Token string
	// ServerAddr 要转发的本地 HTTP 服务器监听地址（空则转发到核心服务器）
	ServerAddr string
}

type HTTPRequestInfo struct {
	Path        string                 `json:"路径"`
	Type        string                 `json:"来源"`
	QueryParams url.Values             `json:"GET,omitempty"`
	Headers     http.Header            `json:"请求头"`
	IP          string                 `json:"IP"`
	Host        string                 `json:"Host"`
	Post        any                    `json:"POST,omitempty"`
	PostFile    map[string][]*PostFile `json:"POSTFile,omitempty"`
}

type PostFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Data string `json:"data"`
}

type SetCookie struct {
	Name     string `json:"命名"`
	Value    string `json:"数据"`
	Path     string `json:"路径"`
	HttpOnly bool   `json:"禁止JS"`
	MaxAge   int    `json:"存活"`
}

// ==============函数启动的服务器================

// FuncServerInfo 由字典函数「服务器」启动的轻量 HTTP 词库服务器状态。
// Update/Close 为 dic 包注入的操作句柄，不参与 JSON 序列化。
type FuncServerInfo struct {
	Addr    string `json:"addr"`     // 监听地址（含端口）
	Cors    bool   `json:"cors"`     // 跨域开关
	DicData string `json:"dic_data"` // 词库源码文本
	Core    bool   `json:"core"`     // 是否核心服务器（由注册表 coreAddr 派生，供前端展示）
	// HTTPS（TLS）配置
	TLS      bool   `json:"tls"`       // 是否启用 HTTPS
	CertFile string `json:"cert_file"` // 证书文件路径（相对 private/https 或绝对路径）
	KeyFile  string `json:"key_file"`  // 密钥文件路径
	// BeerFrp 穿透配置
	FrpOpen       bool   `json:"frp_open"`        // 是否启用 BeerWebFrp 穿透
	FrpServerAddr string `json:"frp_server_addr"` // BeerWebFrp 服务端地址（ws/wss）
	FrpToken      string `json:"frp_token"`       // BeerWebFrp 隧道密钥
	FrpDebug      bool   `json:"frp_debug"`       // BeerWebFrp 调试日志

	Handler http.Handler                     `json:"-"` // 请求处理器（供 Ngrok 等转发复用）
	Update  func(upd FuncServerUpdate) error `json:"-"`
	Close   func()                           `json:"-"`
}

// FuncServerUpdate 前端编辑函数服务器时提交的增量更新（指针字段为 nil 表示不修改）。
type FuncServerUpdate struct {
	Cors          *bool   `json:"cors"`
	TLS           *bool   `json:"tls"`
	CertFile      *string `json:"cert_file"`
	KeyFile       *string `json:"key_file"`
	FrpOpen       *bool   `json:"frp_open"`
	FrpServerAddr *string `json:"frp_server_addr"`
	FrpToken      *string `json:"frp_token"`
	FrpDebug      *bool   `json:"frp_debug"`
}

// FuncServerRegistry 函数启动的服务器注册表（按监听地址索引，同名地址同时只会有一个存活实例）。
type FuncServerRegistry struct {
	mu       sync.Mutex
	mp       map[string]*FuncServerInfo
	coreAddr string // 核心服务器监听地址，空串表示无核心服务器
}

// FuncServers 全局函数服务器注册表，供前端查询/编辑。
var FuncServers = &FuncServerRegistry{mp: make(map[string]*FuncServerInfo)}

// 一次性配置加载器：由 dic 包注入，供「服务器.设置核心服务器」触发。
var (
	configLoadOnce sync.Once
	configLoadFn   func()
)

// SetConfigLoader 注册一次性配置加载回调（仅需调用一次）。
func SetConfigLoader(fn func()) {
	configLoadFn = fn
}

// LoadConfigOnce 触发一次性配置加载，重复调用只会执行一次。
func LoadConfigOnce() {
	if configLoadFn != nil {
		configLoadOnce.Do(configLoadFn)
	}
}

// Add 添加或更新一个函数服务器。核心身份不在此自动指定，须由词库显式调用「服务器.设置核心服务器」标记。
func (r *FuncServerRegistry) Add(s *FuncServerInfo) {
	if s == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mp[s.Addr] = s
}

// SetCore 将指定地址的服务器标记为核心服务器。返回是否设置成功。
// 核心地址单独记录在 coreAddr 字段中，取出即判断，无需遍历各服务器条目。
func (r *FuncServerRegistry) SetCore(addr string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.mp[addr]; !ok {
		return false
	}
	r.coreAddr = addr
	return true
}

// CoreAddr 返回当前核心服务器地址，无核心服务器时返回空串。
func (r *FuncServerRegistry) CoreAddr() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.coreAddr
}

// Remove 移除指定地址的函数服务器；若移除的是核心服务器则同时清空核心地址。
func (r *FuncServerRegistry) Remove(addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.mp, addr)
	if r.coreAddr == addr {
		r.coreAddr = ""
	}
}

// Get 返回指定地址的函数服务器，不存在时返回 nil。
func (r *FuncServerRegistry) Get(addr string) *FuncServerInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.mp[addr]
}

// UpdateData 同步指定服务器的词库源码快照。
func (r *FuncServerRegistry) UpdateData(addr, dicData string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.mp[addr]; ok {
		e.DicData = dicData
	}
}

// UpdateCors 同步指定服务器的跨域开关快照。
func (r *FuncServerRegistry) UpdateCors(addr string, cors bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.mp[addr]; ok {
		e.Cors = cors
	}
}

// Apply 对指定地址的服务器快照执行一次修改（用于同步运行中服务器的可编辑配置）。
func (r *FuncServerRegistry) Apply(addr string, fn func(*FuncServerInfo)) {
	if fn == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.mp[addr]; ok {
		fn(e)
	}
}

// Len 返回当前已注册（监听中）的函数服务器数量。
func (r *FuncServerRegistry) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.mp)
}

// Snapshot 返回当前全部函数服务器快照（按地址排序），Core 字段按 coreAddr 动态派生。
func (r *FuncServerRegistry) Snapshot() []FuncServerInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := make([]FuncServerInfo, 0, len(r.mp))
	for a, e := range r.mp {
		c := *e
		c.Core = a == r.coreAddr
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Addr < list[j].Addr })
	return list
}
