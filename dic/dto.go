package dic

import (
	"github.com/cjxpj/nebula/dto"
)

type Dic struct {
	text      string
	trigger   string
	G_V       *dto.Val
	V         *dto.Val
	id        int16
	Path      string
	CacheName string
	Cache     bool
	FuncText  []*dto.BuildDic
	ClassText map[string]*dto.DicClass
}

type WebDic struct {
	Text string
	G_V  *dto.Val
	V    *dto.Val
	Path string
}

// run
type DicEntry struct {
	// 返回信息
	Output  *dto.SingleValue
	Text    []string
	G_v     *dto.Val
	V       *dto.Val
	Sys_v   *dto.LocalDicValue
	Trigger bool
	Path    string
	Dic     *dto.BuildValue
}

// if
type IfText struct{}

// build
type Build struct {
	G_v  *dto.Val
	V    *dto.Val
	Path string
}

// 常用结构
type DicTools struct {
	Func           *DicFunc
	GlobalVariable *dto.Val
	LocalVariable  *dto.Val
}

// func
type DicFunc struct {
	// 套娃
	Open bool
	// 全局变量
	GV *dto.Val
	// 局部变量
	V *dto.Val
	// 系统变量
	Sys *dto.LocalDicValue
	// 全局路径
	Path string
	// 准备输出内容
	Output *dto.SingleValue
	Dic    *dto.BuildValue
}
