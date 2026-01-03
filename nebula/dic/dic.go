package dic

import (
	"maps"
	"strings"

	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"

	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.Config{
	EscapeHTML: false, // 禁用 HTML 转义
}.Froze()

// 运行网页词库
func (m *dicImpl) WebDicRun(WD *dic_dto.WebDic) string {

	// 返回数据
	var result string

	t := &run.Build{
		Val: WD.Val,
	}

	dicRun := dic_dto.NewRunDicEntry().
		SetV(WD.Val)

	result = run.ReplaceProcessedContent(WD.Text, "<?n", "?>", func(text string) string {
		// fmt.Println("词库文本:", text)
		// 词条总数据
		lines := strings.Split(text, "\n")
		SplitText := t.Web(lines)
		dicRun.SetDic(SplitText)
		dicRun.Dic.MyFunc = WD.MyFunc
		// fmt.Println("词库:", SplitText)
		RunDic := m.DicRunLine(dicRun, SplitText.Head)
		return RunDic
	})

	return result
}

// 运行内部
func (m *dicImpl) DicRunPrivate(D *dic_dto.Dic, trigger string) string {
	newV := dto.NewDicVal()
	newV.G = D.Val.G
	return m.DicRunPrivateVal(D, trigger, newV)
}

// 运行内部-自义定局部变量
func (m *dicImpl) DicRunPrivateVal(D *dic_dto.Dic, trigger string, v *dto.DicVal) string {

	t := &run.Build{
		Val:  v,
		Path: D.Path,
	}

	SplitText := t.SplitText(D.Text)

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

	dicRun := dic_dto.NewRunDicEntry().
		SetV(D.Val).
		SetDic(SplitText)
	dicRun.Dic.MyFunc = D.MyFunc

	return m.DicRunLine(dicRun, GetDic)

}

// 新建运行
func (m *dicImpl) NewDicRunLine(D *dic_dto.Dic, trigger string) string {
	D.Set_v(dto.NewVal())
	return m.DicRun(D, trigger)
}

// 运行词库(全局变量,词库文本,触发)
func (m *dicImpl) DicRun(D *dic_dto.Dic, trigger string) string {

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

	SplitText := t.SplitText(D.Text)
	// fmt.Println("词库文本:", SplitText)

	if D.FuncText != nil {
		SplitText.LocalFunc = append(SplitText.LocalFunc, D.FuncText...)
	}

	if D.ClassText != nil {
		maps.Copy(SplitText.LocalClass, D.ClassText)
	}

	DicHaderText = SplitText.Head

	DicText = SplitText.Dic

	GetDic, GetDicTrigger, _, _ := run.RunFor(DicText, trigger, 0)
	D.Val.P.Set("触发词", trigger)
	D.Val.P.Set("触发", GetDicTrigger)

	dicRun := dic_dto.NewRunDicEntry().
		SetV(D.Val).
		SetDic(SplitText)
	dicRun.Dic.MyFunc = D.MyFunc

	RunDichader := m.DicRunLine(dicRun, DicHaderText)

	if !dicRun.Sys_v.Stop {
		RunDic = m.DicRunLine(dicRun, GetDic)
	}

	result = RunDichader + RunDic

	return result
}
