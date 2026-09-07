package dic

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/bot/feishubot"
	"github.com/cjxpj/nebula/bot/napcatbot"
	"github.com/cjxpj/nebula/bot/qqbot"
	"github.com/cjxpj/nebula/bot/yunhubot"
	"github.com/cjxpj/nebula/debugLog"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"
	dic_server "github.com/cjxpj/nebula/server"
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

	calldicrun := dic_dto.NewDic(dicPath, data).
		SetGlobal_v(d.V.G)
	maps.Copy(calldicrun.MyFunc, d.Dic.MyFunc)
	calldicrun.SetFunc("调用", dto.DicFunc{
		L: "2..",
		Fn: func(d *dto.DicInputs) (any, error) {
			go func() {
				sleepTime := d.Inputs.Int(1)
				time.Sleep(time.Duration(sleepTime) * time.Millisecond)
				rMsg := dic_api.Api.DicRunPrivate(calldicrun, d.Inputs.StringAfter(2))
				if rMsg != "" {
					debugLog.Infof("%v", rMsg)
				}
			}()
			return "", nil
		}})
	calldicrun.ClassText = d.Dic.Class
	dto.SetThreadVarRaw("_词库路径_", dicPath)

	switch dicType {
	case "继承":
		fv := dto.NewVal()
		fv.Reset(d.V.P.GetAll())
		dto.SetThreadVarRaw("_词库路径_", dicPath)
		calldicrun.Set_v(fv)
		calldicrun.FuncText = d.Dic.DicFuncs
	case "继承函数":
		calldicrun.FuncText = d.Dic.DicFuncs
	case "互通":
		dto.SetThreadVarRaw("_词库路径_", dicPath)
		calldicrun.Set_v(d.V.P)
		calldicrun.FuncText = d.Dic.DicFuncs
	}

	DicRes := dic_api.Api.DicRun(calldicrun, chufa)
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

	calldicrun := dic_dto.NewDic(dicPath, data).
		SetGlobal_v(d.V.G)
	maps.Copy(calldicrun.MyFunc, d.Dic.MyFunc)

	calldicrun.SetFunc("调用", dto.DicFunc{
		L: "2..",
		Fn: func(d *dto.DicInputs) (any, error) {
			go func() {
				sleepTime := d.Inputs.Int(1)
				time.Sleep(time.Duration(sleepTime) * time.Millisecond)
				rMsg := dic_api.Api.DicRunPrivate(calldicrun, d.Inputs.StringAfter(2))
				if rMsg != "" {
					debugLog.Infof("%v", rMsg)
				}
			}()
			return "", nil
		}})
	calldicrun.ClassText = d.Dic.Class
	dto.SetThreadVarRaw("_词库路径_", dicPath)

	switch dicType {
	case "继承":
		fv := dto.NewVal()
		fv.Reset(d.V.P.GetAll())
		dto.SetThreadVarRaw("_词库路径_", dicPath)
		calldicrun.Set_v(fv)
		calldicrun.FuncText = d.Dic.DicFuncs
	case "继承函数":
		calldicrun.FuncText = d.Dic.DicFuncs
	case "互通":
		dto.SetThreadVarRaw("_词库路径_", dicPath)
		calldicrun.Set_v(d.V.P)
		calldicrun.FuncText = d.Dic.DicFuncs
	}

	DicRes := dic_api.Api.DicRun(calldicrun, chufa)
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

	// 判断是否在 Class 中执行
	if classData := d.Dic.ResolveClassData(d.V.P.Get("Class")); classData != nil {
		GetDic, GetDicTrigger, _, _ := run.RunFor(classData.DicFuncs["内部"], trigger, 0)
		funcV := dto.NewVal()
		funcV.Reset(d.V.P.GetAll())
		funcV.Set("触发词", trigger)
		funcV.Set("触发", GetDicTrigger)
		RunDics := dic_dto.NewRunDicEntry().
			SetGlobal_v(d.V.G).
			Set_v(funcV).
			SetDic_v(d.Dic)
		RunDic := dic_api.Api.DicRunLine(RunDics, GetDic)
		return RunDic, nil
	}
	GetDic, GetDicTrigger, _, _ := run.RunFor(d.Dic.DicFuncs["内部"], trigger, 0)
	funcV := dto.NewVal()
	funcV.Reset(d.V.P.GetAll())
	funcV.Set("触发词", trigger)
	funcV.Set("触发", GetDicTrigger)
	RunDics := dic_dto.NewRunDicEntry().
		SetGlobal_v(d.V.G).
		Set_v(funcV).
		SetDic_v(d.Dic)
	RunDic := dic_api.Api.DicRunLine(RunDics, GetDic)
	return RunDic, nil
}

