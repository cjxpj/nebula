package run

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// triggerRegexCache 缓存触发词编译结果，避免每次匹配都重新编译正则。
var triggerRegexCache sync.Map

// compileTriggerRegex 编译触发词正则（带缓存）。编译失败返回 nil。
func compileTriggerRegex(t string) *regexp.Regexp {
	if v, ok := triggerRegexCache.Load(t); ok {
		return v.(*regexp.Regexp)
	}
	regex, err := regexp.Compile("^(" + t + ")$")
	if err != nil {
		return nil
	}
	actual, _ := triggerRegexCache.LoadOrStore(t, regex)
	return actual.(*regexp.Regexp)
}

// triggerPlainCache 缓存触发词是否为纯文本的判断结果，避免每次匹配重复扫描触发词。
var triggerPlainCache sync.Map

// isPlainTrigger 判断触发词是否不含任何正则元字符。
// 纯文本触发词的匹配等价于字符串相等，可跳过正则编译与匹配，加速命中。
func isPlainTrigger(t string) bool {
	if v, ok := triggerPlainCache.Load(t); ok {
		return v.(bool)
	}
	plain := true
	for i := 0; i < len(t); i++ {
		switch t[i] {
		case '.', '*', '+', '?', '(', ')', '[', ']', '{', '}', '|', '^', '$', '\\':
			plain = false
		}
		if !plain {
			break
		}
	}
	triggerPlainCache.Store(t, plain)
	return plain
}

// 自义定替换函数
func ReplaceFunc(input, old string, replaceFunc func(string) string) string {
	var result strings.Builder
	start := 0
	for {
		index := strings.Index(input[start:], old)
		if index == -1 {
			break
		}
		result.WriteString(input[start : start+index])
		result.WriteString(replaceFunc(old))
		start += index + len(old)
	}
	result.WriteString(input[start:])
	return result.String()
}

// funcStrCache 缓存字符串的 $...$ 分段解析结果，避免每次调用 BuildFuncStr 都重复扫描 $ 与转义切分。
var funcStrCache sync.Map

// funcSeg 预编译后的 $...$ 分段。
type funcSeg struct {
	isFunc bool
	text   string   // isFunc=false：字面量文本
	args   []string // isFunc=true：函数调用参数（已按空格切分并处理转义）
	start  int      // 该段在原始字符串中的起始偏移，用于中断时按原语义回退处理剩余文本
}

// parseFuncSegments 把字符串按 $...$ 预切分为分段序列。仅做静态切分，转义/引号处理同 splitWithEscape。
func parseFuncSegments(str string) []funcSeg {
	if !strings.Contains(str, "$") {
		return []funcSeg{{isFunc: false, text: str, start: 0}}
	}
	segs := make([]funcSeg, 0, 4)
	start := 0
	for {
		openIndex := findUnescaped(str, "$", start)
		if openIndex == -1 {
			break
		}
		closeIndex := findUnescaped(str, "$", openIndex+1)
		if closeIndex == -1 {
			break
		}
		if openIndex > start {
			segs = append(segs, funcSeg{isFunc: false, text: str[start:openIndex], start: start})
		}
		segs = append(segs, funcSeg{isFunc: true, args: splitWithEscape(str[openIndex+1 : closeIndex]), start: start})
		start = closeIndex + 1
	}
	if start < len(str) {
		segs = append(segs, funcSeg{isFunc: false, text: str[start:], start: start})
	}
	return segs
}

// getFuncSegments 返回字符串的预编译分段（带缓存）。
func getFuncSegments(str string) []funcSeg {
	if v, ok := funcStrCache.Load(str); ok {
		return v.([]funcSeg)
	}
	segs := parseFuncSegments(str)
	actual, _ := funcStrCache.LoadOrStore(str, segs)
	return actual.([]funcSeg)
}

// 处理函数
func BuildFuncStr(
	str string,
	process func([]string) (string, bool), // 处理函数文本
	process2 func(string) (string, bool), // 处理外部文本，原样给你，不做转义
) string {
	// 快速路径：不含 $ 时整串作为外部文本处理一次，避免 Builder 往返复制。
	if !strings.Contains(str, "$") {
		out, _ := process2(str)
		return out
	}

	segs := getFuncSegments(str)
	var result strings.Builder
	for i, seg := range segs {
		if !seg.isFunc {
			out, stop := process2(seg.text)
			if stop {
				if i == len(segs)-1 {
					result.WriteString(out)
				} else {
					// 原语义：中断后剩余文本（自本段起始）按字面量一次性交给 process2。
					tail, _ := process2(str[seg.start:])
					result.WriteString(tail)
				}
				return result.String()
			}
			result.WriteString(out)
			continue
		}

		in, stop := process(seg.args)
		if stop {
			if i == len(segs)-1 {
				return result.String()
			}
			// 原语义：函数中断后剩余文本（自本段起始）按字面量一次性交给 process2。
			tail, _ := process2(str[seg.start:])
			result.WriteString(tail)
			return result.String()
		}
		result.WriteString(in)
	}

	return result.String()
}

// 找到下一个“未被奇数个反斜杠转义”的子串 sub（这里 sub="$"）
func findUnescaped(s, sub string, start int) int {
	for {
		i := strings.Index(s[start:], sub)
		if i == -1 {
			return -1
		}
		i += start

		// 统计紧挨在前的反斜杠数量
		bs := 0
		for j := i - 1; j >= 0 && s[j] == '\\'; j-- {
			bs++
		}
		if bs%2 == 0 {
			return i // 偶数 → 未转义
		}
		start = i + len(sub)
	}
}

