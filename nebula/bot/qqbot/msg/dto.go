package qqbot_msg

import (
	"encoding/json"
	"time"
)

var MsgCount = 0

// QQBot 封装机器人鉴权和发消息流程
type QQBot struct {
	AppId        string
	ClientSecret string
	Key          *AccessTokenResponse
	TokenTime    time.Time
	// 处理次数
	Count int
	// 调试打印
	Debug bool
}

// AccessTokenResponse 表示 access_token 响应
type AccessTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   string `json:"expires_in"`
}

// 事件
type Payload struct {
	// 事件ID
	Id string `json:"id"`
	// 事件类型
	Op int `json:"op"`
	// 数据
	Data json.RawMessage `json:"d"`
	// 唯一标识
	Seq int `json:"s"`
	// 请求类型
	Type string `json:"t"`
}

// 签名数据来源
type ValidationRequest struct {
	// 请求的token
	Token string `json:"plain_token"`
	// 请求的签名
	Event string `json:"event_ts"`
}

// 返回签名验证结果
type ValidationResult struct {
	PlainToken string `json:"plain_token"`
	Signature  string `json:"signature"`
}

// =============消息处理===============

// =============频道消息处理================

// Author 表示消息发送者信息
type Author struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Avatar      string `json:"avatar"`
	Bot         bool   `json:"bot"`
	UnionOpenID string `json:"union_openid"`
}

// Member 表示成员信息
type Member struct {
	Nick     string   `json:"nick"`
	Roles    []string `json:"roles"`
	JoinedAt string   `json:"joined_at"`
}

// Mention 表示一个被提及的用户
type Mention struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	// 可扩展其他字段，比如头像、是否bot等
}

// 频道消息事件
type GuildMessageEvent struct {
	ID string `json:"id"`
	// 子频道ID
	ChannelID string `json:"channel_id"`
	// 频道ID
	GuildID string `json:"guild_id"`
	// 消息内容
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
	Author    Author `json:"author"`
	Member    Member `json:"member"`
	// 被@的用户列表
	Mentions     []Mention `json:"mentions"`
	Seq          int       `json:"seq"`
	SeqInChannel string    `json:"seq_in_channel"`
}

// =============群消息处理================

// 群用户
type GroupAuthor struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	// 群
	MemberOpenID string `json:"member_openid"`
	MemberRole   string `json:"member_role"`
	// 私聊
	UserOpenID  string `json:"user_openid"`
	UnionOpenID string `json:"union_openid"`
}

// 群消息场景
type GroupMessageScene struct {
	Source string `json:"source"`
}