// 重定向触发词：把当前触发词重定向为指定文本，随后按该文本重新匹配并执行对应正文。
// 仅在词库头部执行阶段生效。
func redirectTrigger(d *dto.DicInputs) (any, error) {
	if !d.Dic.InHeader {
		return "", errors.New("重定向触发词：仅允许在词库头部使用")
	}
	text := d.Inputs.String(1) // 重定向后的触发词
	if text == "" {
		return "", errors.New("重定向触发词：触发词不能为空")
	}
	// 更新触发词，并立即刷新 %参数% / %括号% 等触发变量
	d.V.P.Set("触发词", text)
	dto.RunTrigger(text, text, d.V.P)
	return "", nil
}

// 执行网页词库
func runWebPHPDic(d *dto.DicInputs) (any, error) {
	data := d.Inputs.String(1)
	if data == "" {
		return "", nil
	}
	dicPath := "执行"
	webdic := dic_dto.NewWebDic(dicPath, data).
		SetGlobal_v(d.V.G)
	webdic.MyFunc = d.Dic.MyFunc
	webdicRes := dic_api.Api.WebPHPDicRun(webdic)
	return webdicRes, nil
}

// 执行网页词库文件
func runWebPHPDicFile(d *dto.DicInputs) (any, error) {
	dicPath := d.Inputs.String(1)
	data, err := utils.NewFileQueue(dicPath).ReadFromFile()
	if err != nil {
		return "", nil
	}
	webdic := dic_dto.NewWebDic(dicPath, data).
		SetGlobal_v(d.V.G)
	webdic.MyFunc = d.Dic.MyFunc
	webdicRes := dic_api.Api.WebPHPDicRun(webdic)
	return webdicRes, nil
}

// 执行网页词库
func runWebDic(d *dto.DicInputs) (any, error) {
	data := d.Inputs.String(1)
	if data == "" {
		return "", nil
	}
	dicPath := "执行"
	webdic := dic_dto.NewWebDic(dicPath, data).
		SetGlobal_v(d.V.G)
	webdic.MyFunc = d.Dic.MyFunc
	webdicRes := dic_api.Api.WebDicRun(webdic)
	return webdicRes, nil
}

