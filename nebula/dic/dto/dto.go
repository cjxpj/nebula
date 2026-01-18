package dic_dto

import "github.com/cjxpj/nebula/dto"

type Dic struct {
	Data      *dto.BuildValue
	Val       *dto.DicVal
	Id        int16
	Path      string
	FuncText  []*dto.BuildDic
	ClassText map[string]*dto.DicClass
	// 自义定函数
	MyFunc map[string]dto.DicFunc
}

type WebDic struct {
	Text   string
	Val    *dto.DicVal
	Path   string
	MyFunc map[string]dto.DicFunc
}

// run
type DicEntry struct {
	// 返回信息
	Output  *dto.SingleValue
	Val     *dto.DicVal
	Sys_v   *dto.LocalDicValue
	Trigger bool
	Dic     *dto.BuildValue
}

func (d *DicEntry) Close() {
	d.Output.Clear()
	// 回收局部变量
	d.Val.P.Close()
	// 清空指针
	d.Val = nil
	d.Sys_v = nil
	// 清空回收
	d.Dic.Close()
}

// build
type Build struct {
	Val *dto.DicVal
}

// func
type DicFunc struct {
	// 变量
	Val *dto.DicVal
	// 系统变量
	Sys *dto.LocalDicValue
	// 准备输出内容
	Output *dto.SingleValue
	Dic    *dto.BuildValue
}
