package funcs

import (
	stdjson "encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"html/template"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
	"github.com/gomarkdown/markdown"
	"golang.org/x/net/html"
)

// HTMLNode represents an HTML node in JSON format.
type HTMLNode struct {
	Type       string           `json:"类型"`
	Data       string           `json:"数据"`
	Text       string           `json:"文本,omitempty"`
	Attributes []html.Attribute `json:"属性,omitempty"`
	Children   []HTMLNode       `json:"列表,omitempty"`
}

// ConvertToJSON converts an HTML node to a JSON-friendly structure.
func ConvertToJSON(n *html.Node) HTMLNode {
	node := HTMLNode{
		Type: nodeType(n),
		Data: n.Data,
	}

	// Add attributes for element nodes and sort them.
	if n.Type == html.ElementNode {
		node.Attributes = n.Attr
		sort.Slice(node.Attributes, func(i, j int) bool {
			return node.Attributes[i].Key < node.Attributes[j].Key
		})
	}

	// Recursively process child nodes, preserving document order.
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		node.Children = append(node.Children, ConvertToJSON(c))
	}

	// 拼接内部文本
	node.Text = getInnerText(n)

	return node
}

// getInnerText 递归获取节点内部所有文本内容。
func getInnerText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var s strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		s.WriteString(getInnerText(c))
	}
	return s.String()
}

// nodeType returns a string representation of the node type.
func nodeType(n *html.Node) string {
	switch n.Type {
	case html.ElementNode:
		return "元素"
	case html.TextNode:
		return "文本"
	case html.CommentNode:
		return "注释"
	case html.DoctypeNode:
		return "HTML"
	default:
		return "未知"
	}
}

// FindNodeByPath searches for a node by following the provided path in the HTML tree.
func FindNodeByPath(n *html.Node, path []string) *html.Node {
	if len(path) == 0 {
		return n
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == path[0] {
			return FindNodeByPath(c, path[1:])
		}
	}

	return nil
}

// findAllNodesByPath finds all matching nodes at the given path.
func findAllNodesByPath(n *html.Node, path []string) []*html.Node {
	if len(path) == 0 {
		return []*html.Node{n}
	}

	var result []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == path[0] {
			result = append(result, findAllNodesByPath(c, path[1:])...)
		}
	}

	return result
}

// HtmlParse parses HTML and converts it to JSON based on the given path.
func (f *DicFunc) HtmlParse() (string, error) {
	if f.Len < 1 {
		return "", errors.New("参数数量错误")
	}

	doc, err := html.Parse(strings.NewReader(f.Inputs.String(1)))
	if err != nil {
		return "", fmt.Errorf("解析HTML时出错: %v", err)
	}

	if f.Len > 1 {
		var path = make([]string, 0, len(f.Inputs.List[2:]))
		for _, p := range f.Inputs.List[2:] {
			if strP, ok := p.(string); ok {
				path = append(path, strP)
			}
		}
		return parsePathAndQuery(doc, path)
	}

	jsonD, err := json.MarshalIndent(ConvertToJSON(doc), "", "  ")
	if err != nil {
		return "", fmt.Errorf("转换JSON时出错: %v", err)
	}
	return string(jsonD), nil
}

func markdownToHtml(d *dto.DicInputs) (any, error) {
	res := string(
		markdown.ToHTML(
			[]byte(d.Inputs.String(1)),
			nil,
			nil,
		))
	return res, nil
}

func (f *DicFunc) HtmlEncode() (string, error) {
	if f.Len == 1 {
		return html.EscapeString(f.Inputs.String(1)), nil
	}
	return "", errors.New("参数数量错误")
}

func (f *DicFunc) HtmlDecode() (string, error) {
	if f.Len == 1 {
		return html.UnescapeString(f.Inputs.String(1)), nil
	}
	return "", errors.New("参数数量错误")
}

func htmlParse(d *dto.DicInputs) (any, error) {
	doc, err := html.Parse(strings.NewReader(d.Inputs.String(1)))
	if err != nil {
		return "", fmt.Errorf("解析HTML时出错: %v", err)
	}

	if d.Inputs.Len() > 1 {
		var path []string
		for _, p := range d.Inputs.List[2:] {
			if strP, ok := p.(string); ok {
				path = append(path, strP)
			}
		}
		return parsePathAndQuery(doc, path)
	}

	jsonD, err := stdjson.MarshalIndent(ConvertToJSON(doc), "", "  ")
	if err != nil {
		return "", fmt.Errorf("转换JSON时出错: %v", err)
	}
	return string(jsonD), nil
}

// parsePathAndQuery 解析路径：叶子节点返回文本数组，容器节点返回 JSON 节点数组，末尾数字作为索引。
func parsePathAndQuery(doc *html.Node, path []string) (string, error) {
	if n := len(path); n > 0 {
		if idx, err := strconv.Atoi(path[n-1]); err == nil {
			nodes := findAllNodesByPath(doc, path[:n-1])
			if idx < 0 || idx >= len(nodes) {
				return "", nil
			}
			return marshalResult(nodes[idx])
		}
	}
	nodes := findAllNodesByPath(doc, path)
	if len(nodes) == 0 {
		return "[]", nil
	}
	if isTextOnly(nodes[0]) {
		// 叶子节点 → 文本数组
		texts := make([]string, len(nodes))
		for i, n := range nodes {
			texts[i] = getInnerText(n)
		}
		jsonD, err := stdjson.Marshal(texts)
		if err != nil {
			return "", err
		}
		return string(jsonD), nil
	}
	// 容器节点 → JSON 节点数组
	nodeList := make([]HTMLNode, len(nodes))
	for i, n := range nodes {
		nodeList[i] = ConvertToJSON(n)
	}
	jsonD, err := stdjson.MarshalIndent(nodeList, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonD), nil
}