// 执行网页词库文件
func runWebDicFile(d *dto.DicInputs) (any, error) {
	dicPath := d.Inputs.String(1)
	data, err := utils.NewFileQueue(dicPath).ReadFromFile()
	if err != nil {
		return "", nil
	}
	webdic := dic_dto.NewWebDic(dicPath, data).
		SetGlobal_v(d.V.G)
	webdic.MyFunc = d.Dic.MyFunc
	webdicRes := dic_api.Api.WebDicRun(webdic)
	return webdicRes, nil
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
		dic := dic_dto.NewDic(dicpath, wsFileData).
			SetFunc("断开连接", dto.DicFunc{
				L: "0",
				Fn: func(d *dto.DicInputs) (any, error) {
					conn.Close()
					return "", nil
				}})
		resData := dic_api.Api.DicRunPrivate(dic, "连接成功")
		if resData != "" {
			if err := conn.WriteMessage(websocket.TextMessage, []byte(resData)); err != nil {
				debugLog.Infof("发送消息时出错: %v", err)
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
					debugLog.Infof("读取消息时出错: %v", readMsgErr)
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
				debugLog.Infof("读取文件时出错: %v", err)
				conn.Close() // 关闭连接
				break
			}
			d := dic_dto.NewDic(dicpath, wsfileData)
			d.SetFunc("断开连接", dto.DicFunc{
				L: "0",
				Fn: func(d *dto.DicInputs) (any, error) {
					conn.Close()
					return "", nil
				}})
			d.Val.P.Set("类型", typeName)
			rStr := ""
			if wsClose {
				rStr = dic_api.Api.DicRunPrivate(d, Tstr)
			} else {
				rStr = dic_api.Api.DicRun(d, Tstr)
			}
			// 拦截并处理错误
			if readMsgErr != nil {
				if rStr != "" {
					debugLog.Infof("%v", rStr)
					break
				}
			}
			if rStr != "" {
				if wsClose {
					debugLog.Infof("%v", rStr)
				} else if err := conn.WriteMessage(websocket.TextMessage, []byte(rStr)); err != nil {
					debugLog.Infof("发送消息时出错: %v", err)
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

// 创建WS（监听）
func wsCreate(d *dto.DicInputs) (any, error) {
	addr := d.Inputs.String(1)
	if addr == "" {
		return "", errors.New("创建WS：访问路径不能为空")
	}
	if !strings.HasPrefix(addr, "/") {
		addr = "/" + addr
	}

	dicPath := d.Inputs.String(2)
	if dicPath == "" {
		return "", errors.New("创建WS：词库路径不能为空")
	}
	cors := true
	if d.Inputs.LenOk(3) {
		cors = d.Inputs.Bool(3)
	}

	// 使用默认服务端词库时，若文件不存在则写入内置模板
	if dicPath == "private/websocket/server.n" {
		f := utils.NewFileQueue(dicPath)
		if !f.FileExists() {
			if s, err := appfiles.GetFileString("dic/websocket/server.n"); err == nil {
				f.WriteToFile(s)
			}
		}
	}

	wsConn := &websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	if !cors {
		wsConn.CheckOrigin = nil
	}

	ws := &dto.ServerRouterWebSocket{
		Open:     true,
		Addr:     addr,
		Cors:     cors,
		FilePath: dicPath,
		Conn:     wsConn,
	}
	dto.ServerConfig.AddWs(ws)

	// 返回 WS 对象（面对像），方法内全局变量随 WS 一直存在
	instance := &dto.DicClass{
		LocalValue: dto.NewVal().
			SetRaw("_WS_", ws).
			Set("访问路径", addr).
			Set("跨域", cors),
	}
	dto.SetThreadVarRaw("_词库路径_", dicPath)
	instance.Fn = map[string]dto.DicFunc{
		"设置跨域": {L: "1", Fn: func(d *dto.DicInputs) (any, error) {
			cors := d.Inputs.Bool(1)
			ws.Cors = cors
			ws.Conn = &websocket.Upgrader{
				CheckOrigin: func(r *http.Request) bool { return true },
			}
			if !cors {
				ws.Conn.CheckOrigin = nil
			}
			dto.ServerConfig.AddWs(ws)
			instance.LocalValue.Set("跨域", cors)
			return "", nil
		}},
		"设置词库路径": {L: "1", Fn: func(d *dto.DicInputs) (any, error) {
			p := d.Inputs.String(1)
			if p == "" {
				return "", errors.New("WS设置词库路径：路径不能为空")
			}
			ws.FilePath = p
			dto.ServerConfig.AddWs(ws)
			dto.SetThreadVarRaw("_词库路径_", p)
			return "", nil
		}},
		"设置访问路径": {L: "1", Fn: func(d *dto.DicInputs) (any, error) {
			p := d.Inputs.String(1)
			if p == "" {
				return "", errors.New("WS设置访问路径：路径不能为空")
			}
			if !strings.HasPrefix(p, "/") {
				p = "/" + p
			}
			old := ws.Addr
			ws.Addr = p
			dto.ServerConfig.AddWs(ws)
			if old != "" && old != p {
				dto.ServerConfig.RemoveWs(old)
			}
			instance.LocalValue.Set("访问路径", p)
			return "", nil
		}},
		"设置变量": {L: "2", Fn: func(d *dto.DicInputs) (any, error) {
			name := d.Inputs.String(1)
			if name == "" {
				return "", errors.New("WS设置变量：变量名不能为空")
			}
			val := d.Inputs.Get(2)
			if val == nil {
				val = ""
			}
			instance.LocalValue.Set(name, val)
			return "", nil
		}},
	}
	return instance, nil
}

// =================== 服务器 ===================

// dicServerState 服务器（轻量 HTTP 词库服务器）运行状态
type dicServerState struct {
	mu            sync.RWMutex
	addr          string // 监听地址（含端口），用于判断是否核心服务器以挂载 WebUI
	dicData       string // 词库源码文本（含自定义函数无法序列化时用于回退编译）
	compiledBytes []byte // 编译好的词库数据（gob 序列化），请求时直接反序列化复用
	cors          bool
	srv           *http.Server
	// 词库路径热更新（设置路由词库传入路径时启用）
	dicPath    string    // 词库文件绝对路径，非空则每次请求前检测文件变化并自动重载
	dicModTime time.Time // 上次编译时词库文件的修改时间
	dicSize    int64     // 上次编译时词库文件大小
	// HTTPS
	tls      bool
	certFile string
	keyFile  string
	// BeerFrp 穿透
	frpOpen       bool
	frpServerAddr string
	frpToken      string
	frpDebug      bool
}

// resolveServerTLSPath 解析函数服务器的证书路径：相对路径基于应用数据目录，绝对路径原样返回。
func resolveServerTLSPath(p string) string {
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(utils.GetAppDir(), filepath.FromSlash(p))
}

// createServer 创建独立 HTTP 词库服务器（仅创建配置，不启动；调用对象.启动后开始监听）。
// 每个请求以 URL 路径为触发词运行词库，输出作为 HTTP 响应。
// 用法: s:$创建服务器 <端口地址> <编译好的词库数据>$ 然后 $s.启动$
// 第二个参数只接受「编译好的词库数据」（$编译词库$ 或 //@资源 匹配 .n 自动编译得到的 base64 gob 产物），
// 源码文本请先用 $编译词库$ 编译，或创建后通过「服务器.设置路由词库」传入源码重新编译。
func createServer(d *dto.DicInputs) (any, error) {
	addr := d.Inputs.String(1)
	if addr == "" {
		return "", errors.New("创建服务器：端口地址不能为空")
	}
	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}
	dicData := d.Inputs.Get(2)

	// 第二参数接受编译好的词库数据（gob 字节）：
	// - map（「编译词库」直接返回值）取「编译数据」字段（[]byte）；
	// - []byte（//@资源 变量:xxx.n 的自动编译结果）直接使用。
	// 均不接受源码文本，也无需 base64 编解码。
	compiledBytes, _ := compiledBytesFrom(dicData)
	if dicData != nil && compiledBytes == nil {
		return "", errors.New("创建服务器：第二个参数必须是编译好的词库数据")
	}

	st := &dicServerState{
		addr:          addr,
		dicData:       "",
		compiledBytes: compiledBytes,
		cors:          true,
	}

	instance := &dto.DicClass{
		LocalValue: dto.NewVal().
			SetRaw("_服务器_", st).
			Set("端口地址", addr).
			Set("跨域", true),
	}

	// stopServer 停止服务器并注销，对象.关闭与前端操作均走此闭包
	stopServer := func() {
		st.mu.Lock()
		srv := st.srv
		st.srv = nil
		st.mu.Unlock()
		if srv != nil {
			_ = srv.Close()
		}
		dto.FuncServers.Remove(addr)
	}

	// setDic 字典方法「服务器.设置路由词库」：编译源码并即时生效
	setDic := func(dicData string) error {
		compiled := compileServerText(dicData)
		st.mu.Lock()
		st.dicPath = ""
		st.dicData = dicData
		st.compiledBytes = compiled
		st.mu.Unlock()
		dto.FuncServers.UpdateData(addr, dicData)
		return nil
	}

	// setDicPath 以词库文件路径设置并启用实时热更新：记录文件路径与修改时间，后续请求前自动检测重载。
	setDicPath := func(path, text string) error {
		compiled := compileServerText(text)
		info, _ := os.Stat(path)
		st.mu.Lock()
		st.dicPath = path
		st.dicData = text
		st.compiledBytes = compiled
		if info != nil {
			st.dicModTime = info.ModTime()
			st.dicSize = info.Size()
		}
		st.mu.Unlock()
		dto.FuncServers.UpdateData(addr, text)
		return nil
	}

	// setDicInput 「服务器.设置路由词库」入口：支持编译好的词库数据（map/[]byte）或词库路径/源码。
	setDicInput := func(val any) error {
		// 编译好的词库数据：map（$编译词库$ 返回值）或 []byte（//@资源 编译产物）
		if b, ok := compiledBytesFrom(val); ok {
			st.mu.Lock()
			st.dicPath = ""
			st.dicData = ""
			st.compiledBytes = b
			st.mu.Unlock()
			dto.FuncServers.UpdateData(addr, "")
			return nil
		}
		// 字符串：以 .n 结尾按词库文件路径处理（不存在时自动从内嵌模板生成），否则按源码文本编译
		if s, ok := val.(string); ok {
			if strings.HasSuffix(s, ".n") {
				f := utils.NewFileQueue(s)
				if !f.FileExists() {
					if data, err := appfiles.GetFile("dic/system/" + filepath.Base(s)); err == nil {
						f.WriteFileByte(data)
					}
				}
				if !f.FileExists() {
					return errors.New("服务器.设置路由词库：词库文件不存在：" + s)
				}
				data, err := f.ReadFileByte()
				if err != nil {
					return err
				}
				return setDicPath(f.FileName, string(data))
			}
			return setDic(s)
		}
		return errors.New("服务器.设置路由词库：参数必须是编译好的词库数据或词库路径/源码")
	}

	// startListen 绑定端口并启动监听（HTTP/HTTPS），返回后服务器即处于监听状态
	startListen := func() error {
		st.mu.RLock()
		if st.srv != nil {
			st.mu.RUnlock()
			return errors.New("服务器已启动")
		}
		useTLS := st.tls
		certFile := resolveServerTLSPath(st.certFile)
		keyFile := resolveServerTLSPath(st.keyFile)
		st.mu.RUnlock()

		// 先绑定端口，端口被占用时立即返回错误
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("服务器监听失败: %v", err)
		}

		// HTTPS 模式下提前校验证书，避免异步启动后才暴露错误
		if useTLS {
			if _, err := tls.LoadX509KeyPair(certFile, keyFile); err != nil {
				_ = ln.Close()
				return fmt.Errorf("HTTPS 证书加载失败: %v", err)
			}
		}

		srv := &http.Server{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				st.serveHTTP(w, r)
			}),
		}

		st.mu.Lock()
		if st.srv != nil {
			st.mu.Unlock()
			_ = ln.Close()
			return errors.New("服务器已启动")
		}
		st.srv = srv
		st.mu.Unlock()

		go func() {
			var err error
			if useTLS {
				err = srv.ServeTLS(ln, certFile, keyFile)
			} else {
				err = srv.Serve(ln)
			}
			if err != nil && err != http.ErrServerClosed {
				debugLog.Errorf("服务器(%s)运行失败: %v", addr, err)
			}
		}()
		return nil
	}

	// syncSnapshot 将当前配置同步到注册表快照（仅更新可编辑配置，保留闭包；Core 由 coreAddr 派生，无需处理）
	syncSnapshot := func() {
		st.mu.RLock()
		cors := st.cors
		tlsOn := st.tls
		certFile := st.certFile
		keyFile := st.keyFile
		frpOpen := st.frpOpen
		frpServerAddr := st.frpServerAddr
		frpToken := st.frpToken
		frpDebug := st.frpDebug
		st.mu.RUnlock()

		dto.FuncServers.Apply(addr, func(e *dto.FuncServerInfo) {
			e.Cors = cors
			e.TLS = tlsOn
			e.CertFile = certFile
			e.KeyFile = keyFile
			e.FrpOpen = frpOpen
			e.FrpServerAddr = frpServerAddr
			e.FrpToken = frpToken
			e.FrpDebug = frpDebug
		})
	}

	// restartFrp 按当前配置重启 BeerWebFrp 隧道。
	restartFrp := func() {
		st.mu.RLock()
		open := st.frpOpen
		serverAddr := st.frpServerAddr
		token := st.frpToken
		debug := st.frpDebug
		st.mu.RUnlock()
		dic_server.StartServerFrp(addr, open, serverAddr, token, debug)
	}

	// restartHTTP 关闭旧监听并按当前 TLS 配置重新监听（不重建注册表条目）
	restartHTTP := func() error {
		st.mu.Lock()
		srv := st.srv
		st.srv = nil
		st.mu.Unlock()
		if srv != nil {
			_ = srv.Close()
		}
		return startListen()
	}

	// updateConfig 前端编辑：应用跨域/HTTPS/BeerFrp 增量更新，必要时重启监听或隧道
	updateConfig := func(upd dto.FuncServerUpdate) error {
		st.mu.Lock()
		needHTTPRestart := false
		needFrpRestart := false
		var corsChanged *bool

		if upd.Cors != nil {
			st.cors = *upd.Cors
			corsChanged = upd.Cors
		}
		if upd.TLS != nil && *upd.TLS != st.tls {
			st.tls = *upd.TLS
			needHTTPRestart = true
		}
		if upd.CertFile != nil && *upd.CertFile != st.certFile {
			st.certFile = *upd.CertFile
			needHTTPRestart = true
		}
		if upd.KeyFile != nil && *upd.KeyFile != st.keyFile {
			st.keyFile = *upd.KeyFile
			needHTTPRestart = true
		}
		if upd.FrpOpen != nil && *upd.FrpOpen != st.frpOpen {
			st.frpOpen = *upd.FrpOpen
			needFrpRestart = true
		}
		if upd.FrpServerAddr != nil && *upd.FrpServerAddr != st.frpServerAddr {
			st.frpServerAddr = *upd.FrpServerAddr
			needFrpRestart = true
		}
		if upd.FrpToken != nil && *upd.FrpToken != st.frpToken {
			st.frpToken = *upd.FrpToken
			needFrpRestart = true
		}
		if upd.FrpDebug != nil && *upd.FrpDebug != st.frpDebug {
			st.frpDebug = *upd.FrpDebug
			needFrpRestart = true
		}
		st.mu.Unlock()

		if corsChanged != nil {
			instance.LocalValue.Set("跨域", *corsChanged)
		}

		syncSnapshot()

		if needHTTPRestart {
			if err := restartHTTP(); err != nil {
				return err
			}
		}
		if needFrpRestart {
			restartFrp()
		}
		return nil
	}

	// register 首次启动时将服务器注册到全局列表供前端查询/编辑
	register := func() {
		st.mu.RLock()
		dto.FuncServers.Add(&dto.FuncServerInfo{
			Addr:          addr,
			Cors:          st.cors,
			DicData:       st.dicData,
			TLS:           st.tls,
			CertFile:      st.certFile,
			KeyFile:       st.keyFile,
			FrpOpen:       st.frpOpen,
			FrpServerAddr: st.frpServerAddr,
			FrpToken:      st.frpToken,
			FrpDebug:      st.frpDebug,
			Handler:       http.HandlerFunc(st.serveHTTP),
			Update:        updateConfig,
			Close:         stopServer,
		})
		st.mu.RUnlock()
	}

	// startServer 启动服务器：绑定监听 + 注册 + 启动 BeerFrp 隧道
	startServer := func() error {
		if err := startListen(); err != nil {
			return err
		}
		register()
		restartFrp()
		return nil
	}

	instance.Fn = map[string]dto.DicFunc{
		"启动": {L: "0", Fn: func(d *dto.DicInputs) (any, error) {
			if err := startServer(); err != nil {
				return "", err
			}
			return "true", nil
		}},
		"设置路由词库": {L: "1", Fn: func(d *dto.DicInputs) (any, error) {
			if err := setDicInput(d.Inputs.Get(1)); err != nil {
				return "", err
			}
			return "", nil
		}},
		"关闭": {L: "0", Fn: func(d *dto.DicInputs) (any, error) {
			stopServer()
			return "true", nil
		}},
		"设置变量": {L: "2", Fn: func(d *dto.DicInputs) (any, error) {
			name := d.Inputs.String(1)
			if name == "" {
				return "", errors.New("服务器.设置变量：变量名不能为空")
			}
			val := d.Inputs.Get(2)
			if val == nil {
				val = ""
			}
			instance.LocalValue.Set(name, val)
			return "", nil
		}},
		"设置跨域": {L: "1", Fn: func(d *dto.DicInputs) (any, error) {
			cors := d.Inputs.Bool(1)
			return updateConfig(dto.FuncServerUpdate{Cors: &cors}), nil
		}},
		"设置核心服务器": {L: "0", Fn: func(d *dto.DicInputs) (any, error) {
			if !dto.FuncServers.SetCore(addr) {
				return "", errors.New("服务器.设置核心服务器：服务器尚未启动")
			}
			// 首次标记核心服务器时，触发一次性配置加载（config.yaml / router.n / 机器人等服务初始化）
			dto.LoadConfigOnce()
			return "", nil
		}},
		"设置BeerFrp": {L: "0|1|2|3|4", Fn: func(d *dto.DicInputs) (any, error) {
			// 参数顺序：开关 / 调试 / 令牌 / 服务器地址；默认 true / false / 空 / wss://frp.cjxpj.com
			open := true
			if d.Inputs.Len() >= 1 {
				open = d.Inputs.Bool(1)
			}
			debug := false
			if d.Inputs.Len() >= 2 {
				debug = d.Inputs.Bool(2)
			}
			token := d.Inputs.StringDefault(3, "")
			serverAddr := d.Inputs.StringDefault(4, "wss://frp.cjxpj.com")
			return updateConfig(dto.FuncServerUpdate{
				FrpOpen:       &open,
				FrpServerAddr: &serverAddr,
				FrpToken:      &token,
				FrpDebug:      &debug,
			}), nil
		}},
	}

	return instance, nil
}

