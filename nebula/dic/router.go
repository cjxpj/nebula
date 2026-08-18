//go:build !js

package dic

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/debugLog"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
	"github.com/gorilla/websocket"
)

// 词库路由
func dicWebRouter(w http.ResponseWriter, r *http.Request) {
	s := dto.ServerConfig

	// 运行结果
	var RunData string

	// 访问路径
	path := r.URL.Path

	// 访问类型
	getType := r.Method

	queryParams := r.URL.Query()

	ip := utils.GetClientIP(r)

	for _, ws := range s.WsListSnapshot() {
		if ws.Open && path == ws.Addr {
			handleWsServer(w, r, ws)
			return
		}
	}

	// 输出运行结果
	var send []byte
	responseData := &dto.HTTPRequestInfo{
		Path:        path,
		Type:        getType,
		QueryParams: queryParams,
		Headers:     r.Header,
		IP:          ip,
		Host:        r.Host,
	}

	if getType == "POST" {
		if err := r.ParseMultipartForm(32 << 20); r.MultipartForm != nil && err == nil {
			var resFileData = make(map[string][]*dto.PostFile)
			// fmt.Println(r.MultipartForm.File)
			for fieldName := range r.MultipartForm.File {
				file, h, err := r.FormFile(fieldName)
				if err == nil {
					defer file.Close()
					content, err := io.ReadAll(file)
					if err == nil {
						fileContent := base64.StdEncoding.EncodeToString(content)
						resFileData[fieldName] = append(resFileData[fieldName], &dto.PostFile{
							Name: h.Filename,
							Size: h.Size,
							Data: fileContent,
						})
					}
				}
			}
			responseData.PostFile = resFileData
		}

		var body_map map[string]any
		body, err := io.ReadAll(r.Body)
		if err == nil {
			defer r.Body.Close()
			if err := json.Unmarshal(body, &body_map); err == nil {
				responseData.Post = body_map
			} else {
				strBody := string(body)
				if strBody == "" {
					// 获取POST参数
					r.ParseForm()
					postParams := r.PostForm
					responseData.Post = postParams
				} else {
					responseData.Post = strBody
				}
			}
		} else {
			// 获取POST参数
			r.ParseForm()
			postParams := r.PostForm
			responseData.Post = postParams
		}
	}

	// 将数据转换为JSON格式
	resS, err := json.Marshal(responseData)
	if err != nil {
		utils.Error("访问数据异常")
		return
	}
	responseJSON := string(resS)

	var FileData string

	routerFile := utils.NewFileQueue("private/system/router.n")
	if !routerFile.FileExists() {
		if data, e := appfiles.GetFile("dic/system/router.n"); e == nil {
			routerFile.WriteFileByte(data)
		} else {
			fmt.Println("embed err:", e)
		}
	}

	FileData, err = routerFile.ReadFromFile()
	if err != nil {
		utils.Error("读取路由词库出错")
		return
	}

	// 运行词库
	globalV := dto.NewVal().
		Set("响应状态", "200").
		Set("输出头部", "{}").
		Set("COOKIE", "[]").
		Set("访问数据", string(responseJSON))

	// 请求指针
	globalV.Set("_请求数据_", r)
	globalV.Set("_响应数据_", w)

	dic := dic_dto.NewDic("private/system/router.n", FileData)
	defer dic.Close()

	dic.SetGlobal_v(globalV).
		// 终止服务器
		SetFunc("终止服务器", dto.DicFunc{
			L: "0",
			Fn: func(d *dto.DicInputs) (any, error) {
				if srv, ok := dto.GV.Get("_监听线程_").(*http.Server); ok && srv != nil {
					srv.Close()
					return "true", nil
				}
				return "false", nil
			}}).
		// 设置头部
		SetFunc("设置头部", dto.DicFunc{
			L: "2",
			Fn: func(d *dto.DicInputs) (any, error) {
				if r, ok := d.Inputs.Get(1).(string); ok {
					if r2, ok := d.Inputs.Get(2).(string); ok {
						w.Header().Set(r, r2)
						return "", nil
					}
					return "参数错误2", nil
				}
				return "参数错误1", nil
			}}).
		// GET处理
		SetFunc("GET", dto.DicFunc{
			L: "1|2",
			Fn: func(d *dto.DicInputs) (any, error) {
				if r, ok := d.Inputs.Get(1).(string); ok {
					if s := queryParams.Get(r); s != "" {
						return s, nil
					}
					if r, ok := d.Inputs.Get(2).(string); ok {
						return r, nil
					}
				}
				return "", nil
			}}).
		// POST处理
		SetFunc("POST", dto.DicFunc{
			L: "1|2",
			Fn: func(d *dto.DicInputs) (any, error) {
				key, ok := d.Inputs.Get(1).(string)
				if !ok {
					return "参数必须是字符串", nil
				}

				// 打印类型
				// fmt.Printf("类型: %T\n", responseData.Post)

				// 处理不同类型的 Post
				switch post := responseData.Post.(type) {
				case url.Values:
					if val := post.Get(key); val != "" {
						return val, nil
					}
				case map[string]any:
					if v, exists := post[key]; exists {
						switch num := v.(type) {
						case int:
							return strconv.FormatInt(int64(num), 10), nil
						case int64:
							return strconv.FormatInt(num, 10), nil
						case float64:
							return strconv.FormatFloat(num, 'f', -1, 64), nil
						default:
							return fmt.Sprint(v), nil
						}
					}
				case map[string][]string:
					if arr, exists := post[key]; exists && len(arr) > 0 {
						return arr[0], nil
					}
				}

				// 默认值
				if d.Inputs.LenOk(2) {
					if def, ok := d.Inputs.Get(2).(string); ok {
						return def, nil
					}
				}
				return "", nil
			}})

	RunData = dic_api.Api.DicRun(dic, path)

	SendHeade, _ := globalV.Get("输出头部").(string)
	SendCOOKIE, _ := globalV.Get("COOKIE").(string)

	var headerMap map[string]string
	var cookieMap []*dto.SetCookie

	if SendHeade != "{}" {
		if err = json.Unmarshal([]byte(SendHeade), &headerMap); err == nil {
			for key, value := range headerMap {
				w.Header().Set(key, value)
			}
		}
	}

	if SendCOOKIE != "[]" {
		if err = json.Unmarshal([]byte(SendCOOKIE), &cookieMap); err == nil {
			for _, value := range cookieMap {
				http.SetCookie(w, &http.Cookie{
					Name:     value.Name,
					Value:    value.Value,
					Path:     value.Path,
					HttpOnly: value.HttpOnly,
					MaxAge:   value.MaxAge,
				})
			}
		}
	}

	HeadInt := "200"
	if rH, ok := globalV.Get("响应状态").(string); ok {
		HeadInt = rH
	}

	if num, e := strconv.Atoi(HeadInt); e == nil {
		w.WriteHeader(num)
	}

	send = []byte(RunData)

	// 输出内容到响应
	w.Header().Set("Content-Length", strconv.Itoa(len(send)))
	_, err = w.Write(send)
	if err != nil {
		errMsg := fmt.Sprintf("服务器输出Error: %s", err)
		utils.Error(errMsg)
	}
}

