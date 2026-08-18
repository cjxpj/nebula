package run

import (
	"maps"
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

// 处理函数
func BuildFuncStr(
	str string,
	process func([]string) (string, bool), // 处理函数文本
	process2 func(string) (string, bool), // 处理外部文本，原样给你，不做转义
) string {
	var result strings.Builder
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

		// 外部文本（不做转义）
		if outside := str[start:openIndex]; len(outside) > 0 {
			if out, stop := process2(outside); stop {
				break
			} else {
				result.WriteString(out)
			}
		}

		// 内部文本：按空格分割，\ 处理转义（\ , \\, \$）
		content := str[openIndex+1 : closeIndex]
		args := splitWithEscape(content)
		if in, stop := process(args); stop {
			break
		} else {
			result.WriteString(in)
		}

		start = closeIndex + 1
	}

	// 余下外部文本
	if outside := str[start:]; len(outside) > 0 {
		out, _ := process2(outside)
		result.WriteString(out)
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

// 在 $...$ 内部：按空格切分；支持 "\\"=>"\", "\$"=>"$"
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
			} else {
				// 开启引号：前是空格或行首
				if i == 0 || s[i-1] == ' ' {
					inQuote = true
					continue
				}
			}
			// 非边界位置的引号 → 字面字符
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

// replaceProcessedContent 接受一个字符串、开始和结束的子串，以及一个处理函数作为参数
func ReplaceProcessedContent(str, strStart, strEnd string, process func(string) string) string {
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

// RunFunc 匹配 [函数] 词条：函数名精确匹配 + 参数数量校验（不使用正则）。
// 未声明参数规则时默认 0 个参数。返回函数正文、函数名、变量传出列表（-> 后缀）、
// 参数规则（命中名称但数量不符时非空）以及是否命中。
func RunFunc(jsonData []*dto.BuildDic, callName string, paramCount int) (text []string, name string, tparts string, errRule string, ok bool) {
	for _, item := range jsonData {
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
		return "", strings.TrimSpace(line[len("#引入="):]), true
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

// importStack 记录当前 #引入= 递归加载链上的文件路径，用于检测循环引入。
// 加载为单 goroutine 递归，无需加锁。
type importStack struct {
	files    map[string]bool
	warnings []dto.BuildWarning // 收集编译警告（如循环引入），供前端调试面板展示
}

// newImportStack 创建空的引入链。
func newImportStack() *importStack {
	return &importStack{files: make(map[string]bool)}
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

// addWarning 追加一条编译警告。
func (s *importStack) addWarning(line int, text string) {
	s.warnings = append(s.warnings, dto.BuildWarning{Line: line, Text: text})
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
		dirPath := "private/" + dirName
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
			filesToLoad2[i] = dirPath + "/" + v
		}
		filesToLoad = append(filesToLoad, filesToLoad2...)
	} else {
		if !strings.HasSuffix(path, ".n") {
			path += ".n"
		}
		filesToLoad = append(filesToLoad, "private/"+path)
	}

	for _, filePath := range filesToLoad {
		if !stack.push(filePath) {
			stack.addWarning(lineNum, "循环引入："+filePath+" 已在引入链中，跳过加载（由 "+dicPath+" 引入）")
			continue
		}
		file := utils.NewFile()
		file.SetPath(filePath)

		FileData, err := file.ReadFromFile()
		if err != nil {
			stack.pop(filePath)
			continue
		}
		if str, err := utils.Decrypt(utils.RemoveComments(FileData), appfiles.Key); err == nil {
			FileData = str
		}

		z := buildDic(dicPath, FileData, stack)
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
	return buildDic(dicPath, text, newImportStack())
}

// buildDic 为 BuildDic 的内部实现，携带引入链用于检测循环引入。
func buildDic(dicPath, text string, stack *importStack) *dto.BuildValue {
	// 词条总数据
	// 现在可以安全地使用\n作为分隔符
	lines := strings.Split(text, "\n")

	lines_num := len(lines) - 1

	var (
		// 触发变量
		dicTrigger string // 触发词

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

		// 自定义函数（含bot注入）
		myFunc map[string]dto.DicFunc = make(map[string]dto.DicFunc)
	)

	// 头部区域：文件开头到第一个空行之间为头部（#引入= 与初始化语句），
	// 空行之后为正文；无空行分隔时文件直接按正文解析（如被引入的 [函数] 文件）。
	runhead = false
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
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
			if lineLen > 10 && line[:10] == "//@打印=" {
				debugLog.Infof("[%v]%v", dicPath, line[10:])
			}
			if lineLen > 13 && line[:13] == "//@函数头=" {
				fHeaderName = line[13:]
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
					Trigger:   dicTrigger,
					Text:      dicTexts,
					LineNums:  dicTextLineNums,
					ParamRule: fParamRule,
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
				classN = ""
				buildCategory = ""
				fParamRule = ""
				dicTexts = nil
				dicTextLineNums = nil
			}
		}

	}
	result := &dto.BuildValue{
		Head:         runheadtext,
		HeadLineNums: runheadLineNums,
		Dic:          dicText,
		DicFuncs:     chajianText,
		Class:        classText,
		MyFunc:       myFunc,
		Warnings:     stack.warnings,
	}

	// 打印普通json
	// dd, derr := utils.Json.MarshalIndent(result, "", "  ")
	// if derr != nil {
	// 	fmt.Println("JSON 序列化失败:", derr)
	// } else {
	// 	fmt.Println(string(dd))
	// }

	// if t.Cache && t.Uid != "" {
	// 	dto.GV.Set("cache_"+t.Uid, result)
	// }

	return result
}
