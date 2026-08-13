package run

import (
	"bufio"
	"fmt"
	"io"
	"maps"
	"regexp"
	"strings"
	"sync"

	"github.com/cjxpj/nebula/appfiles"
	dicBuild "github.com/cjxpj/nebula/build"
	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

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
		if out, _ := process2(outside); true {
			result.WriteString(out)
		}
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

func SplitFuncString(input, delimiterStart, delimiterEnd string) []string {
	var result []string
	start := 0

	for start < len(input) {
		// 查找起始分隔符的位置
		openIndex := strings.Index(input[start:], delimiterStart)
		if openIndex == -1 {
			// 如果没有找到起始分隔符，直接添加剩余的部分
			remaining := strings.Fields(input[start:])
			result = append(result, remaining...)
			break
		}
		openIndex += start

		// 添加起始分隔符之前的部分
		parts := strings.Fields(input[start:openIndex])
		result = append(result, parts...)

		// 查找结束分隔符的位置，处理嵌套情况
		nestedLevel := 1
		closeIndex := openIndex + len(delimiterStart)

		for nestedLevel > 0 && closeIndex < len(input) {
			nextOpenIndex := strings.Index(input[closeIndex:], delimiterStart)
			nextCloseIndex := strings.Index(input[closeIndex:], delimiterEnd)

			if nextCloseIndex == -1 {
				// 如果找不到结束分隔符，直接添加剩余的部分
				remaining := strings.Fields(input[start:])
				result = append(result, remaining...)
				return result
			}

			if nextOpenIndex != -1 && nextOpenIndex < nextCloseIndex {
				// 找到嵌套的起始分隔符
				nestedLevel++
				closeIndex += nextOpenIndex + len(delimiterStart)
			} else {
				// 找到结束分隔符
				nestedLevel--
				closeIndex += nextCloseIndex + len(delimiterEnd)
			}
		}

		// 将起始分隔符和结束分隔符之间的内容连同前面的内容作为整体添加到结果中
		result[len(result)-1] += input[openIndex:closeIndex]

		// 更新开始位置
		start = closeIndex
	}

	return result
}

func ReplaceProcessedsContent(str, strStart, strEnd string, process func(string) string) string {
	var result strings.Builder
	start := 0

	for start < len(str) {
		// 查找开始子串的位置
		openIndex := strings.Index(str[start:], strStart)
		if openIndex == -1 {
			// 如果找不到开始标记，添加剩余的部分到结果字符串
			result.WriteString(str[start:])
			break
		}
		openIndex += start

		// 查找结束子串的位置，处理嵌套
		nestedLevel := 1
		closeIndex := openIndex + len(strStart)

		for nestedLevel > 0 && closeIndex < len(str) {
			nextOpenIndex := strings.Index(str[closeIndex:], strStart)
			nextCloseIndex := strings.Index(str[closeIndex:], strEnd)

			if nextCloseIndex == -1 {
				// 如果找不到结束标记，添加剩余的部分到结果字符串
				result.WriteString(str[start:])
				return result.String()
			}

			if nextOpenIndex != -1 && nextOpenIndex < nextCloseIndex {
				// 找到嵌套的开始标记
				nestedLevel++
				closeIndex += nextOpenIndex + len(strStart)
			} else {
				// 找到结束标记
				nestedLevel--
				closeIndex += nextCloseIndex + len(strEnd)
			}
		}

		// 添加从开始到当前开始标记之前的内容到结果字符串
		result.WriteString(str[start:openIndex])

		// 提取标记之间的内容并处理
		content := str[openIndex+len(strStart) : closeIndex-len(strEnd)]
		processedContent := process(content)

		// 将处理后的内容添加到结果字符串
		result.WriteString(processedContent)

		// 更新开始位置为结束标记之后
		start = closeIndex
	}

	return result.String()
}

// 遍历触发词文本
func GetTriggerCodeBlock(jsonData []*dto.RegDicLine, trigger string, runNum int) ([]string, string, int, *regexp.Regexp) {
	jsonDataLen := len(jsonData)

	if runNum > jsonDataLen {
		return nil, "", 0, nil
	}

	// 遍历每个条目并输出
	for i := runNum; i < jsonDataLen; i++ {
		item := jsonData[i]
		text := item.CodeBloack

		// 使用动态编译的正则表达式
		t := item.Trigger

		regex, err := regexp.Compile("^(" + t + ")$")
		if err != nil {
			continue
		}
		if regex.MatchString(trigger) {
			return text, t, i, regex
		}
	}

	return nil, "", 0, nil
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

		regex, err := regexp.Compile("^(" + t + ")$")
		if err != nil {
			continue
		}
		if regex.MatchString(trigger) {
			return text, t, i, regex
		}
	}

	return nil, "", 0, nil
}

