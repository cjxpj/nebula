package dic

import (
	"strings"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"
	"github.com/cjxpj/nebula/utils"

	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.Config{
	EscapeHTML: false, // 禁用 HTML 转义
}.Froze()

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

// 清空词库函数
func (r *DicEntry) ClearDicFuncs() *DicEntry {
	r.Dic.LocalFunc = nil
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

func NewWebDic(path, text string) *WebDic {
	return &WebDic{
		Text:   text,
		Val:    dto.NewDicVal(),
		Path:   path,
		MyFunc: make(map[string]func(val *dto.DicVal, inputs *utils.DicInputs) (any, error)),
	}
}

func (WD *WebDic) SetGlobal_v(v *dto.Val) *WebDic {
	WD.Val.G = v
	return WD
}

func (WD *WebDic) Set_v(v *dto.Val) *WebDic {
	WD.Val.P = v
	return WD
}

// 运行网页词库
func (WD *WebDic) Run() string {

	// 返回数据
	var result string

	t := &run.Build{
		Val: WD.Val,
	}

	dicRun := NewRunDicEntry().
		SetV(WD.Val)

	result = run.ReplaceProcessedContent(WD.Text, "<?n", "?>", func(text string) string {
		// fmt.Println("词库文本:", text)
		// 词条总数据
		lines := strings.Split(text, "\n")
		SplitText := t.Web(lines)
		dicRun.SetDic(SplitText)
		dicRun.Dic.MyFunc = WD.MyFunc
		// fmt.Println("词库:", SplitText)
		RunDic := dicRun.Run(SplitText.Head)
		return RunDic
	})

	return result
}

func NewDic(path, text string) *Dic {
	// 去除注释
	// 解密
	str, err := utils.Decrypt(utils.RemoveComments(text), appfiles.Key)
	if err == nil {
		text = str
	}

	return &Dic{
		text:      text,
		Val:       dto.NewDicVal(),
		id:        0,
		Path:      path,
		FuncText:  nil,
		ClassText: nil,
		MyFunc:    make(map[string]func(val *dto.DicVal, inputs *utils.DicInputs) (any, error)),
	}
}

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
func (D *Dic) SetFunc(name string, fn func(val *dto.DicVal, inputs *utils.DicInputs) (any, error)) *Dic {
	D.MyFunc[name] = fn
	return D
}

// 新建运行
func (D *Dic) NewRun(trigger string) string {
	D.Set_v(dto.NewVal())
	return D.Run(trigger)
}

// 新建变量
func (D *Dic) NewDicVal() *dto.DicVal {
	newV := dto.NewDicVal()
	newV.G = D.Val.G
	return newV
}

// 运行内部
func (D *Dic) RunPrivate(trigger string) string {
	newV := dto.NewDicVal()
	newV.G = D.Val.G
	return D.RunPrivateVal(trigger, newV)
}

// 运行内部-自义定局部变量
func (D *Dic) RunPrivateVal(trigger string, v *dto.DicVal) string {

	t := &run.Build{
		Val:  v,
		Path: D.Path,
	}

	SplitText := t.SplitText(D.text)

	if D.FuncText != nil {
		SplitText.LocalFunc = append(SplitText.LocalFunc, D.FuncText...)
	}

	if D.ClassText != nil {
		for key, val := range D.ClassText {
			SplitText.LocalClass[key] = val
		}
	}

	GetDic, GetDicTrigger, _, _ := run.RunFor(SplitText.LocalStatic, trigger, 0)
	D.Val.P.Set("触发词", trigger)
	D.Val.P.Set("触发", GetDicTrigger)

	dicRun := NewRunDicEntry().
		SetV(D.Val).
		SetDic(SplitText)
	dicRun.Dic.MyFunc = D.MyFunc

	return dicRun.Run(GetDic)

}

// 运行词库(全局变量,词库文本,触发)
func (D *Dic) Run(trigger string) string {

	// 返回数据
	var result string

	// 词库头部数据
	var DicHaderText []string

	// 词库数据
	var DicText []*dto.BuildDic

	// 执行返回数据
	var RunDic string

	t := &run.Build{
		Val:  D.Val,
		Path: D.Path,
	}

	SplitText := t.SplitText(D.text)
	// fmt.Println("词库文本:", SplitText)

	if D.FuncText != nil {
		SplitText.LocalFunc = append(SplitText.LocalFunc, D.FuncText...)
	}

	if D.ClassText != nil {
		for key, val := range D.ClassText {
			SplitText.LocalClass[key] = val
		}
	}

	DicHaderText = SplitText.Head

	DicText = SplitText.Dic

	GetDic, GetDicTrigger, _, _ := run.RunFor(DicText, trigger, 0)
	D.Val.P.Set("触发词", trigger)
	D.Val.P.Set("触发", GetDicTrigger)

	dicRun := NewRunDicEntry().
		SetV(D.Val).
		SetDic(SplitText)
	dicRun.Dic.MyFunc = D.MyFunc

	RunDichader := dicRun.Run(DicHaderText)

	if !dicRun.Sys_v.Stop {
		RunDic = dicRun.Run(GetDic)
	}

	result = RunDichader + RunDic

	return result
}
