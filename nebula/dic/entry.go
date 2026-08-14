package dic

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	dicBuild "github.com/cjxpj/nebula/build"
	"github.com/cjxpj/nebula/count"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/buffer"
	"github.com/dop251/goja_nodejs/console"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/process"
	"github.com/dop251/goja_nodejs/require"
	"github.com/dop251/goja_nodejs/url"

	"github.com/buger/jsonparser"
)

// maxRecursionDepth 限制 $调用$ 最大递归深度，防止无限递归导致 goroutine 栈溢出
const maxRecursionDepth = 100

// 执行
func (m *dicImpl) DicRunLine(r *dic_dto.DicEntry, txt []string) string {
	// 重置文本
	r.Output.Clear()

	// 挂载整合包变量表，供 %类名.变量% 解析
	r.Val.P.Class = r.Dic.ClassValues()

	// 是否生成>默认true
	if r.Trigger {
		trigger := "Main"
		GetDicTrigger := "Main"

		if triggers, ok := r.Val.P.Get("触发词").(string); ok {
			trigger = triggers
		} else {
			r.Val.P.Set("触发词", trigger)
		}

		if GetDicTriggers, ok := r.Val.P.Get("触发").(string); ok {
			GetDicTrigger = GetDicTriggers
		} else {
			r.Val.P.Set("触发", GetDicTrigger)
		}
		// 生成参数跟括号
		dto.RunTrigger(trigger, GetDicTrigger, r.Val.P)
	}

	// 函数包
	funcV := &dic_dto.DicFunc{
		Val:            r.Val,
		Sys:            r.Sys_v,
		Dic:            r.Dic,
		Output:         r.Output,
		RecursionDepth: r.RecursionDepth,
	}

	Entry(r, txt, funcV)

	return r.Output.Get()
}

// handleLoopControl 统一处理循环控制状态（停止、跳转）检查。
// 返回 shouldBreak（跳出当前循环）和 shouldReturn（直接返回 nil）。
// loopType: "ForEach" 或 "For"
func handleLoopControl(r, runDic *dic_dto.DicEntry, loopType string) (shouldBreak, shouldReturn bool) {
	switch loopType {
	case "ForEach":
		if runDic.Sys_v.Stop.Load() {
			runDic.Close()
			r.Sys_v.Stop.Store(true)
			return true, false
		}
		if !runDic.Sys_v.ForEach.IsFor && runDic.Sys_v.Stop.Load() {
			runDic.Close()
			return false, true
		}
		if runDic.Sys_v.ForEach.Jump {
			runDic.Close()
			r.Sys_v.ForEach.Jump = false
			return true, false
		}
	case "For":
		if !runDic.Sys_v.For.IsFor && runDic.Sys_v.Stop.Load() {
			runDic.Close()
			return false, true
		}
		if runDic.Sys_v.For.Jump {
			runDic.Close()
			r.Sys_v.For.Jump = false
			return true, false
		}
		if runDic.Sys_v.Stop.Load() {
			runDic.Close()
			r.Sys_v.Stop.Store(true)
			return false, true
		}
	}
	return false, false
}