// marshalResult 纯文本节点返回文本，含子元素的容器节点返回 JSON。
func marshalResult(n *html.Node) (string, error) {
	if isTextOnly(n) {
		return getInnerText(n), nil
	}
	jsonD, err := stdjson.MarshalIndent(ConvertToJSON(n), "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonD), nil
}

// isTextOnly 判断节点是否只包含文本（无子元素）。
func isTextOnly(n *html.Node) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			return false
		}
	}
	return true
}

func htmlEncode(d *dto.DicInputs) (any, error) {
	return template.HTMLEscapeString(d.Inputs.String(1)), nil
}

func htmlDecode(d *dto.DicInputs) (any, error) {
	return html.UnescapeString(d.Inputs.String(1)), nil
}

func htmlText(d *dto.DicInputs) (any, error) {
	doc, err := html.Parse(strings.NewReader(d.Inputs.String(1)))
	if err != nil {
		return "", fmt.Errorf("解析HTML时出错: %v", err)
	}

	if d.Inputs.Len() > 1 {
		var path []string
		for _, p := range d.Inputs.List[2:] {
			if strP, ok := p.(string); ok {
				path = append(path, strP)
			}
		}
		nodes := findAllNodesByPath(doc, path)
		if len(nodes) == 0 {
			return "", nil
		}
		return getInnerText(nodes[0]), nil
	}

	return getInnerText(doc), nil
}

// ========== HTML 组件（面向对象） ==========

// NHtmlNode HTML 组件节点。Tag 为空表示一段原始 HTML 片段（不加包裹标签）。
type NHtmlNode struct {
	Tag      string
	Attrs    []html.Attribute
	Text     string
	Raw      bool
	Children []*NHtmlNode
	Parent   *NHtmlNode // 父节点，「创建布局」句柄靠它定位自己所在的分支

	layoutLevel int // 「创建布局」建立的容器的层级，用于自动闭合
}

// NHtml HTML 组件构建器，$创建HTML$ 返回该对象，各「设置*」方法往当前容器追加节点，
// $对象.获取$ 输出拼装好的 HTML 字符串。
// 对象方法可能被异步块（#:）所在 goroutine 与主流程同时调用，用 mu 保护内部状态。
type NHtml struct {
	mu        sync.Mutex       // 保护以下字段（含 stack / body 树）
	lang      string           // <html lang>
	title     string           // <title>
	metas     []*NHtmlNode     // <head> 内附加节点（meta / link）
	styles    []string         // <head> 内用户附加的 <style> 内容（排在默认样式之后）
	scripts   []string         // </body> 前 <script> 内容
	body      []*NHtmlNode     // <body> 顶层节点
	stack     []*NHtmlNode     // 当前打开的布局容器栈，栈空时追加到 body
	bodyAttrs []html.Attribute // <body> 自身的属性（不在布局中时由「设置属性」写入）
}

// 自闭合（空）元素，渲染时不输出结束标签。
var htmlVoidTags = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

// 布局层级：0 整屏骨架 → 1 容器 → 2 区块 → 3 排列容器 → 4 组件。
// 「创建布局」时自动闭合同层或更深的布局，所以不需要「结束布局」。
// 层级差要大一点：网格 / 表单 / 行 里还要放卡片，卡片就得比它们更深。
const htmlLayoutMaxLevel = 4

// 布局名称 -> 标签、默认类名与层级；未命中的名称退化为 <div class="名称">，按最深的组件层处理。
var htmlLayouts = map[string]struct {
	Tag   string
	Class string
	Level int
}{
	"页面":   {"div", "nebula-page", 0},
	"全屏":   {"div", "nebula-fullscreen", 0},
	"全屏颜色": {"div", "nebula-fullscreen", 0},
	"全屏居中": {"div", "nebula-center", 0},
	"屏幕居中": {"div", "nebula-center", 0},
	"居中布局": {"div", "nebula-center", 0},
	"居中卡片": {"div", "nebula-center-card", 0},
	"容器":   {"div", "nebula-container", 1},
	"居中":   {"div", "nebula-container", 1},
	"顶部":   {"header", "nebula-header", 2},
	"页头":   {"header", "nebula-header", 2},
	"底部":   {"footer", "nebula-footer", 2},
	"页脚":   {"footer", "nebula-footer", 2},
	"主体":   {"main", "nebula-main", 2},
	"内容":   {"main", "nebula-main", 2},
	"导航":   {"nav", "nebula-nav", 2},
	"导航栏":  {"nav", "nebula-nav", 2},
	"侧栏":   {"aside", "nebula-aside", 2},
	"侧边栏":  {"aside", "nebula-aside", 2},
	"分区":   {"section", "nebula-section", 2},
	"区域":   {"section", "nebula-section", 2},
	"横向":   {"div", "nebula-row", 3},
	"行":    {"div", "nebula-row", 3},
	"纵向":   {"div", "nebula-col", 3},
	"列":    {"div", "nebula-col", 3},
	"网格":   {"div", "nebula-grid", 3},
	"表单":   {"form", "nebula-form", 3},
	"卡片":   {"div", "nebula-card", 4},
}

