package yunhubotapi

type RouterYunHuBot struct {
	// 是否开启
	Open bool
	// 地址
	Addr string
	// 密钥
	Secret string
	// 词库路径
	FilePath string
	// 消息次数
	Count int
}
