package build

import (
	"os"
	"strings"
	"unicode/utf8"
)

// indentUnit 为格式化正文时的缩进单元，统一为 4 个空格，
// 与词库调试界面及命令行 -format 保持一致。
const indentUnit = "    "

// formatFrame 为缩进栈中的一帧，对应内置算法的框信息。
// leaf 表示「叶子框」：其内部只识别各自的关闭标记，不再嵌套其他框。
type formatFrame struct {
	kind  string // 框类型：if/match/for/foreach/text/json/newJson/varNewJson/valChain/valText/valTextr/nodeJs/func
	close string // 关闭标记，仅用于可读性
	leaf  bool   // 是否为叶子框
	json  string // 新建 JSON 框的起始括号（{ 或 [）
	depth int    // 仅新建 JSON 框使用：括号平衡深度
}

// trimLeadingBlank 去掉行首空格与制表符，等价内置算法的 replace(/^[ \t]+/,"")。
func trimLeadingBlank(line string) string {
	return strings.TrimLeft(line, " \t")
}

// formatOpen 识别「框开启」行，等价内置算法的 hs。
func formatOpen(line string) (formatFrame, bool) {
	switch {
	case utf8.RuneCountInString(line) > 7 && strings.HasPrefix(line, "如果>"):
		return formatFrame{kind: "if", close: "<如果"}, true
	case utf8.RuneCountInString(line) > 7 && strings.HasPrefix(line, "匹配>"):
		return formatFrame{kind: "match", close: "<匹配"}, true
	case strings.HasPrefix(line, "判断循环>") && utf8.RuneCountInString(line) > 5:
		return formatFrame{kind: "for", close: "<循环"}, true
	case strings.HasPrefix(line, "循环>"):
		return formatFrame{kind: "for", close: "<循环"}, true
	case strings.HasPrefix(line, "遍历>"):
		return formatFrame{kind: "foreach", close: "<遍历"}, true
	case strings.HasPrefix(line, "纯文本>"):
		return formatFrame{kind: "text", close: "<文本", leaf: true}, true
	case strings.HasPrefix(line, "文本>"):
		return formatFrame{kind: "text", close: "<文本", leaf: true}, true
	case line == "JSON>[":
		return formatFrame{kind: "newJson", close: "]", leaf: true, json: "["}, true
	case line == "JSON>{":
		return formatFrame{kind: "newJson", close: "}", leaf: true, json: "{"}, true
	case strings.HasPrefix(line, "JSON>"):
		return formatFrame{kind: "json", close: "<JSON", leaf: true}, true
	case line == "--js":
		return formatFrame{kind: "nodeJs", close: "--end", leaf: true}, true
	case line == "#:>>>":
		return formatFrame{kind: "valChain", close: "<<<", leaf: true}, true
	case strings.HasPrefix(line, "#:执行函数>"):
		return formatFrame{kind: "func", close: "<函数"}, true
	case strings.HasPrefix(line, "执行函数>"):
		return formatFrame{kind: "func", close: "<函数"}, true
	}

	// 赋予值形式的框，复用编译期同源的 ValTextTest（等价内置算法的 gs）。
	vt, vk, vv := ValTextTest(line)
	if vt == 6 {
		if vk != "" && (strings.HasPrefix(vv, "执行函数>") || strings.HasPrefix(vv, "函数>")) {
			return formatFrame{kind: "func", close: "<函数"}, true
		}
		// 变量名:文本> / 变量名:纯文本>：与 变量名:""" / 变量名:''' 同族的赋值文本框。
		if vk != "" && (strings.HasPrefix(vv, "纯文本>") || strings.HasPrefix(vv, "文本>")) {
			return formatFrame{kind: "text", close: "<文本", leaf: true}, true
		}
		if vv == "{" || vv == "[" {
			// 键名含 -> 表示多键 JSON 取值，不是新建 JSON 框
			if !strings.Contains(vk, "->") {
				closer := "]"
				if vv == "{" {
					closer = "}"
				}
				return formatFrame{kind: "varNewJson", close: closer, leaf: true, json: vv}, true
			}
		} else {
			switch vv {
			case ">>>":
				return formatFrame{kind: "valChain", close: "<<<", leaf: true}, true
			case `"""`:
				return formatFrame{kind: "valText", close: `"""`, leaf: true}, true
			case "'''":
				return formatFrame{kind: "valTextr", close: "'''", leaf: true}, true
			}
		}
	}
	return formatFrame{}, false
}

