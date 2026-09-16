package build

import (
	"os"
	"regexp"
	"strings"
)

var (
	// webTagRe 粗略匹配一行里的 HTML 标签；<! 与 <? 开头（注释、DOCTYPE、处理指令）不匹配。
	webTagRe = regexp.MustCompile(`(?s)</?[a-zA-Z][^>]*>`)
	// webTagNameRe 从标签里取「是否关闭 + 标签名」。
	webTagNameRe = regexp.MustCompile(`^<\s*(/?)\s*([a-zA-Z][a-zA-Z0-9:_.-]*)`)
	// webInlineCommentRe 匹配同一行内闭合的 HTML 注释，统计标签前先剔除。
	webInlineCommentRe = regexp.MustCompile(`(?s)<!--.*?-->`)
	// webRawOpenRe 匹配开始「原样保留区」的标签：脚本/样式/预格式化文本。
	webRawOpenRe = regexp.MustCompile(`(?is)<(script|style|pre|textarea)\b`)
	// webTypeAttrRe 取开始标签里的 type 属性值，与 run 包 webDicTypeAttrRe 同形：
	// 属性名要求位于标签内空白之后，避免把 data-type 之类的属性误当成 type。
	webTypeAttrRe = regexp.MustCompile(`(?is)(?:^|\s)type\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
	// webNebulaOpenRe 匹配内联执行块 <?n ... ?> 的开启标记。
	webNebulaOpenRe = regexp.MustCompile(`<\?n`)
	webNebulaClose  = "?>"
)

// webVoidTags 为 HTML 空元素，不参与缩进层级。
var webVoidTags = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

// FormatWebDic 对网页词库（.wn）的 HTML 结构做缩进排版。
//
// <script type="nebula"> 的正文按 .n 的块结构规则排版后再缩进到脚本块层级；
// 运行时解析前会逐行去掉行首空白（run.TrimWebScriptIndent），缩进不影响执行结果。
// 普通 <script>/<style> 正文剥掉共有缩进后整体缩进到脚本块层级，行的相对缩进不变。
// <pre>/<textarea> 的正文与 HTML 注释逐字节保留，避免破坏预格式化文本。
// 换行符与末尾换行保持原文风格，与 FormatDic 一致。
func FormatWebDic(text string) string {
	eol := detectEOL(text)
	hadTrailingNewline := strings.HasSuffix(text, "\n") || strings.HasSuffix(text, "\r")

	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	if hadTrailingNewline && len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	formatted := formatWebBody(lines)
	if eol != "\n" {
		formatted = strings.ReplaceAll(formatted, "\n", eol)
	}
	if hadTrailingNewline {
		return formatted + eol
	}
	return formatted
}

// FormatWebDicFile 读取网页词库文件并返回格式化结果，以及格式化后内容是否与原文不同。
func FormatWebDicFile(path string) (formatted string, changed bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	formatted = FormatWebDic(string(data))
	return formatted, formatted != string(data), nil
}

// formatWebBody 逐行缩进 HTML 结构，原样保留区与内联执行块的正文单独排版。
func formatWebBody(lines []string) string {
	out := make([]string, 0, len(lines))
	depth := 0
	rawTag := ""          // 当前所处的原样保留区标签名（script/style/pre/textarea）
	rawOpen := ""         // 开启该保留区的行，用于判断 script 的 type
	rawBody := []string{} // 保留区正文行（不含关闭标签行）
	inNebula := false     // 是否处于 <?n ... ?> 内联执行块内
	nebulaBody := []string{}
	nebulaIndent := 0 // 内联块开启行自身的缩进层级
	inComment := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if inComment {
			out = append(out, line)
			if strings.Contains(line, "-->") {
				inComment = false
			}
			continue
		}

		// <?n ... ?> 内联执行块的正文是 Nebula 语句，不能当 HTML 文本缩进：
		// 按 .n 的块结构排版后再整体缩进到块层级（运行时解析前会去掉行首空白，缩进不影响执行）。
		if inNebula {
			if idx := strings.Index(trimmed, webNebulaClose); idx >= 0 {
				if body := trimmed[:idx]; strings.TrimSpace(body) != "" {
					nebulaBody = append(nebulaBody, body)
				}
				out = append(out, webPrefixIndent(webIndentNebulaBody(nebulaBody), nebulaIndent+1)...)
				// 结束标记与它后面的 HTML（如 ?></p>）一起输出在开启行的层级上；
				// 其中的标签照常增减层级，否则后续行的缩进会整体偏移。
				tail := trimmed[idx+len(webNebulaClose):]
				out = append(out, strings.Repeat(indentUnit, nebulaIndent)+webNebulaClose+tail)
				opens, closes := webCountTags(tail)
				depth = max(depth+opens-closes, 0)
				inNebula, nebulaBody, nebulaIndent = false, nil, 0
				continue
			}
			nebulaBody = append(nebulaBody, line)
			continue
		}

		if rawTag != "" {
			lower := strings.ToLower(trimmed)
			if strings.HasPrefix(lower, "</"+rawTag) {
				// 关闭标签单独成行：先排版正文，再输出关闭标签。
				// script/style 的内容对首尾空白不敏感，缩进关闭标签更整齐；
				// pre/textarea 的内容保留空白，关闭标签也必须原样输出，否则会多出可视空白。
				out = append(out, webFormatRawBody(rawTag, rawOpen, rawBody, depth)...)
				if rawTag == "pre" || rawTag == "textarea" {
					out = append(out, line)
				} else {
					out = append(out, strings.Repeat(indentUnit, max(depth-1, 0))+trimmed)
				}
				depth = max(depth-1, 0)
				rawTag, rawOpen, rawBody = "", "", nil
				continue
			}
			if strings.Contains(lower, "</"+rawTag) {
				// 关闭标签与正文同行：该行原样输出，正文照常排版后再补回这一行
				out = append(out, webFormatRawBody(rawTag, rawOpen, rawBody, depth)...)
				out = append(out, line)
				depth = max(depth-1, 0)
				rawTag, rawOpen, rawBody = "", "", nil
				continue
			}
			rawBody = append(rawBody, line)
			continue
		}

		if trimmed == "" {
			out = append(out, "")
			continue
		}

		opens, closes := webCountTags(trimmed)
		indent := depth
		if strings.HasPrefix(trimmed, "</") {
			indent = max(depth-1, 0)
		}
		out = append(out, strings.Repeat(indentUnit, indent)+trimmed)
		depth = max(depth+opens-closes, 0)

		// 本行开启了原样保留区（且未在同一行闭合）时，后续行交给保留区排版
		if name, ok := webRawOpenTag(trimmed); ok {
			rawTag, rawOpen, rawBody = name, trimmed, nil
		} else if loc := webNebulaOpenRe.FindStringIndex(trimmed); loc != nil &&
			!strings.Contains(trimmed[loc[1]:], webNebulaClose) {
			// <?n 与 ?> 不同行：<?n 之后的部分与后续行都是该块的 Nebula 正文
			inNebula = true
			nebulaBody = nil
			if rest := strings.TrimSpace(trimmed[loc[1]:]); rest != "" {
				nebulaBody = []string{rest}
			}
			nebulaIndent = indent
		}
		// 本行开始了未闭合的 HTML 注释
		if idx := strings.Index(trimmed, "<!--"); idx >= 0 && !strings.Contains(trimmed[idx:], "-->") {
			inComment = true
		}
	}

	return strings.Join(out, "\n")
}

// webFormatRawBody 排版原样保留区的正文，inner 为正文应处的层级（开始标签所在层级 + 1）。
// pre/textarea 逐字节保留；<script type="nebula"> 按 .n 块结构缩进；其余 script/style 整体缩进。
func webFormatRawBody(tag, open string, body []string, inner int) []string {
	if len(body) == 0 {
		return nil
	}
	switch tag {
	case "pre", "textarea":
		return body
	case "script":
		if webScriptTypeNebula(open) {
			return webPrefixIndent(webIndentNebulaBody(body), inner)
		}
	}
	return webShiftIndent(body, inner)
}

// webScriptTypeNebula 判断 script 开始标签是否为 type="nebula"。
// 属性值精确匹配，与运行时 isNebulaScript 保持一致。
func webScriptTypeNebula(openTag string) bool {
	m := webTypeAttrRe.FindStringSubmatch(openTag)
	if m == nil {
		return false
	}
	for _, v := range m[1:] {
		if v != "" {
			return v == "nebula"
		}
	}
	return false
}

// webPrefixIndent 给所有非空行统一加上 inner 层缩进。
func webPrefixIndent(lines []string, inner int) []string {
	if inner <= 0 {
		return lines
	}
	prefix := strings.Repeat(indentUnit, inner)
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			out = append(out, "")
			continue
		}
		out = append(out, prefix+line)
	}
	return out
}

// webIndentNebulaBody 用 .n 的块结构规则排版脚本块正文。
// 脚本块没有触发词，整段按「头部初始化区」处理：块开启行顶格、块内逐级缩进；
// //@关闭缩进 到 //@启用缩进 之间行首空白有语义，原样透传，与 formatBody 一致。
func webIndentNebulaBody(lines []string) []string {
	out := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])

		if trimmed == "" {
			out = append(out, "")
			i++
			continue
		}

		if trimmed == "//@关闭缩进" {
			out = append(out, "//@关闭缩进")
			i++
			for i < len(lines) && strings.TrimSpace(lines[i]) != "//@启用缩进" {
				out = append(out, lines[i])
				i++
			}
			if i < len(lines) {
				out = append(out, "//@启用缩进")
				i++
			}
			continue
		}

		if trimmed == "//@启用缩进" {
			out = append(out, "//@启用缩进")
			i++
			continue
		}

		region, next := formatRegion(lines, i)
		if len(region) == 0 {
			// 防御：区域为空说明首行是停止标记（正常情况下已在上方分支处理），
			// 原样输出并前进，避免下标不前进造成死循环。
			out = append(out, trimLeadingBlank(lines[i]))
			i++
			continue
		}
		i = next
		out = append(out, formatIndent(region)...)
	}
	return out
}

// webShiftIndent 把正文整体缩进到 inner 层：剥掉正文共有的最小缩进后统一补上目标层级，
// 行与行之间的相对缩进保持不变。
// JS 多行模板串（反引号跨行）内的行原样保留，避免改写字符串内容。
func webShiftIndent(body []string, inner int) []string {
	base := -1
	shift := make([]bool, len(body)) // 该行是否参与整体平移
	inTemplate := false
	for i, line := range body {
		if !inTemplate && strings.TrimSpace(line) != "" {
			shift[i] = true
			if n := len(line) - len(strings.TrimLeft(line, " \t")); base < 0 || n < base {
				base = n
			}
		}
		if webBacktickCount(line)%2 == 1 {
			inTemplate = !inTemplate
		}
	}
	if base <= 0 && inner <= 0 {
		return body
	}
	if base < 0 {
		base = 0
	}

	prefix := strings.Repeat(indentUnit, inner)
	out := make([]string, 0, len(body))
	for i, line := range body {
		if !shift[i] {
			out = append(out, line)
			continue
		}
		out = append(out, prefix+line[base:])
	}
	return out
}

// webBacktickCount 统计一行里未被反斜杠转义的反引号数量，用于识别 JS 多行模板串。
func webBacktickCount(line string) int {
	n := 0
	for i := 0; i < len(line); i++ {
		if line[i] != '`' {
			continue
		}
		bs := 0
		for j := i - 1; j >= 0 && line[j] == '\\'; j-- {
			bs++
		}
		if bs%2 == 0 {
			n++
		}
	}
	return n
}