// 附件
type Attachment struct {
	URL         string `json:"url"`
	Filename    string `json:"filename"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Size        int    `json:"size"`
	ContentType string `json:"content_type"`
	Content     string `json:"content"`
}

// 群消息事件
type GroupMessageEvent struct {
	ID        string      `json:"id"`
	Content   string      `json:"content"`
	Timestamp string      `json:"timestamp"`
	Author    GroupAuthor `json:"author"`
	// 被@的用户列表
	Mentions []Mention `json:"mentions"`
	// 附件列表
	Attachments []Attachment `json:"attachments"`
	// 群
	GroupID string `json:"group_id"`
	// 群
	GroupOpenID  string            `json:"group_openid"`
	MessageScene GroupMessageScene `json:"message_scene"`
	MessageType  int               `json:"message_type"`
}

// 群富媒体
type GroupMessageFile struct {
	// 	媒体类型：1 图片，2 视频，3 语音，4 文件（暂不开放）
	// 资源格式要求
	//  图片：png/jpg，视频：mp4，语音：silk
	Type int    `json:"file_type"`
	Url  string `json:"file_url"`
	Srv  bool   `json:"srv_send_msg"` // 是否发送到群内
	// base64编码
	Data string `json:"file_data"`
}

// 群富媒体返回
type GroupMessageFileResponse struct {
	Uuid string `json:"file_uuid"`
	Info string `json:"file_info"`
	TTL  int    `json:"ttl"`
	ID   string `json:"id"`
}

// ChannelSend 频道发送
type ChannelSend struct {
	Content string `json:"content"`
	MsgId   string `json:"msg_id"`
}

// MessageToSend 是发送的文本消息体
type MessageToSend struct {
	// 消息类型：0 是文本，2 是 markdown， 3 ark，4 embed，7 media 富媒体
	MsgType int    `json:"msg_type"`
	Content string `json:"content"`
	// 群富媒体
	Media *GroupMessageFileResponse `json:"media,omitempty"`
	// 被动消息必填
	MsgId    string    `json:"msg_id"`
	MsgSeq   int       `json:"msg_seq"`
	Markdown *Markdown `json:"markdown,omitempty"`
	// 消息按钮，仅 markdown 消息支持
	Keyboard *Keyboard `json:"keyboard,omitempty"`
}

// Markdown
type Markdown struct {
	Content                  string            `json:"content,omitempty"`
	CustomTemplateId         string            `json:"custom_template_id,omitempty"`
	Params                   []*MarkdownParams `json:"params,omitempty"`
	ForceVerifyImageResource bool              `json:"force_verify_image_resource,omitempty"`
}

// MarkdownParams
type MarkdownParams struct {
	Key    string   `json:"key"`
	Values []string `json:"values"`
}

// MessageResponse 是发送成功后返回的结构体
type MessageResponse struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	Content   string `json:"content"`
}

// GroupMemberRole 群成员角色
type GroupMemberRole struct {
	Role string `json:"role"` // "owner" | "admin" | "member"
}

// 群成员加入/退出事件（兼容 GROUP_MEMBER_ADD/REMOVE 和 GROUP_ADD_ROBOT/DEL_ROBOT）
type GroupMemberEvent struct {
	GroupOpenID    string `json:"group_openid"`     // 群 OpenID
	MemberOpenID   string `json:"member_openid"`    // 成员 OpenID（GROUP_MEMBER_ADD/REMOVE）
	OpMemberOpenID string `json:"op_member_openid"` // 操作者 OpenID（GROUP_ADD_ROBOT/DEL_ROBOT）
	UserOpenID     string `json:"user_openid"`      // 成员用户 OpenID（跨应用统一标识，可能为空）
	Timestamp      int64  `json:"timestamp"`        // 事件时间戳（Unix 秒）
}

// ============= 群禁言管理 ============

// SetMuteMember 设置禁言请求中的单个成员
type SetMuteMember struct {
	Op           string `json:"op"`             // add 增加禁言，update 更新到期时间，del 解除禁言
	MemberOpenID string `json:"member_openid"`  // 被禁言成员的 openid
	MuteExpireAt string `json:"mute_expire_at"` // 禁言到期时间 RFC3339 格式；del 时可为空串
}

// SetMemberMuteRequest 设置用户禁言请求
type SetMemberMuteRequest struct {
	Members []SetMuteMember `json:"members"` // 用户禁言列表，单次最多 10 个
}

// MuteStatusResponse 查询群禁言状态返回
type MuteStatusResponse struct {
	GlobalRule MuteGlobalRule   `json:"global_rule"` // 群级禁言规则
	Members    []MuteMemberInfo `json:"members"`     // 当前禁言中的用户列表
}

// MuteGlobalRule 群级禁言规则
type MuteGlobalRule struct {
	Mode           string              `json:"mode"`            // none 未开启，always 始终禁言，schedule 定时禁言
	ScheduleRules  []MuteScheduleRule  `json:"schedule_rules"`  // 定时禁言规则
	RecurringRules []MuteRecurringRule `json:"recurring_rules"` // 周期禁言规则
}

// MuteScheduleRule 定时禁言规则
type MuteScheduleRule struct {
	TaskID  string `json:"task_id"`
	StartAt string `json:"start_at"`
	EndAt   string `json:"end_at"`
	Enabled bool   `json:"enabled"`
}

// MuteRecurringRule 周期禁言规则
type MuteRecurringRule struct {
	TaskID    string `json:"task_id"`
	Weekdays  []int  `json:"weekdays"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Enabled   bool   `json:"enabled"`
}