// 按钮样式名称 -> 附加类名。
var htmlBtnStyles = map[string]string{
	"主要": "nebula-btn-primary", "primary": "nebula-btn-primary",
	"次要": "nebula-btn-secondary", "secondary": "nebula-btn-secondary",
	"成功": "nebula-btn-success", "success": "nebula-btn-success",
	"警告": "nebula-btn-warning", "warning": "nebula-btn-warning",
	"危险": "nebula-btn-danger", "danger": "nebula-btn-danger",
	"幽灵": "nebula-btn-ghost", "ghost": "nebula-btn-ghost",
}

// 语义样式名 -> 内置类名，供各「设置*」方法最后那个「附加样式」参数使用。
// 写中文样式名就能套上内置模板，不用自己写 CSS；表里没有的值原样当作类名，兼容自定义 class。
var htmlStyles = map[string]string{
	"品牌": "nebula-brand", "brand": "nebula-brand",
	"主视觉": "nebula-hero", "首屏": "nebula-hero", "hero": "nebula-hero",
	"徽标": "nebula-badge", "标签": "nebula-badge", "badge": "nebula-badge", "tag": "nebula-badge",
	"引导": "nebula-lead", "引言": "nebula-lead", "lead": "nebula-lead",
	"图标": "nebula-icon", "icon": "nebula-icon",
	"弱化": "nebula-muted", "次要文字": "nebula-muted", "muted": "nebula-muted",
	"悬停": "nebula-hover", "悬浮": "nebula-hover", "hover": "nebula-hover",
	"按钮组": "nebula-actions", "操作": "nebula-actions", "actions": "nebula-actions",
}

// htmlStyleClass 把语义样式名解析成内置类名；未命中时原样返回，可以直接写自定义类名。
func htmlStyleClass(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if cls, ok := htmlStyles[name]; ok {
		return cls
	}
	return name
}

