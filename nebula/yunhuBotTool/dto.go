package yunhubottool

// 根对象
type Payload struct {
	Version string `json:"version"`
	Header  Header `json:"header"`
	Event   Event  `json:"event"`
}

// header 部分
type Header struct {
	EventID   string `json:"eventId"`
	EventType string `json:"eventType"`
	EventTime int64  `json:"eventTime"`
}

// event 部分
type Event struct {
	Sender  Sender  `json:"sender"`
	Chat    Chat    `json:"chat"`
	Message Message `json:"message"`
}

// sender 部分
type Sender struct {
	SenderID        string `json:"senderId"`
	SenderType      string `json:"senderType"`
	SenderUserLevel string `json:"senderUserLevel"`
	SenderNickname  string `json:"senderNickname"`
}

// chat 部分
type Chat struct {
	ChatID   string `json:"chatId"`
	ChatType string `json:"chatType"`
}

// message 部分
type Message struct {
	MsgID           string  `json:"msgId"`
	ParentID        string  `json:"parentId"`
	SendTime        int64   `json:"sendTime"`
	ChatID          string  `json:"chatId"`
	ChatType        string  `json:"chatType"`
	ContentType     string  `json:"contentType"`
	Content         Content `json:"content"`
	InstructionID   int     `json:"instructionId"`
	InstructionName string  `json:"instructionName"`
	CommandID       int     `json:"commandId"`
	CommandName     string  `json:"commandName"`
}

// content 部分
type Content struct {
	Text string `json:"text"`
}
