package dto

import (
	"strings"
)

var ServerConfig = &ServerConfigInfo{}

type MysqlResultInfo struct {
	RowsAffected int64 `json:"rows_affected"`
	LastInsertID int64 `json:"last_insert_id,omitempty"`
}

// 单值寄存结构体
type SingleValue struct {
	Data strings.Builder
}