// 在 $...$ 内部：按空格切分；支持 "\\"=>"\", "\$"=>"$"，成对引号内 "\n"=>换行
// 支持 "..." 双引号包裹：引号前的空格或行首开启引号，引号后的空格或行尾关闭引号，中间的空格不参与切分
// \" 始终表示字面引号；JSON 等非边界位置的 " 保持原样不被当作引号
func splitWithEscape(s string) []string {
	var args []string
	var b strings.Builder
	escaped := false
	inQuote := false

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if escaped {
			switch ch {
			case '\\':
				b.WriteByte('\\') // \\ → \
			case '$':
				b.WriteByte('$') // \$ → $
			case '"':
				b.WriteByte('"') // \" → 字面引号
			case 'n':
				if inQuote {
					b.WriteByte('\n') // \n → 换行（仅成对引号内）
				} else {
					b.WriteByte('\\')
					b.WriteByte('n')
				}
			default:
				// 未知转义：按字面写回
				b.WriteByte('\\')
				b.WriteByte(ch)
			}
			escaped = false
			continue
		}

		if ch == '\\' {
			escaped = true
			continue
		}

		if ch == '"' {
			if inQuote {
				// 关闭引号：后跟空格或行尾
				if i+1 >= len(s) || s[i+1] == ' ' {
					inQuote = false
					continue
				}
			} else if (i == 0 || s[i-1] == ' ') && hasClosingQuote(s, i+1) {
				// 开启引号：前是空格或行首，且存在成对结尾
				inQuote = true
				continue
			}
			// 非边界位置或没有结尾的引号 → 字面字符
			b.WriteByte(ch)
			continue
		}

		if !inQuote && ch == ' ' {
			if b.Len() > 0 {
				args = append(args, b.String())
				b.Reset()
			}
			continue
		}

		b.WriteByte(ch)
	}

	// 结尾是悬空反斜杠：当作字面反斜杠
	if escaped {
		b.WriteByte('\\')
	}
	if b.Len() > 0 {
		args = append(args, b.String())
	}
	return args
}

// hasClosingQuote 判断从 from 开始往后是否存在成对的结束引号（未被反斜杠转义，
// 且后跟空格或行尾）。用于确定当前开启的引号是否有结尾。
func hasClosingQuote(s string, from int) bool {
	escaped := false
	for i := from; i < len(s); i++ {
		ch := s[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			if i+1 >= len(s) || s[i+1] == ' ' {
				return true
			}
		}
	}
	return false
}

// replaceProcessedContent 接受一个字符串、开始和结束的子串，以及一个处理函数作为参数
func ReplaceProcessedContent(str, strStart, strEnd string, process func(string) string) string {
	// 快速路径：不含起始标记时直接返回原字符串，避免 Builder 往返复制
	if !strings.Contains(str, strStart) {
		return str
	}
	var result strings.Builder
	start := 0

	for {
		// 查找开始子串的下一个位置
		openIndex := strings.Index(str[start:], strStart)
		if openIndex == -1 {
			break
		}
		openIndex += start

		// 查找结束子串的下一个位置（从openIndex之后开始）
		closeIndex := strings.Index(str[openIndex+len(strStart):], strEnd)
		if closeIndex == -1 {
			break
		}
		closeIndex += openIndex + len(strStart)

		// 添加从开始到当前[之前的内容到结果字符串
		result.WriteString(str[start:openIndex])

		// 提取[]内的内容并处理
		content := str[openIndex+len(strStart) : closeIndex]
		processedContent := process(content)

		// 将处理后的内容添加到结果字符串
		result.WriteString(processedContent)

		// 更新开始位置为]之后
		start = closeIndex + len(strEnd)
	}

	// 添加剩余的部分到结果字符串
	result.WriteString(str[start:])

	return result.String()
}

// 遍历触发词文本
func RunFor(jsonData []*dto.BuildDic, trigger string, runNum int) ([]string, string, int, *regexp.Regexp) {
	jsonDataLen := len(jsonData)

	if runNum > jsonDataLen {
		return nil, "", 0, nil
	}

	// 遍历每个条目并输出
	for i := runNum; i < jsonDataLen; i++ {
		item := jsonData[i]
		text := item.Text

		// 使用动态编译的正则表达式
		t := item.Trigger

		// 纯文本触发词：直接字符串相等匹配，跳过正则编译与匹配（多数触发词为纯文本）
		if isPlainTrigger(t) {
			if trigger == t {
				return text, t, i, nil
			}
			continue
		}

		regex := compileTriggerRegex(t)
		if regex == nil {
			continue
		}
		if regex.MatchString(trigger) {
			return text, t, i, regex
		}
	}

	return nil, "", 0, nil
}

// RunForIndexed 使用预构建的触发词索引做匹配：纯文本 O(1) 命中，正则按原顺序线性匹配。
// 返回首个命中（原始下标最小）的词条正文、触发词与原始下标，语义与 RunFor 一致（不含正则对象）。
func RunForIndexed(idx *dto.TriggerIndex, list []*dto.BuildDic, trigger string, runNum int) ([]string, string, int) {
	if idx == nil || runNum >= len(list) {
		return nil, "", 0
	}

	best := len(list) // 无命中哨兵

	// 纯文本命中：取 >= runNum 的最小下标（索引按原始顺序排列）
	if is := idx.Plain[trigger]; len(is) > 0 {
		for _, i := range is {
			if i >= runNum {
				best = i
				break
			}
		}
	}

	// 正则命中：按原始顺序线性匹配，取 >= runNum 且 < best 的最小下标
	for _, i := range idx.Regex {
		if i < runNum {
			continue
		}
		if i >= best {
			break // regex 下标按原始顺序排列，之后只会更大
		}
		re := compileTriggerRegex(list[i].Trigger)
		if re != nil && re.MatchString(trigger) {
			best = i
			break
		}
	}

	if best >= len(list) {
		return nil, "", 0
	}
	item := list[best]
	return item.Text, item.Trigger, best
}