// newServerDic 将「创建服务器」的词库源码文本编译为词库。
// 行切分与文件加载一致（去掉结尾换行），避免结尾空行把全文误判为头部。
func newServerDic(text string) *dic_dto.Dic {
	lines := utils.SplitLines([]byte(text))
	raw := []byte(text)
	// 单行密文：整块解密后重新切分（与 NewDicFile 一致）
	if len(lines) == 1 {
		if str, err := utils.Decrypt(utils.RemoveComments(lines[0]), appfiles.Key); err == nil {
			lines = utils.SplitLines([]byte(str))
			raw = []byte(str)
		}
	}
	split := run.BuildDicLinesWithRaw("服务器", lines, raw)
	return &dic_dto.Dic{
		Data:   split,
		Val:    dto.NewDicVal(),
		Path:   "服务器",
		MyFunc: split.MyFunc,
	}
}

// compileServerText 将词库源码文本编译为 gob 字节，供请求时反序列化复用。
// 含自定义函数（bot 注入）的词库无法序列化，返回 nil，运行时回退到按源码重新编译。
func compileServerText(text string) []byte {
	if text == "" {
		return nil
	}
	dic := newServerDic(text)
	data, err := run.MarshalBuildValue(dic.Data)
	if err != nil {
		return nil
	}
	return data
}