// htmlDefaultStyle 内置默认样式，每次渲染都自动插入 <head>，不需要手动启用。
// 一套预设设计系统：CSS 变量管颜色，.nebula-theme-dark 切换深色，其余是布局与组件的类名。
const htmlDefaultStyle = `:root{--nebula-primary:#4f46e5;--nebula-primary-dark:#4338ca;--nebula-bg:#f5f6fa;--nebula-surface:#ffffff;--nebula-text:#1f2937;--nebula-muted:#6b7280;--nebula-border:#e5e7eb;--nebula-radius:12px;--nebula-shadow:0 1px 2px rgba(16,24,40,.05),0 10px 30px -18px rgba(16,24,40,.35);--nebula-ring:0 0 0 3px rgba(79,70,229,.18);--nebula-hero-from:#4f46e5;--nebula-hero-to:#7c3aed;--nebula-badge-bg:rgba(79,70,229,.12);--nebula-badge-text:var(--nebula-primary)}
.nebula-theme-dark{--nebula-primary:#818cf8;--nebula-primary-dark:#6366f1;--nebula-bg:#0f172a;--nebula-surface:#1e293b;--nebula-text:#e2e8f0;--nebula-muted:#94a3b8;--nebula-border:#334155;--nebula-shadow:0 1px 2px rgba(0,0,0,.4),0 12px 32px -20px rgba(0,0,0,.8);--nebula-ring:0 0 0 3px rgba(129,140,248,.28);--nebula-hero-from:#4338ca;--nebula-hero-to:#6d28d9;--nebula-badge-bg:rgba(129,140,248,.16)}
.nebula-theme-light{--nebula-primary:#4f46e5;--nebula-primary-dark:#4338ca;--nebula-bg:#f5f6fa;--nebula-surface:#ffffff;--nebula-text:#1f2937;--nebula-muted:#6b7280;--nebula-border:#e5e7eb}
*{box-sizing:border-box}
html,body{margin:0;padding:0}
body{min-height:100vh;background:var(--nebula-bg);color:var(--nebula-text);font-family:system-ui,-apple-system,"Segoe UI","Microsoft YaHei",sans-serif;font-size:15px;line-height:1.7;-webkit-font-smoothing:antialiased}
h1,h2,h3,h4,h5,h6{margin:0 0 12px;font-weight:700;line-height:1.3;letter-spacing:-.01em}
h1{font-size:32px}h2{font-size:26px}h3{font-size:21px}h4{font-size:18px}h5{font-size:16px}h6{font-size:14px;color:var(--nebula-muted)}
p{margin:0 0 12px}
a{color:var(--nebula-primary);text-decoration:none;transition:color .15s ease}
a:hover{text-decoration:underline}
small{color:var(--nebula-muted)}
ul,ol{margin:0 0 12px;padding-left:22px}
li{margin:4px 0}
hr{margin:20px 0;border:0;border-top:1px solid var(--nebula-border)}
img{max-width:100%;height:auto;border-radius:10px}
input,select,textarea{width:100%;padding:10px 12px;border:1px solid var(--nebula-border);border-radius:10px;background:var(--nebula-surface);color:var(--nebula-text);font:inherit;font-size:14px;transition:border-color .15s ease,box-shadow .15s ease}
input:focus,select:focus,textarea:focus{outline:none;border-color:var(--nebula-primary);box-shadow:var(--nebula-ring)}
.nebula-page{min-height:100vh;display:flex;flex-direction:column}
.nebula-header{flex:0 0 auto;padding:16px 0;border-bottom:1px solid var(--nebula-border)}
.nebula-footer{flex:0 0 auto;padding:20px 0;border-top:1px solid var(--nebula-border);color:var(--nebula-muted);font-size:13px}
.nebula-main{flex:1 1 auto;width:100%}
.nebula-container{width:100%;max-width:1080px;margin:0 auto;padding:32px 20px}
.nebula-section{margin:20px 0}
.nebula-nav{display:flex;flex-wrap:wrap;gap:8px 18px;align-items:center}
.nebula-nav a{color:inherit;font-weight:600;font-size:14px}
.nebula-nav a:hover{color:var(--nebula-primary);text-decoration:none}
.nebula-aside{flex:0 0 auto;width:240px}
.nebula-row{display:flex;flex-wrap:wrap;gap:12px;align-items:center}
.nebula-col{display:flex;flex-direction:column;gap:12px}
.nebula-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(240px,1fr));gap:16px}
.nebula-card{padding:20px;border:1px solid var(--nebula-border);border-radius:var(--nebula-radius);background:var(--nebula-surface);box-shadow:var(--nebula-shadow)}
.nebula-form{display:flex;flex-direction:column;gap:14px;max-width:420px}
.nebula-fullscreen{width:100%;min-height:100vh;display:flex;flex-direction:column;padding:48px 20px}
.nebula-center{width:100%;min-height:100vh;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:16px;padding:48px 20px;text-align:center}
.nebula-center-card{width:100%;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:32px 20px}
.nebula-center-card>*{width:100%;max-width:420px;padding:32px;border:1px solid var(--nebula-border);border-radius:var(--nebula-radius);background:var(--nebula-surface);box-shadow:var(--nebula-shadow)}
.nebula-btn{display:inline-flex;align-items:center;justify-content:center;gap:6px;padding:9px 18px;border:1px solid var(--nebula-border);border-radius:10px;background:var(--nebula-surface);color:var(--nebula-text);font-size:14px;font-weight:600;line-height:1.4;text-decoration:none;cursor:pointer;transition:transform .15s ease,box-shadow .15s ease,background .15s ease,border-color .15s ease}
.nebula-btn:hover{transform:translateY(-1px);box-shadow:var(--nebula-shadow);text-decoration:none}
.nebula-btn:active{transform:translateY(0)}
.nebula-btn:focus-visible{outline:none;box-shadow:var(--nebula-ring)}
.nebula-btn-primary{background:var(--nebula-primary);border-color:var(--nebula-primary);color:#fff}
.nebula-btn-primary:hover{background:var(--nebula-primary-dark);border-color:var(--nebula-primary-dark)}
.nebula-btn-secondary{background:var(--nebula-muted);border-color:var(--nebula-muted);color:#fff}
.nebula-btn-success{background:#10b981;border-color:#10b981;color:#fff}
.nebula-btn-warning{background:#f59e0b;border-color:#f59e0b;color:#fff}
.nebula-btn-danger{background:#ef4444;border-color:#ef4444;color:#fff}
.nebula-btn-ghost{background:transparent;border-color:var(--nebula-border);color:inherit}
.nebula-brand{font-size:17px;font-weight:800;letter-spacing:-.02em;margin:0}
.nebula-hero{background:linear-gradient(135deg,var(--nebula-hero-from),var(--nebula-hero-to));border:0;border-radius:18px;padding:44px 36px;box-shadow:var(--nebula-shadow);color:#fff}
.nebula-hero h1,.nebula-hero h2,.nebula-hero h3,.nebula-hero h4,.nebula-hero h5,.nebula-hero h6{color:#fff}
.nebula-hero h1{font-size:34px;margin:0 0 12px}
.nebula-hero .nebula-badge{background:rgba(255,255,255,.18);color:#fff}
.nebula-hero .nebula-lead{color:rgba(255,255,255,.88)}
.nebula-hero .nebula-btn-primary{background:#fff;border-color:#fff;color:var(--nebula-hero-from)}
.nebula-hero .nebula-btn-primary:hover{background:rgba(255,255,255,.88);border-color:rgba(255,255,255,.88)}
.nebula-hero .nebula-btn-ghost{border-color:rgba(255,255,255,.55);color:#fff}
.nebula-card>.nebula-btn{margin:6px 10px 0 0}
.nebula-badge{display:inline-block;padding:4px 12px;border-radius:999px;background:var(--nebula-badge-bg);color:var(--nebula-badge-text);font-size:13px;font-weight:600;margin:0 0 16px}
.nebula-lead{max-width:620px;font-size:17px;color:var(--nebula-muted);margin:0 0 18px}
.nebula-icon{font-size:24px;line-height:1;margin:0 0 8px}
.nebula-muted{color:var(--nebula-muted);font-size:14px;margin:0}
.nebula-hover{transition:transform .18s ease,box-shadow .18s ease}
.nebula-hover:hover{transform:translateY(-4px);box-shadow:var(--nebula-shadow)}
.nebula-actions{display:flex;flex-wrap:wrap;align-items:center;gap:10px;margin:6px 0 0}
@media (max-width:640px){.nebula-container{padding:20px 16px}.nebula-aside{width:100%}.nebula-row{flex-direction:column;align-items:stretch}.nebula-center,.nebula-fullscreen,.nebula-center-card{padding:32px 16px}.nebula-hero{padding:32px 20px}}
`

