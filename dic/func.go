package dic

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/count"
	"github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"
	"github.com/cjxpj/nebula/utils"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/websocket"
	"golang.ngrok.com/ngrok"
	"golang.ngrok.com/ngrok/config"
)

func (d *DicFunc) Runs(text string) string {
	output := run.ReplaceProcessedContents(text, "$", "$", func(valStr string) string {
		res, _ := d.Funcs(valStr)
		return res
	}, func(s string) string {
		return d.V.Text(s)
	})
	return output
}

func (d *DicFunc) Run(text string) string {
	output := run.ReplaceProcessedContent(text, "$", "$", func(valStr string) string {
		res, _ := d.Funcs(valStr)
		return res
	})
	return output
}

func (d *DicFunc) Runss(text string) string {
	output := run.ReplaceProcessedsContent(text, "$", ";", func(valStr string) string {
		res, _ := d.Funcs(valStr)
		return res
	})
	return output
}

func (d *DicFunc) Funcs(text string) (string, bool) {

	// 外部储存路径
	DataPath := "database"
	// 内部路径
	LocalPath := "private"
	// 执行路径
	// RunPath := "public"

	DataPath = d.Path + "/" + DataPath
	LocalPath = d.Path + "/" + LocalPath
	// RunPath = d.Path + "/" + RunPath

	// 面对象
	if len(text) > 1 && (text[0] == '.' || text[0] == '%') {
		lines := strings.Split(text[1:], " ")
		classN := lines[0]
		var isV bool
		if text[0] == '%' {
			isV = true
		}
		if classN == "自己" {
			classN = d.V.Get("Class").(string)
		}
		classData := d.Dic.LocalClass[classN]
		if classData == nil {
			return "非整合包", true
		}

		linesLen := len(lines)

		if isV {
			if linesLen == 3 {
				resVT := d.V.Text(count.RunCountText(d.V, lines[1]))
				resVTs := d.V.Text(count.RunCountText(d.V, lines[2]))
				classData.LocalValue.Set(resVT, resVTs)
				return "", true
			}
			if linesLen == 2 {
				resVT := d.V.Text(count.RunCountText(d.V, lines[1]))
				resV, _ := classData.LocalValue.Get(resVT).(string)
				return resV, true
			}
			return "未知整合包变量方法", true
		}

		if linesLen <= 1 {
			return "未知整合包方法", true
		}

		TStr := strings.Join(lines[1:], " ")
		// 整合包局部函数
		if str, Tstr, _, regex := run.RunFor(classData.LocalFunc, TStr, 0); regex != nil {
			funcv := dto.NewVal()
			funcv.Set("触发", Tstr)
			funcv.Set("触发词", TStr)
			funcv.Set("Class", classN)
			dto.ValRunTrigger(TStr, Tstr, funcv, d.V)
			resRunDic := NewRunDicEntry(d.Path).
				SetText(str).
				CloseTrigger().
				SetGlobal_v(d.GV).
				Set_v(funcv).
				SetDic_v(d.Dic).
				Run()
			return resRunDic, true
		}
	} else {
		// 局部函数
		if str, Tstr, _, regex, tparts := run.RunFors(d.Dic.LocalFunc, text, 0); regex != nil {
			funcv := dto.NewVal()
			give, ok := d.V.Get("_继承_").(string)
			if ok && give != "" {
				for _, v := range strings.Split(give, ",") {
					set, ok := d.V.Get(v).(string)
					if ok {
						funcv.Set(v, set)
					}
				}
				d.V.Set("_继承_", "")
			}
			funcv.Set("触发", Tstr)
			funcv.Set("触发词", text)
			dto.ValRunTrigger(text, Tstr, funcv, d.V)
			RunDic := NewRunDicEntry(d.Path).
				SetText(str).
				CloseTrigger().
				SetGlobal_v(d.GV).
				Set_v(funcv).
				SetDic_v(d.Dic)
			resRunDic := RunDic.Run()
			if tparts != "" {
				subParts := strings.Split(tparts, ",")
				for _, setv := range subParts {
					getv := RunDic.V.Get(setv)
					d.V.Set(setv, getv)
				}
			}
			return resRunDic, true
		}
	}

	var lines []string
	if d.Open {
		lines = run.SplitFuncString(text, "$", ";")
	} else {
		lines = strings.Split(text, " ")
	}
	inputsLen := len(lines)
	inputs := make([]string, inputsLen)
	inputsLen--

	if d.Open {
		for i, line := range lines {
			res := d.V.Text(d.Runss(count.RunCountText(d.V, line)))
			inputs[i] = res
		}
	} else {
		for i, line := range lines {
			res := d.V.Text(count.RunCountText(d.V, line))
			inputs[i] = res
		}
	}

	f := &funcs.DicFunc{
		Len:    inputsLen,
		Inputs: inputs,
		Path:   d.Path,
	}

	switch lines[0] {

	case "读文件":
		if inputsLen == 1 || inputsLen == 2 {
			path := d.Path + "/" + inputs[1]
			file := utils.NewFileQueue(path)
			data, err := file.ReadFile()
			if err != nil {
				if inputsLen == 2 {
					return inputs[2], true
				}
				return "", true
			}
			return data, true
		}
		return "", true

	case "写文件":
		if inputsLen == 2 {
			path := d.Path + "/" + inputs[1]
			file := utils.NewFileQueue(path)
			file.WriteToFile(inputs[2])
		}
		return "", true

	case "读":
		return f.ReadKeyStringFile(DataPath), true

	case "写":
		return f.WriteKeyStringFile(DataPath), true

	case "捕获输出":
		if inputsLen == 0 {
			return d.Output.Get(), true
		}
		return "", true

	case "拦截输出":
		if inputsLen == 0 {
			res := d.Output.Get()
			d.Output.Clear()
			return res, true
		}
		return "", true

	case "STOP":
		if inputsLen == 0 {
			utils.LogStop(d.Output.Get(), d.Path)
			return "", true
		}
		return "", true

	case "打印":
		if inputsLen == 1 {
			fmt.Println(inputs[1])
		}
		return "", true

	case "终止服务器":
		if inputsLen == 1 {
			srv, ok := dto.GV.Get("server_" + inputs[1]).(*http.Server)
			if ok {
				if err := srv.Close(); err != nil {
					return "error", true
				}
				dto.GV.Set("server_"+inputs[1], nil)
				return "true", true
			}
			return "false", true
		}
		return "", true

	case "启动服务器":
		if inputsLen == 1 || inputsLen == 2 {
			URL := inputs[1]
			Gpath := "web"
			if inputsLen == 2 {
				Gpath = "web/" + inputs[2]
			}
			go func() {
				file := utils.NewFile()

				// 路由词库
				file.SetPath(Gpath + "/private/system/router.n")
				if !file.FileExists() {
					file.WriteFileByte(appfiles.DicRouter)
				}
				if routerData, err := file.ReadFile(); err == nil {
					t := &run.Build{
						G_v:  d.GV,
						V:    d.V,
						Path: d.Path,
					}
					build := t.SplitText(routerData)
					dto.GV.Set("cache_路由", build)
				}

				// 主页文件
				file.SetPath(Gpath + "/public")
				if !file.DirExists() {
					file.SetPath(Gpath + "/public/index.wn")
					if !file.FileExists() {
						file.WriteToFile(`
<?n
什么:简单
?>
<h1>
<?n
开发越来越%什么%！
?>
</h1>
`)
					}

					// 默认样板文件
					file.SetPath(Gpath + "/public/api.n")
					if !file.FileExists() {
						file.WriteToFile(`输出:%版本%

Main
%输出%
`)
					}
					// 404文件
					file.SetPath(Gpath + "/public/404.wn")
					if !file.FileExists() {
						file.WriteFileByte(appfiles.Dic404)
					}
				}
				handler := &ServeRouter{path: Gpath}

				if oks, ok := d.V.Get("Cache").(string); ok && oks == "true" {
					handler.Cache = true
				}
				if oks, ok := d.V.Get("WebSocket").(string); ok && oks == "true" {
					handler.Ws = true
					handler.WsConfig = &websocket.Upgrader{}
					wsfile := utils.NewFile()
					wsfile.SetPath(Gpath + "/private/websocket/connect.n")
					if !wsfile.FileExists() {
						wsfile.WriteToFile("\nMain\n连接成功\n")
					}

					wsfile.SetPath(Gpath + "/private/websocket/msg.n")
					if !wsfile.FileExists() {
						wsfile.WriteToFile("\nMain\n消息回复：%消息%\n")
					}

					wsfile.SetPath(Gpath + "/private/websocket/close.n")
					if !wsfile.FileExists() {
						wsfile.WriteToFile("\nMain|Error\n断开连接\n")
					}
				}
				// err := http.ListenAndServe(URL, http.HandlerFunc(handler.WebRun))
				// if err != nil {
				// 	errMsg := fmt.Sprintf("启动失败>%s", err)
				// 	utils.Error(errMsg, d.Path)
				// }

				if _, ok := dto.GV.Get("server_" + URL).(*http.Server); !ok {
					srv := &http.Server{
						Addr:    URL,
						Handler: http.HandlerFunc(handler.WebRun), // 你的处理函数
					}

					dto.GV.Set("server_"+URL, srv)

					// 使用 Goroutine 启动服务器
					go func() {
						if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
							utils.Error("启动失败>"+err.Error(), d.Path)
						}
					}()
				}
			}()
			go func() {
				file_start := utils.NewFileQueue(Gpath + "/private/system/start.n")
				if !file_start.FileExists() {
					file_start.WriteToFile(`
Main
#:$删除文件夹 缓存$
启动成功
`)
				}
				FileData, _ := file_start.ReadFromFile()
				Gval := dto.NewVal()
				Gval.Reset(d.GV.GetAll())
				Dic := NewDic(FileData, "Main", Gpath).
					SetGlobal_v(Gval)
				RunData := Dic.Run()
				utils.Log(RunData, Gpath)
			}()
			return URL, true
		}
		return "", true

	case "Ngrok":
		if inputsLen == 0 || inputsLen == 1 || inputsLen == 2 {
			if ok, NgrokOpen := dto.GV.Get("Ngrok").(bool); ok && NgrokOpen {
				d.V.Set("报错", "已启动了一个隧道")
				return "false", true
			}
			dto.GV.Set("Ngrok", false)
			d.V.Set("报错", "null")

			Gpath := "web"
			if inputsLen == 2 {
				Gpath = "web/" + inputs[2]
			}

			var authToken string
			if inputsLen == 0 {
				authTokenOptions := []string{
					"2abzrmBDIPyXUkuPdxCYmjTJJDa_2LBiWGFFwewXpxFd4KU3n",
					"2fxaUPFlFox97uGLr2WRM5XSMwO_4X6fihRKMNzwwgM6pgAFr",
				}
				authToken = authTokenOptions[rand.Intn(len(authTokenOptions))]
			} else {
				authToken = inputs[1]
			}

			fs := &funcs.DicFunc{
				Len:    2,
				Inputs: []string{"访问POST", "https://cjxpj.cn/ifuser.n", "key=nebula"},
			}
			runOK := fs.AccessPost(d.Path)

			if runOK == "true" {
				if listener, err := ngrok.Listen(context.Background(),
					config.HTTPEndpoint(),
					ngrok.WithAuthtoken(authToken),
				); err == nil {
					handler := &ServeRouter{path: Gpath}
					if oks, ok := d.V.Get("WebSocket").(string); ok && oks == "true" {
						handler.Ws = true
						handler.WsConfig = &websocket.Upgrader{}
						wsfile := utils.NewFile()
						wsfile.SetPath(Gpath + "/private/websocket/connect.n")
						if !wsfile.FileExists() {
							wsfile.WriteToFile("\nMain\n连接成功\n")
						}

						wsfile.SetPath(Gpath + "/private/websocket/msg.n")
						if !wsfile.FileExists() {
							wsfile.WriteToFile("\nMain\n消息回复：%消息%\n")
						}

						wsfile.SetPath(Gpath + "/private/websocket/close.n")
						if !wsfile.FileExists() {
							wsfile.WriteToFile("\nMain|Error\n断开连接\n")
						}
					}
					go func() {
						if err := http.Serve(listener, http.HandlerFunc(handler.WebRun)); err != nil {
							d.V.Set("报错", err.Error())
						}
					}()
					dto.GV.Set("Ngrok", true)
					return listener.URL(), true
				} else {
					return "配置失败", true
				}
			} else {
				return "此服务已停止提供", true
			}
		}
		return "", true

	case "WS返回":
		if inputsLen == 1 {
			if conn_ws, ok := d.GV.Get("WebSocket").(*websocket.Conn); ok {
				conn_ws.WriteMessage(websocket.TextMessage, []byte(inputs[1]))
				return "", true
			}
		}
		return "", true

	case "WS记录":
		if inputsLen == 1 {
			if conn_ws, ok := d.GV.Get("WebSocket").(*websocket.Conn); ok {
				namev := "ws_" + inputs[1]
				dto.GV.Set(namev, conn_ws)
				return "", true
			}
		}
		return "", true

	case "WS发送":
		if inputsLen == 2 {
			if conn_ws, ok := dto.GV.Get("ws_" + inputs[1]).(*websocket.Conn); ok {
				if err := conn_ws.WriteMessage(websocket.TextMessage, []byte(inputs[2])); err != nil {
					return err.Error(), true
				}
				return "true", true
			}
		}
		return "", true

	case "WS连接断开":
		if inputsLen == 1 {
			if conn_ws, ok := dto.GV.Get("ws_" + inputs[1]).(*websocket.Conn); ok {
				conn_ws.Close()
				return "", true
			}
		}
		return "", true

	case "WS连接":
		if inputsLen == 3 {
			namev := "ws_" + inputs[1]
			addr := inputs[2]

			var funcTrigger string
			var regex *regexp.Regexp
			obj := d.V.GetObj(inputs[3])
			if t, ok := obj["type"].(string); !ok {
				if t != "函数框" {
					return "非函数", true
				}
				return "非函数", true
			}

			funcTrigger = obj["trigger"].(string)
			regex = regexp.MustCompile("^" + funcTrigger + "$")

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

			d.V.Set("报错", "null")

			// 创建 WebSocket 连接
			conn, _, err := websocket.DefaultDialer.Dial(scheme+"://"+addr, nil)
			if err != nil {
				d.V.Set("报错", err.Error())
				return "连接错误", true
			}
			defer conn.Close()

			dto.GV.Set(namev, conn)

			messageTypeMap := map[int]string{
				websocket.TextMessage:   "文本消息",
				websocket.BinaryMessage: "二进制消息",
			}

			// 读取来自 WebSocket 服务器的消息
			for {
				messageType, message, err := conn.ReadMessage()
				typeName, ok := messageTypeMap[messageType]
				if !ok {
					typeName = "未知消息类型"
				}

				Tstr := string(message)
				// fmt.Println("收到:", typeName, Tstr)

				matches := regex.FindStringSubmatch(Tstr)
				if len(matches) > 0 || funcTrigger == "" {
					funcv := dto.NewVal()
					funcv.Reset(d.V.GetAll())
					funcv.Set("触发", funcTrigger)
					funcv.Set("触发词", Tstr)
					funcv.Set("消息来源", typeName)
					content := obj["content"].([]string)
					go func() {
						RunDic := NewRunDicEntry(d.Path).
							SetText(content).
							SetGlobal_v(d.GV).
							Set_v(funcv).
							SetDic_v(d.Dic)
						RunDic.Run()
					}()
				}

				if err != nil {
					d.V.Set("报错", err.Error())
					conn.Close()
					return "消息错误", true
				}
			}
		}
		return "", true

	case "函数列表":
		if inputsLen == 0 {
			var keys []string
			for _, m := range d.Dic.LocalFunc {
				keys = append(keys, m.Trigger)
			}
			for N := range d.Dic.LocalClass {
				keys = append(keys, "."+N)
			}
			// 移除重复的键
			keyMap := make(map[string]bool)
			uniqueKeys := []string{}
			for _, k := range keys {
				if _, value := keyMap[k]; !value {
					keyMap[k] = true
					uniqueKeys = append(uniqueKeys, k)
				}
			}
			// 将切片转换为JSON
			resByte, err := json.Marshal(uniqueKeys)
			if err != nil {
				return "[]", true
			}
			return string(resByte), true
		}
		return "", true

	case "系统":
		if inputsLen >= 1 {
			sfile := utils.NewFileQueue(LocalPath + "/system/cache.txt")
			if inputsLen == 1 {
				str, err := sfile.ReadFileKey(inputs[1])
				if err != nil {
					return "", true
				}
				return str, true
			}
			if inputsLen == 2 {
				sfile.WriteFileKey(inputs[1], inputs[2])
			}
		}
		return "", true

	case "文件后缀":
		if inputsLen == 1 {
			res := filepath.Ext(inputs[1])
			return res, true
		}
		return "", true

	case "异步函数", "函数":
		if inputsLen == 2 || inputsLen == 1 {
			Tstr := ""
			if inputsLen == 2 {
				Tstr = inputs[2]
			}
			obj := d.V.GetObj(inputs[1])
			if t, ok := obj["type"].(string); ok {
				if t == "函数框" {
					funcTrigger := obj["trigger"].(string)
					regex := regexp.MustCompile("^" + funcTrigger + "$")
					matches := regex.FindStringSubmatch(Tstr)
					if len(matches) > 0 || funcTrigger == "" {
						funcv := dto.NewVal()
						funcv.Reset(d.V.GetAll())
						funcv.Set("触发", funcTrigger)
						funcv.Set("触发词", Tstr)
						content := obj["content"].([]string)
						if lines[0] == "异步函数" {
							go func() {
								resDic := NewRunDicEntry(d.Path).
									SetText(content).
									SetGlobal_v(d.GV).
									Set_v(funcv).
									SetDic_v(d.Dic).
									Run()
								if resDic != "" {
									utils.Log(resDic, d.Path)
								}
							}()
						} else {
							resDics := NewRunDicEntry(d.Path).
								SetText(content).
								SetGlobal_v(d.GV).
								Set_v(funcv).
								SetDic_v(d.Dic)
							resDic := resDics.Run()
							return resDic, true
						}
					}
				}
			}
		}
		return "", true

	case "回调":
		if inputsLen >= 1 {
			trigger := strings.Join(inputs[1:], " ")
			GetDic, GetDicTrigger, _, _ := run.RunFor(d.Dic.LocalStatic, trigger, 0)
			funcV := dto.NewVal()
			funcV.Reset(d.V.GetAll())
			funcV.Set("触发词", trigger)
			funcV.Set("触发", GetDicTrigger)
			RunDics := NewRunDicEntry(d.Path).
				SetText(GetDic).
				SetGlobal_v(d.GV).
				Set_v(funcV).
				SetDic_v(d.Dic)
			RunDic := RunDics.Run()
			return RunDic, true
		}
		return "", true

	case "下载文件":
		if inputsLen == 2 {
			path := inputs[2]
			path = d.Path + "/" + path
			file := utils.NewFileQueue(path)
			if file.Download(inputs[1]) {
				return "true", true
			}
			return "false", true
		}
		return "", true

	case "删除文件":
		if inputsLen == 1 {
			path := inputs[1]
			path = d.Path + "/" + path
			file := utils.NewFileQueue(path)
			file.DeleteFile()
			return "", true
		}
		return "", true

	case "删除文件夹":
		if inputsLen == 1 {
			path := inputs[1]
			path = d.Path + "/" + path
			file := utils.NewFileQueue(path)
			file.DeleteFolder()
			return "", true
		}
		return "", true

	case "替换":
		if inputsLen == 4 {
			tStr := inputs[3]
			num, err := strconv.Atoi(inputs[4])
			if err != nil {
				return "非数字", true
			}
			res := strings.Replace(inputs[1], inputs[2], tStr, num)
			return res, true
		}
		if inputsLen == 2 || inputsLen == 3 {
			var tStr string
			if inputsLen == 3 {
				tStr = inputs[3]
				if tStr == lines[3] && strings.HasPrefix(lines[3], "%") && strings.HasSuffix(lines[3], "%") && strings.Count(lines[3], "%") == 2 {
					var regex *regexp.Regexp
					obj := d.V.GetObj(lines[3][1 : len(lines[3])-1])
					if t, ok := obj["type"].(string); ok && t == "函数框" {
						funcTrigger := obj["trigger"].(string)
						regex = regexp.MustCompile("^" + funcTrigger + "$")
						num := 0
						res := run.ReplaceFunc(inputs[1], inputs[2], func(s string) string {
							num++
							strNum := strconv.Itoa(num)
							matches := regex.FindStringSubmatch(strNum)
							if len(matches) > 0 || funcTrigger == "" {
								funcv := dto.NewVal()
								funcv.Reset(d.V.GetAll())
								funcv.Set("触发", funcTrigger)
								funcv.Set("触发词", strNum)
								content := obj["content"].([]string)
								RunDic := NewRunDicEntry(d.Path).
									SetText(content).
									SetGlobal_v(d.GV).
									Set_v(funcv).
									SetDic_v(d.Dic)
								return RunDic.Run()
							}
							return ""
						})
						return res, true
					}
				}
			}
			res := strings.ReplaceAll(inputs[1], inputs[2], tStr)
			return res, true
		}

		return "", true

	case "正则替换":
		if inputsLen == 2 || inputsLen == 3 {
			matcheA, err := regexp.Compile(inputs[2])
			if err != nil {
				return "", true
			}
			var tStr string
			if inputsLen == 3 {
				tStr = inputs[3]
				if tStr == lines[3] && strings.HasPrefix(lines[3], "%") && strings.HasSuffix(lines[3], "%") && strings.Count(lines[3], "%") == 2 {
					var regex *regexp.Regexp
					obj := d.V.GetObj(lines[3][1 : len(lines[3])-1])
					if t, ok := obj["type"].(string); ok && t == "函数框" {
						funcTrigger := obj["trigger"].(string)
						regex = regexp.MustCompile("^" + funcTrigger + "$")
						res := matcheA.ReplaceAllStringFunc(inputs[1], func(s string) string {
							matches := regex.FindStringSubmatch(s)
							if len(matches) > 0 || funcTrigger == "" {
								funcv := dto.NewVal()
								funcv.Reset(d.V.GetAll())
								funcv.Set("触发", funcTrigger)
								funcv.Set("触发词", s)
								content := obj["content"].([]string)
								RunDic := NewRunDicEntry(d.Path).
									SetText(content).
									SetGlobal_v(d.GV).
									Set_v(funcv).
									SetDic_v(d.Dic)
								return RunDic.Run()
							}
							return ""
						})
						return res, true
					}
				}
			}
			replacedText := matcheA.ReplaceAllString(inputs[1], tStr)
			return replacedText, true

		}
		return "", true

	case "存在文件":
		if inputsLen == 1 {
			path := d.Path + "/" + inputs[1]
			file := utils.NewFileQueue(path)
			res := strconv.FormatBool(file.FileExists())
			return res, true
		}
		return "", true

	case "存在文件夹":
		if inputsLen == 1 {
			path := d.Path + "/" + inputs[1]
			file := utils.NewFileQueue(path)
			res := strconv.FormatBool(file.DirExists())
			return res, true
		}
		return "", true

	case "执行网页词库":
		if inputsLen == 1 {
			data := inputs[1]
			DicRes := NewWebDic(data, d.Path).
				SetGlobal_v(d.GV).
				Run()
			return DicRes, true
		}
		return "", true

	case "执行词库":
		if inputsLen == 1 || inputsLen == 2 || inputsLen == 3 {
			data := inputs[1]

			// 触发
			chufa := "Main"
			if inputsLen >= 2 {
				chufa = inputs[2]
			}

			dicType := "独立"
			if inputsLen == 3 {
				dicType = inputs[3]
			}

			calldicrun := NewDic(data, chufa, d.Path).
				SetGlobal_v(d.GV)
			calldicrun.ClassText = d.Dic.LocalClass

			switch dicType {
			case "继承":
				fv := dto.NewVal()
				fv.Reset(d.V.GetAll())
				calldicrun.Set_v(fv)
				calldicrun.FuncText = d.Dic.LocalFunc
			case "继承函数":
				calldicrun.FuncText = d.Dic.LocalFunc
			case "互通":
				calldicrun.Set_v(d.V)
				calldicrun.FuncText = d.Dic.LocalFunc
			}

			DicRes := calldicrun.Run()
			return DicRes, true
		}
		return "", true

	case "加载动态库":
		return f.Pack_Load(), true

	case "回调动态库":
		return f.Pack_Run(), true

	case "邮件":
		return f.Email(), true

	case "MD转HTML":
		return f.MarkdownHTML(), true

	case "终端":
		return f.RunCommand(d.V), true

	case "终端等待输入":
		return f.RunCommandInput(), true

	case "分割":
		return f.Split(), true

	case "字符拼接":
		return f.Join(), true

	case "AES加密":
		return f.AesEn(d.V), true

	case "AES解密":
		return f.AesDe(d.V), true

	case "去除左右":
		return f.RemoveLR(), true

	case "去除左":
		return f.RemoveL(), true

	case "去除右":
		return f.RemoveR(), true

	case "MD5编码":
		return f.Md5(), true

	case "B64编码":
		return f.Base64En(), true

	case "B64解码":
		return f.Base64De(), true

	case "URL编码":
		return f.UrlEn(), true

	case "URL解码":
		return f.UrlDe(), true

	case "URL链接编码":
		return f.UrlPathEn(), true

	case "URL链接解码":
		return f.UrlPathDe(), true

	case "查找字":
		return f.Find(), true

	case "判断值":
		return f.IfNONull(), true

	case "判断空值":
		return f.IfNull(), true

	case "取中间":
		return f.TakeTheMiddle(), true

	case "截取":
		return f.Intercept(), true

	case "延迟":
		return f.AppSleep(), true

	case "锁变量":
		return f.Local_lockValue(d.V), true

	case "变量文本":
		return f.Local_valueText(d.V), true

	case "线程变量":
		return f.App_value(), true

	case "变量":
		return f.Local_value(d.V), true

	case "全局变量":
		return f.Global_value(d.GV), true

	case "正则匹配":
		return f.RegexpMatche(), true

	case "正则":
		return f.Regexp(), true

	case "加密词库":
		return f.EncodeDic(d.Path), true

	case "大写字母":
		return f.ToUpper(), true

	case "小写字母":
		return f.ToLower(), true

	case "ZIP":
		return f.UnZip(d.Path), true

	case "文件列表":
		return f.FileList(d.Path), true

	case "文件夹大小":
		return f.DirSize(d.Path), true

	case "文件大小":
		return f.FileSize(d.Path), true

	case "重命名":
		return f.FileRename(), true

	case "复制粘贴":
		return f.FileCopy(), true

	case "日志":
		return f.Log(d.Path), true

	case "字符切片":
		return f.StringSlice(), true

	case "文本长度":
		return f.StringSliceLen(), true

	case "长度":
		return f.StringLen(), true

	case "计算":
		return f.Count(), true

	case "中文转拼音":
		return f.PinYin(), true

	case "数字格式化":
		return f.NumberFormatting(), true

	case "主机":
		return f.Host_information(), true

	case "MYSQL连接":
		return f.MysqlConn(), true

	case "MYSQL查询":
		return f.MysqlQuery(), true

	case "MYSQL执行":
		return f.MysqlExec(), true

	case "MYSQL断开":
		return f.MysqlClose(), true

	case "随机数":
		return f.RandNum(), true

	case "随机文本":
		return f.RandString(), true

	case "时间戳格式化时间":
		return f.TimestampFormattingTime(), true

	case "数字转中文":
		return f.NumToString(), true

	case "JSON解析":
		return f.QueryJson(), true

	case "JSON判断":
		return f.IsJson(), true

	case "JSON记录":
		return f.SetJson(d.Sys), true

	case "JSON存":
		return f.JsonSet(d.Sys), true

	case "JSON存字":
		return f.JsonSetString(d.Sys), true

	case "JSON追加":
		return f.JsonAdd(d.Sys), true

	case "JSON删":
		return f.JsonDelete(d.Sys), true

	case "JSON取":
		return f.JsonGet(d.Sys), true

	case "JSON存在":
		return f.JsonIsKey(d.Sys), true

	case "JSON长度":
		return f.JsonLen(d.Sys), true

	case "JSON取出":
		return f.JsonGetAll(d.Sys), true

	case "JSON美化":
		return f.JsonPrettyPrint(), true

	case "HTML解析":
		return f.HtmlParse(), true

	case "HTML编码":
		return f.HtmlEncode(), true

	case "HTML解码":
		return f.HtmlDecode(), true

	case "访问":
		return f.AccessGet(d.Path), true

	case "编码":
		return f.EnUtf8(), true

	case "解码":
		return f.DeUtf8(), true

	case "访问POST":
		return f.AccessPost(d.Path), true

	case "通信记录":
		return f.AccessSet(d.Sys), true

	case "通信头部":
		return f.AccessSetHeader(d.Sys), true

	case "通信GET":
		return f.AccessSetGet(d.Sys), true

	case "通信POST":
		return f.AccessSetPost(d.Sys), true

	case "通信POST文件":
		return f.AccessSetPostFile(d.Sys), true

	case "通信发包":
		return f.AccessSend(d.Path, d.Sys), true

	case "通信取出":
		return f.AccessGetSendAll(d.Sys), true

	case "通信取出结果":
		return f.AccessGetSend(d.Sys), true

	case "GIF拆帧":
		return f.GetGif(), true

	case "绘图":
		return f.DrawImg(LocalPath), true

	case "排序":
		return f.Sort(), true
	}

	return "$" + text + "$", false
}

// t := lines[0]
// if fn, ok := funcAll[t]; ok {
// 	funcAlls.Len = inputsLen
// 	funcAlls.Inputs = inputs
// 	res := fn.(func() string)()
// 	return res, true
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
				return result.String(), true
			} else {
				result.WriteByte(ch) // 普通字符，直接添加
			}
		}
	}

	// 如果到这里没有返回，说明没有找到匹配的结束引号
	return "", false
}
*/
