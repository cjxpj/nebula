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

// ============== 按小节检索原文（供 AI 的 search_docs 工具） ==============

const (
	// dicDocRawMaxHits 一次检索最多返回的小节数
	dicDocRawMaxHits = 12
	// dicDocRawPerDoc 单篇最多返回的小节数，避免一篇长文档把结果占满
	dicDocRawPerDoc = 3
	// dicDocRawSectionRunes 函数名命中的小节最多返回的字符数，超出以命中处为中心裁剪
	dicDocRawSectionRunes = 1200
	// dicDocRawBriefRunes 只是被正文提到（命中不在函数名上）的小节最多返回的字符数。
	// 取较短窗口，否则「画布」这类常见词会把好几篇文档整篇拖进上下文，失去「检索代替通读」的意义。
	dicDocRawBriefRunes = 300
	// dicDocRawClipLead 裁剪长小节时，命中处之前保留的上下文字符数
	dicDocRawClipLead = 200
)

// dicDocRawHit 一条「按小节」的检索命中：定位到哪一篇的哪一节，并附上该节原文。
type dicDocRawHit struct {
	Path  string `json:"path"`
	Title string `json:"title"`
	Group string `json:"group"`
	// Heading 命中所在小节的标题（无标题的小节为空）
	Heading string `json:"heading"`
	// Func 该节内是否有命中完整落在函数名（$函数名 参数…$）上
	Func bool `json:"func"`
	// Text 小节原文（markdown，超长时以命中处为中心裁剪）
	Text string `json:"text"`
}

// dicDocSection 文档里的一节：标题 + 原文区间（按字节偏移）。
type dicDocSection struct {
	Heading string
	Start   int
	End     int
}

// dicDocRawSearch 在内置文档原文里检索关键词，返回命中所在小节的原文。
// 与 dicDocSearch 的分工：那个返回「纯文本碎片 + 篇内高亮序号」，供文档页的结果列表定位；
// 这个返回整节 markdown，供 AI 直接照着写代码——拿到相关小节就不必通读整篇文档。
// keywords 之间是「与」关系（都出现才算命中），便于用多个词把范围收窄到想要的那一节。
// 结果顺序：含函数名命中的节优先（多半是在找函数用法），同档保持篇序 / 篇内顺序。
func dicDocRawSearch(keywords []string) []dicDocRawHit {
	kws := make([]string, 0, len(keywords))
	for _, k := range keywords {
		if k = strings.TrimSpace(strings.ToLower(k)); k != "" {
			kws = append(kws, k)
		}
	}
	hits := []dicDocRawHit{}
	if len(kws) == 0 {
		return hits
	}
	for _, doc := range dicDocList() {
		for _, sec := range dicDocSections(doc.content) {
			text := doc.content[sec.Start:sec.End]
			if !dicDocContainsAll(text, kws) {
				continue
			}
			// 函数名命中的多半是「查这个函数怎么用」，给足正文；只是正文提到关键词的
			// 只给一小段窗口，否则「画布」这类常见词会把好几篇文档整篇拖进上下文。
			funcHit := dicDocSectionFuncHit(text, kws)
			limit := dicDocRawBriefRunes
			if funcHit {
				limit = dicDocRawSectionRunes
			}
			hits = append(hits, dicDocRawHit{
				Path:    doc.Path,
				Title:   doc.Title,
				Group:   doc.Group,
				Heading: sec.Heading,
				Func:    funcHit,
				Text:    dicDocSectionClip(text, kws, limit),
			})
		}
	}
	// 函数名命中优先，同档保持原有篇序 / 篇内序
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Func && !hits[j].Func })
	kept := hits[:0]
	perDoc := map[string]int{}
	for _, h := range hits {
		if len(kept) >= dicDocRawMaxHits || perDoc[h.Path] >= dicDocRawPerDoc {
			continue
		}
		perDoc[h.Path]++
		kept = append(kept, h)
	}
	return kept
}

