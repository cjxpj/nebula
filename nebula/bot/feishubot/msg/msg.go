package feishubot_msg

// ImMessageReceiveV1 飞书“接收消息”事件根结构
type ImMessageReceiveV1 struct {
	Schema string `json:"schema"`
	Header Header `json:"header"`
	Event  Event  `json:"event"`
}

// Header 事件元数据
type Header struct {
	AppID      string `json:"app_id"`
	EventID    string `json:"event_id"`
	Token      string `json:"token"`
	CreateTime string `json:"create_time"` // 官方 string，可转 int64
	EventType  string `json:"event_type"`
	TenantKey  string `json:"tenant_key"`
}

// Event 业务数据
type Event struct {
	Message Message `json:"message"`
	Sender  Sender  `json:"sender"`
}

// Message 消息体
type Message struct {
	ChatID      string `json:"chat_id"`
	ChatType    string `json:"chat_type"`
	Content     string `json:"content"` // 这里是 JSON 字符串，可再反序列化
	CreateTime  string `json:"create_time"`
	MessageID   string `json:"message_id"`
	MessageType string `json:"message_type"`
	UpdateTime  string `json:"update_time"`
}

// Sender 发送者
type Sender struct {
	SenderID   SenderID `json:"sender_id"`
	SenderType string   `json:"sender_type"`
	TenantKey  string   `json:"tenant_key"`
}

type SenderID struct {
	OpenID  string  `json:"open_id"`
	UnionID *string `json:"union_id,omitempty"`
	UserID  *string `json:"user_id,omitempty"` // 可能为 null
}
