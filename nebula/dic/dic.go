package dic

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"maps"
	"strings"

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
		log.Fatal(err)
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
		fmt.Println("模板解析失败:", err)
	}
	if err == nil {
		var buf bytes.Buffer
		if err := tpl.Execute(&buf, data); err != nil {
			fmt.Println("模板渲染失败:", err)
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

	if D.FuncText != nil {
		D.Data.LocalFunc = append(D.Data.LocalFunc, D.FuncText...)
	}

	if D.ClassText != nil {
		for key, val := range D.ClassText {
			D.Data.LocalClass[key] = val
		}
	}

	GetDic, GetDicTrigger, _, _ := run.RunFor(D.Data.LocalStatic, trigger, 0)
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

	if D.FuncText != nil {
		D.Data.LocalFunc = append(D.Data.LocalFunc, D.FuncText...)
	}

	if D.ClassText != nil {
		maps.Copy(D.Data.LocalClass, D.ClassText)
	}

	DicHaderText = D.Data.Head

	DicText = D.Data.Dic

	GetDic, GetDicTrigger, _, _ := run.RunFor(DicText, trigger, 0)
	D.Val.P.Set("触发词", trigger)
	D.Val.P.Set("触发", GetDicTrigger)

	dicRun := dic_dto.NewRunDicEntry().
		SetV(D.Val).
		SetDic(D.Data)
	dicRun.Dic.MyFunc = D.MyFunc

	RunDichader := m.DicRunLine(dicRun, DicHaderText)

	if !dicRun.Sys_v.Stop {
		RunDic = m.DicRunLine(dicRun, GetDic)
	}

	result = RunDichader + RunDic

	return result
}
