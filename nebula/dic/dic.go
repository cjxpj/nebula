package dic

import (
	"bytes"
	"html/template"
	"maps"
	"strings"
	"time"

	"github.com/cjxpj/nebula/debugLog"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"
	"golang.org/x/net/html"

	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.Config{
	EscapeHTML: false, // 禁用 HTML 转义
}.Froze()

type scriptNebula struct {
	Id   string `json:"id"`
	Text string `json:"text"`
}

func isNebulaScript(n *html.Node) bool {
	if n.Type != html.ElementNode || n.Data != "script" {
		return false
	}

	for _, a := range n.Attr {
		if a.Key == "type" && a.Val == "nebula" {
			return true
		}
	}
	return false
}

func removeNebulaScripts(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling

		if isNebulaScript(c) {
			n.RemoveChild(c)
		} else {
			removeNebulaScripts(c)
		}

		c = next
	}
}

func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func extractText(n *html.Node) string {
	var s string
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			s += c.Data
		}
	}
	return s
}

// 在 html 或 body 直接子树中查找 <script type="nebula">
func findNebulaScripts(doc *html.Node) []scriptNebula {
	var result []scriptNebula

	// 找 html
	var htmlNode *html.Node
	for c := doc.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "html" {
			htmlNode = c
			break
		}
	}
	if htmlNode == nil {
		return result
	}

	// 扫描 head 和 body
	for c := htmlNode.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode {
			continue
		}

		if c.Data == "head" || c.Data == "body" {
			for cc := c.FirstChild; cc != nil; cc = cc.NextSibling {
				if cc.Type == html.ElementNode &&
					cc.Data == "script" &&
					isNebulaScript(cc) {

					result = append(result, scriptNebula{
						Id:   getAttr(cc, "id"),
						Text: strings.TrimSpace(extractText(cc)),
					})
				}
			}
		}
	}

	return result
}

func (m *dicImpl) WebPHPDicRun(WD *dic_dto.WebDic) string {

	// 返回数据
	var result string

	dicRun := dic_dto.NewRunDicEntry().
		SetV(WD.Val)

	result = run.ReplaceProcessedContent(WD.Text, "<?n", "?>", func(text string) string {
		// fmt.Println("词库文本:", text)
		// 词条总数据
		lines := strings.Split(text, "\n")
		SplitText := run.Web(WD.Path, lines)
		dicRun.SetDic(SplitText)
		dicRun.Dic.MyFunc = WD.MyFunc
		maps.Copy(dicRun.Dic.MyFunc, SplitText.MyFunc)
		// fmt.Println("词库:", SplitText)
		RunDic := m.DicRunLine(dicRun, SplitText.Head)
		return RunDic
	})

	return result
}

// 运行网页词库
func (m *dicImpl) WebDicRun(WD *dic_dto.WebDic) string {

	// 返回数据
	var result string

	// t := &run.Build{
	// 	Val: WD.Val,
	// }

	dicRun := dic_dto.NewRunDicEntry().
		SetV(WD.Val)

	// 解析成节点树
	doc, err := html.Parse(strings.NewReader(WD.Text))
	if err != nil {
		debugLog.Error(err)
	}

	data := make(map[string]any)

	// 1. 执行 nebula script，收集数据
	for _, s := range findNebulaScripts(doc) {
		lines := strings.Split(s.Text, "\n")
		res := m.NewDicRunLine(dicRun, lines)
		if s.Id != "" {
			data[s.Id] = res
		} else {
			maps.Copy(data, dicRun.Val.P.GetAll())
		}
	}

	// 2. 只移除 type="nebula" 的 script
	removeNebulaScripts(doc)

	var htmlBuf bytes.Buffer
	html.Render(&htmlBuf, doc)

	// 2. 使用 Go 模板引擎渲染
	tpl, err := template.New("page").Parse(htmlBuf.String())
	if err != nil {
		debugLog.Infof("模板解析失败: %v", err)
	}
	if err == nil {
		var buf bytes.Buffer
		if err := tpl.Execute(&buf, data); err != nil {
			debugLog.Infof("模板渲染失败: %v", err)
		}
		// 3. 模板渲染结果
		result = buf.String()
	}

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

	D.Data.MergeFuncs(D.FuncText)

	if D.ClassText != nil {
		maps.Copy(D.Data.LocalClass, D.ClassText)
	}

	GetDic, GetDicTrigger, _, _ := run.RunFor(D.Data.DicFuncs["内部"], trigger, 0)
	D.Val.P.Set("触发词", trigger)
	D.Val.P.Set("触发", GetDicTrigger)

	dicRun := dic_dto.NewRunDicEntry().
		SetV(D.Val).
		SetDic(D.Data)
	dicRun.Dic.MyFunc = D.MyFunc

	return m.DicRunLine(dicRun, GetDic)

}

// 运行特殊触发
func (m *dicImpl) DicRunEvent(D *dic_dto.Dic, event string, trigger string) string {
	newV := dto.NewDicVal()
	newV.G = D.Val.G
	return m.DicRunEventVal(D, event, trigger, newV)
}

