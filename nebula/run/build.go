package run

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/cjxpj/nebula/appfiles"
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

// 在 $...$ 内部：按空格切分；支持 "\ " 保留空格，"\\"=>"\", "\$"=>"$"
func splitWithEscape(s string) []string {
	var args []string
	var b strings.Builder
	escaped := false

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if escaped {
			switch ch {
			case ' ':
				b.WriteByte(' ') // \  + 空格 → 字面空格
			case '\\':
				b.WriteByte('\\') // \\ → \
			case '$':
				b.WriteByte('$') // \$ → $
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

		if ch == ' ' {
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

// 运行网页词库
func (t *Build) Web(lines []string) *dto.BuildValue {

	var (
		// 多行注释
		zhushi      bool
		dicText     []string
		funcText    []*dto.BuildDic
		chajianText []*dto.BuildDic
		// 整合包
		classText map[string]*dto.DicClass = make(map[string]*dto.DicClass)
		// 缩进
		suojin bool
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

			var filesToLoad []string

			// 判断是目录还是文件
			if strings.HasSuffix(path, "/*") {
				dirPath := "private/" + strings.TrimSuffix(path, "/*")
				fileLoad := utils.NewFileQueue(dirPath)
				if !fileLoad.DirExists() {
					fmt.Println("加载目录不存在：", dirPath)
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

				z := t.SplitText(FileData)
				funcText = append(funcText, z.LocalStatic...)

				chajianText = append(chajianText, z.LocalFunc...)
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
		Head:        dicText,
		LocalStatic: funcText,
		LocalFunc:   chajianText,
		LocalClass:  classText,
	}
	return result
}

func (t *Build) SplitText(text string) *dto.BuildValue {
	// 词条总数据
	// 将所有的\r\n替换为\n
	// text = strings.ReplaceAll(text, "\r\n", "\n")

	// 现在可以安全地使用\n作为分隔符
	lines := strings.Split(text, "\n")

	lines_num := len(lines) - 1

	var (
		// 触发变量
		dicTrigger string // 触发词

		// 词库条目
		dicText  []*dto.BuildDic // 词库条目
		dicTexts []string        // 准备添加到词库中的词条

		// 内部状态变量
		neibu    bool
		funcText []*dto.BuildDic // 与函数相关的词库条目

		// 插件变量
		chajian     bool
		chajianText []*dto.BuildDic // 与插件相关的词库条目

		// 头部变量
		runheadtext []string // 头部文本条目
		runhead     bool

		// 多行注释标志
		zhushi bool

		// 多行词条标志
		duohang bool

		// 词库类
		classText map[string]*dto.DicClass = make(map[string]*dto.DicClass)

		classN   string // 当前类名
		isClassN bool   // 类名存在标志

		fRunAll bool // 函数框选

		suojin bool // 缩进

		fHeaderName string // 函数头部名称
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
				fmt.Println("["+t.Path+"]", line[10:])
			}
			if lineLen > 13 && line[:13] == "//@函数头=" {
				fHeaderName = line[13:]
			}
			continue
		}

		if isClassN && line == "#Class" {
			isClassN = false
			continue
		}

		if runhead {
			if lineLen > 8 && line[:8] == "#引入=" {
				path := strings.TrimSpace(line[8:])

				var filesToLoad []string

				// 判断是目录还是文件
				if strings.HasSuffix(path, "/*") {
					dirPath := "private/" + strings.TrimSuffix(path, "/*")
					fileLoad := utils.NewFileQueue(dirPath)
					if !fileLoad.DirExists() {
						fmt.Println("加载目录不存在：", dirPath)
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

					z := t.SplitText(FileData)
					funcText = append(funcText, z.LocalStatic...)

					if fHeaderName != "" {
						for _, value := range z.LocalFunc {
							value.Trigger = fHeaderName + "." + value.Trigger
						}
					}

					chajianText = append(chajianText, z.LocalFunc...)
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
					}
				}

			} else {
				// 判断触发为空就执行记录
				dicTrigger = line

				if !isClassN {
					if after, ok := strings.CutPrefix(line, "#Class="); ok && after != "" {
						isClassN = true
						classN = after
						continue
					}
				}

				if lineLen > 3 {
					switch line[:3] {
					case "[F]":
						chajian = true
						if fHeaderName != "" {
							dicTrigger = fHeaderName + line[3:]
						} else {
							dicTrigger = line[3:]
						}
					case "[L]":
						neibu = true
						dicTrigger = line[3:]
					default:
						if lineLen > 8 {
							switch line[:8] {
							case "[函数]":
								chajian = true
								if fHeaderName != "" {
									dicTrigger = fHeaderName + line[8:]
								} else {
									dicTrigger = line[8:]
								}
							case "[内部]":
								neibu = true
								dicTrigger = line[8:]
							}
						}
					}
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
					Trigger: dicTrigger,
					Text:    dicTexts,
				}
				if neibu {
					neibu = false
					if isClassN {
						if classText[classN] == nil {
							Nv := dto.NewVal()
							classText[classN] = &dto.DicClass{
								LocalValue: Nv,
							}
						}
						classText[classN].LocalStatic = append(classText[classN].LocalStatic, json)
					} else {
						funcText = append(funcText, json)
					}
				} else if chajian {
					chajian = false
					if isClassN {
						if classText[classN] == nil {
							Nv := dto.NewVal()
							classText[classN] = &dto.DicClass{
								LocalValue: Nv,
							}
						}
						classText[classN].LocalFunc = append(classText[classN].LocalFunc, json)
					} else {
						chajianText = append(chajianText, json)
					}
				} else {
					dicText = append(dicText, json)
				}
				dicTrigger = ""
				dicTexts = nil
			}
		}

	}
	result := &dto.BuildValue{
		Head:        runheadtext,
		Dic:         dicText,
		LocalStatic: funcText,
		LocalFunc:   chajianText,
		LocalClass:  classText,
	}

	// 打印普通json
	// dd, derr := json.MarshalIndent(result, "", "  ")
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
