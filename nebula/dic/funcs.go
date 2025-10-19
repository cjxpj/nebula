package dic

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"
	"github.com/cjxpj/nebula/utils"
	"github.com/gorilla/websocket"
)

// 执行词库
func runDic(d *dto.DicInputs) (any, error) {
	data := d.Inputs.String(1)
	if data == "" {
		return "", nil
	}
	dicPath := "执行"

	// 触发
	chufa := d.Inputs.StringDefault(2, "Main")

	// 执行模式
	dicType := d.Inputs.StringDefault(3, "独立")

	calldicrun := NewDic(dicPath, data).
		SetGlobal_v(d.V.G)
	calldicrun.MyFunc = d.Dic.MyFunc
	calldicrun.SetFunc("调用", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("2..") {
			return "", errors.New("调用参数错误")
		}
		go func() {
			sleepTime := inputs.Int(1)
			time.Sleep(time.Duration(sleepTime) * time.Millisecond)
			rMsg := calldicrun.RunPrivate(inputs.StringAfter(2))
			if rMsg != "" {
				fmt.Println(rMsg)
			}
		}()
		return "", nil
	})
	calldicrun.ClassText = d.Dic.LocalClass
	calldicrun.Val.P.Set("_词库路径_", dicPath)

	switch dicType {
	case "继承":
		fv := dto.NewVal()
		fv.Reset(d.V.P.GetAll())
		fv.Set("_词库路径_", dicPath)
		calldicrun.Set_v(fv)
		calldicrun.FuncText = d.Dic.LocalFunc
	case "继承函数":
		calldicrun.FuncText = d.Dic.LocalFunc
	case "互通":
		d.V.P.Set("_词库路径_", dicPath)
		calldicrun.Set_v(d.V.P)
		calldicrun.FuncText = d.Dic.LocalFunc
	}

	DicRes := calldicrun.Run(chufa)
	return DicRes, nil
}

// 执行词库文件
func runDicFile(d *dto.DicInputs) (any, error) {
	dicPath := d.Inputs.String(1)
	data, err := utils.NewFileQueue(dicPath).ReadFromFile()
	if err != nil {
		return "", nil
	}

	// 触发
	chufa := d.Inputs.StringDefault(2, "Main")

	// 执行模式
	dicType := d.Inputs.StringDefault(3, "独立")

	calldicrun := NewDic(dicPath, data).
		SetGlobal_v(d.V.G)
	calldicrun.MyFunc = d.Dic.MyFunc
	calldicrun.SetFunc("调用", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("2..") {
			return "", errors.New("调用参数错误")
		}
		go func() {
			sleepTime := inputs.Int(1)
			time.Sleep(time.Duration(sleepTime) * time.Millisecond)
			rMsg := calldicrun.RunPrivate(inputs.StringAfter(2))
			if rMsg != "" {
				fmt.Println(rMsg)
			}
		}()
		return "", nil
	})
	calldicrun.ClassText = d.Dic.LocalClass
	calldicrun.Val.P.Set("_词库路径_", dicPath)

	switch dicType {
	case "继承":
		fv := dto.NewVal()
		fv.Reset(d.V.P.GetAll())
		fv.Set("_词库路径_", dicPath)
		calldicrun.Set_v(fv)
		calldicrun.FuncText = d.Dic.LocalFunc
	case "继承函数":
		calldicrun.FuncText = d.Dic.LocalFunc
	case "互通":
		d.V.P.Set("_词库路径_", dicPath)
		calldicrun.Set_v(d.V.P)
		calldicrun.FuncText = d.Dic.LocalFunc
	}

	DicRes := calldicrun.Run(chufa)
	return DicRes, nil
}

// 回调词库
func callDic(d *dto.DicInputs) (any, error) {
	var triggerParts []string
	for _, part := range d.Inputs.List[1:] {
		if strPart, ok := part.(string); ok {
			triggerParts = append(triggerParts, strPart)
		}
	}
	trigger := strings.Join(triggerParts, " ")

	// 判断是否在整合包中执行
	if classN, ok := d.V.P.Get("Class").(string); ok {
		classData := d.Dic.LocalClass[classN]
		if classData != nil {
			GetDic, GetDicTrigger, _, _ := run.RunFor(classData.LocalStatic, trigger, 0)
			funcV := dto.NewVal()
			funcV.Reset(d.V.P.GetAll())
			funcV.Set("触发词", trigger)
			funcV.Set("触发", GetDicTrigger)
			RunDics := NewRunDicEntry().
				SetGlobal_v(d.V.G).
				Set_v(funcV).
				SetDic_v(d.Dic)
			RunDic := RunDics.Run(GetDic)
			return RunDic, nil
		}
	}
	GetDic, GetDicTrigger, _, _ := run.RunFor(d.Dic.LocalStatic, trigger, 0)
	funcV := dto.NewVal()
	funcV.Reset(d.V.P.GetAll())
	funcV.Set("触发词", trigger)
	funcV.Set("触发", GetDicTrigger)
	RunDics := NewRunDicEntry().
		SetGlobal_v(d.V.G).
		Set_v(funcV).
		SetDic_v(d.Dic)
	RunDic := RunDics.Run(GetDic)
	return RunDic, nil
}

