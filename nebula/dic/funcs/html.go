package funcs

import (
	stdjson "encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"html/template"

	"github.com/cjxpj/nebula/dto"
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
	var s string
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		s += getInnerText(c)
	}
	return s
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
