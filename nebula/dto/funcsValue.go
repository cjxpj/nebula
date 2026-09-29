package dto

import "sync"

// FuncRules 内置函数参数数量规则注册表（函数名 -> 参数规则，如 "1|2"）。
// 由 funcs.Register 在注册时写入，供 run 编译期做函数参数数量静态检查，
// 避免 run 直接 import funcs 造成循环依赖。
var FuncRules sync.Map

// RegisterFuncRule 写入内置函数参数规则。
func RegisterFuncRule(name, l string) {
	FuncRules.Store(name, l)
}

// UnregisterFuncRule 移除内置函数参数规则。
func UnregisterFuncRule(name string) {
	FuncRules.Delete(name)
}

// GetFuncRule 读取内置函数参数规则；未注册返回 ("", false)。
func GetFuncRule(name string) (string, bool) {
	v, ok := FuncRules.Load(name)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// ctxFuncRules 上下文函数参数规则表：函数名 -> 参数数量规则（同一函数在多个上下文存在时取并集，如 "1|2|3"）。
//
// 这些函数不注册进 funcs.FuncList，因此不会改变运行时解析优先级、也不会触发「禁止覆盖系统内置函数」
// 检查（用户仍可在词库中用 [函数] 定义同名函数并将其作为局部函数优先解析）。它们仅在调用方按上下文
// 注入时才真实存在：
//   - 网页接收：设置头部 / GET / POST，由 HTTP 链路（dic/webhttp.go）与词库调试 / AI 运行注入；
//   - 机器人平台：QQ 官方（bot/qqbot）、NapCat（bot/napcatbot）、飞书（bot/feishubot）、
//     SecludedBot（bot/secludedbot）的发送 / 群管理 / 菜单等函数，由各 bot 包在消息上下文
//     与 #引入=@平台 时注入；同名函数（如 群单发 / 私聊 / 撤回 / 踢）取各平台参数规则并集。
//
// 登记在此是为了让积木编程工具箱（funcs.ListFuncs）与编译期参数检查（run/compile_check）能识别它们。
var ctxFuncRules = map[string]string{
	// 网页接收
	"设置头部": "2",
	"GET":  "1|2",
	"POST": "1|2",

	// QQ 机器人：账号 / 群发 / 私聊
	"获取账号":  "0|1",
	"搜索账号":  "1",
	"群单发":   "1|2|3",
	"群单发图":  "4",
	"群单发MD": "3|4",
	"群单发语音": "3",
	"群单发视频": "3",
	"私聊":    "1|2|3",
	"私聊图":   "4",
	"私聊MD":  "3|4",

	// QQ 机器人：消息处理（回复上下文）
	"发送文本": "1|2",
	"发送MD": "1..",
	"发送视频": "1|2|4",
	"发送语音": "1|2|3",
	"流式发送": "1|2|3|4|5",

	// QQ 机器人：群管理
	"禁":         "2|3|4",
	"踢":         "2",
	"踢黑":        "2",
	"群信息":       "2",
	"获取群信息":     "0|1",
	"获取机器人群内状态": "0|1",
	"入群审批":      "3|4|5|6",
	"撤回":        "1|2|3",
	"撤回私聊":      "1|2|3",

	// QQ 机器人：菜单 / 面板
	"获取菜单": "0",
	"设置菜单": "1",
	"面板列表": "1|2|3",
	"创建面板": "1",
	"面板详情": "1",
	"修改面板": "2",
	"删除面板": "1",
	"面板关联": "3",

	// QQ 机器人：通用
	"调用":   "2..",
	"IMG":  "0|1",
	"设置状态": "1",

	// NapCat（bot/napcatbot）
	"群列表":     "0",
	"禁言":      "3",
	"点赞":      "2|3", // NapCat 2 / Secluded 3
	"戳一戳":     "1|2",
	"全体禁言":    "1",
	"全体解禁":    "1",
	"设置群头衔":   "3",
	"设置群管理":   "2",
	"取消群管理":   "2",
	"获取群成员信息": "2",
	"获取群成员列表": "1", // NapCat / 飞书 / Secluded
	"获取好友列表":  "0", // NapCat / Secluded
	"设置群名":    "2",
	"设置群成员名字": "3",
	"获取消息详情":  "1",
	"发送音乐卡片":  "5",
	"构造聊天记录":  "2",
	"发送聊天记录":  "3..",

	// 飞书（bot/feishubot）
	"图片":   "2",
	"回复":   "2",
	"表情回复": "2",
	"上传图片": "1",
	"发送图片": "2",
	"私聊图片": "2",

	// SecludedBot（bot/secludedbot）
	"SEC打印":    "2",
	"SEC发包":    "1|2",
	"获取群列表":    "0",
	"启动获取群列表":  "0",
	"邀请加群":     "2",
	"修改群名":     "2",
	"管理员变动":    "3",
	"拍一拍":      "2",
	"群打卡":      "1",
	"群通知处理":    "4|5",
	"灰色消息":     "2",
	"添加好友":     "1|2",
	"删除好友":     "1",
	"设置好友备注":   "2",
	"获取不活跃列表":  "1",
	"获取用户信息":   "0",
	"设置消息接收模式": "2",
	"群待办":      "3",
	"Xml卡片":    "2",
	"Json卡片":   "2",
	"空间点赞":     "2",
	"添加群白名单":   "1",
	"删除群白名单":   "1",
	"贴表情":      "3",
	"创建群聊":     "1",
	"加群":       "1|2",
}

// CtxFuncRule 读取上下文函数的参数规则；未登记返回 ("", false)。
func CtxFuncRule(name string) (string, bool) {
	r, ok := ctxFuncRules[name]
	return r, ok
}

// EachCtxFunc 遍历全部上下文函数。
func EachCtxFunc(fn func(name, rule string)) {
	for n, r := range ctxFuncRules {
		fn(n, r)
	}
}

// 函数框
type FuncBox struct {
	// Default 调用时的默认参数：$%变量%$ 无参调用时以该值作为触发词，留空默认 "0"。
	Default  string
	Content  []string
	LineNums []int // 每行内容对应的原始文件行号（1-based），用于调试报错定位
}

// 单个函数
type DicFunc struct {
	// 长度
	L string
	// 函数
	Fn func(d *DicInputs) (any, error)
}

// 一次性注册函数
type RegisterDicFunc struct {
	Name string
	L    string
	Fn   func(*DicInputs) (any, error)
}