// 创建HTML（返回面向对象的对象），可选参数为页面标题。
func htmlNew(d *dto.DicInputs) (any, error) {
	h := &NHtml{lang: "zh-CN"}
	if title := strings.TrimSpace(d.Inputs.String(1)); title != "" {
		h.title = title
	}
	return newHtmlClass(h), nil
}

// newHtmlClass 将 HTML 构建器包装为面向对象的 Class，方法闭包捕获同一实例。
func newHtmlClass(h *NHtml) *dto.DicClass {
	return newHtmlClassAt(h, nil)
}

// newHtmlClassAt 在构建器的某个位置创建对象：node 为 nil 表示整页（沿用构建器自己的插入栈），
// 否则表示「创建布局」返回的布局分支句柄，调用它的方法时会先切回该分支再往里面追加。
func newHtmlClassAt(h *NHtml, node *NHtmlNode) *dto.DicClass {
	instance := &dto.DicClass{
		LocalValue: dto.NewVal().Set("_HTML_", h),
	}
	// method 生成绑定到当前位置的方法。
	method := func(fn func(*dto.DicInputs) (any, error), l string) dto.DicFunc {
		return wrapHtml(h, node, fn, l)
	}
	instance.Fn = map[string]dto.DicFunc{
		"设置页面标题": method(htmlSetPageTitle, "1"),
		"设置语言":   method(htmlSetLang, "1"),
		"设置样式":   method(htmlSetStyle, "1"),
		"设置背景":   method(htmlSetBackground, "1"),
		"设置元信息":  method(htmlSetMeta, "2"),
		"设置图标":   method(htmlSetIcon, "1"),
		"设置脚本":   method(htmlSetScript, "1"),
		"创建布局":   method(htmlCreateLayout, "1|2"),
		"设置属性":   method(htmlSetAttr, "2"),
		"设置标题":   method(htmlAddHeading, "1|2|3"),
		"设置文本":   method(htmlAddText, "1|2"),
		"设置按钮":   method(htmlAddButton, "1|2|3|4"),
		"设置链接":   method(htmlAddLink, "2|3"),
		"设置图片":   method(htmlAddImage, "1|2|3"),
		"设置输入框":  method(htmlAddInput, "1|2|3|4"),
		"设置列表":   method(htmlAddList, "1.."),
		"设置分割线":  method(htmlAddDivider, "0"),
		"设置原始":   method(htmlAddRaw, "1"),
		"获取":     method(htmlGet, "0|1"),
	}
	return instance
}

// wrapHtml 将依赖 HTML 构建器（第一参数）的自由函数包装为对象方法。
// node 非空时先把插入位置切回该布局分支，实现「在某个分支下继续创建」。
func wrapHtml(h *NHtml, node *NHtmlNode, fn func(*dto.DicInputs) (any, error), l string) dto.DicFunc {
	return dto.DicFunc{
		L: l,
		Fn: func(d *dto.DicInputs) (any, error) {
			h.mu.Lock()
			defer h.mu.Unlock()
			if node != nil {
				h.stack = h.chain(node)
			}
			inputs := utils.NewDicInputs()
			list := make([]any, 0, d.Inputs.Len()+2)
			list = append(list, "")
			list = append(list, h)
			for i := 1; i <= d.Inputs.Len(); i++ {
				list = append(list, d.Inputs.Get(i))
			}
			inputs.Set(list)
			return fn(dto.NewDicInputsWithOutput(d.Dic, d.V, &inputs, d.Output))
		},
	}
}

// getHtml 取出对象方法第一参数上的 HTML 构建器。
func getHtml(d *dto.DicInputs, idx int) (*NHtml, error) {
	h, ok := d.Inputs.Get(idx).(*NHtml)
	if !ok || h == nil {
		return nil, errors.New("参数必须是HTML组件对象")
	}
	return h, nil
}

// append 把节点追加到当前打开的布局容器；栈空时追加到 <body>。
func (h *NHtml) append(n *NHtmlNode) {
	if len(h.stack) == 0 {
		n.Parent = nil
		h.body = append(h.body, n)
		return
	}
	cur := h.stack[len(h.stack)-1]
	n.Parent = cur
	cur.Children = append(cur.Children, n)
}

// chain 返回从最外层布局到 node 的插入栈，供布局分支句柄切回自己所在的分支。
func (h *NHtml) chain(node *NHtmlNode) []*NHtmlNode {
	var reversed []*NHtmlNode
	for n := node; n != nil; n = n.Parent {
		reversed = append(reversed, n)
	}
	stack := make([]*NHtmlNode, 0, len(reversed))
	for i := len(reversed) - 1; i >= 0; i-- {
		stack = append(stack, reversed[i])
	}
	return stack
}

// current 返回当前插入位置的节点；栈空时返回 nil（表示 <body> 本身）。
func (h *NHtml) current() *NHtmlNode {
	if len(h.stack) == 0 {
		return nil
	}
	return h.stack[len(h.stack)-1]
}