// dicDocSections 按标题行把正文切成小节。
// 代码围栏（```）里的 # 行不算标题，否则文档示例代码里的 # 会把小节错误切开。
func dicDocSections(text string) []dicDocSection {
	secs := []dicDocSection{}
	inFence := false
	for off := 0; off < len(text); {
		end := strings.IndexByte(text[off:], '\n')
		line := text[off:]
		if end >= 0 {
			line = text[off : off+end]
		}
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "```"):
			inFence = !inFence
		case !inFence && dicDocIsHeadingLine(trimmed):
			if n := len(secs); n > 0 {
				secs[n-1].End = off
			}
			secs = append(secs, dicDocSection{Heading: dicDocHeadingText(trimmed), Start: off, End: len(text)})
		}
		if end < 0 {
			break
		}
		off += end + 1
	}
	if len(secs) == 0 {
		return []dicDocSection{{Start: 0, End: len(text)}}
	}
	// 首个标题之前的内容（如篇首说明）单独成一节
	if secs[0].Start > 0 {
		secs = append([]dicDocSection{{Start: 0, End: secs[0].Start}}, secs...)
	}
	return secs
}

// dicDocIsHeadingLine 判断一行是不是 markdown 标题（# 后跟空白或行尾）。
func dicDocIsHeadingLine(line string) bool {
	if !strings.HasPrefix(line, "#") {
		return false
	}
	rest := strings.TrimLeft(line, "#")
	return rest == "" || strings.HasPrefix(rest, " ")
}

// dicDocHeadingText 取标题行的标题文本（去掉 # 前缀），非标题行返回空串。
func dicDocHeadingText(line string) string {
	if !dicDocIsHeadingLine(line) {
		return ""
	}
	return strings.TrimSpace(strings.TrimLeft(line, "#"))
}

// dicDocContainsAll 判断文本是否同时包含全部关键词（关键词已转小写）。
func dicDocContainsAll(text string, kws []string) bool {
	hay := strings.ToLower(text)
	for _, k := range kws {
		if !strings.Contains(hay, k) {
			return false
		}
	}
	return true
}

// dicDocSectionFuncHit 判断本节里是否有某个关键词的命中完整落在函数名（$函数名 参数…$）上。
func dicDocSectionFuncHit(section string, kws []string) bool {
	spans := dicDocFuncSpans(section)
	if len(spans) == 0 {
		return false
	}
	hay := strings.ToLower(section)
	for _, k := range kws {
		size := utf8.RuneCountInString(k)
		for offset := 0; ; {
			i := strings.Index(hay[offset:], k)
			if i < 0 {
				break
			}
			at := utf8.RuneCountInString(section[:offset+i])
			if dicDocInFuncName(spans, at, at+size) {
				return true
			}
			offset += i + len(k)
		}
	}
	return false
}

// dicDocSectionClip 裁剪小节原文：超长时以最靠前的命中处为中心取窗口，保证命中内容一定在结果里。
func dicDocSectionClip(section string, kws []string, max int) string {
	runes := []rune(section)
	if len(runes) <= max {
		return strings.TrimRight(section, "\n")
	}
	best := -1
	hay := strings.ToLower(section)
	for _, k := range kws {
		i := strings.Index(hay, k)
		if i < 0 {
			continue
		}
		if at := utf8.RuneCountInString(section[:i]); best < 0 || at < best {
			best = at
		}
	}
	start := 0
	if best > dicDocRawClipLead {
		start = best - dicDocRawClipLead
	}
	if start+max > len(runes) {
		start = len(runes) - max
	}
	out := strings.TrimRight(string(runes[start:start+max]), "\n")
	if start > 0 {
		out = "…" + out
	}
	return out + "\n…（本节较长已截断）"
}

// dicDocIndexText 文档索引文本：列出全部分组与文档，供 AI 了解有哪些文档可读、该传什么 doc 值。
func dicDocIndexText() string {
	list := dicDocList()
	if len(list) == 0 {
		return "内置文档资源缺失。"
	}
	var b strings.Builder
	b.WriteString("内置文档清单（检索内容用 search_docs；读整篇用 read_dic_doc 传 doc=\"资源路径\"）：\n")
	group := ""
	for i := range list {
		if list[i].Group != group {
			group = list[i].Group
			b.WriteString("\n【")
			b.WriteString(group)
			b.WriteString("】\n")
		}
		fmt.Fprintf(&b, "- %s（%s，%d 字）\n", list[i].Title, list[i].Path, list[i].Chars)
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
