package funcs

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cjxpj/nebula/dto"

	"sort"

	"github.com/gomarkdown/markdown"
	"golang.org/x/net/html"
)

// HTMLNode represents an HTML node in JSON format.
type HTMLNode struct {
	Type       string           `json:"类型"`
	Data       string           `json:"数据"`
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

	// Recursively process child nodes and sort them.
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		node.Children = append(node.Children, ConvertToJSON(c))
	}

	// Optionally, sort children nodes by their type or data
	sort.Slice(node.Children, func(i, j int) bool {
		return node.Children[i].Data < node.Children[j].Data
	})

	return node
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
		var path []string
		for _, p := range f.Inputs.List[2:] {
			if strP, ok := p.(string); ok {
				path = append(path, strP)
			}
		}
		doc = FindNodeByPath(doc, path)

		if doc == nil {
			return "{}", nil // Return an empty JSON object instead of an empty string.
		}
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