func htmlSetPageTitle(d *dto.DicInputs) (any, error) {
	h, err := getHtml(d, 1)
	if err != nil {
		return nil, err
	}
	h.title = d.Inputs.String(2)
	return nil, nil
}

func htmlSetLang(d *dto.DicInputs) (any, error) {
	h, err := getHtml(d, 1)
	if err != nil {
		return nil, err
	}
	h.lang = strings.TrimSpace(d.Inputs.String(2))
	return nil, nil
}

func htmlSetStyle(d *dto.DicInputs) (any, error) {
	h, err := getHtml(d, 1)
	if err != nil {
		return nil, err
	}
	h.styles = append(h.styles, d.Inputs.String(2))
	return nil, nil
}

func htmlSetMeta(d *dto.DicInputs) (any, error) {
	h, err := getHtml(d, 1)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(d.Inputs.String(2))
	if name == "" {
		return nil, errors.New("元信息名称不能为空")
	}
	h.metas = append(h.metas, &NHtmlNode{
		Tag:   "meta",
		Attrs: []html.Attribute{{Key: "name", Val: name}, {Key: "content", Val: d.Inputs.String(3)}},
	})
	return nil, nil
}

func htmlSetIcon(d *dto.DicInputs) (any, error) {
	h, err := getHtml(d, 1)
	if err != nil {
		return nil, err
	}
	h.metas = append(h.metas, &NHtmlNode{
		Tag:   "link",
		Attrs: []html.Attribute{{Key: "rel", Val: "icon"}, {Key: "href", Val: d.Inputs.String(2)}},
	})
	return nil, nil
}

func htmlSetScript(d *dto.DicInputs) (any, error) {
	h, err := getHtml(d, 1)
	if err != nil {
		return nil, err
	}
	h.scripts = append(h.scripts, d.Inputs.String(2))
	return nil, nil
}

// htmlCreateLayout 新建一个布局容器并进入，后续「设置*」都追加到该容器内；
// 同时返回该容器的句柄（分支函数），可以赋予值后用 $句柄.创建布局 名称$ 在它下面继续开子布局。
// 布局按层级自动嵌套：开新布局时自动闭合同层或更深的布局，无需手动结束。
func htmlCreateLayout(d *dto.DicInputs) (any, error) {
	h, err := getHtml(d, 1)
	if err != nil {
		return nil, err
	}
	kind := strings.TrimSpace(d.Inputs.String(2))
	if kind == "" {
		return nil, errors.New("布局名称不能为空")
	}

	node := &NHtmlNode{Tag: "div", layoutLevel: htmlLayoutMaxLevel}
	cls := kind
	if l, ok := htmlLayouts[kind]; ok {
		node.Tag = l.Tag
		cls = l.Class
		node.layoutLevel = l.Level
	}

	id := ""
	if extra := strings.TrimSpace(d.Inputs.String(3)); extra != "" {
		if v, ok := strings.CutPrefix(extra, "#"); ok {
			id = v
		} else {
			cls += " " + htmlStyleClass(extra) // 附加样式并入同一个 class 属性
		}
	}
	node.Attrs = append(node.Attrs, html.Attribute{Key: "class", Val: cls})
	if id != "" {
		node.Attrs = append(node.Attrs, html.Attribute{Key: "id", Val: id})
	}

	// 自动闭合：栈顶是同层或更深的布局就先退出，新布局挂到剩下的层级里
	for len(h.stack) > 0 && h.stack[len(h.stack)-1].layoutLevel >= node.layoutLevel {
		h.stack = h.stack[:len(h.stack)-1]
	}
	h.append(node)
	h.stack = append(h.stack, node)
	return newHtmlClassAt(h, node), nil
}

// htmlSetAttr 给当前布局容器设置属性；不在布局中时设置到 <body>。
func htmlSetAttr(d *dto.DicInputs) (any, error) {
	h, err := getHtml(d, 1)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(d.Inputs.String(2))
	if name == "" {
		return nil, errors.New("属性名不能为空")
	}
	attr := html.Attribute{Key: name, Val: d.Inputs.String(3)}
	if cur := h.current(); cur != nil {
		cur.Attrs = append(cur.Attrs, attr)
		return nil, nil
	}
	// 不在布局中时，属性直接写到 <body> 上
	if h.bodyAttrs == nil {
		h.bodyAttrs = []html.Attribute{}
	}
	h.bodyAttrs = append(h.bodyAttrs, attr)
	return nil, nil
}

// htmlSetBackground 设置页面背景：写「深色」「浅色」切换预设主题，其它值作为 CSS 背景（颜色 / 渐变 / 图片）。
// 该设置始终作用于页面 <body>，与当前是否在布局中无关。
func htmlSetBackground(d *dto.DicInputs) (any, error) {
	h, err := getHtml(d, 1)
	if err != nil {
		return nil, err
	}
	val := strings.TrimSpace(d.Inputs.String(2))
	if val == "" {
		return nil, errors.New("背景不能为空")
	}
	switch val {
	case "深色", "dark":
		h.addBodyAttr("class", "nebula-theme-dark")
	case "浅色", "light":
		h.addBodyAttr("class", "nebula-theme-light")
	default:
		h.addBodyAttr("style", "background:"+val)
	}
	return nil, nil
}

