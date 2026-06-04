package secludedbot_dto

// RouterSecludedBot 是在全局配置中保存的 Secluded 对接信息
type RouterSecludedBot struct {
	// 是否开启
	Open bool
	// WebSocket 对接地址 (ws://host:port 或 wss://...)
	Addr string
	// 令牌
	Token string
	// 词库路径
	FilePath string
	// 机器人账户（QQ号）
	Account string
}
