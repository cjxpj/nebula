package dto

import (
	"database/sql"
)

// 系统变量
type LocalDicValue struct {
	For struct {
		Success   bool        `json:"success"`
		Run       interface{} `json:"for"`
		Num       int         `json:"num"`
		VlaueName string      `json:"vlaueName"`
		Content   []string    `json:"content"`
		IsFor     bool        `json:"IsFor"`
		Jump      bool        `json:"jump"`
	} `json:"循环框"`
	Func struct {
		Success   bool     `json:"success"`
		Num       int      `json:"num"`
		VlaueName string   `json:"vlaueName"`
		Trigger   string   `json:"trigger"`
		Content   []string `json:"content"`
	} `json:"函数框"`
	Text struct {
		Success   bool   `json:"success"`
		ReadValue bool   `json:"readValue"`
		LineFeed  string `json:"lineFeed"`
		VlaueName string `json:"vlaueName"`
		Content   string `json:"content"`
	} `json:"文本框"`
	ValText struct {
		Success   bool   `json:"success"`
		ReadValue bool   `json:"readValue"`
		LineFeed  string `json:"lineFeed"`
		VlaueName string `json:"vlaueName"`
		Content   string `json:"content"`
	} `json:"赋予值文本框"`
	Lua struct {
		Success   bool   `json:"success"`
		VlaueName string `json:"vlaueName"`
		VlaueList string `json:"vlaueList"`
		Content   string `json:"content"`
	} `json:"Lua框"`
	Js struct {
		Success   bool   `json:"success"`
		VlaueName string `json:"vlaueName"`
		VlaueList string `json:"vlaueList"`
		Content   string `json:"content"`
	} `json:"Js框"`
	IfFunc struct {
		Success bool       `json:"success"`
		IsElse  bool       `json:"IsElse"`
		Num     int        `json:"num"`
		IfNum   int        `json:"ifnum"`
		If      []string   `json:"if"`
		Else    []string   `json:"Else"`
		Run     [][]string `json:"Run"`
		IsIf    bool       `json:"IsIf"`
		Jump    bool       `json:"jump"`
	} `json:"判断框"`
	SetJson struct {
		Success   bool        `json:"success"`
		VlaueName string      `json:"vlaueName"`
		Json      interface{} `json:"json"`
		OkLen     bool        `json:"OkLen"`
		Len       int         `json:"Len"`
	} `json:"Json框"`
	Database *sql.DB     `json:"database"`
	Json     interface{} `json:"json"`
	Access   interface{} `json:"access"`
	Stop     bool        `json:"stop"`
}

// 词库结构
type BuildDic struct {
	Trigger string   `json:"trigger"`
	Text    []string `json:"text"`
}

type DicClass struct {
	LocalValue *Val        `json:"变量"`
	LocalFunc  []*BuildDic `json:"函数"`
}

type BuildValue struct {
	Head        []string             `json:"头部"`
	Dic         []*BuildDic          `json:"词库"`
	LocalStatic []*BuildDic          `json:"内部"`
	LocalFunc   []*BuildDic          `json:"函数"`
	LocalClass  map[string]*DicClass `json:"整合包"`
}

type MysqlResultInfo struct {
	RowsAffected int64 `json:"rows_affected"`
	LastInsertID int64 `json:"last_insert_id,omitempty"`
}

// 单值寄存结构体
type SingleValue struct {
	Data string
}
