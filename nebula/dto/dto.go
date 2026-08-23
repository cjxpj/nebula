package dto

import (
	"strings"
)

var ServerConfig = &ServerConfigInfo{}

// CloudToolConnected 查询云工具是否已连接（由 server 包在启动时注入，避免 dic/funcs 直接依赖 server 造成循环引用）
var CloudToolConnected func() bool

type MysqlResultInfo struct {
	RowsAffected int64 `json:"rows_affected"`
	LastInsertID int64 `json:"last_insert_id,omitempty"`
}

// 单值寄存结构体
type SingleValue struct {
	Data strings.Builder
}