// 遍历触发词文本
func RunFors(jsonData []*dto.BuildDic, trigger string, runNum int) ([]string, string, int, *regexp.Regexp, string) {
	jsonDataLen := len(jsonData)

	if runNum > jsonDataLen {
		return nil, "", 0, nil, ""
	}

	// 遍历每个条目并输出
	for i := runNum; i < jsonDataLen; i++ {
		item := jsonData[i]
		text := item.Text

		// 使用动态编译的正则表达式
		t := item.Trigger
		resF := ""

		tindex := strings.LastIndex(t, "->")
		if tindex != -1 {
			resF = t[tindex+2:]
			t = t[:tindex]
		}

		regex, err := regexp.Compile("^(" + t + ")$")
		if err != nil {
			continue
		}
		if regex.MatchString(trigger) {
			return text, t, i, regex, resF
		}
	}

	return nil, "", 0, nil, ""
}

// parseTriggerPrefix 解析触发词行首的 [] 前缀。
// 返回触发类别（空串表示普通触发）、整合包类名（可为空）和去掉前缀后的触发词。
// [F]/[L] 缩写自动归一为 [函数]/[内部]；[:类名] 后缀表示整合包，如 [函数:a]、[F:a]。
func parseTriggerPrefix(line string) (category, class, rest string) {
	if !strings.HasPrefix(line, "[") {
		return "", "", line
	}
	end := strings.Index(line, "]")
	if end <= 1 {
		return "", "", line
	}
	tag := line[1:end]
	rest = line[end+1:]
	if i := strings.Index(tag, ":"); i >= 0 {
		category, class = tag[:i], tag[i+1:]
	} else {
		category = tag
	}
	switch category {
	case "F", "f":
		category = "函数"
	case "L", "l":
		category = "内部"
	}
	return category, class, rest
}