// formatCloseKind 返回框关闭标记对应的框类型，等价内置算法的 fs 映射。
// 注意：文本框与 JSON 框为叶子框，其关闭标记不在其中。
func formatCloseKind(line string) string {
	switch line {
	case "<函数":
		return "func"
	case "<如果":
		return "if"
	case "<匹配":
		return "match"
	case "<循环":
		return "for"
	case "<遍历":
		return "foreach"
	}
	return ""
}

// formatLeafClosed 判断叶子框内的当前行是否为该框的关闭行，
// 返回是否关闭；新建 JSON 框会同步维护括号平衡深度。
func formatLeafClosed(f *formatFrame, line string) bool {
	switch f.kind {
	case "text":
		return line == "<文本"
	case "json":
		return line == "<JSON"
	case "newJson", "varNewJson":
		if strings.HasSuffix(line, "{") || strings.HasSuffix(line, "[") {
			f.depth++
		}
		if line == "}" || line == "]" || line == "}," || line == "]," {
			f.depth--
			if f.depth == 0 && (line == "}" || line == "]") {
				return true
			}
		}
		return false
	case "valChain":
		return line == "<<<"
	case "valText":
		return line == `"""`
	case "valTextr":
		return line == "'''"
	case "nodeJs":
		return line == "--end"
	}
	return false
}

// formatIndent 依据框层级为区域内的行加缩进，等价内置算法的 dt。
func formatIndent(lines []string) []string {
	out := make([]string, 0, len(lines))
	stack := make([]formatFrame, 0, 8)

	// pad 为行加指定层级缩进，空行保持为空。
	pad := func(depth int, line string) string {
		if line == "" {
			return ""
		}
		return strings.Repeat(indentUnit, depth) + line
	}

	for _, raw := range lines {
		line := trimLeadingBlank(raw)
		depth := len(stack)

		if depth > 0 && stack[depth-1].leaf {
			f := &stack[depth-1]
			if formatLeafClosed(f, line) {
				stack = stack[:depth-1]
				out = append(out, pad(depth-1, line))
			} else {
				out = append(out, pad(depth, line))
			}
			continue
		}

		if kind := formatCloseKind(line); kind != "" && depth > 0 && stack[depth-1].kind == kind {
			stack = stack[:depth-1]
			out = append(out, pad(depth-1, line))
			continue
		}

		if f, ok := formatOpen(line); ok {
			out = append(out, pad(depth, line))
			f.depth = 1
			stack = append(stack, f)
			continue
		}

		out = append(out, pad(depth, line))
	}
	return out
}

// formatRegion 从 start 起取一段连续内容，遇空行或缩进开关标记即停，等价内置算法的 ps。
func formatRegion(lines []string, start int) ([]string, int) {
	region := make([]string, 0, 8)
	i := start
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || trimmed == "//@关闭缩进" || trimmed == "//@启用缩进" {
			break
		}
		region = append(region, lines[i])
		i++
	}
	return region, i
}

// formatMultiline 判断区域是否含多行块标记，命中则整段原样保留，等价内置算法的 ms。
func formatMultiline(region []string) bool {
	for _, line := range region {
		trimmed := strings.TrimSpace(line)
		if strings.HasSuffix(trimmed, " #{") || trimmed == "}#" || trimmed == "<?n" || trimmed == "?>" {
			return true
		}
	}
	return false
}

// IsHeadLine 判断一行是否为头部内容：预编译指令（//@）、#引入=/$引入 导入行、赋值行或函数调用行。
func IsHeadLine(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	if strings.HasPrefix(line, "//@") {
		return true
	}
	if strings.HasPrefix(line, "#引入=") {
		return true
	}
	// $引入 目标$ 与 #引入= 等价；变量:$引入 目标$ 为「导入全部函数并返回实例包」的赋予值形式。
	if strings.HasPrefix(line, "$引入 ") && strings.HasSuffix(line, "$") {
		return true
	}
	if idx := strings.Index(line, ":"); idx > 0 {
		if rest := strings.TrimSpace(line[idx+1:]); strings.HasPrefix(rest, "$引入 ") && strings.HasSuffix(rest, "$") {
			return true
		}
	}
	if isFuncCallLine(line) {
		return true
	}
	vt, key, _ := ValTextTest(line)
	return vt != 0 && key != ""
}