// 运行特殊触发-自义定局部变量
func (m *dicImpl) DicRunEventVal(D *dic_dto.Dic, event string, trigger string, v *dto.DicVal) string {

	D.Data.MergeFuncs(D.FuncText)

	if D.ClassText != nil {
		maps.Copy(D.Data.LocalClass, D.ClassText)
	}

	var (
		GetDic        []string
		GetDicTrigger string
	)
	GetDic, GetDicTrigger, _, _ = run.RunFor(D.Data.DicFuncs[event], trigger, 0)
	D.Val.P.Set("触发词", trigger)
	D.Val.P.Set("触发", GetDicTrigger)

	dicRun := dic_dto.NewRunDicEntry().
		SetV(D.Val).
		SetDic(D.Data)
	dicRun.Dic.MyFunc = D.MyFunc

	return m.DicRunLine(dicRun, GetDic)

}

// 新建运行
func (m *dicImpl) NewDicRunLine(D *dic_dto.DicEntry, txt []string) string {
	D.Set_v(dto.NewVal())
	return m.DicRunLine(D, txt)
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

	// fmt.Println("词库文本:", SplitText)

	D.Data.MergeFuncs(D.FuncText)

	if D.ClassText != nil {
		maps.Copy(D.Data.LocalClass, D.ClassText)
	}

	DicHaderText = D.Data.Head

	DicText = D.Data.Dic

	GetDic, GetDicTrigger, triggerIdx, _ := run.RunFor(DicText, trigger, 0)
	D.Val.P.Set("触发词", trigger)
	D.Val.P.Set("触发", GetDicTrigger)

	dicRun := dic_dto.NewRunDicEntry().
		SetV(D.Val).
		SetDic(D.Data)
	dicRun.Dic.MyFunc = D.MyFunc

	// 设置头部行号映射
	dicRun.LineNums = D.Data.HeadLineNums
	RunDichader := m.DicRunLine(dicRun, DicHaderText)

	if !dicRun.Sys_v.Stop.Load() {
		// 设置 body 行号映射（仅当触发器匹配时）
		if GetDic != nil && triggerIdx < len(DicText) {
			dicRun.LineNums = DicText[triggerIdx].LineNums
		}
		RunDic = m.DicRunLine(dicRun, GetDic)
	}

	result = RunDichader + RunDic

	dicRun.Close()

	return result
}

// 运行词库（带超时）：超过 timeout 后置停止标志强行打断执行，返回当前已产出结果
func (m *dicImpl) DicRunTimeout(D *dic_dto.Dic, trigger string, timeout time.Duration) (result string, timedOut bool) {
	// 无超时限制：直接复用 DicRun，避免不必要的 goroutine
	if timeout <= 0 {
		return m.DicRun(D, trigger), false
	}

	D.Data.MergeFuncs(D.FuncText)

	if D.ClassText != nil {
		maps.Copy(D.Data.LocalClass, D.ClassText)
	}

	GetDic, GetDicTrigger, triggerIdx, _ := run.RunFor(D.Data.Dic, trigger, 0)
	D.Val.P.Set("触发词", trigger)
	D.Val.P.Set("触发", GetDicTrigger)

	dicRun := dic_dto.NewRunDicEntry().
		SetV(D.Val).
		SetDic(D.Data)
	dicRun.Dic.MyFunc = D.MyFunc

	// 预存 body 行号映射，供 goroutine 内使用（仅当触发器匹配时）
	var bodyLineNums []int
	if GetDic != nil && triggerIdx < len(D.Data.Dic) {
		bodyLineNums = D.Data.Dic[triggerIdx].LineNums
	}

	type runResult struct {
		text string
	}
	done := make(chan runResult, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				debugLog.Errorf("DicRunTimeout panic: %v", r)
				done <- runResult{}
			}
		}()
		dicRun.LineNums = D.Data.HeadLineNums
		RunDichader := m.DicRunLine(dicRun, D.Data.Head)
		text := RunDichader
		if !dicRun.Sys_v.Stop.Load() {
			dicRun.LineNums = bodyLineNums
			text += m.DicRunLine(dicRun, GetDic)
		}
		done <- runResult{text}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case r := <-done:
		dicRun.Close()
		return r.text, false
	case <-timer.C:
		// 超时：置停止标志，引擎会在行间检查时尽快退出
		dicRun.Sys_v.Stop.Store(true)
		// 给引擎短暂宽限，尽量在返回前安全退出，避免并发访问已释放的变量
		select {
		case r := <-done:
			dicRun.Close()
			return r.text, true
		case <-time.After(3 * time.Second):
			// 强制终止：goroutine 可能仍持有 dicRun，无法安全调用 Close()
			// 但必须清理以避免资源泄漏 —— 3 秒后引擎大概率已退出行间循环
			dicRun.Close()
			return "", true
		}
	}
}