// MuteMemberInfo 禁言中的成员信息
type MuteMemberInfo struct {
	MemberOpenID string `json:"member_openid"`
	MuteExpireAt string `json:"mute_expire_at"`
	Username     string `json:"username"`
	UnionOpenID  string `json:"union_openid"`
}

// ============= 群信息 ============

// GroupInfo 群信息返回
type GroupInfo struct {
	GroupOpenID     string   `json:"group_openid"`      // 群 OpenID
	GroupName       string   `json:"group_name"`        // 群名称
	GroupFingerMemo string   `json:"group_finger_memo"` // 群简介
	GroupClassText  string   `json:"group_class_text"`  // 群分类
	GroupTags       []string `json:"group_tags"`        // 群标签列表
	GroupMemberNum  int      `json:"group_member_num"`  // 群成员人数
}

// BotState 机器人群内状态返回
type BotState struct {
	MemberOpenID      string `json:"member_openid"`       // 机器人的 OpenID
	JoinedAt          string `json:"joined_at"`           // 入群时间，RFC3339 格式
	AllowProactiveMsg bool   `json:"allow_proactive_msg"` // 是否接收主动推送
	RecvMsgSetting    string `json:"recv_msg_setting"`    // 接收消息类型: all / only_mention / mention_and_context
	MemberRole        string `json:"member_role"`         // 群成员角色: member / owner / admin
}

// ============= 入群申请审批 ============

// JoinRequestItem 入群申请单条记录
type JoinRequestItem struct {
	JoinRequestID string     `json:"join_request_id"` // 申请 ID
	RiskTips      string     `json:"risk_tips"`       // 安全提示语
	UnionOpenID   string     `json:"union_openid"`    // 统一标识
	MemberOpenID  string     `json:"member_openid"`   // 申请人 OpenID
	Username      string     `json:"username"`        // 申请人昵称
	ApplyAt       string     `json:"apply_at"`        // 申请时间（RFC3339）
	ApplySource   string     `json:"apply_source"`    // 申请来源：self_apply / invited
	InvitedBy     string     `json:"invited_by"`      // 邀请人 OpenID
	Bot           bool       `json:"bot"`             // 是否为机器人账号
	VerifyInfo    VerifyInfo `json:"verify_info"`     // 验证方式
}

// VerifyInfo 入群验证信息
type VerifyInfo struct {
	Method        string     `json:"method"`         // verify_message / admin_review_qa
	VerifyMessage string     `json:"verify_message"` // 验证消息
	ReviewQAList  []ReviewQA `json:"review_qa_list"` // 问答列表
}

// ReviewQA 问答
type ReviewQA struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// JoinRequestListResponse 入群申请列表返回
type JoinRequestListResponse struct {
	List       []JoinRequestItem `json:"list"`        // 入群申请列表
	NextCursor string            `json:"next_cursor"` // 下一页游标，空串表示已到末页
}

// ApproveJoinRequest 审批入群请求
type ApproveJoinRequest struct {
	Op                   string `json:"op"`                      // approve 通过 / decline 拒绝
	JoinRequestID        string `json:"join_request_id"`         // 申请 ID
	RejectReason         string `json:"reject_reason"`           // 拒绝理由
	AddToMemberBlacklist bool   `json:"add_to_member_blacklist"` // 是否同时加入群黑名单
}

// ============= 入群申请事件 ============

// JoinRequestEvent 用户入群申请事件（GROUP_JOIN_REQUEST）
type JoinRequestEvent struct {
	GroupOpenID string `json:"group_openid"` // 群 OpenID
	ApplicantID string `json:"applicant_id"` // 申请人 OpenID
	ApplyTime   string `json:"apply_time"`   // 申请时间戳（秒）
	ApplyReason string `json:"apply_reason"` // 申请理由
	RequestID   string `json:"request_id"`   // 申请 ID
}

// ============= 消息按钮 (Keyboard) ============

// Keyboard 消息按钮，仅 markdown 消息支持。支持模版模式(id)和自定义模式(content)二选一
type Keyboard struct {
	ID      string           `json:"id,omitempty"`      // 按钮模版 ID（申请后获得）
	Content *KeyboardContent `json:"content,omitempty"` // 自定义按钮内容
}

