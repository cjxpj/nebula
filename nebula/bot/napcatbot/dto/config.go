package napcatbot_dto

import (
	"regexp"
)

var (
	ReQQAt  = regexp.MustCompile(`\[CQ:at,qq=([0-9]+)\]`)
	ReQQImg = regexp.MustCompile(`\[CQ:image,summary=(.*?),file=([0-9a-zA-Z]+)\.(jpg|png|gif|jpeg|webp),sub_type=([0-9]+),url=(https?:\/\/.+),file_size=([0-9]+)\]`)
)

type RouterNapCatBot struct {
	// 是否开启
	Open bool
	// API地址
	APIAddr string
	// 地址
	Addr string
	// 密钥
	Secret string
	// 词库路径
	FilePath string
}