// RunFuncIndexed 使用预构建的函数索引（函数名 -> 词条）做 O(1) 查找。
func RunFuncIndexed(idx map[string][]*dto.BuildDic, callName string, paramCount int) (text []string, name string, tparts string, errRule string, ok bool) {
	for _, item := range idx[callName] {
		t := item.Trigger
		resF := ""
		if i := strings.LastIndex(t, "->"); i != -1 {
			resF, t = t[i+2:], t[:i]
		}

		rule := item.ParamRule
		if rule == "" {
			rule = "0"
		}

		if callName != t {
			continue
		}
		if utils.MatchLenRule(paramCount, rule) {
			return item.Text, t, resF, "", true
		}
		// 命中函数名但参数数量不符，返回规则供上层报错
		return nil, t, resF, rule, false
	}
	return nil, "", "", "", false
}

// parseTriggerPrefix 解析触发词行首的 [] 前缀。
// 返回触发类别（空串表示普通触发）、Class 类名（可为空）、参数数量规则（可为空）和去掉前缀后的触发词。
// [F]/[L] 缩写自动归一为 [函数]/[内部]；[:类名] 后缀表示 Class，如 [函数:a]、[F:a]；
// [|规则] 后缀表示参数数量校验，如 [函数|1|2]、[函数|1..]（仅对「函数」类别生效）。
func parseTriggerPrefix(line string) (category, class, param, rest string) {
	if !strings.HasPrefix(line, "[") {
		return "", "", "", line
	}
	end := strings.Index(line, "]")
	if end <= 1 {
		return "", "", "", line
	}
	tag := line[1:end]
	rest = line[end+1:]

	// 参数数量规则：首个 | 之后的内容
	if before, after, ok := strings.Cut(tag, "|"); ok {
		tag = before
		param = after
	}

	// Class 类名后缀
	if before, after, ok := strings.Cut(tag, ":"); ok {
		category, class = before, after
	} else {
		category = tag
	}

	switch category {
	case "F", "f":
		category = "函数"
	case "L", "l":
		category = "内部"
	}
	return category, class, param, rest
}

// parseImportLine 解析 #引入= 行，支持两种形式：
//
//	#引入=目标        → varName 为空
//	变量:#引入=目标    → 返回变量名与目标（导入全部函数并返回实例包）
//
// 返回变量名（可为空）、导入目标以及是否为引入行。
func parseImportLine(line string) (varName, target string, ok bool) {
	if strings.HasPrefix(line, "#引入=") {
		target = strings.TrimSpace(line[len("#引入="):])
		if target == "" {
			return "", "", false
		}
		return "", target, true
	}
	if idx := strings.Index(line, ":#引入="); idx > 0 {
		name := strings.TrimSpace(line[:idx])
		target := strings.TrimSpace(line[idx+len(":#引入="):])
		if name != "" && target != "" {
			return name, target, true
		}
	}
	return "", "", false
}

// resolveResourcePath 将 //@资源 的文件路径解析为「相对应用数据目录」的路径：
// 相对路径基于应用数据目录（private/）解析（与 #引入= 一致），绝对路径原样返回。
// 返回路径统一带 private/ 前缀（后续经 utils.NewFileQueue 读取时补应用数据主目录）。
func resolveResourcePath(rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}
	rel = filepath.ToSlash(rel)
	if !strings.HasPrefix(rel, "private/") {
		rel = "private/" + rel
	}
	return filepath.FromSlash(rel)
}

// compileDicResource 将 .n 词库资源编译并序列化为 gob 字节，供 //@资源 匹配 .n 后缀时自动编译。
// 含自定义函数（bot 注入）无法序列化时返回 nil，调用方回退按原始文本存入。
func compileDicResource(dicPath string, raw []byte) []byte {
	lines := utils.SplitLines(raw)
	// 单行密文：整块解密后重新切分（与 NewDicFile 一致）
	if len(lines) == 1 {
		if str, err := utils.Decrypt(utils.RemoveComments(lines[0]), appfiles.Key); err == nil {
			lines = utils.SplitLines([]byte(str))
			raw = []byte(str)
		}
	}
	split := BuildDicLinesWithRaw(dicPath, lines, raw)
	b, err := MarshalBuildValue(split)
	if err != nil {
		return nil
	}
	return b
}

// parseResourceVar 解析「变量名:文件路径」形式的资源声明行（多行 //@资源 块内使用）。
func parseResourceVar(line string) (name, rel string, ok bool) {
	t := strings.TrimSpace(line)
	idx := strings.IndexByte(t, ':')
	if idx <= 0 {
		return "", "", false
	}
	name = strings.TrimSpace(t[:idx])
	rel = strings.TrimSpace(t[idx+1:])
	if name == "" || rel == "" {
		return "", "", false
	}
	return name, rel, true
}

// loadResource 读取并登记一个资源：.n 自动编译为 gob 字节 / 文本两种形式。
func loadResource(name, rel string, resources map[string]any, stack *importStack, lineNo int) {
	abs := resolveResourcePath(rel)
	data, err := utils.NewFileQueue(abs).ReadFileByte()
	if err != nil {
		stack.addError(lineNo, "资源文件读取失败："+err.Error())
		return
	}
	if strings.HasSuffix(rel, ".n") {
		if b := compileDicResource(abs, data); b != nil {
			resources[name] = b
		} else {
			resources[name] = string(data)
		}
	} else {
		resources[name] = string(data)
	}
	// 记录资源文件内容 hash，供磁盘编译缓存失效校验
	stack.deps[abs] = dicHashBytes(data)
}

// isBlankOrComment 判断一行是否为空白或注释行（用于头部/正文分隔）。
// //@ 开头的指令不算注释，它们是有实际作用的编译指令。
func isBlankOrComment(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return true
	}
	if strings.HasPrefix(line, "//@") {
		return false
	}
	return strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*")
}