// addBodyAttr 往 <body> 上追加属性；同名属性按空格（class）或分号（其它）合并。
func (h *NHtml) addBodyAttr(key, val string) {
	for i := range h.bodyAttrs {
		if h.bodyAttrs[i].Key != key {
			continue
		}
		old := strings.TrimSpace(h.bodyAttrs[i].Val)
		if old == "" {
			h.bodyAttrs[i].Val = val
			return
		}
		if key == "class" {
			if !strings.Contains(" "+old+" ", " "+val+" ") {
				h.bodyAttrs[i].Val = old + " " + val
			}
			return
		}
		h.bodyAttrs[i].Val = strings.TrimRight(old, ";") + ";" + val
		return
	}
	if h.bodyAttrs == nil {
		h.bodyAttrs = []html.Attribute{}
	}
	h.bodyAttrs = append(h.bodyAttrs, html.Attribute{Key: key, Val: val})
}

func htmlAddHeading(d *dto.DicInputs) (any, error) {
	h, err := getHtml(d, 1)
	if err != nil {
		return nil, err
	}
	text := d.Inputs.String(2)
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("标题内容不能为空")
	}
	level := 1
	if lv, ok := d.Inputs.IntOk(3); ok {
		level = lv
	}
	if level < 1 || level > 6 {
		return nil, errors.New("标题级别只能是 1~6")
	}
	node := &NHtmlNode{Tag: "h" + strconv.Itoa(level), Text: text}
	if cls := htmlStyleClass(d.Inputs.String(4)); cls != "" {
		node.Attrs = append(node.Attrs, html.Attribute{Key: "class", Val: cls})
	}
	h.append(node)
	return nil, nil
}

func htmlAddText(d *dto.DicInputs) (any, error) {
	h, err := getHtml(d, 1)
	if err != nil {
		return nil, err
	}
	node := &NHtmlNode{Tag: "p", Text: d.Inputs.String(2)}
	if cls := htmlStyleClass(d.Inputs.String(3)); cls != "" {
		node.Attrs = append(node.Attrs, html.Attribute{Key: "class", Val: cls})
	}
	h.append(node)
	return nil, nil
}

// htmlAddButton 生成按钮组件：链接为「-」或空时输出 <button>，否则输出 <a>。
func htmlAddButton(d *dto.DicInputs) (any, error) {
	h, err := getHtml(d, 1)
	if err != nil {
		return nil, err
	}
	text := d.Inputs.String(2)
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("按钮文本不能为空")
	}
	href := strings.TrimSpace(d.Inputs.String(3))

	classes := "nebula-btn"
	if style := strings.TrimSpace(d.Inputs.String(4)); style != "" {
		if sc, ok := htmlBtnStyles[style]; ok {
			classes += " " + sc
		} else {
			classes += " " + style
		}
	}
	if extra := htmlStyleClass(d.Inputs.String(5)); extra != "" {
		classes += " " + extra
	}

	node := &NHtmlNode{Tag: "a", Text: text, Attrs: []html.Attribute{{Key: "class", Val: classes}}}
	if href == "" || href == "-" {
		node.Tag = "button"
		node.Attrs = append(node.Attrs, html.Attribute{Key: "type", Val: "button"})
	} else {
		node.Attrs = append(node.Attrs, html.Attribute{Key: "href", Val: href})
	}
	h.append(node)
	return nil, nil
}

func htmlAddLink(d *dto.DicInputs) (any, error) {
	h, err := getHtml(d, 1)
	if err != nil {
		return nil, err
	}
	text := d.Inputs.String(2)
	href := strings.TrimSpace(d.Inputs.String(3))
	if strings.TrimSpace(text) == "" || href == "" {
		return nil, errors.New("链接文本与地址都不能为空")
	}
	node := &NHtmlNode{Tag: "a", Text: text, Attrs: []html.Attribute{{Key: "href", Val: href}}}
	if cls := htmlStyleClass(d.Inputs.String(4)); cls != "" {
		node.Attrs = append(node.Attrs, html.Attribute{Key: "class", Val: cls})
	}
	h.append(node)
	return nil, nil
}

func htmlAddImage(d *dto.DicInputs) (any, error) {
	h, err := getHtml(d, 1)
	if err != nil {
		return nil, err
	}
	src := strings.TrimSpace(d.Inputs.String(2))
	if src == "" {
		return nil, errors.New("图片地址不能为空")
	}
	node := &NHtmlNode{Tag: "img", Attrs: []html.Attribute{{Key: "src", Val: src}}}
	if alt := d.Inputs.String(3); alt != "" {
		node.Attrs = append(node.Attrs, html.Attribute{Key: "alt", Val: alt})
	}
	if cls := htmlStyleClass(d.Inputs.String(4)); cls != "" {
		node.Attrs = append(node.Attrs, html.Attribute{Key: "class", Val: cls})
	}
	h.append(node)
	return nil, nil
}

