package dto

import (
	"database/sql"
	"strings"

	"github.com/cjxpj/nebula/utils"
)

// 系统变量
type LocalDicValue struct {
	For struct {
		Success   bool     `json:"success"`
		Run       any      `json:"for"`
		Num       int      `json:"num"`
		VlaueName string   `json:"vlaueName"`
		Content   []string `json:"content"`
		IsFor     bool     `json:"IsFor"`
		Jump      bool     `json:"jump"`
	} `json:"循环框"`
	ForEach LocalDicValueForEach `json:"遍历框"`
	Func    struct {
		Success   bool     `json:"success"`
		Num       int      `json:"num"`
		VlaueName string   `json:"vlaueName"`
		Trigger   string   `json:"trigger"`
		Content   []string `json:"content"`
	} `json:"函数框"`
	Text struct {
		Success   bool            `json:"success"`
		ReadValue bool            `json:"readValue"`
		LineFeed  string          `json:"lineFeed"`
		VlaueName string          `json:"vlaueName"`
		Content   strings.Builder `json:"content"`
	} `json:"文本框"`
	ValText struct {
		Success   bool     `json:"success"`
		VlaueName string   `json:"vlaueName"`
		Content   []string `json:"content"`
	} `json:"赋予值文本框"`
	ValTextr struct {
		Success   bool     `json:"success"`
		VlaueName string   `json:"vlaueName"`
		Content   []string `json:"content"`
	} `json:"赋予值纯文本框"`
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
		Success   bool   `json:"success"`
		VlaueName string `json:"vlaueName"`
		Json      any    `json:"json"`
		OkLen     bool   `json:"OkLen"`
		Len       int    `json:"Len"`
	} `json:"Json框"`
	SetNewJson struct {
		Success   bool   `json:"success"`
		VlaueName string `json:"vlaueName"`
		Json      string `json:"json"`
		// true时候是{},false时候是[]
		JsonType bool `json:"JsonType"`
		Len      int  `json:"Len"`
	} `json:"新建Json框"`
	Database *sql.DB `json:"database"`
	Stop     bool    `json:"stop"`
}

type LocalDicValueForEach struct {
	Success   bool     `json:"success"`
	Run       any      `json:"for"`
	Num       int      `json:"num"`
	VlaueName string   `json:"vlaueName"`
	Content   []string `json:"content"`
	IsFor     bool     `json:"IsFor"`
	Jump      bool     `json:"jump"`
}

// 词库结构
type BuildDic struct {
	Trigger string   `json:"trigger"`
	Text    []string `json:"text"`
}

type DicClass struct {
	LocalValue  *Val        `json:"变量"`
	LocalFunc   []*BuildDic `json:"函数"`
	LocalStatic []*BuildDic `json:"内部"`
}

type BuildValue struct {
	Head        []string                                                           `json:"头部"`
	Dic         []*BuildDic                                                        `json:"词库"`
	LocalStatic []*BuildDic                                                        `json:"内部"`
	LocalFunc   []*BuildDic                                                        `json:"函数"`
	LocalClass  map[string]*DicClass                                               `json:"整合包"`
	MyFunc      map[string]func(val *DicVal, inputs *utils.DicInputs) (any, error) `json:"自定义函数"`
}

// 克隆
func (v *BuildValue) Clone() *BuildValue {
	return &BuildValue{
		Head:        v.Head,
		Dic:         v.Dic,
		LocalStatic: v.LocalStatic,
		LocalFunc:   v.LocalFunc,
		LocalClass:  v.LocalClass,
		MyFunc:      v.MyFunc,
	}
}

// 关闭回收
func (v *BuildValue) Close() {
	v.LocalFunc = nil
	v.LocalStatic = nil
	v.LocalClass = nil
	v.Dic = nil
	v.Head = nil
}

type MysqlResultInfo struct {
	RowsAffected int64 `json:"rows_affected"`
	LastInsertID int64 `json:"last_insert_id,omitempty"`
}

// 单值寄存结构体
type SingleValue struct {
	Data strings.Builder
}

// 词库参数数据
type DicInputs struct {
	// 执行词库数据
	Dic *BuildValue
	// 变量数据
	V *DicVal
	// 输入参数数据
	Inputs *utils.DicInputs
}