// compiledBytesFrom 从输入提取编译好的词库数据（gob 字节）。
// 支持 map（「编译词库」返回值的「编译数据」字段）与 []byte（//@资源 编译产物）；非编译数据返回 false。
func compiledBytesFrom(val any) ([]byte, bool) {
	switch v := val.(type) {
	case map[string]any:
		if b, ok := v["编译数据"].([]byte); ok && len(b) > 0 {
			if _, err := run.UnmarshalBuildValue(b); err == nil {
				return b, true
			}
		}
	case []byte:
		if len(v) > 0 {
			if _, err := run.UnmarshalBuildValue(v); err == nil {
				return v, true
			}
		}
	}
	return nil, false
}

// compileDic 编译词库源码，直接返回 map 结构数据（避免 base64 编解码与 JSON 序列化造成数据错误）：
// 「编译数据」为原始 gob 字节（[]byte）、「报错数据」「警告数据」各为 []dto.BuildWarning。
// 用法: $编译词库 <词库数据>$
func compileDic(d *dto.DicInputs) (any, error) {
	data := d.Inputs.String(1)
	if data == "" {
		return map[string]any{}, nil
	}
	dic := newServerDic(data)

	// 编译数据：原始 gob 字节（含自定义函数无法序列化时为 nil）
	var compiled []byte
	if b, err := run.MarshalBuildValue(dic.Data); err == nil {
		compiled = b
	}

	// 报错数据 / 警告数据：按编译警告级别拆分
	errs := make([]dto.BuildWarning, 0)
	warns := make([]dto.BuildWarning, 0)
	for _, w := range dic.Data.Warnings {
		if w.Level == "error" {
			errs = append(errs, w)
		} else {
			warns = append(warns, w)
		}
	}

	return map[string]any{
		"编译数据": compiled,
		"报错数据": errs,
		"警告数据": warns,
	}, nil
}