// 运行网页词库
func Web(dicPath string, lines []string) *dto.BuildValue {

	var (
		// 多行注释
		zhushi   bool
		dicText  []string
		funcDict map[string][]*dto.BuildDic = make(map[string][]*dto.BuildDic)
		// 整合包
		classText map[string]*dto.DicClass = make(map[string]*dto.DicClass)
		// 缩进
		suojin bool
		// 自定义函数（含bot注入）
		myFunc map[string]dto.DicFunc = make(map[string]dto.DicFunc)
	)
	for _, line := range lines {
		if line != "" {
			if !suojin {
				line = strings.TrimLeft(line, " ")
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

		if lineLen > 8 && line[:8] == "#引入=" {
			path := strings.TrimSpace(line[8:])

			// @NapCat / @QQBot 编译期注入函数，不进入词行
			switch path {
			case "@NapCat":
				maps.Copy(myFunc, dto.BotFuncsRegistry["NapCat"])
				continue
			case "@QQBot":
				maps.Copy(myFunc, dto.BotFuncsRegistry["QQBot"])
				continue
			}

			var filesToLoad []string

			// 判断是目录还是文件（Web）
			if dirName, ok := strings.CutSuffix(path, "/*"); ok {
				dirPath := "private/" + dirName
				fileLoad := utils.NewFileQueue(dirPath)
				if !fileLoad.DirExists() {
					debugLog.Infof("加载目录不存在：%v", dirPath)
					continue
				}
				filesToLoad2, err := fileLoad.GetFileList()
				if err != nil {
					continue
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

			// 依次加载文件
			for _, filePath := range filesToLoad {
				// fmt.Println("加载文件：", filePath)

				file := utils.NewFile()
				file.SetPath(filePath)

				FileData, err := file.ReadFromFile()
				if err != nil {
					continue
				}

				if str, err := utils.Decrypt(utils.RemoveComments(FileData), appfiles.Key); err == nil {
					FileData = str
				}

				z := BuildDic(dicPath, FileData)
				for k, v := range z.DicFuncs {
					funcDict[k] = append(funcDict[k], v...)
				}
				maps.Copy(myFunc, z.MyFunc)
				for key, value := range z.LocalClass {
					if classText[key] == nil {
						classText[key] = value
					}
				}
			}
			continue
		}
		dicText = append(dicText, line)
	}

	result := &dto.BuildValue{
		Head:       dicText,
		DicFuncs:   funcDict,
		LocalClass: classText,
		MyFunc:     myFunc,
	}
	return result
}

func BuildDic(dicPath, text string) *dto.BuildValue {
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

		// 自定义函数（含bot注入）
		myFunc map[string]dto.DicFunc = make(map[string]dto.DicFunc)
	)

	if lines[0] != "" {
		runhead = true
	}

	for dic_i, line := range lines {
		if line != "" {
			if !suojin {
				line = strings.TrimLeft(line, " ")
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
			if lineLen > 8 && line[:8] == "#引入=" {
				path := strings.TrimSpace(line[8:])

				// @NapCat / @QQBot 编译期注入函数，不进入词行
				switch path {
				case "@NapCat":
					maps.Copy(myFunc, dto.BotFuncsRegistry["NapCat"])
					continue
				case "@QQBot":
					maps.Copy(myFunc, dto.BotFuncsRegistry["QQBot"])
					continue
				}

				var filesToLoad []string

				// 判断是目录还是文件（BuildDic）
				if dirName, ok := strings.CutSuffix(path, "/*"); ok {
					dirPath := "private/" + dirName
					fileLoad := utils.NewFileQueue(dirPath)
					if !fileLoad.DirExists() {
						debugLog.Infof("加载目录不存在：%v", dirPath)
						continue
					}
					filesToLoad2, err := fileLoad.GetFileList()
					if err != nil {
						continue
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

				// 依次加载文件（只从本地，不支持Gitee等远程）
				for _, filePath := range filesToLoad {
					// fmt.Println("加载文件：", filePath)

					file := utils.NewFile()
					file.SetPath(filePath)

					FileData, err := file.ReadFromFile()
					if err != nil {
						continue
					}

					if str, err := utils.Decrypt(utils.RemoveComments(FileData), appfiles.Key); err == nil {
						FileData = str
					}

					z := BuildDic(dicPath, FileData)
					if fHeaderName != "" {
						for _, value := range z.DicFuncs["函数"] {
							value.Trigger = fHeaderName + "." + value.Trigger
						}
					}

					for k, v := range z.DicFuncs {
						chajianText[k] = append(chajianText[k], v...)
					}
					maps.Copy(myFunc, z.MyFunc)
					for key, value := range z.LocalClass {
						if classText[key] == nil {
							classText[key] = value
						}
					}
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

				switch category, class, rest := parseTriggerPrefix(line); category {
				case "函数":
					chajian = true
					classN = class
					buildCategory = "函数"
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
					Trigger:  dicTrigger,
					Text:     dicTexts,
					LineNums: dicTextLineNums,
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
		LocalClass:   classText,
		MyFunc:       myFunc,
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

func parseParams(parts []string) []dto.Param {
	params := make([]dto.Param, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		if name, def, ok := strings.Cut(p, "="); ok {
			params = append(params, dto.Param{
				Name:    name,
				Default: def,
			})
		} else {
			params = append(params, dto.Param{
				Name:    p,
				Default: "",
			})
		}
	}
	return params
}

func Parse(dicPath string, reader io.Reader) *dto.DicInfoData {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	var (
		dicTrigger string
		dicTexts   dto.DicLine

		dicText         []*dto.RegDicLine
		dicTextLineNums []int
		funcFn          map[string]*dto.BuildDicFunc = make(map[string]*dto.BuildDicFunc)

		dicValue = dto.NewVal()
		// []*dto.DicInfo

		neibu   bool
		chajian bool
		special string

		runhead         = true
		runheadtext     dto.DicLine
		runheadLineNums []int

		zhushi  bool
		duohang bool
		fRunAll bool
		suojin  bool

		classText = make(map[string]*dto.DicClassInfo)
		classN    string

		fHeaderName string
	)

	commit := func() {
		if dicTrigger == "" {
			return
		}

		/* ===== 函数 ===== */
		if chajian {
			chajian = false

			parts := strings.Split(dicTrigger, " ")
			if len(parts) == 0 {
				dicTrigger = ""
				classN = ""
				dicTexts = nil
				dicTextLineNums = nil
				return
			}

			fn := &dto.BuildDicFunc{
				Name:   parts[0],
				Params: parseParams(parts[1:]),
				Text:   dicTexts,
				Func:   nil,
			}

			if classN != "" {
				if classText[classN] == nil {
					classText[classN] = &dto.DicClassInfo{
						LocalValue: dto.NewVal(),
						DicFuncs:   make(map[string]*dto.BuildDicFunc),
					}
				}
				classText[classN].DicFuncs[fn.Name] = fn
			} else {
				funcFn[fn.Name] = fn
			}

			dicTrigger = ""
			classN = ""
			dicTexts = nil
			dicTextLineNums = nil
			return
		}

		/* ===== 普通 ===== */

		item := &dto.RegDicLine{
			Trigger:    dicTrigger,
			CodeBloack: dicTexts,
		}

		// 内部/特殊已并入统一函数，遗留解析不再单独收集
		if neibu {
			neibu = false
		} else if special != "" {
			special = ""
		} else {
			dicText = append(dicText, item)
		}

		dicTrigger = ""
		classN = ""
		dicTexts = nil
		dicTextLineNums = nil
	}

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		lineLen := len(line)

		if line != "" && !suojin {
			line = strings.TrimLeft(line, "\t")
		}

		/* ===== 多行注释 ===== */

		if zhushi {
			if lineLen >= 2 && line[lineLen-2:] == "*/" {
				zhushi = false
			}
			continue
		}
		if strings.HasPrefix(line, "/*") {
			zhushi = true
			continue
		}

		/* ===== 单行注释 / 指令 ===== */

		if strings.HasPrefix(line, "//") {
			switch line {
			case "//@关闭缩进":
				suojin = true
			case "//@启用缩进":
				suojin = false
			}

			if sendMsg, ok := strings.CutPrefix(line, "//@打印="); ok {
				fmt.Println("["+dicPath+"]", sendMsg)
			}

			if fRunAll, ok := strings.CutPrefix(line, "//@函数头="); ok && fRunAll != "" {
				fHeaderName = fRunAll
			}
			continue
		}

		/* ===== 头部 ===== */

		if runhead {
			// #引入= 是预编译指令，不进入词行
			if lineLen > 8 && line[:8] == "#引入=" {
				path := strings.TrimSpace(line[8:])
				switch path {
				case "@NapCat", "@QQBot":
					continue
				}
			}
			if vType, vPrefix, vSuffix := dicBuild.ValTextTest(line); vType == 6 {
				if packPath, ok := strings.CutPrefix(vSuffix, "#引入="); ok && packPath != "" {
					var files []string
					var wg sync.WaitGroup
					// 判断是目录还是文件（Parse）
					if dirName, ok := strings.CutSuffix(packPath, "/*"); ok {
						dir := "private/" + dirName
						fq := utils.NewFileQueue(dir)
						if fq.DirExists() {
							list, _ := fq.GetFileList()
							for _, v := range list {
								files = append(files, dir+"/"+v)
							}
						}
					} else {
						if !strings.HasSuffix(packPath, ".n") {
							packPath += ".n"
						}
						files = append(files, "private/"+packPath)
					}
					for _, fp := range files {
						wg.Add(1)
						go func() {
							defer wg.Done()
							data, err := utils.NewFileQueue(fp).OpenFile()
							if err != nil {
								return
							}
							defer data.Close()
							sub := Parse(fp, data)
							dicValue.Set(vPrefix, sub)
						}()
					}
					wg.Wait()
					continue
				}
			}

			if line == "" {
				runhead = false
				continue
			}
			runheadtext = append(runheadtext, line)
			runheadLineNums = append(runheadLineNums, lineNum)
			continue
		}

		/* ===== 正文 ===== */

		if dicTrigger != "" {
			if fRunAll {
				if line == "}#" {
					fRunAll = false
				} else {
					dicTexts = append(dicTexts, line)
					dicTextLineNums = append(dicTextLineNums, lineNum)
				}
				continue
			}

			if !duohang && line == "<?n" {
				duohang = true
				continue
			}

			if duohang {
				if line == "?>" {
					duohang = false
				} else {
					dicTexts = append(dicTexts, line)
					dicTextLineNums = append(dicTextLineNums, lineNum)
				}
				continue
			}

			if line == "" {
				commit()
				continue
			}

			dicTexts = append(dicTexts, line)
			dicTextLineNums = append(dicTextLineNums, lineNum)
			continue
		}

		if line == "" {
			continue
		}

		/* ===== 新触发 ===== */

		dicTrigger = line

		switch category, class, rest := parseTriggerPrefix(line); category {
		case "函数":
			chajian = true
			classN = class
			dicTrigger = rest
		case "内部":
			neibu = true
			classN = class
			dicTrigger = rest
		case "":
		default:
			special = category
			classN = class
			dicTrigger = rest
		}

		if fHeaderName != "" && chajian {
			dicTrigger = fHeaderName + "." + dicTrigger
		}

		if strings.HasSuffix(dicTrigger, " #{") {
			fRunAll = true
			dicTrigger = dicTrigger[:len(dicTrigger)-3]
		}
	}

	if err := scanner.Err(); err != nil {
		debugLog.Infof("Scanner error for %s: %v", dicPath, err)
	}

	commit()

	return &dto.DicInfoData{
		Data: &dto.DicInfo{
			Value:      dicValue,
			Path:       dicPath,
			Head:       runheadtext,
			Dic:        dicText,
			DicFuncs:   funcFn,
			LocalClass: classText,
		},
		Value: dto.NewVal(),
	}
}