// importStack 记录当前 #引入= 递归加载链上的文件路径，用于检测循环引入。
// 加载为单 goroutine 递归，无需加锁。
type importStack struct {
	files    map[string]bool
	warnings []dto.BuildWarning

	// deps 记录所有已成功加载文件（含主文件）的路径与内容 hash，用于磁盘编译缓存失效校验。
	deps map[string]string
}

// newImportStack 创建空的引入链。
func newImportStack() *importStack {
	return &importStack{
		files: make(map[string]bool),
		deps:  make(map[string]string),
	}
}

// push 将文件压入引入链；若该文件已在链上则返回 false（循环引入）。
func (s *importStack) push(path string) bool {
	if s.files[path] {
		return false
	}
	s.files[path] = true
	return true
}

// pop 将文件从引入链弹出。
func (s *importStack) pop(path string) {
	delete(s.files, path)
}

// addWarning 追加一条编译警告（黄色）。
func (s *importStack) addWarning(line int, text string) {
	s.warnings = append(s.warnings, dto.BuildWarning{Line: line, Text: text, Level: "warning"})
}

// addError 追加一条编译错误（红色）。
func (s *importStack) addError(line int, text string) {
	s.warnings = append(s.warnings, dto.BuildWarning{Line: line, Text: text, Level: "error"})
}

// dicCacheVersion 磁盘编译缓存格式版本，结构变化时递增以淘汰旧缓存。
const dicCacheVersion = 5

// dicCacheEntry 词库编译结果的磁盘缓存结构（gob 序列化）。
// 只缓存可序列化词条；含 bot 注入（MyFunc 非空）的词库不落缓存，故无需序列化 Go 函数。
type dicCacheEntry struct {
	Version int
	// Deps 所有依赖文件（含主文件）路径 -> 内容 hash，用于失效校验。
	Deps map[string]string

	// BuildValue 可序列化部分
	Head          []string
	HeadLineNums  []int
	Dic           []*dto.BuildDic
	DicFuncs      map[string][]*dto.BuildDic
	ClassFuncs    map[string]map[string][]*dto.BuildDic
	BotImports    []string
	Warnings      []dto.BuildWarning
	Resources     map[string]any
	OnceResources map[string]bool
}

// dicHash 计算字符串的 sha256 十六进制摘要。
func dicHash(s string) string {
	return dicHashBytes([]byte(s))
}

func dicHashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// dicCachePath 返回某词库对应的磁盘缓存文件路径（private/.dic_cache 目录下）。
func dicCachePath(dicPath string) string {
	return filepath.Join(utils.GetAppDir(), "private", ".dic_cache", dicHash(dicPath)+".gob")
}

// readDicFileContent 读取词库文件，返回编译输入行与用于内容 hash 的原始字节；
// 若为单行密文则整块解密后重新切分，此时原始字节取解密后的内容。
func readDicFileContent(filePath string) (lines []string, raw []byte, err error) {
	data, err := utils.NewFileQueue(filePath).ReadFileByte()
	if err != nil {
		return nil, nil, err
	}
	lines = utils.SplitLines(data)
	raw = data
	if len(lines) == 1 {
		if str, err := utils.Decrypt(utils.RemoveComments(lines[0]), appfiles.Key); err == nil {
			lines = strings.Split(str, "\n")
			raw = []byte(str)
		}
	}
	return lines, raw, nil
}

// classFuncsOf 提取 Class 表里可序列化的 DicFuncs 部分。
func classFuncsOf(class map[string]*dto.DicClass) map[string]map[string][]*dto.BuildDic {
	if len(class) == 0 {
		return nil
	}
	out := make(map[string]map[string][]*dto.BuildDic, len(class))
	for name, c := range class {
		if c != nil {
			out[name] = c.DicFuncs
		}
	}
	return out
}

// rebuildBuildValue 从缓存条目重建 BuildValue。
// 缓存仅覆盖无 bot 注入的词库，故 MyFunc 与各 Class.Fn 均为空，直接用 NewDicClass 初始化。
func rebuildBuildValue(e *dicCacheEntry) *dto.BuildValue {
	result := &dto.BuildValue{
		Head:          e.Head,
		HeadLineNums:  e.HeadLineNums,
		Dic:           e.Dic,
		DicFuncs:      e.DicFuncs,
		Class:         make(map[string]*dto.DicClass),
		MyFunc:        make(map[string]dto.DicFunc),
		BotImports:    e.BotImports,
		Warnings:      e.Warnings,
		Resources:     e.Resources,
		OnceResources: e.OnceResources,
		Deps:          e.Deps,
	}
	for name, funcs := range e.ClassFuncs {
		cls := dto.NewDicClass()
		cls.DicFuncs = funcs
		result.Class[name] = cls
	}
	return result
}

// MarshalBuildValue 将编译产物序列化为 gob 字节，供打包进独立可执行文件。
// 仅纯本地词库（MyFunc 与各 Class.Fn 均为空）可序列化；含 bot 注入的自定义函数无法序列化。
func MarshalBuildValue(v *dto.BuildValue) ([]byte, error) {
	if v == nil {
		return nil, errors.New("编译产物为空")
	}
	if len(v.MyFunc) != 0 {
		return nil, errors.New("词库含自定义函数（bot 注入），暂不支持打包为可执行文件")
	}
	for name, c := range v.Class {
		if c != nil && len(c.Fn) != 0 {
			return nil, fmt.Errorf("类 %q 含自定义方法，暂不支持打包为可执行文件", name)
		}
	}
	e := &dicCacheEntry{
		Version:       dicCacheVersion,
		Head:          v.Head,
		HeadLineNums:  v.HeadLineNums,
		Dic:           v.Dic,
		DicFuncs:      v.DicFuncs,
		ClassFuncs:    classFuncsOf(v.Class),
		BotImports:    v.BotImports,
		Warnings:      v.Warnings,
		Resources:     v.Resources,
		OnceResources: v.OnceResources,
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(e); err != nil {
		return nil, fmt.Errorf("序列化编译产物失败: %w", err)
	}
	return buf.Bytes(), nil
}

// UnmarshalBuildValue 反序列化编译产物并重建 BuildValue（MyFunc 与 Class.Fn 为空）。
func UnmarshalBuildValue(data []byte) (*dto.BuildValue, error) {
	var e dicCacheEntry
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&e); err != nil {
		return nil, fmt.Errorf("解码编译产物失败: %w", err)
	}
	if e.Version != dicCacheVersion {
		return nil, fmt.Errorf("编译产物版本不匹配：期望 %d，实际 %d", dicCacheVersion, e.Version)
	}
	return rebuildBuildValue(&e), nil
}

