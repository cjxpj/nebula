package dic_server

import (
	"archive/zip"
	"bytes"
	"fmt"
	"html"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	mdhtml "github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

// ============== 内置说明文档（appfiles/static/docs 下的 md） ==============
//
// 过去全部文档挤在单篇 static/dic.md 里：AI 一次读全文挤占上下文，文档页也只能整篇渲染。
// 现按章节拆成多篇 md 随程序分发（embed 资源，不落盘）：
// 文档页按左侧目录树选篇阅读，AI 用 read_dic_doc 指定 doc 只读需要的那篇。

const (
	// dicDocDir 文档资源目录（相对 embed 资源根）
	dicDocDir = "docs"
	// dicDocTopGroup 直接放在 docs/ 下的文档在目录树中的分组名
	dicDocTopGroup = "平台功能"
)

// dicDocEntry 一篇内置文档：资源路径 + 标题 + 目录树分组。
type dicDocEntry struct {
	// Path 资源路径（相对 embed 资源根，如 docs/1-内置函数/04-文件操作.md）
	Path string `json:"path"`
	// Title 文档标题（取自正文首个一级标题）
	Title string `json:"title"`
	// Group 目录树分组（由所在目录名去序号前缀得到）
	Group string `json:"group"`
	// Chars 正文字符数
	Chars int `json:"chars"`

	content string // 正文（进程内缓存，不对外序列化）
}

var (
	dicDocsOnce sync.Once
	dicDocs     []dicDocEntry
)

// dicDocList 返回全部内置文档（按资源路径字典序，进程内只扫描解析一次）。
func dicDocList() []dicDocEntry {
	dicDocsOnce.Do(func() {
		files, err := appfiles.ListFiles(dicDocDir)
		if err != nil {
			return
		}
		for _, f := range files {
			if !strings.HasSuffix(strings.ToLower(f), ".md") {
				continue
			}
			data, err := appfiles.GetFile(f)
			if err != nil {
				continue
			}
			text := strings.ReplaceAll(string(data), "\r\n", "\n")
			title := dicDocFirstHeading(text)
			if title == "" {
				title = dicDocNameFromPath(f)
			}
			dicDocs = append(dicDocs, dicDocEntry{
				Path:    f,
				Title:   title,
				Group:   dicDocGroup(f),
				Chars:   len([]rune(text)),
				content: text,
			})
		}
	})
	return dicDocs
}

// dicDocFirstHeading 取正文首个非空行的一级标题（非一级标题则视为无标题）。
func dicDocFirstHeading(text string) string {
	for _, ln := range strings.Split(text, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		if strings.HasPrefix(ln, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(ln, "# "))
		}
		return ""
	}
	return ""
}

// dicDocNameFromPath 由资源路径兜底取名（去目录、扩展名与序号前缀）。
func dicDocNameFromPath(p string) string {
	name := strings.TrimSuffix(path.Base(p), ".md")
	if i := strings.Index(name, "-"); i > 0 {
		name = name[i+1:]
	}
	return name
}

// dicDocGroup 由资源路径推目录树分组：docs/1-内置函数/04-文件操作.md → 内置函数；docs/2-机器人功能/01-QQ官方机器人.md → 机器人功能；docs/3-JavaScript集成.md → 平台功能。
func dicDocGroup(p string) string {
	dir := path.Dir(p)
	if dir == dicDocDir {
		return dicDocTopGroup
	}
	return dicDocNameFromPath(path.Base(dir) + ".md")
}

// dicDocFind 按名称解析一篇文档。name 可以是：
// 完整资源路径（docs/1-内置函数/04-文件操作.md）、省略 docs/ 前缀的路径、
// 只给文件名（04-文件操作 / 04-文件操作.md），或直接给标题（文件操作）。
// 命中多篇时报错并列出候选，避免读到错的那篇。
func dicDocFind(name string) (*dicDocEntry, error) {
	list := dicDocList()
	if len(list) == 0 {
		return nil, fmt.Errorf("内置文档资源缺失")
	}
	key := strings.TrimPrefix(strings.TrimSpace(strings.ReplaceAll(name, "\\", "/")), "./")
	if key == "" {
		return nil, fmt.Errorf("请提供文档名称")
	}
	// 1) 完整路径（可省略 docs/ 前缀与 .md 后缀）
	for _, cand := range []string{key, key + ".md", dicDocDir + "/" + key, dicDocDir + "/" + key + ".md"} {
		for i := range list {
			if list[i].Path == cand {
				return &list[i], nil
			}
		}
	}
	// 2) 只给文件名：按资源路径后缀匹配
	matches := dicDocMatch(list, func(e *dicDocEntry) bool {
		return strings.HasSuffix(e.Path, "/"+key) || strings.HasSuffix(e.Path, "/"+key+".md")
	})
	// 3) 只给标题
	if len(matches) == 0 {
		title := strings.TrimSuffix(key, ".md")
		matches = dicDocMatch(list, func(e *dicDocEntry) bool { return e.Title == title })
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("文档不存在：%s（不带 doc 参数调用可列出全部文档）", name)
	}
	if len(matches) > 1 {
		paths := make([]string, 0, len(matches))
		for _, e := range matches {
			paths = append(paths, e.Path)
		}
		return nil, fmt.Errorf("「%s」对应多篇文档：%s，请改用完整路径", name, strings.Join(paths, "、"))
	}
	return matches[0], nil
}