// coreServer 返回核心服务器的 WebUI 地址，供启动词库打印。无核心服务器时返回空串。
// 用法: $核心服务器$
func coreServer(d *dto.DicInputs) (any, error) {
	addr := dto.FuncServers.CoreAddr()
	if addr == "" {
		return "", nil
	}
	return coreServerWebUI(addr), nil
}

// coreServerWebUI 根据核心服务器监听地址拼接 WebUI 地址（http://localhost:端口/管理面板路径）。
func coreServerWebUI(addr string) string {
	host := "localhost"
	port := ""
	if h, p, err := net.SplitHostPort(addr); err == nil {
		if h != "" && h != "0.0.0.0" && h != "::" {
			host = h
		}
		port = p
	} else {
		port = strings.TrimPrefix(addr, ":")
	}
	opui := "/"
	if dto.ServerConfig.OPUI != nil && dto.ServerConfig.OPUI.Addr != "" {
		opui = dto.ServerConfig.OPUI.Addr
	}
	webui := "http://" + net.JoinHostPort(host, port) + opui
	// 若已生成快捷登录码，拼接到 WebUI 地址上，实现打开链接即快捷登录
	if tk := dic_server.GetOpuiQuickToken(); tk != "" {
		webui += "?key=" + tk
	}
	return webui
}