// dicCacheMu 保护磁盘缓存写入，避免并发写坏缓存文件。
var dicCacheMu sync.Mutex

// loadDicCache 读取并校验磁盘编译缓存；命中则返回重建的 BuildValue，否则返回 nil。
func loadDicCache(dicPath, mainHash string) *dto.BuildValue {
	data, err := os.ReadFile(dicCachePath(dicPath))
	if err != nil {
		return nil
	}
	var e dicCacheEntry
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&e); err != nil {
		return nil
	}
	if e.Version != dicCacheVersion || e.Deps[dicPath] != mainHash {
		return nil
	}
	for dep, h := range e.Deps {
		if dep == dicPath {
			continue
		}
		_, raw, err := readDicFileContent(dep)
		if err != nil || dicHashBytes(raw) != h {
			return nil
		}
	}
	debugLog.Debugf("命中词库编译缓存：%v", dicPath)
	return rebuildBuildValue(&e)
}

// saveDicCache 将编译结果写入磁盘缓存。先在当前 goroutine 同步编码为字节
// （此时编译结果尚未对外可见，避免异步读取共享数据），再异步落盘，
// 避免磁盘 IO 阻塞词库加载。
func saveDicCache(dicPath string, e *dicCacheEntry) {
	// 启动阶段不写编译缓存，避免启动时自动创建 private/.dic_cache 目录
	if utils.InStartupMode() {
		return
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(e); err != nil {
		return
	}
	go writeDicCacheFile(dicPath, buf.Bytes())
}

// writeDicCacheFile 将编码后的缓存字节写入磁盘（临时文件 + 原子替换，并发安全）。
func writeDicCacheFile(dicPath string, data []byte) {
	dicCacheMu.Lock()
	defer dicCacheMu.Unlock()

	p := dicCachePath(dicPath)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return
	}
	if err := os.Rename(tmp, p); err != nil {
		return
	}
	debugLog.Debugf("已写入词库编译缓存：%v", dicPath)
}

// ClearDicCache 清空磁盘编译缓存目录，供程序启动时调用，
// 避免旧缓存（含已删除词库的残留缓存）在进程间累积。
func ClearDicCache() {
	_ = os.RemoveAll(filepath.Join(utils.GetAppDir(), "private", ".dic_cache"))
}

// injectBotFuncs 处理 @xxx 的编译期 bot 函数注入，命中返回注入的函数表（未命中返回 nil）。
func injectBotFuncs(path string, myFunc map[string]dto.DicFunc) map[string]dto.DicFunc {
	reg, ok := dto.BotFuncsRegistry[strings.TrimPrefix(path, "@")]
	if !ok {
		return nil
	}
	maps.Copy(myFunc, reg)
	return reg
}

// importPackage 加载一个 #引入= 目标（bot 注入或本地文件/目录），返回其包与是否目录。
func importPackage(dicPath, path, fHeaderName string, funcMap map[string][]*dto.BuildDic, classMap map[string]*dto.DicClass, myFunc map[string]dto.DicFunc, stack *importStack, lineNum int) (isDir bool, pkg *dto.DicClass) {
	if botFuncs := injectBotFuncs(path, myFunc); botFuncs != nil {
		pkg = dto.NewDicClass()
		maps.Copy(pkg.Fn, botFuncs)
		return false, pkg
	}
	return loadImport(dicPath, path, fHeaderName, funcMap, classMap, myFunc, stack, lineNum)
}

// loadImport 加载 #引入= 目标（本地文件或目录），将函数/类/自定义函数合并到目标，
// 并返回本次导入的全部函数组成的「包」（供变量:#引入= 的赋予值形式使用）。
// fHeaderName 非空时给「函数」触发词加前缀。isDir 表示目标是否为目录（目录形式不返回实例）。
func loadImport(dicPath, path, fHeaderName string, funcMap map[string][]*dto.BuildDic, classMap map[string]*dto.DicClass, myFunc map[string]dto.DicFunc, stack *importStack, lineNum int) (isDir bool, pkg *dto.DicClass) {
	dirName, isDir := strings.CutSuffix(path, "/*")
	pkg = dto.NewDicClass()

	var filesToLoad []string
	if isDir {
		dirPath := pathpkg.Join("private", dirName)
		fileLoad := utils.NewFileQueue(dirPath)
		if !fileLoad.DirExists() {
			debugLog.Infof("加载目录不存在：%v", dirPath)
			return isDir, pkg
		}
		filesToLoad2, err := fileLoad.GetFileList()
		if err != nil {
			return isDir, pkg
		}
		for i, v := range filesToLoad2 {
			filesToLoad2[i] = pathpkg.Join(dirPath, v)
		}
		filesToLoad = append(filesToLoad, filesToLoad2...)
	} else {
		if !strings.HasSuffix(path, ".n") {
			path += ".n"
		}
		filesToLoad = append(filesToLoad, pathpkg.Join("private", path))
	}

	for _, filePath := range filesToLoad {
		if !stack.push(filePath) {
			stack.addWarning(lineNum, "循环引入："+filePath+" 已在引入链中，跳过加载（由 "+dicPath+" 引入）")
			continue
		}

		FileData, raw, err := readDicFileContent(filePath)
		if err != nil {
			stack.pop(filePath)
			continue
		}
		// 记录依赖文件内容 hash，供磁盘编译缓存失效校验。
		stack.deps[filePath] = dicHashBytes(raw)

		z := buildDic(filePath, FileData, stack)
		stack.pop(filePath)

		if fHeaderName != "" {
			for _, value := range z.DicFuncs["函数"] {
				value.Trigger = fHeaderName + "." + value.Trigger
			}
		}
		for k, v := range z.DicFuncs {
			funcMap[k] = append(funcMap[k], v...)
			pkg.DicFuncs[k] = append(pkg.DicFuncs[k], v...)
		}
		maps.Copy(myFunc, z.MyFunc)
		maps.Copy(pkg.Fn, z.MyFunc)
		for key, value := range z.Class {
			if classMap[key] == nil {
				classMap[key] = value
			}
			for k, v := range value.DicFuncs {
				pkg.DicFuncs[k] = append(pkg.DicFuncs[k], v...)
			}
			maps.Copy(pkg.Fn, value.Fn)
		}
	}
	return isDir, pkg
}