// dicDocMatch 收集满足条件的文档。
func dicDocMatch(list []dicDocEntry, ok func(*dicDocEntry) bool) []*dicDocEntry {
	out := []*dicDocEntry{}
	for i := range list {
		if ok(&list[i]) {
			out = append(out, &list[i])
		}
	}
	return out
}

// dicDocResolveOrFirst 解析指定文档；name 为空时取清单第一篇（文档页不指定篇时的默认首页）。
func dicDocResolveOrFirst(name string) (*dicDocEntry, error) {
	if strings.TrimSpace(name) == "" {
		list := dicDocList()
		if len(list) == 0 {
			return nil, fmt.Errorf("内置文档资源缺失")
		}
		return &list[0], nil
	}
	return dicDocFind(name)
}

// dicDocRenderMarkdown 把文档正文渲染成 HTML（供文档页展示）。
// parser.CommonExtensions 含 MathJax：它会把 $...$ 当行内公式渲染成 \(...\)，
// 而 $ 是本语言的函数调用定界符（如 $文件读文本 路径$），文档里必须原样显示，故去掉该扩展。
// 文档里的裸 HTML 片段（<canvas>、<X1>、<img src="{{.键}}"> 等）同样只是正文示例，
// 默认渲染器会原样透传，浏览器便把它们当真实标签：<canvas> 撑出一块 300x150 的空白，
// <X1>/<URL|...> 这类假标签还会把参数名吞掉，故这里统一按文本转义。
// 注意 gomarkdown 的 Parser 不可复用（Parse 一次即作废），必须每次新建。
func dicDocRenderMarkdown(text string) string {
	p := parser.NewWithExtensions(parser.CommonExtensions &^ parser.MathJax)
	renderer := mdhtml.NewRenderer(mdhtml.RendererOptions{
		Flags: mdhtml.CommonFlags,
		RenderNodeHook: func(w io.Writer, node ast.Node, entering bool) (ast.WalkStatus, bool) {
			switch node.(type) {
			case *ast.HTMLSpan, *ast.HTMLBlock:
				if entering {
					mdhtml.EscapeHTML(w, node.AsLeaf().Literal)
				}
				return ast.GoToNext, true
			}
			return ast.GoToNext, false
		},
	})
	return string(markdown.ToHTML([]byte(text), p, renderer))
}

// ============== 跨篇搜索 ==============

const (
	// dicDocSearchPerDoc 每篇文档最多返回的命中数（避免长文档把结果列表刷屏）
	dicDocSearchPerDoc = 20
	// dicDocSearchContext 命中片段前后各取的字符数
	dicDocSearchContext = 32
)

// dicDocSearchHit 一条搜索命中：定位到哪一篇、篇内第几个高亮，以及命中处上下文。
type dicDocSearchHit struct {
	Path  string `json:"path"`
	Title string `json:"title"`
	Group string `json:"group"`
	// Index 该命中在此篇内的序号（从 0 开始），用于切篇后定位到对应高亮
	Index int `json:"index"`
	// Func 命中是否落在函数名上（$函数名 参数…$ 里的函数名），用于让函数结果排在正文提及之前
	Func bool `json:"func"`
	// Snippet 命中处上下文（纯文本，供结果列表展示）
	Snippet string `json:"snippet"`
}

// dicDocTagRe 匹配 HTML 标签，用于把渲染结果还原成纯文本
var dicDocTagRe = regexp.MustCompile(`<[^>]*>`)

// dicDocFuncNameRe 抓纯文本里的函数名：文档中函数写作 $函数名 参数…$，函数名紧跟 $ 且不含空白
var dicDocFuncNameRe = regexp.MustCompile(`\$([^$\s]+)`)

// dicDocPlainText 把渲染后的 HTML 还原成纯文本：去标签、反转义实体、折叠空白。
// 标签替换成空格而非空串：文档页的高亮只在一个文本节点内生效，这里也应当如此，
// 否则跨标签的命中会被算进来，篇内序号就和页面上真正标出的高亮对不上了。
func dicDocPlainText(htmlText string) string {
	return strings.Join(strings.Fields(html.UnescapeString(dicDocTagRe.ReplaceAllString(htmlText, " "))), " ")
}