// newServerDicFromBytes 从编译产物反序列化词库；失败时按源码文本回退编译。
func newServerDicFromBytes(compiled []byte, text string) *dic_dto.Dic {
	if len(compiled) != 0 {
		if bv, err := run.UnmarshalBuildValue(compiled); err == nil {
			return &dic_dto.Dic{
				Data:   bv,
				Val:    dto.NewDicVal(),
				Path:   "服务器",
				MyFunc: bv.MyFunc,
			}
		}
	}
	return newServerDic(text)
}

// reloadDicIfChanged 路径模式下检测词库文件是否变化，变化则重新编译加载（实时热更新）。
func (st *dicServerState) reloadDicIfChanged() {
	st.mu.RLock()
	dicPath := st.dicPath
	modTime := st.dicModTime
	size := st.dicSize
	st.mu.RUnlock()
	if dicPath == "" {
		return
	}
	info, err := os.Stat(dicPath)
	if err != nil {
		return
	}
	if info.ModTime().Equal(modTime) && info.Size() == size {
		return
	}
	data, err := os.ReadFile(dicPath)
	if err != nil {
		return
	}
	compiled := compileServerText(string(data))
	st.mu.Lock()
	st.dicData = string(data)
	st.compiledBytes = compiled
	st.dicModTime = info.ModTime()
	st.dicSize = info.Size()
	st.mu.Unlock()
	dto.FuncServers.UpdateData(st.addr, string(data))
}