// 执行网页词库
func runWebDic(d *dto.DicInputs) (any, error) {
	data := d.Inputs.String(1)
	if data == "" {
		return "", nil
	}
	dicPath := "执行"
	webdic := NewWebDic(dicPath, data).
		SetGlobal_v(d.V.G)
	webdic.MyFunc = d.Dic.MyFunc
	webdicRes := webdic.Run()
	return webdicRes, nil
}

// 执行网页词库文件
func runWebDicFile(d *dto.DicInputs) (any, error) {
	dicPath := d.Inputs.String(1)
	data, err := utils.NewFileQueue(dicPath).ReadFromFile()
	if err != nil {
		return "", nil
	}
	webdic := NewWebDic(dicPath, data).
		SetGlobal_v(d.V.G)
	webdic.MyFunc = d.Dic.MyFunc
	webdicRes := webdic.Run()
	return webdicRes, nil
}

// 终端.监听执行
func cmdListenRun(d *dto.DicInputs) (any, error) {
	cmd, ok := d.Inputs.Get(1).(*funcs.CmdConfig)
	if !ok {
		return "", errors.New("参数1终端数据错误")
	}

	// 注册动作：输出文本
	stdout, _ := cmd.Cmd.StdoutPipe()
	// 注册动作：错误文本
	stderr, _ := cmd.Cmd.StderrPipe()

	if err := cmd.Cmd.Start(); err != nil {
		log.Fatal(err)
	}

	// 注册动作：断开连接
	dicpath := d.Inputs.String(2)
	cmdfileTool := utils.NewFileQueue(dicpath)
	if !cmdfileTool.FileExists() {
		return "", errors.New("请创建词库监听文件")
	}

	// 监听输出
	go func() {
		multi := io.MultiReader(stdout, stderr)
		scanner := bufio.NewScanner(multi)
		for scanner.Scan() {
			raw := scanner.Bytes()
			line, _ := utils.DecodeType(cmd.Decoder, raw)

			// 重新读取
			cmdfile, _ := cmdfileTool.ReadFromFile()
			dd := NewDic(dicpath, cmdfile)
			dd.SetFunc("断开连接", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
				cmd.Cmd.Process.Kill()
				return "", nil
			})
			dd.SetFunc("输入文本", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
				if !inputs.LenOk(1) {
					return "参数错误", nil
				}
				text := inputs.String(1)
				_, err := cmd.Stdin.Write([]byte(text))
				return "", err
			})

			if res := dd.Run(line); res != "" {
				fmt.Println(res)
			}
		}
		if err := scanner.Err(); err != nil {
			fmt.Println("终端断开:", err)
		}
	}()
	return "", nil
}

// 执行函数
func runFunc(d *dto.DicInputs) (any, error) {
	Tstr := d.Inputs.String(2)
	obj := d.V.P.GetObj(d.Inputs.String(1))
	if t, ok := obj["type"].(string); ok && t == "函数框" {
		funcTrigger := obj["trigger"].(string)
		regex := regexp.MustCompile("^" + funcTrigger + "$")
		matches := regex.FindStringSubmatch(Tstr)
		if len(matches) > 0 || funcTrigger == "" {
			funcv := dto.NewVal()
			funcv.Reset(d.V.P.GetAll())
			funcv.Set("触发", funcTrigger)
			funcv.Set("触发词", Tstr)
			content := obj["content"].([]string)
			resDics := NewRunDicEntry().
				SetGlobal_v(d.V.G).
				Set_v(funcv).
				SetDic_v(d.Dic)
			resDic := resDics.Run(content)
			return resDic, nil
		}
	}
	return "", errors.New("未知函数")
}

// 异步函数
func runAsyncFunc(d *dto.DicInputs) (any, error) {
	Tstr := d.Inputs.String(2)
	obj := d.V.P.GetObj(d.Inputs.String(1))
	if t, ok := obj["type"].(string); ok && t == "函数框" {
		funcTrigger := obj["trigger"].(string)
		regex := regexp.MustCompile("^" + funcTrigger + "$")
		matches := regex.FindStringSubmatch(Tstr)
		if len(matches) > 0 || funcTrigger == "" {
			funcv := dto.NewVal()
			funcv.Reset(d.V.P.GetAll())
			funcv.Set("触发", funcTrigger)
			funcv.Set("触发词", Tstr)
			content := obj["content"].([]string)
			resDics := NewRunDicEntry().
				SetGlobal_v(d.V.G).
				Set_v(funcv).
				SetDic_v(d.Dic)
			go func() {
				resDic := resDics.Run(content)
				if resDic != "" {
					fmt.Println(resDic)
				}
			}()
			return "", nil
		}
	}
	return "", errors.New("未知函数")
}