// isFuncCallLine 判断整行是否为一个 $函数 …$ 调用（形如 $执行词库 xxx Main$）。
// 要求整行就是一个调用（首尾各一个 $），避免把文案里零散的 $ 误判成调用。
// 头部初始化脚本可承载函数调用（$执行词库$、$执行词库文件$、$重定向触发词$ 等），
// 全文无空行时据此判定为头部，避免整行被当成触发词导致「运行没反应」。
func isFuncCallLine(line string) bool {
	return len(line) > 2 && strings.HasPrefix(line, "$") && strings.HasSuffix(line, "$")
}

// FirstHeadLikeLine 判断「全文无空行」的文件是否应按头部初始化脚本解析：
// 跳过空行与普通注释后，首个有效行是赋值/引入/预编译指令行、函数调用行，或框开启行
// （循环>、遍历>、如果>、匹配>、文本>、JSON>、执行函数> 等）时返回 true——
// 框与函数调用同样属于初始化脚本的一部分，头部与正文一样是合法的词块容器。
// 词库编译（run 包）与这里的格式化共用该判定，保证解析与排版结果一致。
func FirstHeadLikeLine(lines []string) bool {
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "/*") || (strings.HasPrefix(line, "//") && !strings.HasPrefix(line, "//@")) {
			continue
		}
		if IsHeadLine(line) {
			return true
		}
		_, blockOpen := formatOpen(line)
		return blockOpen
	}
	return false
}

// formatBody 执行主体排版，等价内置算法的 Ss。
// 首个空行之前为头部（初始化区），头部只做去缩进、不参与块缩进；
// 全文无空行且开头是赋值/引入/指令行或框开启行时，整篇按头部初始化脚本排版。
func formatBody(lines []string) string {
	out := make([]string, 0, len(lines))

	// 存在头部：首个空行位于文件中间（下标大于 0）
	inHead := false
	hasBlank := false
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			hasBlank = true
			inHead = i > 0
			break
		}
	}
	if !hasBlank && FirstHeadLikeLine(lines) {
		inHead = true
	}

	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])

		if trimmed == "" {
			out = append(out, "")
			inHead = false
			i++
			continue
		}

		// //@关闭缩进 到 //@启用缩进 之间行首空白有语义，原样透传
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

		// 单独的 //@启用缩进（没有配对的 //@关闭缩进）也原样透传并前进。
		// 否则 formatRegion 会返回空区域且下标不前进，导致死循环
		//（内置格式化算法在这里同样会卡住，此处需避免）。
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

		if formatMultiline(region) {
			out = append(out, region...)
			continue
		}

		if inHead {
			out = append(out, formatIndent(region)...)
			continue
		}

		// 正文首行为词条（触发词/函数名），保持顶格，其余行整体缩进一级
		out = append(out, trimLeadingBlank(region[0]))
		for _, line := range formatIndent(region[1:]) {
			if line == "" {
				out = append(out, "")
			} else {
				out = append(out, indentUnit+line)
			}
		}
	}

	return strings.Join(out, "\n")
}

// detectEOL 返回文本主要使用的换行符风格（\r\n / \r / \n），默认 \n。
// 按出现次数取多数，避免个别转义产生字面 \r 时把整份词库改成 CR 换行。
func detectEOL(text string) string {
	crlf, lf, cr := 0, 0, 0
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\n':
			if i > 0 && text[i-1] == '\r' {
				crlf++
			} else {
				lf++
			}
		case '\r':
			if i+1 >= len(text) || text[i+1] != '\n' {
				cr++
			}
		}
	}
	if crlf > 0 && crlf >= lf && crlf >= cr {
		return "\r\n"
	}
	if cr > lf && cr > crlf {
		return "\r"
	}
	return "\n"
}

// FormatDic 按「格式化词库（自动缩进块结构）」算法重新排版词库源码。
// 换行符保持原文风格（\r\n / \r / \n），保留原文末尾是否有换行；
// 这样对已是正确缩进的文件（如 CRLF 词库）不会因换行符转换而被判为「有变化」。
func FormatDic(text string) string {
	eol := detectEOL(text)
	hadTrailingNewline := strings.HasSuffix(text, "\n") || strings.HasSuffix(text, "\r")

	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	if hadTrailingNewline && len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	formatted := formatBody(lines)
	if eol != "\n" {
		formatted = strings.ReplaceAll(formatted, "\n", eol)
	}
	if hadTrailingNewline {
		return formatted + eol
	}
	return formatted
}

// FormatDicFile 读取词库文件并返回格式化结果，以及格式化后内容是否与原文不同。
func FormatDicFile(path string) (formatted string, changed bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	formatted = FormatDic(string(data))
	return formatted, formatted != string(data), nil
}
