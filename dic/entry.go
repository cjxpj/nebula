package dic

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/buger/jsonparser"
	"github.com/cjxpj/nebula/count"
	"github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
	"github.com/dop251/goja"
	lua "github.com/yuin/gopher-lua"
)

// 执行
func (r *DicEntry) Run() string {
	// 重置文本
	r.Output.Data = ""
	// 文本段
	txt := r.Text
	// 全局变量
	global_v := r.G_v
	// 局部变量
	v := r.V

	// 是否生成>默认true
	if r.Trigger {
		trigger := "Main"
		GetDicTrigger := "Main"

		if triggers, ok := v.Get("触发词").(string); ok {
			trigger = triggers
		} else {
			v.Set("触发词", trigger)
		}

		if GetDicTriggers, ok := v.Get("触发").(string); ok {
			GetDicTrigger = GetDicTriggers
		} else {
			v.Set("触发", GetDicTrigger)
		}
		// 生成参数跟括号
		dto.RunTrigger(trigger, GetDicTrigger, v)
	}

	var RunDicindex int16
	var isif bool
	var lock bool

	// 系统局部变量
	sysVal := r.Sys_v

	// 函数包
	funcV := &DicFunc{
		GV:     global_v,
		V:      v,
		Sys:    sysVal,
		Path:   r.Path,
		Dic:    r.Dic,
		Output: r.Output,
	}

	dicTool := &DicTools{
		GlobalVariable: global_v,
		LocalVariable:  v,
		Func:           funcV,
	}

	for index := 0; index < len(txt); index++ {

		text := txt[index]

		textLen := len(text)

		RunDicindex++

		// 赋予值文本框
		if sysVal.ValText.Success {
			content := sysVal.ValText.Content
			line_feed := sysVal.ValText.LineFeed
			if text == "\"\"\"" {
				valName := sysVal.ValText.VlaueName
				if valName != "" {
					v.Set(valName, content)
				} else {
					r.Output.Add(content)
				}
				sysVal.ValText.Content = ""
				sysVal.ValText.Success = false
				continue
			}
			if txt[index+1] == "\"\"\"" {
				if sysVal.ValText.ReadValue {
					content = content + v.Text(text)
				} else {
					content = content + text
				}
			} else {
				if sysVal.ValText.ReadValue {
					content = content + v.Text(text) + line_feed
				} else {
					content = content + text + line_feed
				}
			}
			sysVal.ValText.Content = content
			continue
		}

		if sysVal.Text.Success {
			content := sysVal.Text.Content
			line_feed := sysVal.Text.LineFeed
			if text == "<文本" {
				valName := sysVal.Text.VlaueName
				if valName != "" {
					v.Set(valName, content)
				} else {
					r.Output.Add(content)
				}
				sysVal.Text.Content = ""
				sysVal.Text.Success = false
				continue
			}
			if txt[index+1] == "<文本" {
				if sysVal.Text.ReadValue {
					content = content + v.Text(text)
				} else {
					content = content + text
				}
			} else {
				if sysVal.Text.ReadValue {
					content = content + v.Text(text) + line_feed
				} else {
					content = content + text + line_feed
				}
			}
			sysVal.Text.Content = content
			continue
		}

		if sysVal.SetJson.Success {
			if text == "<JSON" {
				valName := sysVal.SetJson.VlaueName
				resS, err := json.Marshal(sysVal.SetJson.Json)
				if err == nil {
					jsonString := string(resS)
					if valName != "" {
						v.Set(valName, jsonString)
					} else {
						r.Output.Add(jsonString)
					}
				}
				sysVal.SetJson.Success = false
				continue
			}
			if startIdx := strings.IndexByte(text, '='); startIdx != -1 {
				endIdx := startIdx + 1
				if text[startIdx-1] == ':' && textLen >= endIdx {
					key := text[:startIdx-1]
					keys := strings.Split(key, "->")
					if keys[0] == "[]" {
						if !sysVal.SetJson.OkLen {
							if getLen, ok := sysVal.SetJson.Json.(map[string]interface{}); ok {
								sysVal.SetJson.Len = len(getLen)
							}
							if getLen, ok := sysVal.SetJson.Json.([]interface{}); ok {
								sysVal.SetJson.Len = len(getLen)
							}
							sysVal.SetJson.OkLen = true
						} else {
							sysVal.SetJson.Len++
						}
						keys[0] = strconv.Itoa(sysVal.SetJson.Len)
					}
					value := v.Text(text[endIdx:])
					for k, setv := range keys {
						keys[k] = v.Text(setv)
					}
					sysVal.SetJson.Json = funcs.JsonSetValue(sysVal.SetJson.Json, keys, value, false)
					continue
				}
				if textLen >= endIdx {
					key := text[:startIdx]
					keys := strings.Split(key, "->")
					if keys[0] == "[]" {
						if !sysVal.SetJson.OkLen {
							if getLen, ok := sysVal.SetJson.Json.(map[string]interface{}); ok {
								sysVal.SetJson.Len = len(getLen)
							}
							if getLen, ok := sysVal.SetJson.Json.([]interface{}); ok {
								sysVal.SetJson.Len = len(getLen)
							}
							sysVal.SetJson.OkLen = true
						} else {
							sysVal.SetJson.Len++
						}
						keys[0] = strconv.Itoa(sysVal.SetJson.Len)
					}
					value := v.Text(text[endIdx:])
					for k, setv := range keys {
						keys[k] = v.Text(setv)
					}
					sysVal.SetJson.Json = funcs.JsonSetValue(sysVal.SetJson.Json, keys, value, true)
					continue
				}
			}
		}

		if sysVal.Func.Success {
			forNum := sysVal.Func.Num
			content := sysVal.Func.Content
			funcTrigger := sysVal.Func.Trigger
			if textLen > 7 && text[:7] == "函数>" {
				forNum++
				sysVal.Func.Num = forNum
			}
			if text == "<函数" {
				if forNum == 0 {
					v.Set(sysVal.Func.VlaueName, map[string]interface{}{
						"type":    "函数框",
						"trigger": funcTrigger,
						"content": content,
					})

					sysVal.Func.Content = []string{}
					sysVal.Func.Success = false
					continue
				}
				forNum--
				sysVal.Func.Num = forNum
			}
			content = append(content, text)
			sysVal.Func.Content = content
			continue
		}

		if sysVal.For.Success {
			forNum := sysVal.For.Num
			content := sysVal.For.Content
			if textLen >= 7 && text[:7] == "循环>" {
				forNum++
				sysVal.For.Num = forNum
			}
			if text == "<循环" {
				if forNum == 0 {
					valName := sysVal.For.VlaueName
					RunDic := NewRunDicEntry(r.Path).
						SetGlobal_v(r.G_v).
						Set_v(r.V).
						SetDic_v(r.Dic).
						SetRunFor()
					RunDic.Trigger = false

					switch objfor_v := sysVal.ForGetRun().(type) {
					case int:
						RunDic.SetText(content)

						for i := 1; i <= objfor_v; i++ {
							strNum := strconv.Itoa(i)
							v.Set(valName, strNum)
							resRun := RunDic.Run()
							r.Output.Add(resRun)
							if !sysVal.For.IsFor && RunDic.Sys_v.Stop {
								return r.Output.Get()
							}

							if RunDic.Sys_v.For.Jump {
								sysVal.For.Jump = false
								break
							}
							if RunDic.Sys_v.Stop {
								sysVal.Stop = true
								return r.Output.Get()
							}
							if setNum := v.Get(valName).(string); setNum != strNum {
								Xi, err := strconv.Atoi(setNum)
								if err != nil {
									break
								}
								i = Xi
							}
						}

					case []byte:
						RunDic.SetText(content)
						var StopProcessing = errors.New("stop")
						startIdx := strings.IndexByte(valName, ',')
						if startIdx == -1 {
							return ""
						}
						endIdx := startIdx + 1
						v1 := valName[:startIdx]
						v2 := valName[endIdx:]
						jsonparser.ObjectEach(objfor_v, func(keyByte []byte, valueByte []byte, dataType jsonparser.ValueType, offset int) error {
							key := string(keyByte)
							value := string(valueByte)
							v.Set(v1, key)
							v.Set(v2, value)
							resRun := RunDic.Run()
							r.Output.Add(resRun)
							if RunDic.Sys_v.For.Jump || RunDic.Sys_v.Stop {
								sysVal.For.Jump = false
								return StopProcessing
							}
							return nil
						})
						if RunDic.Sys_v.Stop {
							sysVal.Stop = true
							return r.Output.Get()
						}

					case []interface{}:
						RunDic.SetText(content)

						startIdx := strings.IndexByte(valName, ',')
						if startIdx == -1 {
							return ""
						}
						endIdx := startIdx + 1
						v1 := valName[:startIdx]
						v2 := valName[endIdx:]

						for key, value := range objfor_v {
							strNum := strconv.Itoa(key)
							v.Set(v1, strNum)
							if strVal, ok := value.(string); ok {
								v.Set(v2, strVal)
							} else {
								resS, err := json.Marshal(value)
								if err == nil {
									v.Set(v2, string(resS))
								}
							}
							resRun := RunDic.Run()
							r.Output.Add(resRun)

							if RunDic.Sys_v.Stop {
								sysVal.Stop = true
								return r.Output.Get()
							}

							if !RunDic.Sys_v.For.IsFor && RunDic.Sys_v.Stop {
								return r.Output.Get()
							}
							if RunDic.Sys_v.For.Jump {
								sysVal.For.Jump = false
								break
							}
						}
					}

					sysVal.For.Num = 0
					sysVal.For.Run = 0
					sysVal.For.Content = []string{}
					sysVal.For.Success = false
					continue
				}
				forNum--
				sysVal.For.Num = forNum
			}
			content = append(content, text)
			sysVal.For.Content = content
			continue
		}

		if sysVal.IfFunc.Success {
			forNum := sysVal.IfFunc.Num
			if textLen > 7 && text[:7] == "如果>" {
				forNum++
				sysVal.IfFunc.Num = forNum
			}
			if text == "<如果" {
				if forNum == 0 {
					RunDic := NewRunDicEntry(r.Path).
						SetGlobal_v(r.G_v).
						Set_v(r.V).
						SetDic_v(r.Dic).
						SetRunIf()
					if sysVal.For.IsFor {
						RunDic.SetRunFor()
					}
					RunDic.Trigger = false

					for i := 0; i <= sysVal.IfFunc.IfNum; i++ {
						var ifval bool = Pd(dicTool, r.Path, sysVal.IfFunc.If[i])
						if ifval {
							RunDic.SetText(sysVal.IfFunc.Run[i])
							resRun := RunDic.Run()
							r.Output.Add(resRun)

							if sysVal.For.IsFor && RunDic.Sys_v.For.IsFor && RunDic.Sys_v.For.Jump {
								return r.Output.Get()
							}

							if RunDic.Sys_v.Stop {
								sysVal.Stop = true
								return r.Output.Get()
							}
							break
						} else {
							if i != sysVal.IfFunc.IfNum {
								continue
							}
							RunDic.SetText(sysVal.IfFunc.Else)
							resRun := RunDic.Run()
							r.Output.Add(resRun)
						}

						if sysVal.For.IsFor && RunDic.Sys_v.For.IsFor && RunDic.Sys_v.For.Jump {
							return r.Output.Get()
						}

						if !sysVal.IfFunc.IsIf && RunDic.Sys_v.Stop {
							return r.Output.Get()
						}

						if RunDic.Sys_v.IfFunc.Jump {
							sysVal.IfFunc.Jump = false
							break
						}
						if RunDic.Sys_v.Stop {
							sysVal.Stop = true
							return r.Output.Get()
						}
					}

					sysVal.IfFunc.If = []string{}
					sysVal.IfFunc.Else = []string{}
					sysVal.IfFunc.Run = [][]string{}
					sysVal.IfFunc.IfNum = 0
					sysVal.IfFunc.IsElse = false
					sysVal.IfFunc.Success = false
					continue
				}
				forNum--
				sysVal.IfFunc.Num = forNum
			}

			if forNum == 0 {
				if !sysVal.IfFunc.IsElse && text == ">否则" {
					sysVal.IfFunc.IsElse = true
					continue
				}
				if !sysVal.IfFunc.IsElse && textLen > 14 && text[:14] == ">否则如果:" {
					sysVal.IfFunc.IfNum++
					sysVal.IfFunc.If = append(sysVal.IfFunc.If, text[14:])
					continue
				}
			}

			if sysVal.IfFunc.IsElse {
				sysVal.IfFunc.Else = append(sysVal.IfFunc.Else, text)
			} else {
				if sysVal.IfFunc.IfNum >= len(sysVal.IfFunc.Run) {
					sysVal.IfFunc.Run = append(sysVal.IfFunc.Run, []string{})
				}
				sysVal.IfFunc.Run[sysVal.IfFunc.IfNum] = append(sysVal.IfFunc.Run[sysVal.IfFunc.IfNum], text)
			}
			continue
		}

		if sysVal.Js.Success {
			content := sysVal.Js.Content
			if text == "<Js" {
				vm := goja.New()

				input := sysVal.Js.VlaueList
				args := strings.Split(input, ",")

				for i, arg := range args {
					key := fmt.Sprintf("参数%d", i)
					vm.Set(key, v.Text(arg))
				}

				res, err := vm.RunString(content)
				if err != nil {
					errMsg := fmt.Sprintf("报错:%s", err)
					return errMsg
				}
				resStr := res.String()

				valName := sysVal.Js.VlaueName
				if valName != "" {
					v.Set(valName, resStr)
				} else {
					r.Output.Add(resStr)
				}

				sysVal.Js.Content = ""
				sysVal.Js.Success = false
				continue
			}
			content = content + text + "\n"
			sysVal.Js.Content = content
			continue
		}

		if sysVal.Lua.Success {
			content := sysVal.Lua.Content
			if text == "<Lua" {
				// 创建一个新的 Lua 解释器
				L := lua.NewState()
				defer L.Close()

				// 执行 Lua 脚本
				if err := L.DoString(content); err != nil {
					return "Lua加载错误"
				}

				// 获取Lua的main函数
				fn := L.GetGlobal("main")
				if fn.Type() != lua.LTFunction {
					return "不存在main"
				}

				// 调用main函数
				L.Push(fn)

				L_input := sysVal.Lua.VlaueList
				args := strings.Split(L_input, ",")

				// 准备要传递给 Lua 的参数
				luaArgs := make([]lua.LValue, len(args))
				for i, arg := range args {
					luaArgs[i] = lua.LString(v.Text(arg))
				}

				for _, arg := range luaArgs {
					L.Push(arg)
				}

				// 调用Lua函数，没有参数，有一个返回值
				if err := L.PCall(len(luaArgs), lua.MultRet, nil); err != nil {
					errMsg := fmt.Sprintf("报错:%s", err)
					return errMsg
				}
				// 从栈中弹出返回值L.Pop(1)
				results := L.Get(-1)
				// 从栈中弹出返回值
				L.Pop(1)
				res := results.String()
				valName := sysVal.Lua.VlaueName
				if valName != "" {
					v.Set(valName, res)
				} else {
					r.Output.Add(res)
				}

				sysVal.Lua.Content = ""
				sysVal.Lua.Success = false
				continue
			}
			content = content + text + "\n"
			sysVal.Lua.Content = content
			continue
		}

		if isif {
			if lock {

				if text == "如果尾" || text == "end" {
					lock = false
					isif = false
					continue
				}

				if text == "else" || text == "否则" {
					lock = false
					isif = false
					continue
				}

				if text == "返回" && txt[index+1] == "如果尾" {
					lock = false
					isif = false
					index++
					continue
				}

				if textLen > 5 && text[:5] == "elif:" {
					isif = true
					lock = true
					var ifval bool = Pd(dicTool, r.Path, text[5:])
					if ifval {
						lock = false
						continue
					}
					continue
				}

				if textLen > 13 && text[:13] == "否则如果:" {
					isif = true
					lock = true
					var ifval bool = Pd(dicTool, r.Path, text[13:])
					if ifval {
						lock = false
						continue
					}
					continue
				}
			}
			if !lock {
				if text == "如果尾" || text == "end" {
					lock = false
					isif = false
					continue
				}
				if textLen > 5 && text[:5] == "elif:" {
					break
				}
				if textLen > 13 && text[:13] == "否则如果:" {
					break
				}
				if text == "else" || text == "否则" {
					break
				}
				if text == "返回" && txt[index+1] == "如果尾" {
					break
				}
			}
		}

		if textLen > 3 && text[:3] == "if:" {
			isif = true
			lock = true
			var ifval bool = Pd(dicTool, r.Path, text[3:])
			if ifval {
				lock = false
				continue
			}
			continue
		}

		if textLen > 7 && text[:7] == "如果:" {
			isif = true
			lock = true
			var ifval bool = Pd(dicTool, r.Path, text[7:])
			if ifval {
				lock = false
				continue
			}
			continue
		}

		if lock {
			continue
		}

		if text == ">跳过" && sysVal.For.IsFor {
			return r.Output.Get()
		}

		if text == ">终止循环" && sysVal.For.IsFor {
			sysVal.For.Jump = true
			return r.Output.Get()
		}

		if text == ">跳过" && sysVal.IfFunc.IsIf {
			sysVal.IfFunc.Jump = true
			return r.Output.Get()
		}

		if text == ">终止" {
			sysVal.Stop = true
			return r.Output.Get()
		}

		if textLen >= 7 && text[:7] == "函数>" {
			if startIdx := strings.IndexByte(text, '='); startIdx != -1 {
				endIdx := startIdx + 1
				if textLen >= endIdx {
					key := text[7:startIdx]
					value := text[endIdx:]
					sysVal.Func.VlaueName = key
					sysVal.Func.Trigger = value
					sysVal.Func.Success = true
					continue
				}
			}
			key := text[7:]
			sysVal.Func.VlaueName = key
			sysVal.Func.Trigger = ""
			sysVal.Func.Success = true
			continue
		}

		if textLen > 7 && text[:7] == "如果>" {
			key := text[7:]
			sysVal.IfFunc.If = append(sysVal.IfFunc.If, key)
			sysVal.IfFunc.Success = true
			continue
		}

		if textLen >= 3 && text[:3] == "Js>" {
			if startIdx := strings.IndexByte(text, '='); startIdx != -1 {
				endIdx := startIdx + 1
				if textLen >= endIdx {
					key := text[3:startIdx]
					value := text[endIdx:]
					sysVal.Js.VlaueName = key
					sysVal.Js.VlaueList = value
					sysVal.Js.Success = true
					continue
				}
			}
			key := text[3:]
			sysVal.Js.VlaueName = key
			sysVal.Js.VlaueList = ""
			sysVal.Js.Success = true
			continue
		}

		if textLen >= 4 && text[:4] == "Lua>" {
			if startIdx := strings.IndexByte(text, '='); startIdx != -1 {
				endIdx := startIdx + 1
				if textLen >= endIdx {
					key := text[4:startIdx]
					value := text[endIdx:]
					sysVal.Lua.VlaueName = key
					sysVal.Lua.VlaueList = value
					sysVal.Lua.Success = true
					continue
				}
			}
			key := text[4:]
			sysVal.Lua.VlaueName = key
			sysVal.Lua.VlaueList = ""
			sysVal.Lua.Success = true
			continue
		}

		if textLen >= 10 && text[:10] == "纯文本>" {
			if startIdx := strings.IndexByte(text, '='); startIdx != -1 {
				endIdx := startIdx + 1
				if textLen >= endIdx {
					key := text[10:startIdx]
					value := text[endIdx:]
					runText := v.Text(value)
					sysVal.Text.Success = true
					sysVal.Text.ReadValue = false
					sysVal.Text.VlaueName = key
					sysVal.Text.LineFeed = runText
					continue
				}
			}
			runText := v.Text(text[10:])
			sysVal.Text.Success = true
			sysVal.Text.ReadValue = false
			sysVal.Text.VlaueName = ""
			sysVal.Text.LineFeed = runText
			continue
		}

		if textLen >= 5 && text[:5] == "JSON>" {
			if startIdx := strings.IndexByte(text, '='); startIdx != -1 {
				endIdx := startIdx + 1
				if textLen >= endIdx {
					key := text[5:startIdx]
					value := text[endIdx:]
					runText := v.Text(value)
					err := json.Unmarshal([]byte(runText), &sysVal.SetJson.Json)
					if err == nil {
						sysVal.SetJson.Success = true
						sysVal.SetJson.VlaueName = key
					}
					continue
				}
			}
			runText := v.Text(text[5:])
			err := json.Unmarshal([]byte(runText), &sysVal.SetJson.Json)
			if err == nil {
				sysVal.SetJson.Success = true
				sysVal.SetJson.VlaueName = ""
			}
			continue
		}

		if textLen >= 7 && text[:7] == "文本>" {
			if startIdx := strings.IndexByte(text, '='); startIdx != -1 {
				endIdx := startIdx + 1
				if textLen >= endIdx {
					key := text[7:startIdx]
					value := text[endIdx:]
					runText := v.Text(value)
					sysVal.Text.Success = true
					sysVal.Text.ReadValue = true
					sysVal.Text.VlaueName = key
					sysVal.Text.LineFeed = runText
					sysVal.Text.Content = ""
					continue
				}
			}
			runText := v.Text(text[7:])
			sysVal.Text.Success = true
			sysVal.Text.ReadValue = true
			sysVal.Text.VlaueName = ""
			sysVal.Text.LineFeed = runText
			sysVal.Text.Content = ""
			continue
		}

		if textLen >= 7 && text[:7] == "循环>" {
			if startIdx := strings.IndexByte(text, '='); startIdx != -1 {
				endIdx := startIdx + 1
				if textLen >= endIdx {
					key := text[7:startIdx]
					value := text[endIdx:]
					runText := v.Text(value)
					// 将字符串解析为整数
					intValue, err := strconv.Atoi(runText)
					if err == nil {
						sysVal.For.Run = intValue
					} else {
						var testjs map[string]interface{}
						if json.Unmarshal([]byte(runText), &testjs) == nil {
							sysVal.For.Run = []byte(runText)
						} else {
							var thisjson []interface{}
							if json.Unmarshal([]byte(runText), &thisjson) == nil {
								sysVal.For.Run = thisjson
							} else {
								sysVal.For.Run = 32767
							}
						}
					}
					sysVal.For.VlaueName = key
					sysVal.For.Success = true
					continue
				}
			}
			key := text[7:]
			sysVal.For.VlaueName = key
			sysVal.For.Run = 32767
			sysVal.For.Success = true
			continue
		}

		if textLen > 8 {
			if text[:8] == ">跳行+" {
				runText := v.Text(text[8:])
				num, err := strconv.Atoi(runText)
				if err != nil {
					continue
				}
				index = index + num
				continue
			}

			if text[:8] == ">跳行-" {
				runText := v.Text(text[8:])
				num, err := strconv.Atoi(runText)
				if err != nil {
					continue
				}
				index--
				index = index - num
				continue
			}
		}

		if textLen > 2 && text[:2] == "#:" {
			funcV.Runs(count.RunCountText(v, text[2:]))
			continue
		}

		var found bool

		// 赋予值
		for i, r := range text {
			if i == 0 {
				continue
			}
			if !(unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Scripts["Han"], r) || r == '_') {
				endIdx := i + 2
				if textLen >= endIdx {
					if r == '-' && text[i+1] == ':' {
						found = true
						prefix := text[:i]
						suffix := text[endIdx:]
						suffix = funcV.Runs(count.RunCountText(v, suffix))
						var valStr string
						if str, ok := v.Get(prefix).(string); ok {
							valStr = str
						}
						one, err1 := strconv.ParseFloat(valStr, 64)
						two, err2 := strconv.ParseFloat(suffix, 64)
						if err1 != nil && err2 != nil {
							break
						}
						resNum := strconv.FormatFloat(one-two, 'f', -1, 64)
						v.Set(prefix, resNum)
						break
					}
					if r == '+' && text[i+1] == ':' {
						found = true
						prefix := text[:i]
						suffix := text[endIdx:]
						suffix = funcV.Runs(count.RunCountText(v, suffix))
						var valStr string
						if str, ok := v.Get(prefix).(string); ok {
							valStr = str
						}
						one, err1 := strconv.ParseFloat(valStr, 64)
						two, err2 := strconv.ParseFloat(suffix, 64)
						if err1 != nil || err2 != nil {
							v.Set(prefix, valStr+suffix)
							break
						}
						resNum := strconv.FormatFloat(one+two, 'f', -1, 64)
						v.Set(prefix, resNum)
						break
					}
				}
				if r == ':' {
					endIdx++ // 3
					if textLen >= endIdx {
						if text[i+1] == '$' && text[i+2] == ':' {
							found = true
							prefix := text[:i]
							suffix := text[endIdx:]
							runText := funcV.Run(suffix)
							v.Set(prefix, runText)
							break
						}
						if text[i+1] == '%' && text[i+2] == ':' {
							found = true
							prefix := text[:i]
							suffix := text[endIdx:]
							runText := v.Text(suffix)
							v.Set(prefix, runText)
							break
						}
					}
					endIdx += 2 // 5
					if textLen >= endIdx {
						if text[i+1] == '(' && text[i+2] == '$' && text[i+3] == ')' && text[i+4] == ':' {
							found = true
							prefix := text[:i]
							suffix := text[endIdx:]
							funcV.Open = true
							runText := funcV.Runss(suffix)
							funcV.Open = false
							v.Set(prefix, runText)
							break
						}
					}
					endIdx -= 3 // 2
					if textLen >= endIdx && text[i+1] == ':' {
						found = true
						prefix := text[:i]
						suffix := text[endIdx:]
						v.Set(prefix, suffix)
						break
					}
					endIdx-- // 1
					prefix := text[:i]
					suffix := text[endIdx:]
					found = true
					if suffix == "\"\"\"" {
						sysVal.ValText.Success = true
						sysVal.ValText.ReadValue = false
						sysVal.ValText.VlaueName = prefix
						sysVal.ValText.LineFeed = "\n"
						break
					}

					GetIfKeys := strings.Split(suffix, "?:")
					GetIfKeysLen := len(GetIfKeys) - 1
					for RunI, GetIfKey := range GetIfKeys {
						keys := strings.Split(GetIfKey, "->")

						var runText, runMatche string
						var resNull, resMatche bool
						if RunI == 0 {
							if len(keys[0]) > 1 {
								fh := keys[0][len(keys[0])-1]
								if fh == '!' {
									resNull = true
									keys[0] = keys[0][:len(keys[0])-1]
								}
								if fh == '@' {
									resMatche = true
									keys[0] = keys[0][:len(keys[0])-1]
									newKeys := []string{}
									runMatche = v.Text(keys[1])
									newKeys = append(newKeys, keys[0])
									newKeys = append(newKeys, keys[2:]...)
									keys = newKeys
								}
							}

							var Obj interface{}
							runText = funcV.Runs(count.RunCountText(v, keys[0]))
							if runText == "" {
								v.Set(prefix, "")
								continue
							}
							if !utils.IsJSON(runText) && (resNull || resMatche) {
								if resMatche {
									matcheA, err := regexp.Compile(runMatche)
									if err != nil {
										return ""
									}
									matches := matcheA.FindStringSubmatch(runText)
									if len(matches) > 0 {
										runText = matches[0]
									} else {
										runText = ""
									}
								}
								v.Set(prefix, runText)
								continue
							}

							if err := json.Unmarshal([]byte(runText), &Obj); err == nil {
								var IsNull bool
								keysv := keys[1:]
								for i, str := range keysv {
									keysv[i] = v.Text(str)
								}
								for _, key := range keysv {
									switch objData := Obj.(type) {
									case string:
										break
									case map[string]interface{}:
										if Objs, ok := objData[key]; ok {
											Obj = Objs
										} else {
											IsNull = true
											break
										}
									case []interface{}:
										if num, err := strconv.Atoi(key); err == nil {
											if num >= 0 && num < len(objData) {
												Objs := objData[num]
												Obj = Objs
											} else {
												IsNull = true
												break
											}
										}
									}
								}
								// 返回null
								if resNull && IsNull {
									v.Set(prefix, "null")
									continue
								}

								// 解析失败正则返回空
								if resMatche && IsNull {
									v.Set(prefix, "")
									continue
								}
								switch objData := Obj.(type) {
								case string:
									if resMatche {
										matcheA, err := regexp.Compile(runMatche)
										if err != nil {
											return ""
										}
										matches := matcheA.FindStringSubmatch(objData)
										if len(matches) > 0 {
											objData = matches[0]
										} else {
											objData = ""
										}
									}

									v.Set(prefix, objData)
									if objData == "" || objData == "0" || objData == "null" || objData == "false" {
										continue
									}
								default:
									resS, err := json.Marshal(objData)
									if err == nil {
										jsonString := string(resS)

										if resMatche {
											matcheA, err := regexp.Compile(runMatche)
											if err != nil {
												return ""
											}
											matches := matcheA.FindStringSubmatch(jsonString)
											if len(matches) > 1 {
												jsonString = matches[1]
											} else {
												jsonString = ""
											}
										}

										v.Set(prefix, jsonString)
										if jsonString == "" || jsonString == "0" || jsonString == "null" || jsonString == "false" {
											continue
										}
									}
								}
								break
							} else if resNull {
								v.Set(prefix, "null")
								continue
							} else if resMatche {
								v.Set(prefix, "")
								continue
							}
						}

						if RunI > 0 {
							runText = funcV.Runs(count.RunCountText(v, GetIfKey))
							if GetIfKeysLen == RunI {
								v.Set(prefix, runText)
								break
							}
							if runText == "" || runText == "0" || runText == "null" || runText == "false" {
								continue
							}
						}
						v.Set(prefix, runText)
						break
					}
					break
				}
				break
			}
		}

		if found {
			found = false
			continue
		}

		r.Output.Add(funcV.Runs(text))
	}
	return r.Output.Get()
}