// dicDocFuncSpans 找出纯文本里所有函数名的字符区间（按 rune 计），用于判断命中是否落在函数名上。
func dicDocFuncSpans(text string) [][2]int {
	matches := dicDocFuncNameRe.FindAllStringSubmatchIndex(text, -1)
	spans := make([][2]int, 0, len(matches))
	for _, m := range matches {
		spans = append(spans, [2]int{
			utf8.RuneCountInString(text[:m[2]]),
			utf8.RuneCountInString(text[:m[3]]),
		})
	}
	return spans
}

// dicDocInFuncName 判断 [at, end) 这段命中是否完整落在某个函数名里。
func dicDocInFuncName(spans [][2]int, at, end int) bool {
	for _, s := range spans {
		if s[0] <= at && end <= s[1] {
			return true
		}
	}
	return false
}

// dicDocSearch 在全部内置文档里搜索关键词（大小写不敏感）。
// 说明文档已拆成多篇，搜索必须跨篇进行，否则只能搜到当前篇，其它篇的内容就搜不到了。
// 结果顺序：先「命中函数名」的（用户多半是在找函数），再「正文里提到」的；
// 同一档内保持「篇序 + 篇内出现顺序」。命中序号以渲染后的纯文本为准
// （与文档页在 HTML 文本节点里做高亮的口径一致）。
func dicDocSearch(keyword string) []dicDocSearchHit {
	hits := []dicDocSearchHit{}
	kw := strings.TrimSpace(keyword)
	if kw == "" {
		return hits
	}
	needle := strings.ToLower(kw)
	size := utf8.RuneCountInString(kw)
	for _, doc := range dicDocList() {
		text := dicDocPlainText(dicDocRenderMarkdown(doc.content))
		runes := []rune(text)
		hay := strings.ToLower(text)
		spans := dicDocFuncSpans(text)
		count := 0
		for offset := 0; ; {
			i := strings.Index(hay[offset:], needle)
			if i < 0 {
				break
			}
			pos := offset + i
			at := utf8.RuneCountInString(text[:pos])
			hits = append(hits, dicDocSearchHit{
				Path:    doc.Path,
				Title:   doc.Title,
				Group:   doc.Group,
				Index:   count,
				Func:    dicDocInFuncName(spans, at, at+size),
				Snippet: dicDocSnippet(runes, at, size),
			})
			count++
			offset = pos + len(needle)
		}
	}
	// 函数名命中优先，同档保持原有篇序/篇内序
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Func && !hits[j].Func })
	// 限流放在排序之后：否则篇内靠后的函数名命中会被前面的正文命中挤掉
	kept := hits[:0]
	perDoc := map[string]int{}
	for _, h := range hits {
		if perDoc[h.Path] >= dicDocSearchPerDoc {
			continue
		}
		perDoc[h.Path]++
		kept = append(kept, h)
	}
	return kept
}

// dicDocSnippet 取命中处前后各若干字符作为上下文，两端按需补省略号。
func dicDocSnippet(runes []rune, at, size int) string {
	start := at - dicDocSearchContext
	if start < 0 {
		start = 0
	}
	end := at + size + dicDocSearchContext
	if end > len(runes) {
		end = len(runes)
	}
	s := string(runes[start:end])
	if start > 0 {
		s = "…" + s
	}
	if end < len(runes) {
		s += "…"
	}
	return s
}

// dicDocIndexText 文档索引文本：列出全部分组与文档，供 AI 了解有哪些文档可读、该传什么 doc 值。
func dicDocIndexText() string {
	list := dicDocList()
	if len(list) == 0 {
		return "内置文档资源缺失。"
	}
	var b strings.Builder
	b.WriteString("内置文档清单（读某一篇：read_dic_doc 传 doc=\"资源路径\"）：\n")
	group := ""
	for i := range list {
		if list[i].Group != group {
			group = list[i].Group
			b.WriteString("\n【" + group + "】\n")
		}
		b.WriteString(fmt.Sprintf("- %s（%s，%d 字）\n", list[i].Title, list[i].Path, list[i].Chars))
	}
	return strings.TrimRight(b.String(), "\n")
}

// dicDocsZip 把全部内置文档打包成 zip。
// 压缩包内沿用 docs 下的相对路径（如 1-内置函数/04-文件操作.md），保留原有的分组结构。
func dicDocsZip() ([]byte, error) {
	list := dicDocList()
	if len(list) == 0 {
		return nil, fmt.Errorf("内置文档资源缺失")
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i := range list {
		name := strings.TrimPrefix(list[i].Path, dicDocDir+"/")
		f, err := zw.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := f.Write([]byte(list[i].content)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