func Entry(r *dic_dto.DicEntry, txt []string, funcV *dic_dto.DicFunc) error {
	if r.RecursionDepth > maxRecursionDepth {
		r.Sys_v.Stop.Store(true)
		r.Output.Add(fmt.Sprintf("[%s]递归深度超限(line:%d)：$调用$ 递归深度已超过 %d 层，请检查是否存在死循环调用",
			r.Val.Get("_词库路径_"), funcV.CurLine, maxRecursionDepth))
		return nil
	}

	var RunDicindex int16
	var isif bool
	var lock bool

	txtLen := len(txt)

	for index := 0; index < txtLen; index++ {

		text := txt[index]

		textLen := len(text)

		RunDicindex++
		// 从行号映射表获取真实文件行号
		if int(RunDicindex)-1 < len(r.LineNums) && r.LineNums[int(RunDicindex)-1] > 0 {
			funcV.CurLine = r.LineNums[int(RunDicindex)-1]
		} else {
			funcV.CurLine = int(RunDicindex)
		}

		if r.Sys_v.Stop.Load() {
			return nil
		}

		if r.Sys_v.NodeJs.Success {
			if text != "--end" {
				r.Sys_v.NodeJs.Content = append(r.Sys_v.NodeJs.Content, text)
			}
			if index == txtLen-1 || text == "--end" {
				r.Sys_v.NodeJs.Success = false

				registry := new(require.Registry)
				loop := eventloop.NewEventLoop()
				vm := goja.New()
				registry.Enable(vm)
				console.Enable(vm)
				url.Enable(vm)
				buffer.Enable(vm)
				process.Enable(vm)

				vm.Set("dic", func(call goja.FunctionCall) goja.Value {
					dicLine := call.Argument(0).String()
					// 分割\n
					dicLineArr := strings.Split(dicLine, "\n")

					RunDics := dic_dto.NewRunDicEntry().
						SetGlobal_v(r.Val.G).
						Set_v(r.Val.P).
						WithRecursionDepth(r.RecursionDepth)
					res := dic_api.Api.DicRunLine(RunDics, dicLineArr)
					// 返回文本
					return vm.ToValue(res)
				})

				vm.Set("dic_value", func(call goja.FunctionCall) goja.Value {
					k := call.Argument(0).String()
					v := call.Argument(1).String()
					r.Val.P.Set(k, v)
					return goja.Undefined()
				})

				// 将 setTimeout 和 setInterval 注册到 JavaScript 环境中
				vm.Set("setTimeout", func(call goja.FunctionCall) goja.Value {
					fn := call.Argument(0)
					delay := call.Argument(1).ToInteger()

					// 确保第一个参数是函数
					if fnFn, ok := goja.AssertFunction(fn); ok {
						loop.SetTimeout(func(vm *goja.Runtime) {
							fnFn(goja.Undefined())
						}, time.Duration(delay)*time.Millisecond)
					}
					return goja.Undefined()
				})

				vm.Set("setInterval", func(call goja.FunctionCall) goja.Value {
					fn := call.Argument(0)
					delay := call.Argument(1).ToInteger()

					// 确保第一个参数是函数
					if fnFn, ok := goja.AssertFunction(fn); ok {
						loop.SetInterval(func(vm *goja.Runtime) {
							fnFn(goja.Undefined())
						}, time.Duration(delay)*time.Millisecond)
					}
					return goja.Undefined()
				})

				// 启动事件循环
				loop.Start()
				defer loop.Stop()

				for k, v := range r.Val.GetAll() {
					vm.Set(k, v)
				}

				scriptText := strings.Join(r.Sys_v.NodeJs.Content, "\n")
				r.Sys_v.NodeJs.Content = []string{}
				res, err := vm.RunString(scriptText)
				if err != nil {
					r.Output.Add(fmt.Sprintf("JS错误(line:%d)：%v", funcV.CurLine, err))
					return nil
				}
				if res == goja.Undefined() {
					continue
				}
				resStr := res.String()
				r.Output.Add(resStr)
				continue
			}
			continue
		}

		// 赋予值纯文本框
		if r.Sys_v.ValTextr.Success {
			if text == `'''` {
				valName := r.Sys_v.ValTextr.VlaueName
				r.Val.P.Set(valName, strings.Join(r.Sys_v.ValTextr.Content, "\n"))
				r.Sys_v.ValTextr.Content = []string{}
				r.Sys_v.ValTextr.Success = false
				continue
			}
			r.Sys_v.ValTextr.Content = append(r.Sys_v.ValTextr.Content, strings.ReplaceAll(text, `\'''`, `'''`))
			continue
		}

		// 赋予值纯文本框
		if r.Sys_v.ValText.Success {
			if text == `"""` {
				valName := r.Sys_v.ValText.VlaueName
				r.Val.P.Set(valName, strings.Join(r.Sys_v.ValText.Content, "\n"))
				r.Sys_v.ValText.Content = []string{}
				r.Sys_v.ValText.Success = false
				continue
			}
			r.Sys_v.ValText.Content = append(r.Sys_v.ValText.Content, strings.ReplaceAll(utils.AnyIsString(r.Val.Text(text)), `\"""`, `"""`))
			continue
		}

		// 赋予值连续执行框 >>>
		if r.Sys_v.ValChain.Success {
			if text == "<<<" {
				r.Sys_v.ValChain.Success = false
				continue
			}
			// 空行跳过，避免清空当前值
			if text == "" {
				continue
			}
			runValSet(r, funcV, r.Sys_v.ValChain.VlaueName, text)
			continue
		}

		if r.Sys_v.Text.Success {
			if text == "<文本" {
				valName := r.Sys_v.Text.VlaueName
				if valName != "" {
					r.Val.P.Set(valName, r.Sys_v.Text.Content.String())
				} else {
					r.Output.Add(r.Sys_v.Text.Content.String())
				}
				r.Sys_v.Text.Content.Reset()
				r.Sys_v.Text.Success = false
				continue
			}
			if index+1 < txtLen && txt[index+1] == "<文本" {
				if r.Sys_v.Text.ReadValue {
					r.Sys_v.Text.Content.WriteString(utils.AnyIsString(r.Val.Text(text)))
				} else {
					r.Sys_v.Text.Content.WriteString(text)
				}
			} else {
				if r.Sys_v.Text.ReadValue {
					r.Sys_v.Text.Content.WriteString(utils.AnyIsString(r.Val.Text(text)))
					r.Sys_v.Text.Content.WriteString(r.Sys_v.Text.LineFeed)
				} else {
					r.Sys_v.Text.Content.WriteString(text)
					r.Sys_v.Text.Content.WriteString(r.Sys_v.Text.LineFeed)
				}
			}
			continue
		}

		// JSON结尾
		if r.Sys_v.SetNewJson.Success {
			r.Sys_v.SetNewJson.Json = r.Sys_v.SetNewJson.Json + text
			if strings.HasSuffix(text, "{") || strings.HasSuffix(text, "[") {
				r.Sys_v.SetNewJson.Len++
			}
			if text == "}" || text == "]" || text == "}," || text == "]," {
				r.Sys_v.SetNewJson.Len--
				if r.Sys_v.SetNewJson.Len == 0 && (text == "}" || text == "]") {
					valName := r.Sys_v.SetNewJson.VlaueName
					if valName != "" {
						r.Val.P.Set(valName, NewJson(r, r.Val.P, r.Sys_v.SetNewJson.Json))
					} else {
						r.Output.Add(NewJson(r, r.Val.P, r.Sys_v.SetNewJson.Json))
					}
					r.Sys_v.SetNewJson.Success = false
					continue
				}
			}
			continue
		}

		if r.Sys_v.SetJson.Success {
			if text == "<JSON" {
				valName := r.Sys_v.SetJson.VlaueName
				resS, err := json.Marshal(r.Sys_v.SetJson.Json)
				if err == nil {
					jsonString := string(resS)
					if valName != "" {
						r.Val.P.Set(valName, jsonString)
					} else {
						r.Output.Add(jsonString)
					}
				}
				r.Sys_v.SetJson.Success = false
				continue
			}
			if startIdx := strings.IndexByte(text, '='); startIdx != -1 {
				endIdx := startIdx + 1
				if text[startIdx-1] == ':' && textLen >= endIdx {
					key := text[:startIdx-1]
					keys := strings.Split(key, "->")
					if keys[0] == "[]" {
						if !r.Sys_v.SetJson.OkLen {
							if getLen, ok := r.Sys_v.SetJson.Json.(map[string]any); ok {
								r.Sys_v.SetJson.Len = len(getLen)
							}
							if getLen, ok := r.Sys_v.SetJson.Json.([]any); ok {
								r.Sys_v.SetJson.Len = len(getLen)
							}
							r.Sys_v.SetJson.OkLen = true
						} else {
							r.Sys_v.SetJson.Len++
						}
						keys[0] = strconv.Itoa(r.Sys_v.SetJson.Len)
					}
					value := utils.AnyIsString(r.Val.Text(text[endIdx:]))
					for k, setv := range keys {
						keys[k] = utils.AnyIsString(r.Val.Text(setv))
					}
					r.Sys_v.SetJson.Json = funcs.JsonSetValue(r.Sys_v.SetJson.Json, keys, value, false)
					continue
				}
				if textLen >= endIdx {
					key := text[:startIdx]
					keys := strings.Split(key, "->")
					if keys[0] == "[]" {
						if !r.Sys_v.SetJson.OkLen {
							if getLen, ok := r.Sys_v.SetJson.Json.(map[string]any); ok {
								r.Sys_v.SetJson.Len = len(getLen)
							}
							if getLen, ok := r.Sys_v.SetJson.Json.([]any); ok {
								r.Sys_v.SetJson.Len = len(getLen)
							}
							r.Sys_v.SetJson.OkLen = true
						} else {
							r.Sys_v.SetJson.Len++
						}
						keys[0] = strconv.Itoa(r.Sys_v.SetJson.Len)
					}
					value := utils.AnyIsString(r.Val.Text(text[endIdx:]))
					for k, setv := range keys {
						keys[k] = utils.AnyIsString(r.Val.Text(setv))
					}
					r.Sys_v.SetJson.Json = funcs.JsonSetValue(r.Sys_v.SetJson.Json, keys, value, true)
					continue
				}
			}
		}

		if r.Sys_v.Func.Success {
			forNum := r.Sys_v.Func.Num
			content := r.Sys_v.Func.Content
			funcTrigger := r.Sys_v.Func.Trigger
			if textLen > 7 && text[:7] == "函数>" {
				forNum++
				r.Sys_v.Func.Num = forNum
			}
			if text == "<函数" {
				if forNum == 0 {
					if r.Sys_v.Func.VlaueName == "" {
						// 赋予值名留空：不存储函数框，直接执行内容并输出返回
						funcv := dto.NewVal().
							Reset(r.Val.P.GetAll()).
							Set("触发", funcTrigger).
							Set("触发词", "")
						RunDic := dic_dto.NewRunDicEntry().
							SetGlobal_v(r.Val.G).
							Set_v(funcv).
							SetDic_v(r.Dic).
							WithRecursionDepth(r.RecursionDepth)
						resRunDic := dic_api.Api.DicRunLine(RunDic, content)
						r.Output.Add(resRunDic)
					} else {
						// 插入函数框
						r.Val.P.Set(r.Sys_v.Func.VlaueName, &dto.FuncBox{
							Trigger: funcTrigger,
							Content: content,
						})
					}

					r.Sys_v.Func.Content = []string{}
					r.Sys_v.Func.Success = false
					continue
				}
				forNum--
				r.Sys_v.Func.Num = forNum
			}
			content = append(content, text)
			r.Sys_v.Func.Content = content
			continue
		}

		if r.Sys_v.ForEach.Success {
			forNum := r.Sys_v.ForEach.Num
			content := r.Sys_v.ForEach.Content
			if textLen >= 7 && text[:7] == "遍历>" {
				forNum++
				r.Sys_v.ForEach.Num = forNum
			}
			if text == "<遍历" {
				if forNum == 0 {
					valName := r.Sys_v.ForEach.VlaueName
					RunDic := dic_dto.NewRunDicEntry().
						SetV(r.Val).
						SetDic_v(r.Dic).
						SetRunForEach().
						WithRecursionDepth(r.RecursionDepth)
					RunDic.Trigger = false
					startIdx := strings.IndexByte(valName, ',')
					endIdx := startIdx + 1
					v1 := "_"
					v2 := "_"
					if startIdx != -1 {
						v1 = valName[:startIdx]
						v2 = valName[endIdx:]
					} else {
						v1 = valName
					}

					switch objforV := r.Sys_v.ForEachGetRun().(type) {
					case []byte:
						jsonparser.ObjectEach(objforV, func(keyByte []byte, valueByte []byte, dataType jsonparser.ValueType, offset int) error {
							key := string(keyByte)
							value := string(valueByte)
							r.Val.P.Set(v1, key)
							r.Val.P.Set(v2, value)
							resRun := dic_api.Api.DicRunLine(RunDic, content)
							r.Output.Add(resRun)
							if RunDic.Sys_v.ForEach.Jump || RunDic.Sys_v.Stop.Load() {
								r.Sys_v.ForEach.Jump = false
								return errors.New("stop")
							}
							return nil
						})
						if RunDic.Sys_v.Stop.Load() {
							r.Sys_v.Stop.Store(true)
							return nil
						}

					case []any:
						for key, value := range objforV {
							strNum := strconv.Itoa(key)
							r.Val.P.Set(v1, strNum)
							if strVal, ok := value.(string); ok {
								r.Val.P.Set(v2, strVal)
							} else {
								resS, err := json.Marshal(value)
								if err == nil {
									r.Val.P.Set(v2, string(resS))
								}
							}
							resRun := dic_api.Api.DicRunLine(RunDic, content)
							r.Output.Add(resRun)

							shouldBreak, shouldReturn := handleLoopControl(r, RunDic, "ForEach")
							if shouldBreak {
								break
							}
							if shouldReturn {
								return nil
							}
						}
					default:
						r.Sys_v.ForEach.Close()
						continue
					}

					r.Sys_v.ForEach.Close()
					continue
				}
				forNum--
				r.Sys_v.ForEach.Num = forNum
			}
			content = append(content, text)
			r.Sys_v.ForEach.Content = content
			continue
		}

		if r.Sys_v.For.Success {
			forNum := r.Sys_v.For.Num
			content := r.Sys_v.For.Content
			if textLen >= 7 && text[:7] == "循环>" {
				forNum++
				r.Sys_v.For.Num = forNum
			}
			if text == "<循环" {
				if forNum == 0 {
					valName := r.Sys_v.For.VlaueName
					RunDic := dic_dto.NewRunDicEntry().
						SetV(r.Val).
						SetDic_v(r.Dic).
						SetRunFor().
						WithRecursionDepth(r.RecursionDepth)
					RunDic.Trigger = false

					if r.Sys_v.ForGetRun() == nil {
						i := 0
						for {
							i++
							strNum := strconv.Itoa(i)
							r.Val.P.Set(valName, strNum)
							resRun := dic_api.Api.DicRunLine(RunDic, content)
							r.Output.Add(resRun)
							shouldBreak, shouldReturn := handleLoopControl(r, RunDic, "For")
							if shouldBreak {
								break
							}
							if shouldReturn {
								return nil
							}
							if setNum, ok := r.Val.P.Get(valName).(string); ok && setNum != strNum {
								Xi, err := strconv.Atoi(setNum)
								if err != nil {
									break
								}
								i = Xi
							}
						}
					} else if runi, ok := r.Sys_v.ForGetRun().(int); ok {
						for i := 1; i <= runi; i++ {
							strNum := strconv.Itoa(i)
							r.Val.P.Set(valName, strNum)
							resRun := dic_api.Api.DicRunLine(RunDic, content)
							r.Output.Add(resRun)
							shouldBreak, shouldReturn := handleLoopControl(r, RunDic, "For")
							if shouldBreak {
								break
							}
							if shouldReturn {
								return nil
							}
							if setNum, ok := r.Val.P.Get(valName).(string); ok && setNum != strNum {
								Xi, err := strconv.Atoi(setNum)
								if err != nil {
									break
								}
								i = Xi
							}
						}
					}

					r.Sys_v.For.Num = 0
					r.Sys_v.For.Run = 0
					r.Sys_v.For.Content = []string{}
					r.Sys_v.For.Success = false
					continue
				}
				forNum--
				r.Sys_v.For.Num = forNum
			}
			content = append(content, text)
			r.Sys_v.For.Content = content
			continue
		}

		if r.Sys_v.IfFunc.Success {
			forNum := r.Sys_v.IfFunc.Num
			if textLen > 7 && text[:7] == "如果>" {
				forNum++
				r.Sys_v.IfFunc.Num = forNum
			}
			if text == "<如果" {
				if forNum == 0 {
					RunDic := dic_dto.NewRunDicEntry().
						SetV(r.Val).
						SetDic_v(r.Dic).
						SetRunIf().
						WithRecursionDepth(r.RecursionDepth)
					if r.Sys_v.For.IsFor {
						RunDic.SetRunFor()
					}
					if r.Sys_v.ForEach.IsFor {
						RunDic.SetRunForEach()
					}
					RunDic.Trigger = false

					for i := 0; i <= r.Sys_v.IfFunc.IfNum; i++ {
						var ifval bool = Pd(funcV, r.Sys_v.IfFunc.If[i])
						if ifval {
							resRun := dic_api.Api.DicRunLine(RunDic, r.Sys_v.IfFunc.Run[i])
							r.Output.Add(resRun)

							if r.Sys_v.For.IsFor && RunDic.Sys_v.For.IsFor && RunDic.Sys_v.For.Jump {
								r.Sys_v.For.Jump = true
								return nil
							}

							if r.Sys_v.ForEach.IsFor && RunDic.Sys_v.ForEach.IsFor && RunDic.Sys_v.ForEach.Jump {
								r.Sys_v.ForEach.Jump = true
								return nil
							}

							if RunDic.Sys_v.Stop.Load() {
								r.Sys_v.Stop.Store(true)
								return nil
							}
							break
						} else {
							if i != r.Sys_v.IfFunc.IfNum {
								continue
							}
							resRun := dic_api.Api.DicRunLine(RunDic, r.Sys_v.IfFunc.Else)
							r.Output.Add(resRun)
						}

						if r.Sys_v.For.IsFor && RunDic.Sys_v.For.IsFor && RunDic.Sys_v.For.Jump {
							r.Sys_v.For.Jump = true
							RunDic.Close()
							return nil
						}

						if r.Sys_v.ForEach.IsFor && RunDic.Sys_v.ForEach.IsFor && RunDic.Sys_v.ForEach.Jump {
							r.Sys_v.ForEach.Jump = true
							RunDic.Close()
							return nil
						}

						if !r.Sys_v.IfFunc.IsIf && RunDic.Sys_v.Stop.Load() {
							RunDic.Close()
							return nil
						}

						if RunDic.Sys_v.IfFunc.Jump {
							RunDic.Close()
							r.Sys_v.IfFunc.Jump = false
							break
						}
						if RunDic.Sys_v.Stop.Load() {
							RunDic.Close()
							r.Sys_v.Stop.Store(true)
							return nil
						}
					}

					r.Sys_v.IfFunc.If = []string{}
					r.Sys_v.IfFunc.Else = []string{}
					r.Sys_v.IfFunc.Run = [][]string{}
					r.Sys_v.IfFunc.IfNum = 0
					r.Sys_v.IfFunc.IsElse = false
					r.Sys_v.IfFunc.Success = false
					continue
				}
				forNum--
				r.Sys_v.IfFunc.Num = forNum
			}

			if forNum == 0 {
				if !r.Sys_v.IfFunc.IsElse && text == ">否则" {
					r.Sys_v.IfFunc.IsElse = true
					continue
				}
				if !r.Sys_v.IfFunc.IsElse && textLen > 14 && text[:14] == ">否则如果:" {
					r.Sys_v.IfFunc.IfNum++
					r.Sys_v.IfFunc.If = append(r.Sys_v.IfFunc.If, text[14:])
					continue
				}
			}

			if r.Sys_v.IfFunc.IsElse {
				r.Sys_v.IfFunc.Else = append(r.Sys_v.IfFunc.Else, text)
			} else {
				if r.Sys_v.IfFunc.IfNum >= len(r.Sys_v.IfFunc.Run) {
					r.Sys_v.IfFunc.Run = append(r.Sys_v.IfFunc.Run, []string{})
				}
				r.Sys_v.IfFunc.Run[r.Sys_v.IfFunc.IfNum] = append(r.Sys_v.IfFunc.Run[r.Sys_v.IfFunc.IfNum], text)
			}
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

				if text == "返回" && index+1 < txtLen && txt[index+1] == "如果尾" {
					lock = false
					isif = false
					index++
					continue
				}

				if textLen > 5 && text[:5] == "elif:" {
					isif = true
					lock = true
					var ifval bool = Pd(funcV, text[5:])
					if ifval {
						lock = false
						continue
					}
					continue
				}

				if textLen > 13 && text[:13] == "否则如果:" {
					isif = true
					lock = true
					var ifval bool = Pd(funcV, text[13:])
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
				if text == "返回" && index+1 < txtLen && txt[index+1] == "如果尾" {
					break
				}
			}
		}

		if textLen > 3 && text[:3] == "if:" {
			isif = true
			lock = true
			var ifval bool = Pd(funcV, text[3:])
			if ifval {
				lock = false
				continue
			}
			continue
		}

		if textLen > 7 && text[:7] == "如果:" {
			isif = true
			lock = true
			var ifval bool = Pd(funcV, text[7:])
			if ifval {
				lock = false
				continue
			}
			continue
		}

		if lock {
			continue
		}

		if text == ">跳过" && r.Sys_v.For.IsFor {
			return nil
		}

		if text == ">跳过" && r.Sys_v.ForEach.IsFor {
			return nil
		}

		if text == ">终止循环" && r.Sys_v.For.IsFor {
			r.Sys_v.For.Jump = true
			return nil
		}

		if text == ">终止遍历" && r.Sys_v.ForEach.IsFor {
			r.Sys_v.ForEach.Jump = true
			return nil
		}

		if text == ">跳过" && r.Sys_v.IfFunc.IsIf {
			r.Sys_v.IfFunc.Jump = true
			return nil
		}

		if text == ">终止" {
			r.Sys_v.Stop.Store(true)
			return nil
		}

		if retretMsg, ok := strings.CutPrefix(text, ">终止 "); ok && retretMsg != "" {
			r.Sys_v.Stop.Store(true)
			r.Output.Add(retretMsg)
			return nil
		}

		if textLen >= 7 && text[:7] == "函数>" {
			if startIdx := strings.IndexByte(text, '='); startIdx != -1 {
				endIdx := startIdx + 1
				if textLen >= endIdx {
					key := text[7:startIdx]
					value := text[endIdx:]
					r.Sys_v.Func.VlaueName = key
					r.Sys_v.Func.Trigger = value
					r.Sys_v.Func.Success = true
					continue
				}
			}
			key := text[7:]
			r.Sys_v.Func.VlaueName = key
			r.Sys_v.Func.Trigger = ""
			r.Sys_v.Func.Success = true
			continue
		}

		if textLen > 7 && text[:7] == "如果>" {
			key := text[7:]
			r.Sys_v.IfFunc.If = append(r.Sys_v.IfFunc.If, key)
			r.Sys_v.IfFunc.Success = true
			continue
		}

		if textLen >= 10 && text[:10] == "纯文本>" {
			if startIdx := strings.IndexByte(text, '='); startIdx != -1 {
				endIdx := startIdx + 1
				if textLen >= endIdx {
					key := text[10:startIdx]
					value := text[endIdx:]
					runText := utils.AnyIsString(r.Val.Text(value))
					r.Sys_v.Text.Success = true
					r.Sys_v.Text.ReadValue = false
					r.Sys_v.Text.VlaueName = key
					r.Sys_v.Text.LineFeed = runText
					continue
				}
			}
			runText := utils.AnyIsString(r.Val.Text(text[10:]))
			r.Sys_v.Text.Success = true
			r.Sys_v.Text.ReadValue = false
			r.Sys_v.Text.VlaueName = ""
			r.Sys_v.Text.LineFeed = runText
			continue
		}

		if textLen == 6 && text == "JSON>[" {
			r.Sys_v.SetNewJson.Success = true
			r.Sys_v.SetNewJson.Json = "["
			r.Sys_v.SetNewJson.JsonType = true
			r.Sys_v.SetNewJson.Len = 1
			r.Sys_v.SetNewJson.VlaueName = ""
			continue
		}

		if textLen == 6 && text == "JSON>{" {
			r.Sys_v.SetNewJson.Success = true
			r.Sys_v.SetNewJson.Json = "{"
			r.Sys_v.SetNewJson.JsonType = true
			r.Sys_v.SetNewJson.Len = 1
			r.Sys_v.SetNewJson.VlaueName = ""
			continue
		}

		if textLen >= 5 && text[:5] == "JSON>" {
			if startIdx := strings.IndexByte(text, '='); startIdx != -1 {
				endIdx := startIdx + 1
				if textLen >= endIdx {
					key := text[5:startIdx]
					value := text[endIdx:]
					runText := utils.AnyIsString(r.Val.Text(value))
					err := json.Unmarshal([]byte(runText), &r.Sys_v.SetJson.Json)
					if err == nil {
						r.Sys_v.SetJson.Success = true
						r.Sys_v.SetJson.VlaueName = key
					}
					continue
				}
			}
			runText := utils.AnyIsString(r.Val.Text(text[5:]))
			err := json.Unmarshal([]byte(runText), &r.Sys_v.SetJson.Json)
			if err == nil {
				r.Sys_v.SetJson.Success = true
				r.Sys_v.SetJson.VlaueName = ""
			}
			continue
		}

		if getInput, ok := strings.CutPrefix(text, "文本>"); ok {
			if startIdx := strings.IndexByte(getInput, '='); startIdx != -1 {
				endIdx := startIdx + 1
				if textLen >= endIdx {
					key := getInput[:startIdx]
					value := getInput[endIdx:]
					runText := utils.AnyIsString(r.Val.Text(value))
					r.Sys_v.Text.Success = true
					r.Sys_v.Text.ReadValue = true
					r.Sys_v.Text.VlaueName = key
					r.Sys_v.Text.LineFeed = runText
					r.Sys_v.Text.Content.Reset()
					continue
				}
			}
			runText := utils.AnyIsString(r.Val.Text(getInput))
			r.Sys_v.Text.Success = true
			r.Sys_v.Text.ReadValue = true
			r.Sys_v.Text.VlaueName = ""
			r.Sys_v.Text.LineFeed = runText
			r.Sys_v.Text.Content.Reset()
			continue
		}

		if textLen >= 7 && text[:7] == "遍历>" {
			if startIdx := strings.IndexByte(text, '='); startIdx != -1 {
				endIdx := startIdx + 1
				if textLen >= endIdx {
					key := text[7:startIdx]
					value := text[endIdx:]
					runText := utils.AnyToString(Runs(funcV, utils.AnyToString(count.RunCountText(r.Val, value))))
					var testjs map[string]any
					if json.Unmarshal([]byte(runText), &testjs) == nil {
						r.Sys_v.ForEach.Run = []byte(runText)
					} else {
						var thisjson []any
						if json.Unmarshal([]byte(runText), &thisjson) == nil {
							r.Sys_v.ForEach.Run = thisjson
						}
					}
					r.Sys_v.ForEach.VlaueName = key
					r.Sys_v.ForEach.Success = true
					continue
				}
			}
			key := text[7:]
			r.Sys_v.ForEach.VlaueName = key
			r.Sys_v.ForEach.Success = true
			continue
		}

		if textLen >= 7 && text[:7] == "循环>" {
			if startIdx := strings.IndexByte(text, '='); startIdx != -1 {
				endIdx := startIdx + 1
				if textLen >= endIdx {
					key := text[7:startIdx]
					value := text[endIdx:]
					runText := utils.AnyToString(Runs(funcV, utils.AnyToString(count.RunCountText(r.Val, value))))
					// 将字符串解析为整数
					intValue, err := strconv.Atoi(runText)
					if err == nil {
						r.Sys_v.For.Run = intValue
					} else {
						r.Sys_v.For.Run = 1
					}
					r.Sys_v.For.VlaueName = key
					r.Sys_v.For.Success = true
					continue
				}
			}
			key := text[7:]
			r.Sys_v.For.VlaueName = key
			r.Sys_v.For.Run = nil
			r.Sys_v.For.Success = true
			continue
		}

		if jumpPd, ok := strings.CutPrefix(text, ">跳行("); ok && jumpPd != "" {
			// 分割")>>"
			if PdText := strings.SplitN(jumpPd, ")>>", 2); len(PdText) == 2 {
				if Pd(funcV, PdText[0]) {
					runText := utils.AnyIsString(r.Val.Text(PdText[1]))
					seti, err := strconv.Atoi(runText)
					if err != nil {
						continue
					}
					if seti < 0 {
						seti -= 1
					}
					index = index + seti
				}
				continue
			}
		}

		if text == "--js" {
			r.Sys_v.NodeJs.Success = true
			continue
		}

		if textLen > 2 && text[:2] == "#:" {
			go func() {
				// 创建独立的执行上下文，避免主流程终止/STOP 时异步执行被掐断
				independentFuncV := &dic_dto.DicFunc{
					Val: funcV.Val,
					Sys: &dto.LocalDicValue{},
					Dic: funcV.Dic,
				}
				Runs(independentFuncV, utils.AnyToString(count.RunCountText(r.Val, text[2:])))
			}()
			continue
		}

		if strings.HasPrefix(text, "https://") || strings.HasPrefix(text, "http://") {
			res := utils.AnyToString(Runs(funcV, text))
			if r.Sys_v.Stop.Load() {
				return nil
			}
			r.Output.Add(res)
			continue
		}

		// 类成员变量赋值：.成员名:值（写入当前实例）
		if textLen > 2 && text[0] == '.' {
			if idx := strings.IndexByte(text, ':'); idx > 1 {
				if classData, ok := r.Val.P.Get("Class").(*dto.DicClass); ok && classData != nil {
					classData.LocalValue.Set(text[1:idx], RunsAny(funcV, text[idx+1:]))
					continue
				}
			}
		}

		vType, vPrefix, vSuffix := dicBuild.ValTextTest(text)
		if vType != 0 {
			var vSetData string
			// fmt.Println(vType, vPrefix, vSuffix)

			switch vType {
			case 1, 2, 7, 8:
				vSetData = utils.AnyToString(Runs(funcV, utils.AnyToString(count.RunCountText(r.Val, vSuffix))))
			}

			switch vType {
			case 1:
				var valStr string
				if str, ok := r.Val.P.Get(vPrefix).(string); ok {
					valStr = str
				}
				one, err1 := strconv.ParseFloat(valStr, 64)
				two, err2 := strconv.ParseFloat(vSetData, 64)
				if err1 == nil && err2 == nil {
					r.Val.P.Set(vPrefix, strconv.FormatFloat(one-two, 'f', -1, 64))
					continue
				}
			case 2:
				var valStr string
				if str, ok := r.Val.P.Get(vPrefix).(string); ok {
					// 追加json
					if j := utils.IsJSONResult(str); j != nil {
						if j, ok := j.([]any); ok {
							j = append(j, vSetData)
							if j, err := json.Marshal(j); err == nil {
								r.Val.P.Set(vPrefix, string(j))
								continue
							}
						}
						if j, ok := j.(map[string]any); ok {
							j[strconv.Itoa(len(j))] = vSetData
							if j, err := json.Marshal(j); err == nil {
								r.Val.P.Set(vPrefix, string(j))
								continue
							}
						}
					}
					valStr = str
				}
				one, err1 := strconv.ParseFloat(valStr, 64)
				two, err2 := strconv.ParseFloat(vSetData, 64)
				if err1 == nil && err2 == nil {
					r.Val.P.Set(vPrefix, strconv.FormatFloat(one+two, 'f', -1, 64))
					continue
				} else {
					r.Val.P.Set(vPrefix, valStr+vSetData)
					continue
				}
			case 7: // 乘法
				var valStr string
				if str, ok := r.Val.P.Get(vPrefix).(string); ok {
					valStr = str
				}
				one, err1 := strconv.ParseFloat(valStr, 64)
				two, err2 := strconv.ParseFloat(vSetData, 64)
				if err1 == nil && err2 == nil {
					r.Val.P.Set(vPrefix, strconv.FormatFloat(one*two, 'f', -1, 64))
					// 复读字符串
				} else if err1 != nil && err2 == nil {
					r.Val.P.Set(vPrefix, strings.Repeat(valStr, int(two)))
				}
				continue
			case 8: // 除法
				var valStr string
				if str, ok := r.Val.P.Get(vPrefix).(string); ok {
					valStr = str
				}
				one, err1 := strconv.ParseFloat(valStr, 64)
				two, err2 := strconv.ParseFloat(vSetData, 64)
				if err1 == nil && err2 == nil {
					r.Val.P.Set(vPrefix, strconv.FormatFloat(one/two, 'f', -1, 64))
				}
				continue
			case 3:
				r.Val.P.Set(vPrefix, Runs(funcV, vSuffix))
				continue
			case 4:
				r.Val.P.Set(vPrefix, r.Val.Text(vSuffix))
				continue
			case 5:
				r.Val.P.Set(vPrefix, vSuffix)
				continue
			case 6:
				// fmt.Println("键：", vPrefix, "值：", vSuffix)
				if vPrefix == "" {
					res := utils.AnyToString(Runs(funcV, text))
					if r.Sys_v.Stop.Load() {
						return nil
					}
					r.Output.Add(res)
					continue
				}

				if vSuffix == "" {
					r.Val.P.Set(vPrefix, "")
					continue
				}

				// >>> 连续执行框，往下逐行执行并写回赋予值
				if vSuffix == ">>>" {
					r.Sys_v.ValChain.Success = true
					r.Sys_v.ValChain.VlaueName = vPrefix
					break
				}

				// >>> 连续执行，复用当前赋予值名字（值为 JSON 时不拆分）
				if strings.Contains(vSuffix, ">>>") && utils.IsJSONResult(vSuffix) == nil {
					for _, chainPart := range SplitValChain(vSuffix) {
						if chainPart == "" {
							continue
						}
						runValSet(r, funcV, vPrefix, chainPart)
					}
					continue
				}

				if setJsonHead := strings.Split(vPrefix, "->"); len(setJsonHead) > 1 {
					vPrefix = setJsonHead[0]
					setJsonHead = setJsonHead[1:]
					// 设置json
					if str, ok := r.Val.P.Get(vPrefix).(string); ok {
						if j := utils.IsJSONResult(str); j != nil {
							if j, ok := j.(map[string]any); ok {
								vSetData := utils.AnyToString(Runs(funcV, utils.AnyToString(count.RunCountText(r.Val, vSuffix))))
								j := funcs.JsonSetValue(j, setJsonHead, vSetData, false)
								if j, err := json.Marshal(j); err == nil {
									r.Val.P.Set(vPrefix, string(j))
									continue
								}
								r.Val.P.Set(vPrefix, vSetData)
							}
							if j, ok := j.([]any); ok {
								vSetData := utils.AnyToString(Runs(funcV, utils.AnyToString(count.RunCountText(r.Val, vSuffix))))
								j := funcs.JsonSetValue(j, setJsonHead, vSetData, false)
								if j, err := json.Marshal(j); err == nil {
									r.Val.P.Set(vPrefix, string(j))
									continue
								}
							}
						}
					}
					continue
				}

				// 判断开头[结尾]
				if strings.HasPrefix(vSuffix, "[") && strings.HasSuffix(vSuffix, "]") {
					r.Val.P.Set(vPrefix, count.RunCountText(r.Val, Runs(funcV, vSuffix)))
					continue
				}

				// 判断开头$
				// if strings.HasPrefix(vSuffix, "$") {
				// 	r.Val.P.Set(vPrefix, funcV.Runs(count.RunCountText(r.Val, vSuffix)))
				// 	continue
				// }

				if vSuffix == "{" {
					r.Sys_v.SetNewJson.Success = true
					r.Sys_v.SetNewJson.Json = "{"
					r.Sys_v.SetNewJson.JsonType = true
					r.Sys_v.SetNewJson.Len = 1
					r.Sys_v.SetNewJson.VlaueName = vPrefix
					continue
				}
				if vSuffix == "[" {
					r.Sys_v.SetNewJson.Success = true
					r.Sys_v.SetNewJson.Json = "["
					r.Sys_v.SetNewJson.JsonType = true
					r.Sys_v.SetNewJson.Len = 1
					r.Sys_v.SetNewJson.VlaueName = vPrefix
					continue
				}
				if vSuffix == `"""` {
					r.Sys_v.ValText.Success = true
					r.Sys_v.ValText.VlaueName = vPrefix
					break
				}
				if vSuffix == `'''` {
					r.Sys_v.ValTextr.Success = true
					r.Sys_v.ValTextr.VlaueName = vPrefix
					break
				}

				runValSet(r, funcV, vPrefix, vSuffix)
			}
			continue
		}
		// 判断尾部为\r，替换尾部为换行
		if strings.HasSuffix(text, "\\r") {
			text = text[:len(text)-2] + "\n"
		}
		res := utils.AnyToString(Runs(funcV, text))
		if r.Sys_v.Stop.Load() {
			return nil
		}
		r.Output.Add(res)
	}
	return nil
}

