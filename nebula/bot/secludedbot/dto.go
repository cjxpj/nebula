package secludedbot

// 上行 / 下行包通用头
type Packet struct {
	Seq     int64  `json:"seq,omitempty"`
	Cmd     string `json:"cmd"`
	Rsp     bool   `json:"rsp,omitempty"`
	RawData []byte `json:"-"`
}

// SyncOicq 插件上线包
type SyncOicq struct {
	Pid   string `json:"pid"`
	Name  string `json:"name"`
	Token string `json:"token"`
}

// Response 应答包
type Response struct {
	Status bool   `json:"status,omitempty"`
	Msg    string `json:"msg,omitempty"`
}

// PushOicqMsg 推送消息包（数组形式）
type PushOicqMsg struct {
	Account  string `json:"Account"`
	Bubble   string `json:"Bubble"`
	Debug    string `json:"Debug"`
	Group    string `json:"Group"`
	GroupId  string `json:"GroupId"`
	GroupName string `json:"GroupName"`
	MsgId    string `json:"MsgId"`
	MsgType  string `json:"MsgType"`
	Op       string `json:"Op"`
	OpName   string `json:"OpName"`
	OpUid    string `json:"OpUid"`
	Title    string `json:"Title"`
	Typeface string `json:"Typeface"`
	Uid      string `json:"Uid"`
	Uin      string `json:"Uin"`
	UinName  string `json:"UinName"`
	GolineMode string `json:"GolineMode"`
	// 下面这些在第二个 map 中出现（混合消息结构）
	Friend string `json:"Friend,omitempty"`
	Temp   string `json:"Temp,omitempty"`
	Text   string `json:"Text,omitempty"`
	Img    string `json:"Img,omitempty"`
}

// SendOicqMsg 发送消息包（数组形式，两个 map）
type SendOicqMsg struct {
	Account string `json:"Account,omitempty"`
	Group   string `json:"Group,omitempty"`
	Friend  string `json:"Friend,omitempty"`
	Temp    string `json:"Temp,omitempty"`
	GroupId string `json:"GroupId,omitempty"`
	Uin     string `json:"Uin,omitempty"`
	Reply   string `json:"Reply,omitempty"`
	MsgId   string `json:"MsgId,omitempty"`
	Text    string `json:"Text,omitempty"`
	Img     string `json:"Img,omitempty"`
	Ptt     string `json:"Ptt,omitempty"`
	Value   string `json:"Value,omitempty"`
	Time    string `json:"Time,omitempty"`
}
