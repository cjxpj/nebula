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
func NewRunDicEntry(path string) *DicEntry {
	global_v := dto.NewVal()
	v := dto.NewVal()

	setpath := "web"
	if path != "" {
		setpath = path
	}

	return &DicEntry{
		Sys_v:   &dto.LocalDicValue{},
		Output:  &dto.SingleValue{},
		G_v:     global_v,
		V:       v,
		Trigger: true,
		Path:    setpath,
		Dic:     &dto.BuildValue{},
	}
}

// 设置在循环中
func (r *DicEntry) SetRunFor() *DicEntry {
	r.Sys_v.For.IsFor = true
	return r
}

// 设置在判断中
func (r *DicEntry) SetRunIf() *DicEntry {
	r.Sys_v.IfFunc.IsIf = true
	return r
}

// 设置词库文本
func (r *DicEntry) SetText(s []string) *DicEntry {
	r.Text = s
	return r
}

// 设置词库信息
func (r *DicEntry) SetDic(s *dto.BuildValue) *DicEntry {
	r.Dic = s
	return r
}

// 设置全局路径
func (r *DicEntry) SetPath(s string) *DicEntry {
	r.Path = s
	return r
}

// 设置全局变量
func (r *DicEntry) SetGlobal_v(v *dto.Val) *DicEntry {
	r.G_v = v
	return r
}

// 设置局部变量
func (r *DicEntry) Set_v(v *dto.Val) *DicEntry {
	r.V = v
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

func NewWebDic(text, path string) *WebDic {
	g := dto.NewVal()
	v := dto.NewVal()
	return &WebDic{
		Text: text,
		G_V:  g,
		V:    v,
		Path: path,
	}
}

func (WD *WebDic) SetGlobal_v(v *dto.Val) *WebDic {
	WD.G_V = v
	return WD
}

func (WD *WebDic) Set_v(v *dto.Val) *WebDic {
	WD.V = v
	return WD
}

// 运行网页词库
func (WD *WebDic) Run() string {

	// 返回数据
	var result string

	// 全局变量
	global_v := WD.G_V

	// 局部变量
	dicV := WD.V

	// 词库文本
	str := WD.Text

	// 插入后文本
	var BuildText string

	t := &run.Build{
		G_v:  global_v,
		V:    dicV,
		Path: WD.Path,
	}

	// 使用导入函数
	BuildText, err := t.ImportText(str)
	if err != nil {
		BuildText = str
	}

	dicRun := NewRunDicEntry(WD.Path).
		SetGlobal_v(global_v).
		Set_v(dicV)

	result = run.ReplaceProcessedContent(BuildText, "<?n", "?>", func(text string) string {
		// 词条总数据
		lines := strings.Split(text, "\n")
		SplitText := t.Web(lines)
		DicHaderText := SplitText.Head

		dicRun.SetDic(SplitText).
			SetText(DicHaderText)
		RunDic := dicRun.Run()
		return RunDic
	})

	return result
}

func NewDic(text, trigger, path string) *Dic {
	str, err := utils.Decrypt(text, appfiles.Key)
	if err == nil {
		text = str
	}

	g := dto.NewVal()
	v := dto.NewVal()
	return &Dic{
		text:      text,
		trigger:   trigger,
		G_V:       g,
		V:         v,
		id:        0,
		Path:      path,
		FuncText:  nil,
		ClassText: nil,
	}
}

func (D *Dic) SetGlobal_v(v *dto.Val) *Dic {
	D.G_V = v
	return D
}

func (D *Dic) Set_v(v *dto.Val) *Dic {
	D.V = v
	return D
}

// 运行词库(全局变量,词库文本,触发)
func (D *Dic) Run() string {

	// 返回数据
	var result string

	// 触发文本
	trigger := D.trigger

	// 全局变量
	global_v := D.G_V

	// 局部变量
	dicV := D.V

	// 词库头部数据
	var DicHaderText []string

	// 词库数据
	var DicText []*dto.BuildDic

	// 执行返回数据
	var RunDic string

	// 词库文本
	text := D.text

	var BuildText string

	t := &run.Build{
		G_v:  global_v,
		V:    dicV,
		Path: D.Path,
	}

	// 使用导入函数
	BuildText, err := t.ImportText(text)
	if err != nil {
		BuildText = text
	}

	// 使用导入函数
	// BuildTexts, err := t.Import(BuildText)
	// if err == nil {
	// 	BuildText = BuildTexts
	// }
	var SplitText *dto.BuildValue

	if D.Cache {
		t.Cache = true
		t.Uid = D.CacheName
		if read, ok := dto.GV.Get("cache_" + t.Uid).(*dto.BuildValue); ok {
			SplitText = read
		} else {
			SplitText = t.SplitText(BuildText)
		}
	} else {
		SplitText = t.SplitText(BuildText)
	}

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
	dicV.Set("触发词", trigger)
	dicV.Set("触发", GetDicTrigger)

	dicRun := NewRunDicEntry(D.Path).
		SetGlobal_v(global_v).
		Set_v(dicV).
		SetDic(SplitText)

	dicRun.SetText(DicHaderText)
	RunDichader := dicRun.Run()

	if !dicRun.Sys_v.Stop {
		dicRun.SetText(GetDic)
		RunDic = dicRun.Run()
	}

	result = RunDichader + RunDic

	return result
}
