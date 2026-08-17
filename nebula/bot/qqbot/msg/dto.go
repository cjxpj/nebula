package qqbot_msg

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
)

var MsgCount = 0

// QQBot 封装机器人鉴权和发消息流程
type QQBot struct {
	AppId        string
	ClientSecret string
	Key          *AccessTokenResponse
	TokenTime    time.Time
	// 处理次数（msg_seq 递增，原子操作，无需加锁）
	Count atomic.Int64
	// token 刷新互斥，防止并发同时刷新
	TokenMu sync.Mutex
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
	Source string   `json:"source"`
	Ext    []string `json:"ext"`
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
	MsgId  string `json:"msg_id,omitempty"`
	MsgSeq int    `json:"msg_seq"`
	// 前置事件 ID，用于 INTERACTION_CREATE 等事件的被动消息（无用户消息可回复时使用）
	EventId  string    `json:"event_id,omitempty"`
	Markdown *Markdown `json:"markdown,omitempty"`
	// 消息按钮，仅 markdown 消息支持
	Keyboard *Keyboard `json:"keyboard,omitempty"`
	// 引用回复，填写后以引用形式展示，关联上下文
	MessageReference *MessageReference `json:"message_reference,omitempty"`
}

// MessageReference 引用回复
type MessageReference struct {
	MessageID string `json:"message_id"`
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
	GroupOpenID   string     `json:"group_openid"`    // 群 OpenID（WS 事件携带，列表接口为空）
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
// 事件体字段与入群申请列表记录 JoinRequestItem 一致，直接复用
type JoinRequestEvent = JoinRequestItem

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
	Label        string `json:"label"`                   // 按钮文字
	VisitedLabel string `json:"visited_label,omitempty"` // 点击后按钮文字
	Style        int    `json:"style,omitempty"`         // 样式：0 灰色线框，1 蓝色线框
}

// ButtonAction 按钮动作
type ButtonAction struct {
	Type          int               `json:"type"`                     // 0=跳转，1=回调，2=指令
	Permission    *ButtonPermission `json:"permission"`               // 权限（必填）
	Data          string            `json:"data"`                     // 操作数据
	Reply         bool              `json:"reply,omitempty"`          // 指令按钮：是否带引用回复
	Enter         bool              `json:"enter,omitempty"`          // 指令按钮：点击后直接发送 data
	Anchor        int               `json:"anchor,omitempty"`         // 1=唤起选图器（仅手机端单聊）
	UnsupportTips string            `json:"unsupport_tips,omitempty"` // 不支持时的 toast 文案
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
	Code int `json:"code"`
}

// InteractionResponseCode 互动事件响应结果码
// 参考: PUT /interactions/{interaction_id}
type InteractionResponseCode int

const (
	InteractionCodeSuccess      InteractionResponseCode = 0 // 成功
	InteractionCodeFailed       InteractionResponseCode = 1 // 操作失败
	InteractionCodeTooFrequent  InteractionResponseCode = 2 // 操作频繁
	InteractionCodeDuplicated   InteractionResponseCode = 3 // 重复操作
	InteractionCodeNoPermission InteractionResponseCode = 4 // 没有权限
	InteractionCodeAdminOnly    InteractionResponseCode = 5 // 仅管理员操作
)

// ============= 好友事件 ============

// FriendAddEvent 好友添加事件（FRIEND_ADD）
type FriendAddEvent struct {
	OpenID     string       `json:"openid"`      // 用户 OpenID
	Timestamp  int64        `json:"timestamp"`   // 事件时间戳（Unix 秒）
	Scene      int          `json:"scene"`       // 场景
	SceneParam string       `json:"scene_param"` // 场景参数
	Author     FriendAuthor `json:"author"`      // 操作者
}

// FriendDelEvent 好友删除事件（FRIEND_DEL）
type FriendDelEvent struct {
	OpenID    string       `json:"openid"`    // 用户 OpenID
	Timestamp int64        `json:"timestamp"` // 事件时间戳（Unix 秒）
	Author    FriendAuthor `json:"author"`    // 操作者
}

// FriendAuthor 好友事件中的 author 字段
type FriendAuthor struct {
	UnionOpenID string `json:"union_openid"` // 机器人 UnionOpenID
}

// ============= 自定义菜单 (Menu) ============

// Menu 自定义菜单配置，仅 C2C（单聊）场景生效
type Menu struct {
	Items []*MenuItem `json:"items"` // 菜单项列表，最多 10 个，按列表顺序从左到右展示
}

// MenuItem 单个菜单项
type MenuItem struct {
	Name         string         `json:"name"`                     // 按钮名称，最多 10 个字符，一个中文汉字算 2 个字符
	Type         string         `json:"type"`                     // 按钮类型：switch / send_message / link / menu
	SubMenuItems []*SubMenuItem `json:"sub_menu_items,omitempty"` // 子菜单列表，仅 type=menu 时有效，最多 5 个
	SendMessage  string         `json:"send_message,omitempty"`   // 发送内容，仅 type=send_message 时有效
	Link         string         `json:"link,omitempty"`           // 跳转链接，仅 type=link 时有效，须 https:// 开头
	Switch       *Switch        `json:"switch,omitempty"`         // 开关配置，仅 type=switch 时有效
}