// SplitValChain 按 >>> 分割赋予值内容，跳过 $...$ 函数块内部的 >>>，用于赋予值连续执行
func SplitValChain(text string) []string {
	var parts []string
	var b strings.Builder
	inFunc := false
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c == '$' {
			// 统计前置反斜杠，判断是否被转义，\$ 不参与块状态切换
			bs := 0
			for j := i - 1; j >= 0 && text[j] == '\\'; j-- {
				bs++
			}
			if bs%2 == 0 {
				inFunc = !inFunc
			}
			b.WriteByte(c)
			continue
		}
		if !inFunc && c == '>' && i+2 < len(text) && text[i+1] == '>' && text[i+2] == '>' {
			parts = append(parts, b.String())
			b.Reset()
			i += 2
			continue
		}
		b.WriteByte(c)
	}
	// 忽略末尾空段（如 "a>>>"），保持返回结果整洁
	if b.Len() > 0 {
		parts = append(parts, b.String())
	}
	return parts
}

// runValSet 执行赋予值内容并写回变量，支持 ?: 回退 与 @json路径
func runValSet(r *dic_dto.DicEntry, funcV *dic_dto.DicFunc, vPrefix, value string) {
	GetIfKeys := strings.Split(value, "?:")
	var runText any
questionCycle:
	for _, GetIfKey := range GetIfKeys {
		if strings.HasPrefix(GetIfKey, "@") {
			keys := strings.Split(GetIfKey, "->")
			if len(keys) < 2 {
				runText, stopSetVal := RunsVal(funcV, utils.AnyToString(count.RunCountText(r.Val, GetIfKey)), vPrefix)
				if stopSetVal {
					break
				}
				switch runText {
				case "", "null", "NULL", "Null", "false", "False", "FALSE":
					r.Val.P.Set(vPrefix, runText)
					continue
				}
				r.Val.P.Set(vPrefix, runText)
				break
			}
			for RunI, key := range keys {
				// 第一次加载解析数据
				if RunI == 0 {
					// 读取数据去除@
					runTexts := RunsAny(funcV, key[1:])
					// 推断数据map
					if rJ, ok := runTexts.(map[string]string); ok {
						runText = rJ
						continue
					}
					if rJ, ok := runTexts.(map[string]any); ok {
						runText = rJ
						continue
					}
					// 字符串转换后解析数据
					if runTexts, ok := runTexts.(string); ok {
						if rJ := utils.IsJSONResult(runTexts); rJ != nil {
							runText = rJ
							continue
						}
					}
					continue
				}
				// 解析数据
				switch objData := runText.(type) {
				case map[string]string:
					if rD, ok := objData[key]; ok {
						runText = rD
					} else {
						runText = ""
						break
					}
				case map[string]any:
					if rD, ok := objData[key]; ok {
						switch num := rD.(type) {
						case int:
							runText = strconv.FormatInt(int64(num), 10)
						case int64:
							runText = strconv.FormatInt(num, 10)
						case float64:
							runText = strconv.FormatFloat(num, 'f', -1, 64)
						default:
							runText = num
						}
					} else {
						runText = ""
						break
					}
				case []any:
					if num, err := strconv.Atoi(key); err == nil {
						if num >= 0 && num < len(objData) {
							rD := objData[num]
							runText = rD
						} else {
							runText = ""
							break
						}
					}
				}
				if objData, ok := runText.(string); ok {
					runText = objData
					break
				}
			}
			// 判断是否为空
			if rStr, ok := runText.(string); ok {
				switch rStr {
				case "", "null", "NULL", "Null", "false", "False", "FALSE":
					r.Val.P.Set(vPrefix, runText)
				default:
					r.Val.P.Set(vPrefix, runText)
					break questionCycle
				}
			}
			if runText != nil {
				// 最后一个直接设置
				r.Val.P.Set(vPrefix, utils.AnyToString(runText))
				continue
			}
		} else {
			runText, stopSetVal := RunsVal(funcV, GetIfKey, vPrefix)
			if stopSetVal {
				break
			}
			switch runText {
			case "", "null", "NULL", "Null", "false", "False", "FALSE":
				r.Val.P.Set(vPrefix, runText)
			default:
				r.Val.P.Set(vPrefix, runText)
				break questionCycle
			}
			continue
		}
	}
}