// handleWsServer 处理单个 WebSocket 监听路径
func handleWsServer(w http.ResponseWriter, r *http.Request, ws *dto.ServerRouterWebSocket) {
	if ws == nil || ws.Conn == nil {
		return
	}
	conn, err := ws.Conn.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	path := r.URL.Path
	getType := r.Method
	queryParams := r.URL.Query()
	ip := utils.GetClientIP(r)

	// 心跳机制：防止中间代理/防火墙断开空闲连接
	const (
		pongWait   = 60 * time.Second
		pingPeriod = (pongWait * 9) / 10
		writeWait  = 10 * time.Second
	)
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for range ticker.C {
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if e := conn.WriteMessage(websocket.PingMessage, nil); e != nil {
				return
			}
		}
	}()

	responseData := dto.HTTPRequestInfo{
		Path:        path,
		Type:        getType,
		QueryParams: queryParams,
		Headers:     r.Header,
		IP:          ip,
		Host:        r.Host,
	}

	// 将数据转换为JSON格式
	responseJSON, err := json.Marshal(responseData)
	if err != nil {
		utils.Error("访问数据异常")
		return
	}

	dicPath := ws.FilePath
	if dicPath == "" {
		dicPath = "private/websocket/server.n"
	}

	// 运行词库
	if wsFileData, err := utils.NewFileQueue(dicPath).ReadFromFile(); err == nil {
		dic := dic_dto.NewDic(dicPath, wsFileData)
		dic.Val.P.Set("_词库路径_", dicPath)
		dic.Val.G.Set("访问数据", string(responseJSON))
		dic.SetFunc("断开连接", dto.DicFunc{
			L: "0",
			Fn: func(d *dto.DicInputs) (any, error) {
				conn.Close()
				return "", nil
			}})
		dic.Val.G.Set("_WS连接_", conn)
		resData := dic_api.Api.DicRunPrivate(dic, "连接成功")
		if resData != "" {
			if err := conn.WriteMessage(websocket.TextMessage, []byte(resData)); err != nil {
				debugLog.Infof("发送消息时出错: %v", err)
			}
		}
	}

	go func() {
		messageTypeMap := map[int]string{
			websocket.TextMessage:   "文本消息",
			websocket.BinaryMessage: "二进制消息",
		}

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

			wsfile := utils.NewFileQueue(dicPath)
			wsfileData, err := wsfile.ReadFromFile()
			if err != nil {
				debugLog.Infof("读取文件时出错: %v", err)
				conn.Close() // 关闭连接
				break
			}
			d := dic_dto.NewDic(dicPath, wsfileData)
			d.Val.P.Set("_词库路径_", dicPath)
			d.Val.G.Set("_WS连接_", conn)
			d.Val.G.Set("访问数据", string(responseJSON))
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
}
