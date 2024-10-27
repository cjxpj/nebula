package run

import (
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

// ReplaceProcessedContents 接受一个字符串、开始和结束的子串，以及两个处理函数作为参数
func ReplaceProcessedContents(str, strStart, strEnd string, process, process2 func(string) string) string {
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

		// 提取 $$ 外的文本并处理
		outsideContent := str[start:openIndex]
		processedOutsideContent := process2(outsideContent)
		result.WriteString(processedOutsideContent)

		// 提取 $$ 内的内容并处理
		content := str[openIndex+len(strStart) : closeIndex]
		processedContent := process(content)
		result.WriteString(processedContent)

		// 更新开始位置为 $$ 之后
		start = closeIndex + len(strEnd)
	}

	// 添加剩余的部分到结果字符串并处理
	outsideContent := str[start:]
	processedOutsideContent := process2(outsideContent)
	result.WriteString(processedOutsideContent)

	return result.String()
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

		regex := regexp.MustCompile("^" + t + "$")
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

		regex := regexp.MustCompile("^" + t + "$")
		if regex.MatchString(trigger) {
			return text, t, i, regex, resF
		}
	}

	return nil, "", 0, nil, ""
}

// ImportText 方法用于插入文本
func (t *Build) ImportText(text string) (string, error) {

	result := ReplaceProcessedContent(text, "[插入=", "]", func(val string) string {
		filename := t.Path + "/private/" + val
		file := utils.NewFileQueue(filename)
		FileData, err := file.ReadFromFile()
		if err != nil {
			return ""
		}
		return FileData
	})

	return result, nil
}

// 运行网页词库
func (t *Build) Web(lines []string) *dto.BuildValue {

	// 多行注释
	var zhushi bool
	var dicText []string
	var funcText []*dto.BuildDic
	var chajianText []*dto.BuildDic
	// 整合包
	var classText map[string]*dto.DicClass = make(map[string]*dto.DicClass)

	for _, line := range lines {
		if line != "" {
			line = strings.TrimLeft(line, " ")
		}
		lineLen := len(line)
		if lineLen == 0 {
			continue
		}

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

		if lineLen >= 2 && line[:2] == "//" {
			continue
		}

		if lineLen > 8 && line[:8] == "#引入=" {
			fileList := strings.Split(line[8:], ",")
			path := fileList[0]
			var user, classN, sysurl string
			var tool bool
			if len(path) > 17 && path[8:17] == "gitee.com" {
				sysurl = path[8:17]
				tool = true
				paths := path[18:]
				webfileList := strings.IndexByte(paths, '/')
				user = paths[:webfileList]
				classN = paths[webfileList+1:]
			}
			if len(fileList) >= 2 {
				file := utils.NewFile()
				for _, name := range fileList[1:] {
					if tool {
						filepath := t.Path + "/private/system/pkg/mod/" + sysurl + "/" + user + "/" + classN + "/" + name + ".n"
						file.SetPath(filepath)
						if !file.FileExists() {
							file.WriteToFile(utils.AccessGet([]string{path + "/raw/master/" + name + ".n"}))
						}
					} else {
						filepath := t.Path + "/private/" + path + "/" + name + ".n"
						file.SetPath(filepath)
					}
					FileData, err := file.ReadFromFile()
					if err == nil {
						// 解密
						str, err := utils.Decrypt(FileData, appfiles.Key)
						if err == nil {
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
				}
				continue
			}
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
	text = strings.ReplaceAll(text, "\r\n", "\n")

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
	)

	if lines[0] != "" {
		runhead = true
	}

	for dic_i, line := range lines {
		if line != "" {
			line = strings.TrimLeft(line, " ")
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

		if lineLen >= 2 && line[:2] == "//" {
			continue
		}

		if isClassN && lineLen == 11 && line == "<=整合包" {
			isClassN = false
			continue
		}

		if !isClassN && lineLen > 11 && line[:11] == "整合包=>" {
			classN = line[11:]
			isClassN = true
			continue
		}

		if runhead {
			if lineLen > 8 && line[:8] == "#引入=" {
				fileList := strings.Split(line[8:], ",")
				path := fileList[0]

				// 资源文件
				if classText["Embed"] == nil {
					classText["Embed"] = &dto.DicClass{
						LocalValue: dto.NewVal(),
					}
				}

				var user, classN, sysurl string
				var tool bool
				if len(path) > 17 && path[8:17] == "gitee.com" {
					sysurl = path[8:17]
					tool = true
					paths := path[18:]
					webfileList := strings.IndexByte(paths, '/')
					user = paths[:webfileList]
					classN = paths[webfileList+1:]
				}
				if len(fileList) >= 2 {
					file := utils.NewFile()
					for _, name := range fileList[1:] {
						if tool {
							filepath := t.Path + "/private/system/pkg/mod/" + sysurl + "/" + user + "/" + classN + "/" + name + ".n"
							file.SetPath(filepath)
							if !file.FileExists() {
								file.WriteToFile(utils.AccessGet([]string{path + "/raw/master/" + name + ".n"}))
							}
						} else {
							filepath := t.Path + "/private/" + path + "/" + name + ".n"
							file.SetPath(filepath)
						}
						FileData, err := file.ReadFromFile()
						if err == nil {
							// 解密
							str, err := utils.Decrypt(FileData, appfiles.Key)
							if err == nil {
								FileData = str
							}
							if len(FileData) > 14 && FileData[0:14] == "#资源文件\n" {
								classText["Embed"].LocalValue.Set(name, FileData[14:])
							} else {
								z := t.SplitText(FileData)
								funcText = append(funcText, z.LocalStatic...)
								chajianText = append(chajianText, z.LocalFunc...)
								for key, value := range z.LocalClass {
									if classText[key] == nil {
										if key == "Embed" {
											for vv, vvv := range value.LocalValue.GetAll() {
												classText["Embed"].LocalValue.Set(vv, vvv)
											}
										} else {
											classText[key] = value
										}
									}
								}
							}
						}
					}
					continue
				}
			}
			if line == "" {
				runhead = false
				continue
			}
			runheadtext = append(runheadtext, line)
			continue
		}

		// 如果检测词条不等于空
		if line != "" || (line == "" && duohang) {
			// 没有触发文本变量不是空就添加
			if dicTrigger != "" {

				if !duohang && line == "<?n" {
					duohang = true
					continue
				}

				if duohang && line == "?>" {
					duohang = false
				} else {
					dicTexts = append(dicTexts, line)
				}

			} else {
				// 判断触发为空就执行记录
				dicTrigger = line

				if lineLen > 3 {
					switch line[:3] {
					case "[F]":
						chajian = true
						dicTrigger = line[3:]
					case "[L]":
						neibu = true
						dicTrigger = line[3:]
					default:
						if lineLen > 8 {
							switch line[:8] {
							case "[函数]":
								chajian = true
								dicTrigger = line[8:]
							case "[内部]":
								neibu = true
								dicTrigger = line[8:]
							}
						}
					}
				}

			}
		}

		if dicTrigger != "" {

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
					funcText = append(funcText, json)
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

	if t.Cache && t.Uid != "" {
		dto.GV.Set("cache_"+t.Uid, result)
	}

	return result
}