// WS连接
func wsConnect(d *dto.DicInputs) (any, error) {
	addr := d.Inputs.String(1)

	dicpath := "private/websocket/app.n"
	if d.Inputs.LenOk(2) {
		dicpath = d.Inputs.String(2)
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

// WS断开
func wsClose(d *dto.DicInputs) (any, error) {
	if conn_ws, ok := d.Inputs.Get(1).(*websocket.Conn); ok {
		if err := conn_ws.Close(); err != nil {
			return "", nil
		}
	}
	return "", nil
}

// WS发送
func wsSend(d *dto.DicInputs) (any, error) {
	if conn_ws, ok := d.Inputs.Get(1).(*websocket.Conn); ok {
		if err := conn_ws.WriteMessage(websocket.TextMessage, []byte(d.Inputs.String(2))); err != nil {
			return "", err
		}
	}
	return "", nil
}

// =================== 读词库 ===================
func readDicFile(d *dto.DicInputs) (any, error) {
	filePath := utils.NewFileQueue(d.Inputs.String(1)).FileName
	trigger := d.Inputs.StringDefault(2, "Main")
	useRegex := d.Inputs.String(3) == "true"

	lines, err := readFileLines(filePath)
	if err != nil {
		return nil, err
	}

	result := extractSectionStrict(lines, trigger, useRegex)

	jsonStr, err := toJSONString(result)
	if err != nil {
		return nil, err
	}

	return jsonStr, nil
}

// =================== 写词库 ===================
func writeDicFile(d *dto.DicInputs) (any, error) {
	dicPath := d.Inputs.String(1)
	trigger := d.Inputs.StringDefault(2, "Main")
	content := d.Inputs.StringDefault(3, "")
	useRegex := d.Inputs.String(4) == "true"

	filePath := utils.NewFileQueue(dicPath).FileName
	lines, err := readFileLines(filePath)
	if err != nil {
		return nil, err
	}

	var newLines []string
	var found bool
	var re *regexp.Regexp
	if useRegex {
		re, err = regexp.Compile(trigger)
		if err != nil {
			return nil, err
		}
	}

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// 段落结束空行直接写入
		if line == "" {
			newLines = append(newLines, line)
			continue
		}

		// 每个段落的第一行是触发词
		isTrigger := (useRegex && re.MatchString(line)) || (!useRegex && line == trigger)
		if isTrigger {
			found = true
			newLines = append(newLines, line) // 写触发词行
			// 写入新内容
			if content != "" {
				newLines = append(newLines, splitLines(content)...)
			}
			// 跳过原内容块
			i++ // 下一个就是内容行
			for i < len(lines) && lines[i] != "" {
				i++
			}
			if i < len(lines) {
				newLines = append(newLines, "") // 保留段落结束空行
			}
			continue
		}

		// 非目标触发词段落，原样保留整段
		newLines = append(newLines, line)
		// 写完当前段落剩余内容
		i++
		for i < len(lines) && lines[i] != "" {
			newLines = append(newLines, lines[i])
			i++
		}
		if i < len(lines) {
			newLines = append(newLines, "") // 保留段落结束空行
		}
	}

	// 如果没有找到触发词，则追加
	if !found {
		if len(newLines) > 0 && newLines[len(newLines)-1] != "" {
			newLines = append(newLines, "")
		}
		newLines = append(newLines, trigger)
		if content != "" {
			newLines = append(newLines, splitLines(content)...)
		}
		newLines = append(newLines, "")
	}

	// 写回文件
	if err := os.WriteFile(filePath, []byte(joinLines(newLines)), 0644); err != nil {
		return nil, err
	}
	return "ok", nil
}

// =================== 辅助函数 ===================
func splitLines(s string) []string {
	var res []string
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		res = append(res, scanner.Text())
	}
	return res
}

func joinLines(lines []string) string {
	return strings.Join(lines, "\n")
}

func readFileLines(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

// 严格按段落匹配触发词提取内容块
func extractSectionStrict(lines []string, trigger string, useRegex bool) []string {
	var result []string
	var re *regexp.Regexp
	if useRegex {
		re = regexp.MustCompile(trigger)
	}

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if line == "" {
			continue
		}

		isTrigger := (useRegex && re.MatchString(line)) || (!useRegex && line == trigger)
		if !isTrigger {
			// 跳过整段非匹配段落
			i++
			for i < len(lines) && lines[i] != "" {
				i++
			}
			continue
		}

		// 找到匹配段落，读取内容块
		i++ // 下一个是内容行
		for i < len(lines) && lines[i] != "" {
			result = append(result, lines[i])
			i++
		}
		break // 只取第一个匹配段落
	}

	return result
}

// 转 JSON
func toJSONString(data []string) (string, error) {
	if len(data) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