// serveHTTP 处理单个 HTTP 请求：以 URL 路径为触发词运行词库
func (st *dicServerState) serveHTTP(w http.ResponseWriter, r *http.Request) {
	// 核心服务器承载内置入口（防火墙/管理面板/云工具/机器人 webhook），其余服务器仅跑词库路由
	if dto.FuncServers.CoreAddr() == st.addr {
		// 防火墙（IP 黑名单 + 防火墙词库）
		if dic_server.CheckFirewall(w, r) {
			return
		}

		// 管理面板（WebUI）
		if opui := dto.ServerConfig.OPUI; opui != nil {
			if getpath, ok := strings.CutPrefix(r.URL.Path, opui.Addr); ok {
				if opui.Cors && setCORS(w, r, "*") {
					return
				}
				dic_server.OpUI(w, r, getpath)
				return
			}
		}

		// 内置云工具服务端
		if cloudTool := dto.ServerConfig.CloudTool; cloudTool != nil && cloudTool.Open {
			if r.URL.Path == cloudTool.Addr || strings.HasPrefix(r.URL.Path, cloudTool.Addr+"/") {
				dic_server.CloudToolServerHandler(w, r)
				return
			}
		}

		// 机器人 webhook（飞书/QQ/NapCat/云乎）
		s := dto.ServerConfig
		if feishu := s.FeiShuBot; feishu != nil && feishu.Open && r.URL.Path == feishu.Addr {
			feishubot.BotMessage(w, r)
			return
		}
		if s.QQBots != nil {
			for _, bot := range s.QQBots {
				if bot != nil && bot.Open && !bot.Ws && r.URL.Path == bot.Addr {
					qqbot.BotMessage(w, r, bot)
					return
				}
			}
		}
		if napcat := s.NapCatBot; napcat != nil && napcat.Open && r.URL.Path == napcat.Addr {
			napcatbot.BotMessage(w, r)
			return
		}
		if yunhu := s.YunHuBot; yunhu != nil && yunhu.Open && r.URL.Path == yunhu.Addr {
			yunhubot.BotMessage(w, r)
			return
		}

		// WebSocket 服务器监听路径
		for _, ws := range dto.ServerConfig.WsListSnapshot() {
			if ws.Open && r.URL.Path == ws.Addr {
				handleWsServer(w, r, ws)
				return
			}
		}
	}

	// 词库路径热更新：检测文件变化并自动重载
	st.reloadDicIfChanged()

	st.mu.RLock()
	dicData := st.dicData
	compiledBytes := st.compiledBytes
	cors := st.cors
	st.mu.RUnlock()

	if cors {
		if setCORS(w, r, "*") {
			return
		}
	}

	responseData := newHTTPRequestInfo(r)

	globalV := dto.NewVal().
		Set("响应状态", "200").
		Set("输出头部", "{}").
		Set("COOKIE", "[]").
		Set("网站根目录", dto.DefaultWebRoot)

	// 请求指针
	globalV.SetRaw("_请求数据_", r)
	globalV.SetRaw("_响应数据_", w)

	resS, err := json.Marshal(responseData)
	if err != nil {
		utils.Error("访问数据异常")
		return
	}
	globalV.Set("访问数据", string(resS))

	dic := newServerDicFromBytes(compiledBytes, dicData).SetGlobal_v(globalV)
	// 设置头部/GET/POST 处理
	setDicWebFuncs(dic, w, responseData.QueryParams, responseData)

	RunData := dic_api.Api.DicRun(dic, r.URL.Path)
	writeDicWebOutput(w, globalV, RunData)
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