// 运行网页词库
func Web(dicPath string, lines []string) *dto.BuildValue {
	return web(dicPath, lines, newImportStack())
}

// web 为 Web 的内部实现，携带引入链用于检测循环引入。
func web(dicPath string, lines []string, stack *importStack) *dto.BuildValue {

	var (
		// 多行注释
		zhushi   bool
		dicText  []string
		funcDict map[string][]*dto.BuildDic = make(map[string][]*dto.BuildDic)
		// Class
		classText map[string]*dto.DicClass = make(map[string]*dto.DicClass)
		// 缩进
		suojin bool
		// 自定义函数（含bot注入）
		myFunc map[string]dto.DicFunc = make(map[string]dto.DicFunc)
	)
	for i, line := range lines {
		if line != "" {
			if !suojin {
				line = strings.TrimLeft(line, " \t")
			}
		}
		lineLen := len(line)

		if zhushi {
			if lineLen >= 2 && line[lineLen-2:] == "*/" {
				zhushi = false
			}
			continue
		}
		if !zhushi && lineLen >= 2 && line[:2] == "/*" {
			zhushi = true
			continue
		}

		if lineLen > 2 && line[:2] == "//" {

			switch line {
			case "//@关闭缩进":
				suojin = true
			case "//@启用缩进":
				suojin = false
			}
			continue
		}

		if varName, path, ok := parseImportLine(line); ok {
			isDir, pkg := importPackage(dicPath, path, "", funcDict, classText, myFunc, stack, i+1)
			// 赋予值形式：变量:#引入=目标 → 导入全部函数组成包并返回实例
			if varName != "" && !isDir {
				classText[varName] = pkg
				dicText = append(dicText, varName+":$new "+varName+"$")
			}
			continue
		}
		dicText = append(dicText, line)
	}

	result := &dto.BuildValue{
		Head:     dicText,
		DicFuncs: funcDict,
		Class:    classText,
		MyFunc:   myFunc,
		Warnings: stack.warnings,
	}
	return result
}

func BuildDic(dicPath, text string) *dto.BuildValue {
	return BuildDicLinesWithRaw(dicPath, strings.Split(text, "\n"), []byte(text))
}

// BuildDicLinesWithRaw 以已切分的行与原始内容字节编译词库；raw 用于计算内容 hash（缓存键），
// 供已一次性读入文件的调用方直接使用，避免重新拼接整块文本。
func BuildDicLinesWithRaw(dicPath string, lines []string, raw []byte) *dto.BuildValue {
	return buildDicWithHash(dicPath, lines, dicHashBytes(raw))
}

// BuildDicLinesWithRawNoCache 与 BuildDicLinesWithRaw 相同，但不写磁盘缓存。
// 供「编译检测」等仅需诊断信息、无需缓存加速的场景使用，避免每次保存/打开都产生缓存文件。
func BuildDicLinesWithRawNoCache(dicPath string, lines []string, raw []byte) *dto.BuildValue {
	return buildDicWithHashMode(dicPath, lines, dicHashBytes(raw), false)
}

// buildDicWithHash 为编译入口的内部实现，携带引入链用于检测循环引入，并按内容 hash 校验磁盘缓存。
func buildDicWithHash(dicPath string, lines []string, mainHash string) *dto.BuildValue {
	return buildDicWithHashMode(dicPath, lines, mainHash, true)
}

// buildDicWithHashMode 编译核心；writeCache 为 false 时跳过写缓存（读缓存不受影响）。
func buildDicWithHashMode(dicPath string, lines []string, mainHash string, writeCache bool) *dto.BuildValue {
	stack := newImportStack()
	// 顶层词库同样压入引入链，路径与 #引入= 加载路径保持一致（统一 private/ 前缀与 .n 后缀），
	// 避免被引入文件反向引入顶层时把顶层重复加载，导致同一条循环引入被重复报告。
	dicPath = importFilePath(dicPath)
	stack.push(dicPath)
	stack.deps[dicPath] = mainHash

	// 词库编译缓存由服务器配置开关控制（默认关闭），关闭时跳过读写
	cacheEnabled := dto.ServerConfig.DicCache

	if cacheEnabled {
		if cached := loadDicCache(dicPath, mainHash); cached != nil {
			return cached
		}
	}

	result := buildDic(dicPath, lines, stack)
	result.Deps = stack.deps

	// 含 bot 注入的词库（MyFunc 非空）不落缓存，避免序列化 Go 函数；其余词库写缓存加速后续加载。
	if writeCache && cacheEnabled && len(result.MyFunc) == 0 {
		saveDicCache(dicPath, &dicCacheEntry{
			Version:       dicCacheVersion,
			Deps:          stack.deps,
			Head:          result.Head,
			HeadLineNums:  result.HeadLineNums,
			Dic:           result.Dic,
			DicFuncs:      result.DicFuncs,
			ClassFuncs:    classFuncsOf(result.Class),
			BotImports:    result.BotImports,
			Warnings:      result.Warnings,
			Resources:     result.Resources,
			OnceResources: result.OnceResources,
		})
	}

	return result
}