func htmlAddInput(d *dto.DicInputs) (any, error) {
	h, err := getHtml(d, 1)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(d.Inputs.String(2))
	if name == "" {
		return nil, errors.New("输入框名称不能为空")
	}
	typ := strings.TrimSpace(d.Inputs.String(4))
	if typ == "" {
		typ = "text"
	}
	node := &NHtmlNode{Tag: "input", Attrs: []html.Attribute{
		{Key: "type", Val: typ},
		{Key: "name", Val: name},
	}}
	if ph := d.Inputs.String(3); ph != "" {
		node.Attrs = append(node.Attrs, html.Attribute{Key: "placeholder", Val: ph})
	}
	if cls := htmlStyleClass(d.Inputs.String(5)); cls != "" {
		node.Attrs = append(node.Attrs, html.Attribute{Key: "class", Val: cls})
	}
	h.append(node)
	return nil, nil
}

func htmlAddList(d *dto.DicInputs) (any, error) {
	h, err := getHtml(d, 1)
	if err != nil {
		return nil, err
	}
	ul := &NHtmlNode{Tag: "ul"}
	for i := 2; i <= d.Inputs.Len(); i++ {
		ul.Children = append(ul.Children, &NHtmlNode{Tag: "li", Text: d.Inputs.String(i)})
	}
	h.append(ul)
	return nil, nil
}

func htmlAddDivider(d *dto.DicInputs) (any, error) {
	h, err := getHtml(d, 1)
	if err != nil {
		return nil, err
	}
	h.append(&NHtmlNode{Tag: "hr"})
	return nil, nil
}

// htmlAddRaw 插入一段原始 HTML，不做转义。
func htmlAddRaw(d *dto.DicInputs) (any, error) {
	h, err := getHtml(d, 1)
	if err != nil {
		return nil, err
	}
	h.append(&NHtmlNode{Raw: true, Text: d.Inputs.String(2)})
	return nil, nil
}

// htmlGet 输出拼装好的 HTML。参数可选：完整（默认）/ 片段 / 头部。
func htmlGet(d *dto.DicInputs) (any, error) {
	h, err := getHtml(d, 1)
	if err != nil {
		return nil, err
	}
	switch strings.TrimSpace(d.Inputs.String(2)) {
	case "片段", "主体":
		return h.renderBody(), nil
	case "头部":
		return strings.TrimRight(h.renderHead(), "\n"), nil
	default:
		return h.renderFull(), nil
	}
}

// renderBody 渲染 <body> 内的顶层节点。
func (h *NHtml) renderBody() string {
	var sb strings.Builder
	for i, n := range h.body {
		if i > 0 {
			sb.WriteString("\n")
		}
		renderHtmlNode(&sb, n, "")
	}
	return sb.String()
}

// renderHead 渲染 <head> 内容：默认样式在最前，用户「设置样式」在其后，便于覆盖。
func (h *NHtml) renderHead() string {
	var sb strings.Builder
	sb.WriteString("  <meta charset=\"UTF-8\">\n")
	if h.title != "" {
		fmt.Fprintf(&sb, "  <title>%s</title>\n", html.EscapeString(h.title))
	}
	for _, m := range h.metas {
		renderHtmlNode(&sb, m, "  ")
		sb.WriteString("\n")
	}
	sb.WriteString("  <style>\n" + htmlDefaultStyle + "\n  </style>\n")
	for _, s := range h.styles {
		fmt.Fprintf(&sb, "  <style>\n%s\n  </style>\n", s)
	}
	return sb.String()
}

// renderFull 渲染完整 HTML 文档。
func (h *NHtml) renderFull() string {
	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html>\n")
	fmt.Fprintf(&sb, "<html lang=\"%s\">\n", html.EscapeString(h.lang))
	sb.WriteString("<head>\n")
	sb.WriteString(h.renderHead())
	sb.WriteString("</head>\n")

	sb.WriteString("<body")
	for _, a := range h.bodyAttrs {
		fmt.Fprintf(&sb, " %s=\"%s\"", a.Key, html.EscapeString(a.Val))
	}
	sb.WriteString(">\n")

	if body := h.renderBody(); body != "" {
		sb.WriteString(body)
		sb.WriteString("\n")
	}
	for _, s := range h.scripts {
		fmt.Fprintf(&sb, "  <script>\n%s\n  </script>\n", s)
	}
	sb.WriteString("</body>\n</html>")
	return sb.String()
}

// renderHtmlNode 递归渲染单个节点到 sb。
func renderHtmlNode(sb *strings.Builder, n *NHtmlNode, indent string) {
	if n == nil {
		return
	}
	// Tag 为空表示原始 HTML 片段，原样输出
	if n.Tag == "" {
		sb.WriteString(indent)
		sb.WriteString(n.Text)
		return
	}

	sb.WriteString(indent)
	fmt.Fprintf(sb, "<%s", n.Tag)
	for _, a := range n.Attrs {
		fmt.Fprintf(sb, " %s=\"%s\"", a.Key, html.EscapeString(a.Val))
	}
	sb.WriteString(">")

	if htmlVoidTags[n.Tag] {
		return
	}

	if n.Text != "" {
		if n.Raw {
			sb.WriteString(n.Text)
		} else {
			sb.WriteString(html.EscapeString(n.Text))
		}
	}
	if len(n.Children) > 0 {
		sb.WriteString("\n")
		for _, c := range n.Children {
			renderHtmlNode(sb, c, indent+"  ")
			sb.WriteString("\n")
		}
		sb.WriteString(indent)
	}
	sb.WriteString("</")
	fmt.Fprintf(sb, "%s>", n.Tag)
}
