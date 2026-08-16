package dic_dto

import (
	"maps"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"
	"github.com/cjxpj/nebula/utils"
)

// 关闭回收
func (D *Dic) Close() {
	D.Val.Close()
	D.MyFunc = nil
}

func (D *Dic) SetGlobal_v(v *dto.Val) *Dic {
	D.Val.G = v
	return D
}

func (D *Dic) Set_v(v *dto.Val) *Dic {
	D.Val.P = v
	return D
}

// 设置函数
func (D *Dic) SetFunc(name string, fn dto.DicFunc) *Dic {
	D.MyFunc[name] = fn
	return D
}

// 添加全部函数
func (D *Dic) AddFuncs(fn map[string]dto.DicFunc) *Dic {
	maps.Copy(D.MyFunc, fn)
	return D
}

// 新建变量
func (D *Dic) NewDicVal() *dto.DicVal {
	newV := dto.NewDicVal()
	newV.G = D.Val.G
	return newV
}

// 设置在循环中
func (r *DicEntry) SetRunFor() *DicEntry {
	r.Sys_v.For.IsFor = true
	return r
}

// 设置在遍历中
func (r *DicEntry) SetRunForEach() *DicEntry {
	r.Sys_v.ForEach.IsFor = true
	return r
}

// 设置在判断中
func (r *DicEntry) SetRunIf() *DicEntry {
	r.Sys_v.IfFunc.IsIf = true
	return r
}

// 设置词库信息
func (r *DicEntry) SetDic(s *dto.BuildValue) *DicEntry {
	r.Dic = s
	return r
}

// 设置变量V
func (r *DicEntry) SetV(v *dto.DicVal) *DicEntry {
	r.Val = v
	return r
}

// 设置全局变量
func (r *DicEntry) SetGlobal_v(v *dto.Val) *DicEntry {
	r.Val.G = v
	return r
}

// 设置局部变量
func (r *DicEntry) Set_v(v *dto.Val) *DicEntry {
	r.Val.P = v
	return r
}

// 清空词库函数（保留内部/特殊，仅清空函数类）。
// 复制一份 DicFuncs 并替换为独立 BuildValue，避免修改父级共享数据。
func (r *DicEntry) ClearDicFuncs() *DicEntry {
	if r.Dic == nil || r.Dic.DicFuncs == nil {
		return r
	}
	funcs := make(map[string][]*dto.BuildDic, len(r.Dic.DicFuncs))
	for k, v := range r.Dic.DicFuncs {
		if k != "函数" {
			funcs[k] = v
		}
	}
	r.Dic = &dto.BuildValue{
		Head:         r.Dic.Head,
		HeadLineNums: r.Dic.HeadLineNums,
		Dic:          r.Dic.Dic,
		DicFuncs:     funcs,
		Class:        r.Dic.Class,
		MyFunc:       r.Dic.MyFunc,
		BotImports:   r.Dic.BotImports,
	}
	return r
}

// 继承词库变量
func (r *DicEntry) SetDic_v(v *dto.BuildValue) *DicEntry {
	r.Dic = v
	return r
}

func (r *DicEntry) OpenTrigger() {
	r.Trigger = true
}

func (r *DicEntry) CloseTrigger() *DicEntry {
	r.Trigger = false
	return r
}

// WithRecursionDepth 基于父级深度设置递归深度+1，防止 $调用$ 无限递归
func (r *DicEntry) WithRecursionDepth(parentDepth int) *DicEntry {
	r.RecursionDepth = parentDepth + 1
	return r
}

func (WD *WebDic) SetGlobal_v(v *dto.Val) *WebDic {
	WD.Val.G = v
	return WD
}

func (WD *WebDic) Set_v(v *dto.Val) *WebDic {
	WD.Val.P = v
	return WD
}

func RunDic(path string) (*Dic, error) {
	d, err := utils.NewFileQueue(path).ReadFromFile()
	if err != nil {
		return nil, err
	}
	return NewDic(path, d), nil
}

func NewDic(path, text string) *Dic {
	// 去除注释后解密
	str, err := utils.Decrypt(utils.RemoveComments(text), appfiles.Key)
	if err == nil {
		text = str
	}

	val := dto.NewDicVal()
	SplitText := run.BuildDic(path, text)

	return &Dic{
		Data:      SplitText,
		Val:       val,
		Id:        0,
		Path:      path,
		FuncText:  nil,
		ClassText: nil,
		MyFunc:    SplitText.MyFunc,
	}
}

// 执行词库
func NewRunDicEntry() *DicEntry {
	return &DicEntry{
		Sys_v:  &dto.LocalDicValue{},
		Output: &dto.SingleValue{},
		Val: &dto.DicVal{
			G: dto.NewVal(),
			P: dto.NewVal(),
		},
		Trigger: true,
		Dic:     &dto.BuildValue{},
	}
}

func NewWebDic(path, text string) *WebDic {
	return &WebDic{
		Text:   text,
		Val:    dto.NewDicVal(),
		Path:   path,
		MyFunc: make(map[string]dto.DicFunc),
	}
}
