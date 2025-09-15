package dic

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/cjxpj/nebula/count"
	"github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"
	"github.com/cjxpj/nebula/utils"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/websocket"
)

// 函数跟变量
func (d *DicFunc) Runs(text string) string {
	// fmt.Println("开始执行函数")
	// fmt.Println(text)
	output := run.BuildFuncStr(text, func(valStr []string) (string, bool) {
		// fmt.Println(valStr)
		resAny, err := d.Funcs(valStr)
		if err != nil {
			log.Printf("[%s]%s：%v", d.Val.Get("_词库路径_"), valStr[0], err)
		}
		if resStr, ok := resAny.(string); ok {
			return resStr, false
		}
		return "", false
	}, func(s string) (string, bool) {
		return utils.AnyIsString(d.Val.Text(s)), false
	})
	return output
}

// 执行，函数跟变量
func (d *DicFunc) RunsAny(text string) any {
	// 拦截外部赋予值
	var resA any
	output := run.BuildFuncStr(text, func(valStr []string) (string, bool) {
		resAny, err := d.Funcs(valStr)
		if err != nil {
			fmt.Println(err)
		}
		if resAny == nil {
			return "", false
		}
		if resStr, ok := resAny.(string); ok {
			return resStr, false
		}
		resA = resAny
		return "", true
	}, func(s string) (string, bool) {
		resAny := d.Val.Text(s)
		if resStr, ok := resAny.(string); ok {
			return resStr, false
		}
		resA = resAny
		return "", true
	})
	if resA != nil {
		return resA
	}
	return output
}

// 赋予值执行，函数跟变量
func (d *DicFunc) RunsVal(text string, setVal string) (string, bool) {
	// 拦截外部赋予值
	strNo := false
	output := run.BuildFuncStr(text, func(valStr []string) (string, bool) {
		resAny, err := d.Funcs(valStr)
		if err != nil {
			fmt.Println(err)
		}
		if resStr, ok := resAny.(string); ok {
			return resStr, false
		}
		d.Val.P.Set(setVal, resAny)
		strNo = true
		return "", false
	}, func(s string) (string, bool) {
		return utils.AnyIsString(d.Val.Text(s)), false
	})
	return output, strNo
}

// 纯函数
func (d *DicFunc) Run(text string) string {
	output := run.BuildFuncStr(text, func(valStr []string) (string, bool) {
		resAny, err := d.Funcs(valStr)
		if err != nil {
			log.Printf("[%s]%s：%v", d.Val.Get("_词库路径_"), valStr[0], err)
		}
		if resStr, ok := resAny.(string); ok {
			return resStr, false
		}
		return "", false
	}, func(s string) (string, bool) {
		return s, false
	})
	return output
}

func caseRestart() (string, error) {
	// 获取当前程序的路径
	exe, err := exec.LookPath(os.Args[0])
	if err != nil {
		return "", fmt.Errorf("无法找到程序路径: %v", err)
	}

	// 使用 os/exec 调用自身程序
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 启动新进程
	err = cmd.Start()
	if err != nil {
		return "", fmt.Errorf("重启失败: %v", err)
	}

	// 根据操作系统退出当前进程
	defer os.Exit(0)

	return "", nil
}