// KeyboardContent 自定义按钮内容
type KeyboardContent struct {
	Rows []*KeyboardRow `json:"rows"` // 按钮行，最多 5 行
}

// KeyboardRow 一行按钮，最多 5 个按钮
type KeyboardRow struct {
	Buttons []*Button `json:"buttons"`
}

// Button 单个按钮
type Button struct {
	ID         string            `json:"id,omitempty"` // 按钮 ID，同一个 keyboard 内需唯一
	RenderData *ButtonRenderData `json:"render_data"`  // 按钮渲染数据（必填）
	Action     *ButtonAction     `json:"action"`       // 按钮动作（必填）
}

// ButtonRenderData 按钮渲染样式
type ButtonRenderData struct {
	Label        string `json:"label"`         // 按钮文字
	VisitedLabel string `json:"visited_label"` // 点击后按钮文字
	Style        int    `json:"style"`         // 样式：0 灰色线框，1 蓝色线框
}

// ButtonAction 按钮动作
type ButtonAction struct {
	Type          int               `json:"type"`             // 0=跳转，1=回调，2=指令
	Permission    *ButtonPermission `json:"permission"`       // 权限（必填）
	Data          string            `json:"data"`             // 操作数据
	Reply         bool              `json:"reply,omitempty"`  // 指令按钮：是否带引用回复
	Enter         bool              `json:"enter,omitempty"`  // 指令按钮：点击后直接发送 data
	Anchor        int               `json:"anchor,omitempty"` // 1=唤起选图器（仅手机端单聊）
	UnsupportTips string            `json:"unsupport_tips"`   // 不支持时的 toast 文案（必填）
}

// ButtonPermission 按钮操作权限
type ButtonPermission struct {
	Type           int      `json:"type"`                       // 0=指定用户，1=仅管理者，2=所有人，3=指定身份组
	SpecifyUserIDs []string `json:"specify_user_ids,omitempty"` // 有权限的用户 ID 列表
	SpecifyRoleIDs []string `json:"specify_role_ids,omitempty"` // 有权限的身份组 ID 列表（仅频道）
}

// ============= 交互事件 (INTERACTION_CREATE) ============

// InteractionEvent 用户交互事件（按钮点击、快捷菜单等）
type InteractionEvent struct {
	ID                string           `json:"id"`                  // 事件 ID，用于被动回复
	Type              int              `json:"type"`                // 11=消息按钮回调，12=快捷菜单
	Scene             string           `json:"scene"`               // c2c/group/guild
	ChatType          int              `json:"chat_type"`           // 0=频道，1=群聊，2=单聊
	Timestamp         string           `json:"timestamp"`           // RFC3339
	GuildID           string           `json:"guild_id"`            // 频道 ID
	ChannelID         string           `json:"channel_id"`          // 子频道 ID
	UserOpenID        string           `json:"user_openid"`         // 用户 OpenID
	GroupOpenID       string           `json:"group_openid"`        // 群 OpenID
	GroupMemberOpenID string           `json:"group_member_openid"` // 群成员 OpenID
	Data              *InteractionData `json:"data"`                // 交互数据
	Version           int              `json:"version"`
	ApplicationID     string           `json:"application_id"`
}

// InteractionData 交互数据
type InteractionData struct {
	Type     int                  `json:"type"`     // 与外层 type 一致
	Resolved *InteractionResolved `json:"resolved"` // 解析后的数据
}

// InteractionResolved 解析后的交互数据
type InteractionResolved struct {
	ButtonData  string `json:"button_data"`  // 按钮 data 字段值
	ButtonID    string `json:"button_id"`    // 按钮 id 字段值
	UserID      string `json:"user_id"`      // 频道用户 ID
	FeatureID   string `json:"feature_id"`   // 快捷菜单功能 ID
	MessageID   string `json:"message_id"`   // 操作的消息 ID
	FeedbackOpt string `json:"feedback_opt"` // 反馈选项
	Checked     int    `json:"checked"`      // 是否选中
	Action      string `json:"action"`       // 操作类型
}

// InteractionResponse 交互回应
type InteractionResponse struct {
	Code int `json:"code"` // 0=成功，非 0=失败
}
