package napcatbottool

// API数据
type APIResponse struct {
	Status  string         `json:"status"`  // 状态码
	Retcode int            `json:"retcode"` // 返回码
	Data    MessagePayload `json:"data"`    // 消息数据
}

// Message数据
type MessagePayload struct {
	SelfID      int64     `json:"self_id"`        // 机器人
	UserID      int64     `json:"user_id"`        // 发言人
	Time        int64     `json:"time"`           // 时间戳
	MessageID   int64     `json:"message_id"`     // 消息 ID
	MessageSeq  int64     `json:"message_seq"`    // 消息序列号
	RealID      int64     `json:"real_id"`        // 消息真实 ID
	RealSeq     string    `json:"real_seq"`       // 消息真实序列号
	MessageType string    `json:"message_type"`   // 消息类型
	GroupID     int64     `json:"group_id"`       // 群号
	GroupName   string    `json:"group_name"`     // 群名
	RawMessage  string    `json:"raw_message"`    // 原始消息
	Font        int       `json:"font"`           // 字体
	SubType     string    `json:"sub_type"`       // 子类型
	Message     []Elem    `json:"message"`        // 消息内容
	MessageFmt  string    `json:"message_format"` // 消息格式
	PostType    string    `json:"post_type"`      // 发送类型
	Sender      Sender    `json:"sender"`         // 发送者信息
	NoticeType  string    `json:"notice_type"`    // 通知类型
	TargetID    int64     `json:"target_id"`      // 目标 ID
	RawInfo     []RawInfo `json:"raw_info"`       // 原始信息
	File        File      `json:"file"`           // 上传的文件信息

	Nickname        string `json:"nickname"`          // 群成员昵称
	Card            string `json:"card"`              // 群名片
	Sex             string `json:"sex"`               // 性别
	Age             int    `json:"age"`               // 年龄
	Area            string `json:"area"`              // 地区
	Level           string `json:"level"`             // 群等级
	QQLevel         int    `json:"qq_level"`          // QQ 等级
	JoinTime        int64  `json:"join_time"`         // 入群时间戳
	LastSentTime    int64  `json:"last_sent_time"`    // 最后发言时间戳
	TitleExpireTime int64  `json:"title_expire_time"` // 群头衔过期时间
	Unfriendly      bool   `json:"unfriendly"`        // 是否不良成员
	CardChangeable  bool   `json:"card_changeable"`   // 是否允许修改群名片
	IsRobot         bool   `json:"is_robot"`          // 是否为机器人
	ShutUpTimestamp int64  `json:"shut_up_timestamp"` // 禁言到期时间
	Role            string `json:"role"`              // 成员角色（member/admin/owner）
	Title           string `json:"title"`             // 群头衔
}

// Sender 发送者信息
type Sender struct {
	UserID   int64  `json:"user_id"`  // 发送者 ID
	Nickname string `json:"nickname"` // 发送者昵称
	Card     string `json:"card"`     // 发送者名片
	Role     string `json:"role"`     // 发送者角色
}

// Elem 消息段元素
type Elem struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

// 根据 type 字段区分用途：qq / img / nor
type RawInfo struct {
	Col  string `json:"col,omitempty"` // 文字颜色（type=qq 时为空）
	Nm   string `json:"nm,omitempty"`  // 名称（type=qq 时为空）
	Type string `json:"type"`          // 段类型：qq（@某人）、img（表情图）、nor（普通文字）
	UID  string `json:"uid,omitempty"` // 用户 uid，type=qq 时出现
	Jp   string `json:"jp,omitempty"`  // 点击图片后的跳转链接，type=img 时出现
	Src  string `json:"src,omitempty"` // 图片 URL，type=img 时出现
	Txt  string `json:"txt,omitempty"` // 文字内容，type=nor 时出现
	Tp   string `json:"tp,omitempty"`  // 客户端保留字段，数字 0 或字符串 "0" 都可能出现
}

// File 描述上传的文件元数据
type File struct {
	ID    string `json:"id"`    // 文件 ID（用于下载或撤回）
	Name  string `json:"name"`  // 文件名（含后缀）
	Size  int64  `json:"size"`  // 文件大小（字节）
	BusID int    `json:"busid"` // 业务 ID（一般固定 102）
}