func (d *DicFunc) Funcs(linesStr []string) (any, error) {
	linesLen := len(linesStr)
	if linesLen <= 0 {
		return "$$", nil
	}

	funcName := linesStr[0]
	// 面对象
	if len(funcName) > 1 && (funcName[0] == '.' || funcName[0] == '%') {
		linesStr[0] = funcName[1:]

		var isV bool
		if funcName[0] == '%' {
			isV = true
		}
		if funcName == "自己" {
			funcName = d.Val.P.Get("Class").(string)
		}
		classData := d.Dic.LocalClass[funcName]
		if classData == nil {
			return "", errors.New("非整合包")
		}

		if isV {
			if linesLen == 3 {
				resVT := utils.AnyIsString(d.Val.Text(count.RunCountText(d.Val, linesStr[1])))
				resVTs := utils.AnyIsString(d.Val.Text(count.RunCountText(d.Val, linesStr[2])))
				classData.LocalValue.Set(resVT, resVTs)
				return "", nil
			}
			if linesLen == 2 {
				resVT := utils.AnyIsString(d.Val.Text(count.RunCountText(d.Val, linesStr[1])))
				resV, _ := classData.LocalValue.Get(resVT).(string)
				return resV, nil
			}
			return "未知整合包变量方法", nil
		}

		if linesLen <= 1 {
			return "未知整合包方法", nil
		}

		TStr := strings.Join(linesStr[1:], " ")
		// 整合包局部函数
		if str, Tstr, _, regex := run.RunFor(classData.LocalFunc, TStr, 0); regex != nil {
			funcv := dto.NewVal()
			funcv.Set("触发", Tstr)
			funcv.Set("触发词", TStr)
			funcv.Set("Class", funcName)
			dto.ValRunTrigger(TStr, Tstr, funcv, d.Val.P)
			RunDic := NewRunDicEntry().
				CloseTrigger().
				SetGlobal_v(d.Val.G).
				Set_v(funcv).
				SetDic_v(d.Dic.Clone())
			RunDic.ClearDicFuncs()
			resRunDic := RunDic.Run(str)
			return resRunDic, nil
		}
	} else {
		text := strings.Join(linesStr, " ")
		// 局部函数
		if str, Tstr, _, regex, tparts := run.RunFors(d.Dic.LocalFunc, text, 0); regex != nil {
			funcv := dto.NewVal()
			give, ok := d.Val.P.Get("_继承_").(string)
			if ok && give != "" {
				for _, v := range strings.Split(give, ",") {
					set, ok := d.Val.P.Get(v).(string)
					if ok {
						funcv.Set(v, set)
					}
				}
				d.Val.P.Set("_继承_", "")
			}
			funcv.Set("触发", Tstr)
			funcv.Set("触发词", text)
			dto.ValRunTrigger(text, Tstr, funcv, d.Val.P)
			RunDic := NewRunDicEntry().
				CloseTrigger().
				SetGlobal_v(d.Val.G).
				Set_v(funcv).
				SetDic_v(d.Dic.Clone())
			RunDic.ClearDicFuncs()

			resRunDic := RunDic.Run(str)
			if tparts != "" {
				subParts := strings.Split(tparts, ",")
				for _, setv := range subParts {
					getv := RunDic.Val.P.Get(setv)
					d.Val.P.Set(setv, getv)
				}
			}
			return resRunDic, nil
		}
	}

	inputsLen := len(linesStr)
	lines := make([]any, inputsLen)
	inputs := utils.NewDicInputs()
	inputs.Set(make([]any, inputsLen))
	inputsLen--

	for i, line := range linesStr {
		lines[i] = line
		inputs.List[i] = d.Val.Text(count.RunCountText(d.Val, line))
	}

	f := &funcs.DicFunc{
		Len:       inputsLen,
		InputData: linesStr,
		Inputs:    inputs,
	}

	// 自定义函数
	if fn, ok := d.Dic.MyFunc[lines[0].(string)]; ok {
		return fn(d.Val, inputs)
	}

	// 系统函数
	if fnInfo, ok := funcs.GetFunc(lines[0].(string)); ok {
		if inputs.LenOk(fnInfo.L) {
			return fnInfo.Fn(dto.NewDicInputs(d.Dic, d.Val, inputs))
		}
	}

	switch lines[0].(string) {
	case "捕获输出":
		if inputsLen == 0 {
			return d.Output.Get(), nil
		}
		return "", nil

	case "拦截输出":
		if inputsLen == 0 {
			res := d.Output.Get()
			d.Output.Clear()
			return res, nil
		}
		return "", nil

	case "STOP":
		if inputsLen == 0 {
			utils.LogStop(d.Output.Get())
			return "", nil
		}
		return "", nil

	case "重启":
		if inputsLen == 0 {
			return caseRestart()
		}
		return "", errors.New("重启参数错误")

	case "WS发送":
		if inputsLen == 2 {
			if conn_ws, ok := inputs.Get(1).(*websocket.Conn); ok {
				if err := conn_ws.WriteMessage(websocket.TextMessage, []byte(inputs.String(2))); err != nil {
					return "", err
				}
				return "", nil
			}
		}
		return "", nil

	case "WS断开":
		if inputsLen == 1 {
			if conn_ws, ok := inputs.Get(1).(*websocket.Conn); ok {
				conn_ws.Close()
				return "", nil
			}
		}
		return "", nil

	case "WS连接":
		if inputs.LenOk(1, 2) {
			addr := inputs.String(1)

			dicpath := "private/websocket/app.n"
			if inputsLen == 2 {
				dicpath = inputs.String(2)
			}

			// 确定 URL 的 Scheme 是 ws 还是 wss
			scheme := "ws"
			if strings.HasPrefix(addr, "wss://") || strings.HasPrefix(addr, "https://") {
				scheme = "wss"
			}

			// 移除前缀，确保 Host 和 Path 部分正确
			addr = strings.TrimPrefix(addr, "ws://")
			addr = strings.TrimPrefix(addr, "wss://")
			addr = strings.TrimPrefix(addr, "http://")
			addr = strings.TrimPrefix(addr, "https://")

			addUrl := scheme + "://" + addr
			// 创建 WebSocket 连接
			conn, _, err := websocket.DefaultDialer.Dial(addUrl, nil)
			if err != nil {
				return "", err
			}

			// 运行词库
			if wsFileData, err := utils.NewFileQueue(dicpath).ReadFromFile(); err == nil {
				dic := NewDic(dicpath, wsFileData).
					SetFunc("断开连接", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
						conn.Close()
						return "", nil
					})
				resData := dic.RunPrivate("连接成功")
				if resData != "" {
					if err := conn.WriteMessage(websocket.TextMessage, []byte(resData)); err != nil {
						fmt.Println("发送消息时出错:", err)
					}
				}
			}

			messageTypeMap := map[int]string{
				websocket.TextMessage:   "文本消息",
				websocket.BinaryMessage: "二进制消息",
			}

			go func() {
				// 读取来自 WebSocket 服务器的消息
				for {
					Tstr := ""
					wsClose := false
					messageType, message, readMsgErr := conn.ReadMessage()
					if readMsgErr != nil {
						// 判断是否是正常关闭
						if websocket.IsUnexpectedCloseError(readMsgErr, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
							fmt.Println("读取消息时出错:", readMsgErr)
							Tstr = "断开连接"
							wsClose = true
						} else {
							Tstr = "断开连接"
							wsClose = true
						}
						conn.Close()
					} else {
						Tstr = string(message)
					}
					typeName, ok := messageTypeMap[messageType]
					if !ok {
						typeName = "未知消息"
					}
					// fmt.Println("收到:", typeName, Tstr)

					wsfile := utils.NewFileQueue(dicpath)
					wsfileData, err := wsfile.ReadFromFile()
					if err != nil {
						fmt.Println("读取文件时出错:", err)
						conn.Close() // 关闭连接
						break
					}
					d := NewDic(dicpath, wsfileData)
					d.SetFunc("断开连接", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
						conn.Close()
						return "", nil
					})
					d.Val.P.Set("类型", typeName)
					rStr := ""
					if wsClose {
						rStr = d.RunPrivate(Tstr)
					} else {
						rStr = d.Run(Tstr)
					}
					// 拦截并处理错误
					if readMsgErr != nil {
						if rStr != "" {
							fmt.Println(rStr)
							break
						}
					}
					if rStr != "" {
						if wsClose {
							fmt.Println(rStr)
						} else if err := conn.WriteMessage(websocket.TextMessage, []byte(rStr)); err != nil {
							fmt.Println("发送消息时出错:", err)
						}
					}
				}
			}()
			return conn, nil
		}
		return "", nil

	case "替换":
		if inputsLen == 4 {
			tStr := inputs.List[3].(string)
			num, err := strconv.Atoi(inputs.List[4].(string))
			if err != nil {
				return "非数字", nil
			}
			res := strings.Replace(inputs.List[1].(string), inputs.List[2].(string), tStr, num)
			return res, nil
		}
		if inputsLen == 2 || inputsLen == 3 {
			var tStr string
			if inputsLen == 3 {
				tStr = inputs.List[3].(string)
				if tStr == lines[3] && strings.HasPrefix(lines[3].(string), "%") && strings.HasSuffix(lines[3].(string), "%") && strings.Count(lines[3].(string), "%") == 2 {
					var regex *regexp.Regexp
					obj := d.Val.P.GetObj(lines[3].(string)[1 : len(lines[3].(string))-1])
					if t, ok := obj["type"].(string); ok && t == "函数框" {
						funcTrigger := obj["trigger"].(string)
						regex = regexp.MustCompile("^" + funcTrigger + "$")
						num := 0
						res := run.ReplaceFunc(inputs.List[1].(string), inputs.List[2].(string), func(s string) string {
							num++
							strNum := strconv.Itoa(num)
							matches := regex.FindStringSubmatch(strNum)
							if len(matches) > 0 || funcTrigger == "" {
								funcv := dto.NewVal()
								funcv.Reset(d.Val.P.GetAll())
								funcv.Set("触发", funcTrigger)
								funcv.Set("触发词", strNum)
								content := obj["content"].([]string)
								RunDic := NewRunDicEntry().
									SetGlobal_v(d.Val.G).
									Set_v(funcv).
									SetDic_v(d.Dic)
								return RunDic.Run(content)
							}
							return ""
						})
						return res, nil
					}
				}
			}
			res := strings.ReplaceAll(inputs.List[1].(string), inputs.List[2].(string), tStr)
			return res, nil
		}

		return "", nil

	case "正则替换":
		if inputs.LenOk(2, 3) {
			matcheA, err := regexp.Compile(inputs.String(2))
			if err != nil {
				return "", nil
			}
			var tStr string
			if inputs.LenOk(3) {
				tStr = inputs.String(3)
				if tStr == lines[3].(string) && strings.HasPrefix(lines[3].(string), "%") && strings.HasSuffix(lines[3].(string), "%") && strings.Count(lines[3].(string), "%") == 2 {
					var regex *regexp.Regexp
					obj := d.Val.P.GetObj(lines[3].(string)[1 : len(lines[3].(string))-1])
					if t, ok := obj["type"].(string); ok && t == "函数框" {
						funcTrigger := obj["trigger"].(string)
						regex = regexp.MustCompile("^" + funcTrigger + "$")
						res := matcheA.ReplaceAllStringFunc(inputs.String(1), func(s string) string {
							matches := regex.FindStringSubmatch(s)
							if len(matches) > 0 || funcTrigger == "" {
								funcv := dto.NewVal()
								funcv.Reset(d.Val.P.GetAll())
								funcv.Set("触发", funcTrigger)
								funcv.Set("触发词", s)
								content := obj["content"].([]string)
								RunDic := NewRunDicEntry().
									SetGlobal_v(d.Val.G).
									Set_v(funcv).
									SetDic_v(d.Dic)
								return RunDic.Run(content)
							}
							return ""
						})
						return res, nil
					}
				}
			}
			replacedText := matcheA.ReplaceAllString(inputs.String(1), tStr)
			return replacedText, nil

		}
		return "", nil

	case "终端.解码器":
		return f.RunCommandDecoder()

	case "终端.变量":
		return f.RunCommandVar()

	case "终端.断开":
		return f.RunCommandClose()

	case "终端.输入":
		return f.RunCommandInputText()

	case "分割":
		return f.Split()

	case "AES加密":
		return f.AesEn(d.Val.P)

	case "AES解密":
		return f.AesDe(d.Val.P), nil

	case "MD5编码":
		return f.Md5(), nil

	case "B64编码":
		return f.Base64En(), nil

	case "B64解码":
		return f.Base64De(), nil

	case "URL编码":
		return f.UrlEn(), nil

	case "URL解码":
		return f.UrlDe(), nil

	case "URL链接编码":
		return f.UrlPathEn(), nil

	case "URL链接解码":
		return f.UrlPathDe(), nil

	case "判断值":
		return f.IfNONull(), nil

	case "判断空值":
		return f.IfNull(), nil

	case "正则匹配":
		return f.RegexpMatche(), nil

	case "正则":
		return f.Regexp(), nil

	case "加密词库":
		return f.EncodeDic(), nil

	case "大写字母":
		return f.ToUpper(), nil

	case "小写字母":
		return f.ToLower(), nil

	case "ZIP压缩":
		return f.ZipFolder(), nil

	case "ZIP解压":
		return f.UnZip(), nil

	case "文件夹大小":
		return f.DirSize(), nil

	case "文件大小":
		return f.FileSize(), nil

	case "重命名":
		return f.FileRename(), nil

	case "复制粘贴":
		return f.FileCopy(), nil

	case "字符切片":
		return f.StringSlice(), nil

	case "计算":
		return f.Count(), nil

	case "随机数":
		return f.RandNum(), nil

	case "随机文本":
		return f.RandString(), nil

	case "随机大小字母":
		return f.RandLetter(0), nil

	case "随机大写字母":
		return f.RandLetter(1), nil

	case "随机小写字母":
		return f.RandLetter(2), nil

	case "随机大小字母数字":
		return f.RandLetter(3), nil

	case "随机小写字母数字":
		return f.RandLetter(4), nil

	case "随机大写字母数字":
		return f.RandLetter(5), nil

	case "随机数字":
		return f.RandLetter(6), nil

	case "时间戳格式化时间":
		return f.TimestampFormattingTime(), nil

	case "JSON解析":
		return f.QueryJson()

	case "JSON判断":
		return f.IsJson(), nil

	case "JSON存":
		return f.JsonSet(), nil

	case "JSON存字":
		return f.JsonSetString(), nil

	case "JSON追加":
		return f.JsonAdd(), nil

	case "JSON追加字":
		return f.JsonAddString(), nil

	case "JSON删":
		return f.JsonDelete(), nil

	case "JSON存在":
		return f.JsonIsKey(), nil

	case "JSON长度":
		return f.JsonLen(), nil

	case "JSON美化":
		return f.JsonPrettyPrint()

	case "HTML解析":
		return f.HtmlParse()

	case "HTML编码":
		return f.HtmlEncode()

	case "HTML解码":
		return f.HtmlDecode()

	case "访问":
		return f.AccessGet()

	case "编码":
		return f.EnUtf8(), nil

	case "解码":
		return f.DeUtf8(), nil

	case "访问POST":
		return f.AccessPost()

	case "通信记录":
		return f.AccessSet(d.Sys)

	case "通信超时":
		return f.AccessSetTimes(d.Sys)

	case "通信头部":
		return f.AccessSetHeader(d.Sys)

	case "通信GET":
		return f.AccessSetGet(d.Sys)

	case "通信POST":
		return f.AccessSetPost(d.Sys)

	case "通信POST文件":
		return f.AccessSetPostFile(d.Sys)

	case "通信发包":
		return f.AccessSend(d.Sys)

	case "通信取出":
		return f.AccessGetSendAll(d.Sys)

	case "通信取出结果":
		return f.AccessGetSend(d.Sys)

	case "GIF拆帧":
		return f.GetGif(), nil

	case "绘图":
		return f.DrawImg(), nil

	case "排序":
		return f.Sort(), nil

	case "Js":
		return f.RunJs(d.Val.P), nil

	case "范围":
		return f.Range(), nil

	case "Ed25519从种子生成密钥":
		return f.Ed25519_NewKeyFromSeed()

	case "Ed25519签名":
		return f.Ed25519_Sign()

	case "Ed25519验证签名":
		return f.Ed25519_Verify()

	case "Ed25519公钥转换为Curve25519":
		return f.Ed25519_PublicKeyToCurve25519()

	case "Ed25519私钥转换为Curve25519":
		return f.Ed25519_PrivateKeyToCurve25519()

	case "Ed25519从Curve25519生成密钥":
		return f.Ed25519_NewKeyFromCurve25519()

	case "画笔.字体":
		return nil, f.DrawImgLoadFont()

	case "画笔.大小":
		return nil, f.DrawImgSetSize()

	case "绘制.文本":
		return nil, f.DrawImgText()

	case "绘制.点":
		return nil, f.DrawImgPoint()

	case "绘制.线":
		return nil, f.DrawImgLine()

	case "绘制.喷漆":
		return nil, f.DrawImgBrushLine()

	case "绘制.波浪":
		return nil, f.DrawImgWaveLine()

	case "绘制.油漆桶":
		return nil, f.DrawImgFloodFill()

	case "绘制.方形":
		return nil, f.DrawImgRectangleFill()

	case "绘制.方形描边":
		return nil, f.DrawImgRectangleStroke()

	case "绘制.椭圆":
		return nil, f.DrawImgEllipseFill()

	case "绘制.椭圆描边":
		return nil, f.DrawImgEllipse()

	case "绘制.圆形":
		return nil, f.DrawImgPieFill()

	case "绘制.圆形描边":
		return nil, f.DrawImgPie()

	case "绘制.多边形":
		return nil, f.DrawImgPolygon()

	case "绘制.多边形描边":
		return nil, f.DrawImgPolygons()

	case "绘制.图片":
		return nil, f.DrawImgPaste()

	case "画布.旋转":
		return nil, f.DrawImgRotate()

	case "画布.圆形":
		return nil, f.DrawImgRoundCorners()

	case "绘制.随机点":
		return nil, f.DrawImgRandomDots()

	case "绘制.随机线条":
		return nil, f.DrawImgRandomLines()

	case "绘制.马赛克":
		return nil, f.DrawImgMosaic()

	case "绘制.高斯模糊":
		return nil, f.DrawImgGaussianBlur()

	case "画布.马赛克":
		return nil, f.DrawImgAllMosaic()

	case "画布.灰度":
		return nil, f.DrawImgGrayscale()

	case "绘制.圆弧":
		return nil, f.DrawImgArc()

	case "图片相似度":
		return f.ImageSimilarity()

	case "GC回收":
		runtime.GC()
		return "", nil

	case "腾讯.接口":
		return f.TencentGetApi()

	case "腾讯.调用":
		return f.TencentGetApiCall()

	}

	return "$" + strings.Join(linesStr, " ") + "$", nil
}