// webCountTags 统计一行里的开始标签与结束标签数量，用于计算缩进层级。
// 空元素与自闭合标签不计入开始标签。
func webCountTags(line string) (opens, closes int) {
	line = webInlineCommentRe.ReplaceAllString(line, "")
	for _, raw := range webTagRe.FindAllString(line, -1) {
		m := webTagNameRe.FindStringSubmatch(raw)
		if m == nil {
			continue
		}
		if m[1] == "/" {
			closes++
			continue
		}
		name := strings.ToLower(m[2])
		if webVoidTags[name] || strings.HasSuffix(strings.TrimSpace(raw), "/>") {
			continue
		}
		opens++
	}
	return opens, closes
}

// webRawOpenTag 判断一行是否开启了原样保留区，返回标签名。
// 同一行内已闭合（如 <script>...</script>）或自闭合时返回 false。
func webRawOpenTag(line string) (string, bool) {
	loc := webRawOpenRe.FindStringSubmatchIndex(line)
	if loc == nil {
		return "", false
	}
	name := strings.ToLower(line[loc[2]:loc[3]])
	if strings.Contains(strings.ToLower(line[loc[1]:]), "</"+name) {
		return "", false
	}
	if strings.HasSuffix(strings.TrimSpace(line[:loc[1]]), "/>") {
		return "", false
	}
	return name, true
}
