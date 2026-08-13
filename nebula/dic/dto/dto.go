package dic_dto

import "github.com/cjxpj/nebula/dto"

type Dic struct {
	Data      *dto.BuildValue
	Val       *dto.DicVal
	Id        int16
	Path      string
	FuncText  map[string][]*dto.BuildDic
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
	// 当前处理 txt 每行对应的原始文件行号（1-based），与 txt 一一对应
	// 为 nil 时表示无行号映射（如递归调用、非调试场景）
	LineNums []int
	// 递归深度，用于防止 $调用$ 无限递归导致栈溢出
	RecursionDepth int
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
	// 当前执行行号（1-based），用于调试报错定位
	CurLine int
	// 递归深度，用于 Funcs 内部创建子 DicEntry 时传播深度
	RecursionDepth int
}