// importFilePath 将 #引入= 目标或顶层词库路径规范化为统一的文件路径（private/xxx.n）。
func importFilePath(name string) string {
	// 统一分隔符为 /，兼容 Windows 下 filepath.Join 产生的反斜杠，
	// 避免 private\... 匹配不到 private/ 前缀而被重复拼接（如 private/private\...）。
	name = filepath.ToSlash(name)
	if !strings.HasPrefix(name, "private/") {
		name = pathpkg.Join("private", name)
	}
	if !strings.HasSuffix(name, ".n") {
		name += ".n"
	}
	return name
}

// buildDic 为 BuildDic 的内部实现，携带引入链用于检测循环引入。
func buildDic(dicPath string, lines []string, stack *importStack) *dto.BuildValue {
	lines_num := len(lines) - 1

	var (
		// 触发变量
		dicTrigger     string // 触发词
		dicTriggerLine int    // 触发词所在原始文件行号（1-based）

		// 词库条目
		dicText         []*dto.BuildDic // 词库条目
		dicTexts        []string        // 准备添加到词库中的词条
		dicTextLineNums []int           // 词条每行对应的原始文件行号（1-based）

		// 内部状态变量
		neibu bool

		// 特殊触发
		special       string
		buildCategory string // 当前触发的类别：函数/内部/特殊事件名

		// 插件变量
		chajian     bool
		chajianText map[string][]*dto.BuildDic = make(map[string][]*dto.BuildDic) // 统一函数（函数/内部/特殊）

		// 头部变量
		runheadtext     []string // 头部文本条目
		runheadLineNums []int    // 头部每行对应的原始文件行号（1-based）
		runhead         bool

		// 多行注释标志
		zhushi bool

		// 多行词条标志
		duohang bool

		// 词库类
		classText map[string]*dto.DicClass = make(map[string]*dto.DicClass)

		classN string // 当前触发所属类名（[:类名]）

		fRunAll bool // 函数框选

		suojin bool // 缩进

		fHeaderName string // 函数头部名称

		fParamRule string // 当前函数触发的参数数量规则（[函数|规则]）

		// 函数上方 // 注释收集（作为函数说明）与当前函数说明
		pendingComment  []string
		currentFuncDesc string

		// 自定义函数（含bot注入）
		myFunc map[string]dto.DicFunc = make(map[string]dto.DicFunc)

		// 编译期资源变量（//@资源 变量名:路径）
		resources map[string]any = make(map[string]any)

		// 一次性资源变量名集合（//@一次性资源 变量名:路径），内容仍存 resources
		onceResources map[string]bool = make(map[string]bool)

		// 多行 //@资源 / //@一次性资源 块：指令单独一行后，后续「变量名:路径」行逐个声明资源，遇空行/注释结束
		resourceBlock bool
		resourceOnce  bool
	)

	// 头部区域：文件开头到第一个空行（或注释行）之间为头部（#引入= 与初始化语句），
	// 空行/注释行之后为正文；无空行或注释分隔时文件直接按正文解析（如被引入的 [函数] 文件）。
	runhead = false
	for i, l := range lines {
		if isBlankOrComment(l) {
			runhead = i > 0
			break
		}
	}

	for dic_i, line := range lines {
		if line != "" {
			if !suojin {
				line = strings.TrimLeft(line, " \t")
			}
		}
		lineLen := len(line)

		if zhushi {
			if lineLen >= 2 && line[lineLen-2:] == "*/" {
				zhushi = false
				// 结束行：收集 */ 之前的内容
				if content := strings.TrimSpace(line[:lineLen-2]); content != "" {
					pendingComment = append(pendingComment, content)
				}
			} else if content := strings.TrimSpace(line); content != "" {
				// 块注释中间行
				pendingComment = append(pendingComment, content)
			}
			continue
		}
		if !zhushi && lineLen >= 2 && line[:2] == "/*" {
			runhead = false
			if idx := strings.Index(line[2:], "*/"); idx >= 0 {
				// 单行块注释 /* ... */
				if content := strings.TrimSpace(line[2 : 2+idx]); content != "" {
					pendingComment = append(pendingComment, content)
				}
			} else {
				// 多行块注释开始
				zhushi = true
				if content := strings.TrimSpace(line[2:]); content != "" {
					pendingComment = append(pendingComment, content)
				}
			}
			continue
		}

		// 多行 //@资源 / //@一次性资源 块：指令单独一行后，后续「变量名:路径」行逐个声明资源，遇空行/注释/正文结束
		if resourceBlock {
			if line == "" || strings.HasPrefix(line, "//") {
				resourceBlock = false
			} else if name, rel, ok := parseResourceVar(line); ok {
				loadResource(name, rel, resources, stack, dic_i+1)
				if resourceOnce {
					onceResources[name] = true
				}
				continue
			} else {
				resourceBlock = false
			}
		}

		if lineLen > 2 && line[:2] == "//" {
			switch line {
			case "//@关闭缩进":
				suojin = true
			case "//@启用缩进":
				suojin = false
			}
			if lineLen > 10 && line[:10] == "//@打印=" {
				debugLog.Infof("[%v]%v", dicPath, line[10:])
			}
			if lineLen > 13 && line[:13] == "//@函数头=" {
				fHeaderName = line[13:]
			}
			if strings.TrimSpace(line) == "//@资源" {
				// //@资源 单独一行：开启多行资源块，后续「变量名:路径」行逐个声明资源
				resourceBlock = true
				resourceOnce = false
			}
			if strings.TrimSpace(line) == "//@一次性资源" {
				// //@一次性资源 单独一行：开启多行一次性资源块，读取一次后销毁
				resourceBlock = true
				resourceOnce = true
			}
			// 普通 // 注释（非 @ 指令）：收集为函数上方的说明，并视作空行分隔头部与正文
			if !strings.HasPrefix(line, "//@") {
				pendingComment = append(pendingComment, strings.TrimSpace(line[2:]))
				runhead = false
			}
			continue
		}

		if runhead {
			if varName, path, ok := parseImportLine(line); ok {
				isDir, pkg := importPackage(dicPath, path, fHeaderName, chajianText, classText, myFunc, stack, dic_i+1)
				// 赋予值形式：变量:#引入=目标 → 导入全部函数组成包并返回实例
				if varName != "" && !isDir {
					classText[varName] = pkg
					runheadtext = append(runheadtext, varName+":$new "+varName+"$")
					runheadLineNums = append(runheadLineNums, dic_i+1)
				}
				continue
			}

			if line == "" {
				runhead = false
				continue
			}
			runheadtext = append(runheadtext, line)
			runheadLineNums = append(runheadLineNums, dic_i+1)
			continue
		}

		// 如果检测词条不等于空
		if line != "" || (line == "" && duohang) || (line == "" && fRunAll) {
			// 没有触发文本变量不是空就添加
			if dicTrigger != "" {

				if fRunAll {
					if line == "}#" {
						fRunAll = false
					} else {
						dicTexts = append(dicTexts, line)
						dicTextLineNums = append(dicTextLineNums, dic_i+1)
					}
				} else {

					if !duohang && line == "<?n" {
						duohang = true
						continue
					}

					if duohang && line == "?>" {
						duohang = false
					} else {
						dicTexts = append(dicTexts, line)
						dicTextLineNums = append(dicTextLineNums, dic_i+1)
					}
				}
			} else {
				// 判断触发为空就执行记录
				dicTrigger = line
				dicTriggerLine = dic_i + 1

				switch category, class, param, rest := parseTriggerPrefix(line); category {
				case "函数":
					chajian = true
					classN = class
					buildCategory = "函数"
					fParamRule = param
					if fHeaderName != "" {
						dicTrigger = fHeaderName + rest
					} else {
						dicTrigger = rest
					}
				case "内部":
					neibu = true
					classN = class
					buildCategory = "内部"
					dicTrigger = rest
				case "":
				default:
					special = category
					classN = class
					buildCategory = category
					dicTrigger = rest
				}

				// 函数说明：仅 [函数] 触发词关联其上方的 // 注释，其余类别清空
				if buildCategory == "函数" {
					currentFuncDesc = strings.Join(pendingComment, "\n")
				} else {
					currentFuncDesc = ""
				}
				pendingComment = nil

				if strings.HasSuffix(dicTrigger, " #{") {
					fRunAll = true
					dicTrigger = dicTrigger[:len(dicTrigger)-3]
				}

			}
		}

		if dicTrigger != "" {

			if line == "" && fRunAll {
				continue
			}

			if line == "" && duohang {
				continue
			}

			if line == "" || dic_i == lines_num {
				json := &dto.BuildDic{
					Trigger:     dicTrigger,
					Text:        dicTexts,
					LineNums:    dicTextLineNums,
					ParamRule:   fParamRule,
					Desc:        currentFuncDesc,
					TriggerLine: dicTriggerLine,
				}
				if neibu {
					neibu = false
					if classN != "" {
						if classText[classN] == nil {
							classText[classN] = dto.NewDicClass()
						}
						classText[classN].DicFuncs[buildCategory] = append(classText[classN].DicFuncs[buildCategory], json)
					} else {
						chajianText[buildCategory] = append(chajianText[buildCategory], json)
					}
				} else if chajian {
					chajian = false
					if classN != "" {
						if classText[classN] == nil {
							classText[classN] = dto.NewDicClass()
						}
						classText[classN].DicFuncs[buildCategory] = append(classText[classN].DicFuncs[buildCategory], json)
					} else {
						chajianText[buildCategory] = append(chajianText[buildCategory], json)
					}
				} else if special != "" {
					special = ""
					if classN != "" {
						if classText[classN] == nil {
							classText[classN] = dto.NewDicClass()
						}
						classText[classN].DicFuncs[buildCategory] = append(classText[classN].DicFuncs[buildCategory], json)
					} else {
						chajianText[buildCategory] = append(chajianText[buildCategory], json)
					}
				} else {
					dicText = append(dicText, json)
				}
				dicTrigger = ""
				dicTriggerLine = 0
				classN = ""
				buildCategory = ""
				fParamRule = ""
				currentFuncDesc = ""
				dicTexts = nil
				dicTextLineNums = nil
			}
		}

	}
	result := &dto.BuildValue{
		Head:          runheadtext,
		HeadLineNums:  runheadLineNums,
		Dic:           dicText,
		DicFuncs:      chajianText,
		Class:         classText,
		MyFunc:        myFunc,
		Resources:     resources,
		OnceResources: onceResources,
	}

	// 编译期静态检查：框配对/嵌套、触发词正则语法、函数参数数量。
	// 统一追加到 stack.warnings，最后再赋给 result，保证共享的引入链警告切片一致。
	runCompileChecks(result, stack)
	result.Warnings = stack.warnings

	// 打印普通json
	// dd, derr := utils.Json.MarshalIndent(result, "", "  ")
	// if derr != nil {
	// 	fmt.Println("JSON 序列化失败:", derr)
	// } else {
	// 	fmt.Println(string(dd))
	// }

	return result
}