// SubMenuItem 子菜单项
type SubMenuItem struct {
	Name        string `json:"name"`                   // 按钮名称，最多 14 个字符
	Type        string `json:"type"`                   // 按钮类型：send_message / link（不支持 menu）
	SendMessage string `json:"send_message,omitempty"` // 发送内容，仅 type=send_message 时有效
	Link        string `json:"link,omitempty"`         // 跳转链接，仅 type=link 时有效
}

// Switch 开关配置
type Switch struct {
	SwitchID string `json:"switch_id"` // 开关唯一标识，切换后消息 ext 中携带 switch_id=1
	Default  bool   `json:"default"`   // 初始状态：true 默认打开，false 默认关闭
}

// MenuResponse 菜单查询/修改响应
type MenuResponse struct {
	Version int   `json:"version"`        // 当前菜单版本号
	Menu    *Menu `json:"menu,omitempty"` // 当前生效的菜单配置，未设置时为空
}

// SetMenuRequest 修改菜单请求
type SetMenuRequest struct {
	Menu *Menu `json:"menu"` // 菜单配置，覆盖原有完整配置
}

// ============= 指令面板 (Panel) ============

// Panel 指令面板配置内容
type Panel struct {
	Items   []*PanelItem `json:"items,omitempty"`  // 面板元素列表，最多 20 个
	Remark  string       `json:"remark,omitempty"` // 面板备注，最多 255 字符，不对用户展示
	Version int          `json:"version,omitempty"`
}

// PanelItem 面板元素
type PanelItem struct {
	Name      string `json:"name,omitempty"`       // 元素名称，最多 14 字符；type=command 时点击填入输入框
	Desc      string `json:"desc,omitempty"`       // 元素描述，最多 30 字符
	Type      string `json:"type,omitempty"`       // 元素类型：command（指令）/ link（链接跳转）
	OnlyAdmin bool   `json:"only_admin,omitempty"` // 是否仅管理员可操作
	Link      string `json:"link,omitempty"`       // 跳转链接，仅 type=link 时有效
}

// PanelRecord 面板列表记录
type PanelRecord struct {
	PanelID    string `json:"panel_id"`    // 面板 ID
	Scope      string `json:"scope"`       // 生效场景：c2c / group / channel / dm
	TargetType string `json:"target_type"` // 作用范围：all / specific
	Panel      *Panel `json:"panel"`       // 面板配置内容
	CreatedAt  string `json:"created_at"`  // 创建时间
	UpdatedAt  string `json:"updated_at"`  // 更新时间
	Version    int    `json:"version"`     // 版本号
}

// PanelListResponse 面板列表响应
type PanelListResponse struct {
	Records    []*PanelRecord `json:"records"`     // 面板记录列表，按设置时间倒序
	NextCursor string         `json:"next_cursor"` // 下一页游标，空串表示已到末页
	IsEnd      bool           `json:"is_end"`      // 是否已到最后一页
}

// PanelDetailResponse 面板详情响应
type PanelDetailResponse struct {
	PanelID      string   `json:"panel_id"`                // 面板 ID
	Scope        string   `json:"scope"`                   // 生效场景
	TargetType   string   `json:"target_type"`             // 作用范围
	Panel        *Panel   `json:"panel"`                   // 面板配置内容
	CreatedAt    string   `json:"created_at"`              // 创建时间
	UpdatedAt    string   `json:"updated_at"`              // 更新时间
	Version      int      `json:"version"`                 // 版本号
	UserOpenIDs  []string `json:"user_openids,omitempty"`  // 关联用户 openid，仅 c2c 且 specific 时返回
	GroupOpenIDs []string `json:"group_openids,omitempty"` // 关联群 openid，仅 group 且 specific 时返回
}

// CreatePanelRequest 创建面板请求
type CreatePanelRequest struct {
	Scope        string   `json:"scope"`                   // 生效场景（必填）：c2c / group / channel / dm
	TargetType   string   `json:"target_type,omitempty"`   // 作用范围：all / specific（channel/dm 仅 all）
	UserOpenIDs  []string `json:"user_openids,omitempty"`  // 用户 openid 列表，仅 c2c 且 specific 有效
	GroupOpenIDs []string `json:"group_openids,omitempty"` // 群 openid 列表，仅 group 且 specific 有效
	Panel        *Panel   `json:"panel"`                   // 面板配置内容（必填）
}

// CreatePanelResponse 创建面板响应
type CreatePanelResponse struct {
	PanelID string `json:"panel_id"` // 新创建的面板 ID
}

// UpdatePanelRequest 修改面板请求
type UpdatePanelRequest struct {
	Panel *Panel `json:"panel"` // 面板配置内容，覆盖原有元素列表和备注
}

// UpdatePanelResponse 修改面板响应
type UpdatePanelResponse struct {
	Version int `json:"version"` // 修改后的面板版本号
}

// UpdatePanelTargetRequest 修改面板关联对象请求
type UpdatePanelTargetRequest struct {
	Op           string   `json:"op"`                      // 操作类型：add / del
	UserOpenIDs  []string `json:"user_openids,omitempty"`  // 用户 openid 列表，仅 c2c 有效
	GroupOpenIDs []string `json:"group_openids,omitempty"` // 群 openid 列表，仅 group 有效
}