// t := lines[0]
// if fn, ok := funcAll[t]; ok {
// 	funcAlls.Len = inputsLen
// 	funcAlls.Inputs = inputs
// 	res := fn.(func() string)()
// 	return res, nil
// }

// var funcAlls = &funcs.DicFunc{}
// var funcAll = map[string]interface{}{
// "字符切片":   funcAlls.StringSlice,
// "文本长度":   funcAlls.StringSliceLen,
// "长度":     funcAlls.StringLen,
// "计算":     funcAlls.Count,
// "HTML编码": funcAlls.HtmlEncode,
// "HTML解码": funcAlls.HtmlDecode,
// "JSON判断": funcAlls.IsJson,
// "JSON解析": funcAlls.QueryJson,
// "随机文本":   funcAlls.RandString,
// "随机数":    funcAlls.RandNum,
// }

/*
// ParseString 解析由双引号或单引号包裹的字符串，支持转义符号
func ParseString(input string) (string, bool) {
	// 检查输入是否足够长
	if len(input) < 2 {
		return "", false
	}

	// 检查开头是否为双引号或单引号
	startQuote := input[0]
	if startQuote != '"' && startQuote != '\'' {
		return "", false
	}

	var result strings.Builder
	escaped := false // 标记是否在转义状态

	// 遍历输入字符串，从第2个字符开始
	for i := 1; i < len(input); i++ {
		ch := input[i]

		// 如果前一个字符是转义符号
		if escaped {
			// 处理转义字符
			switch ch {
			case 'n':
				result.WriteByte('\n') // 转义为换行符
			case 't':
				result.WriteByte('\t') // 转义为制表符
			case '\\':
				result.WriteByte('\\') // 转义为反斜杠
			case '"':
				result.WriteByte('"') // 转义为双引号
			case '\'':
				result.WriteByte('\'') // 转义为单引号
			default:
				result.WriteByte(ch) // 其他字符原样添加
			}
			escaped = false
		} else {
			if ch == '\\' {
				escaped = true // 下一个字符将被转义
			} else if ch == startQuote {
				// 找到匹配的结束引号
				return result.String(), nil
			} else {
				result.WriteByte(ch) // 普通字符，直接添加
			}
		}
	}

	// 如果到这里没有返回，说明没有找到匹配的结束引号
	return "", false
}
*/
