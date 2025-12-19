package dic

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/napcatbottool/napcatbotapi"
	"github.com/cjxpj/nebula/qqbottool/qqbotapi"
	"github.com/cjxpj/nebula/utils"
	yunhubotapi "github.com/cjxpj/nebula/yunhuBotTool/yunhubotApi"

	"github.com/gorilla/websocket"
	"github.com/patrickmn/go-cache"
	"golang.ngrok.com/ngrok"
	"golang.ngrok.com/ngrok/config"
)

var ServerRouterData = &ServeRouter{}

// 启动服务器
func Start() {
	file := utils.NewFile()
	file.SetPath("README.md").WriteFileByte(appfiles.GetFile("dic.md"))

	file.SetPath("private/ttf/font.ttf")
	if !file.FileExists() {
		file.WriteFileByte(appfiles.GetFile("font.ttf"))
	}

	file.SetPath("private/system/config.n")
	if !file.FileExists() {
		file.WriteFileByte(appfiles.GetFile("dic/config.n"))
	}

	FileData, err := file.ReadFromFile()
	if err != nil {
		utils.ErrorStop("启动配置不存在")
	}

	GV := dto.NewVal()
	GV.Set("版本", appfiles.Version)
	infoDic := NewDic("private/system/config.n", FileData)
	infoDic.SetGlobal_v(GV)

	// fmt.Println("Nebula触发")
	if res := infoDic.NewRun("启动"); res != "" {
		fmt.Println(res)
	}

	infoServerPath := infoDic.NewRun("监听访问路径")

	// 路由词库
	file.SetPath("private/system/router.n")
	if !file.FileExists() {
		file.WriteFileByte(appfiles.GetFile("dic/router.n"))

		// WS词库
		file.SetPath("private/websocket")
		if !file.DirExists() {
			// 服务器
			file.SetPath("private/websocket/server.n")
			if !file.FileExists() {
				file.WriteToFile(appfiles.GetFileString("dic/websocket/server.n"))
			}
			// 客户端
			file.SetPath("private/websocket/app.n")
			if !file.FileExists() {
				file.WriteToFile(appfiles.GetFileString("dic/websocket/app.n"))
			}
		}

		// 主页文件
		file.SetPath("public")
		if !file.DirExists() {
			file.SetPath("public/index.wn")
			if !file.FileExists() {
				file.WriteToFile(appfiles.GetFileString("dic/public/index.wn"))
			}

			// 默认样板文件
			file.SetPath("public/api.n")
			if !file.FileExists() {
				file.WriteToFile(appfiles.GetFileString("dic/public/api.n"))
			}
			// 404文件
			file.SetPath("public/404.wn")
			if !file.FileExists() {
				file.WriteFileByte(appfiles.GetFile("dic/public/404.wn"))
			}
		}

	}

	if _, ok := dto.GV.Get("_监听线程_").(*http.Server); !ok {

		if d := infoDic.NewRun("WebSocket"); d == "是" {
			wsPath := "/" + infoDic.Val.P.GetStr("访问路径")
			wsConn := &websocket.Upgrader{
				CheckOrigin: func(r *http.Request) bool {
					return true // 允许所有跨域连接
				},
			}
			ServerRouterData.Ws = &ServeRouterWebSocket{
				Open: true,
				Addr: wsPath,
				Conn: wsConn,
			}
		}

		if d := infoDic.NewRun("QQBot"); d == "是" {
			appId := infoDic.Val.P.GetStr("APPID")
			secret := infoDic.Val.P.GetStr("密钥")
			dicPath := infoDic.Val.P.GetStr("词库")
			ServerRouterData.QQBot = &qqbotapi.RouterQQBot{
				// 缓存 50 秒，3 分钟内没有访问就删除
				LastMsg:  cache.New(50*time.Second, 3*time.Minute),
				Open:     true,
				Addr:     "/" + infoDic.Val.P.GetStr("访问路径"),
				Secret:   secret,
				FilePath: dicPath,
				API:      qqbotapi.NewQQBot(appId, secret),
			}
			BotDic := utils.NewFileQueue(dicPath)
			if !BotDic.FileExists() {
				BotDic.WriteFileByte(appfiles.GetFile("dic/QQBot.n"))
			}
		}

		if d := infoDic.NewRun("NapCatBot"); d == "是" {
			secret := infoDic.Val.P.GetStr("密钥")
			dicPath := infoDic.Val.P.GetStr("词库")
			ServerRouterData.NapCatBot = &napcatbotapi.RouterNapCatBot{
				Open:     true,
				APIAddr:  infoDic.Val.P.GetStr("发送消息接口"),
				Addr:     "/" + infoDic.Val.P.GetStr("访问路径"),
				Secret:   secret,
				FilePath: dicPath,
			}
			BotDic := utils.NewFileQueue(dicPath)
			if !BotDic.DirExists() {
				BotDic.SetPath(dicPath + "/dic/dic.n")
				BotDic.WriteFileByte(appfiles.GetFile("dic/NapCatBot.n"))
				// 群白名单
				BotDic.SetPath(dicPath + "/groups.txt")
				BotDic.WriteFileByte([]byte("all"))
				// 主人文件
				BotDic.SetPath(dicPath + "/admin.txt")
				BotDic.WriteFileByte([]byte(""))
			}

		}

		if d := infoDic.NewRun("YunHuBot"); d == "是" {
			secret := infoDic.Val.P.GetStr("密钥")
			dicPath := infoDic.Val.P.GetStr("词库")
			ServerRouterData.YunHuBot = &yunhubotapi.RouterYunHuBot{
				Open:     true,
				Addr:     "/" + infoDic.Val.P.GetStr("访问路径"),
				Secret:   secret,
				FilePath: dicPath,
			}
			BotDic := utils.NewFileQueue(dicPath)
			if !BotDic.FileExists() {
				BotDic.WriteFileByte(appfiles.GetFile("dic/YunHuBot.n"))
			}
		}

		srv := &http.Server{
			Addr:    infoServerPath,
			Handler: http.HandlerFunc(ServerRouterData.WebRun),
		}
		dto.GV.Set("_监听线程_", srv)

		if infoNgrokServerPath := infoDic.NewRun("Ngrok"); infoNgrokServerPath == "是" {
			authToken := infoDic.Val.P.GetStr("密钥")
			ngrokUrl := infoDic.Val.P.GetStr("访问链接")
			ngrokUrlHttp := config.HTTPEndpoint()
			// if authToken == "2960965389" {
			// 	authToken = "2abzrmBDIPyXUkuPdxCYmjTJJDa_2LBiWGFFwewXpxFd4KU3n"
			// 	ngrokUrl = "brave-sunfish-lightly.ngrok-free.app"
			// 	// authTokenOptions := []string{
			// 	// 	"2abzrmBDIPyXUkuPdxCYmjTJJDa_2LBiWGFFwewXpxFd4KU3n",
			// 	// 	"2fxaUPFlFox97uGLr2WRM5XSMwO_4X6fihRKMNzwwgM6pgAFr",
			// 	// }
			// 	// authToken = authTokenOptions[rand.Intn(len(authTokenOptions))]
			// }
			if ngrokUrl != "" {
				ngrokUrlHttp = config.HTTPEndpoint(
					config.WithDomain(ngrokUrl),
				)
			}
			if listener, err := ngrok.Listen(context.Background(),
				ngrokUrlHttp,
				ngrok.WithAuthtoken(authToken),
			); err == nil {
				go func() {
					if err := http.Serve(listener, srv.Handler); err != nil {
						utils.Error("Ngrok启动失败>" + err.Error())
						panic(err)
					}
				}()
				if res := infoDic.NewRun("Ngrok启动成功 " + listener.URL()); res != "" {
					fmt.Println(res)
				}
			} else {
				fmt.Println("Ngrok配置失败")
			}
		}
		// 使用 Goroutine 启动服务器
		go func() {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				utils.Error("启动失败>" + err.Error())
				panic(err)
			}
		}()
		if res := infoDic.NewRun("启动成功"); res != "" {
			fmt.Println(res)
		}
	}
}

func getClientIP(r *http.Request) string {
	// 尝试从X-Forwarded-For头获取IP
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// X-Forwarded-For可能包含多个IP，通常第一个是最原始的客户端IP
		parts := strings.Split(forwarded, ", ")
		if len(parts) > 0 && net.ParseIP(parts[0]) != nil {
			return parts[0]
		}
	}

	// 尝试从X-Real-IP头获取IP
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" && net.ParseIP(realIP) != nil {
		return realIP
	}

	// 如果X-Forwarded-For和X-Real-IP都不存在或无效，则回退到RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	return ip
}

// var websocket_connect *websocket.Conn

// 路由
func (s *ServeRouter) WebRun(w http.ResponseWriter, r *http.Request) {

	if s.QQBot != nil && s.QQBot.Open && r.URL.Path == s.QQBot.Addr {
		s.QQBotRun(w, r)
		return
	}

	if s.NapCatBot != nil && s.NapCatBot.Open && r.URL.Path == s.NapCatBot.Addr {
		s.NapCatBotRun(w, r)
		return
	}

	if s.YunHuBot != nil && s.YunHuBot.Open && r.URL.Path == s.YunHuBot.Addr {
		s.YunHuBotRun(w, r)
		return
	}

	// 运行结果
	var RunData string

	// 访问路径
	path := r.URL.Path

	// 访问类型
	getType := r.Method

	queryParams := r.URL.Query()

	ip := getClientIP(r)

	if s.Ws != nil && s.Ws.Open && path == s.Ws.Addr {
		// 检查是否为 WebSocket 升级请求
		conn, err := s.Ws.Conn.Upgrade(w, r, nil)
		if err == nil {
			responseData := RequestInfo{
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

			// websocket_connect = conn

			// 运行词库
			if wsFileData, err := utils.NewFileQueue("private/websocket/server.n").ReadFromFile(); err == nil {
				dic := NewDic("private/websocket/server.n", wsFileData)
				dic.Val.G.Set("访问数据", string(responseJSON))
				dic.SetFunc("断开连接", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
					conn.Close()
					return "", nil
				})
				dic.Val.G.Set("_WS连接_", conn)
				resData := dic.RunPrivate("连接成功")
				if resData != "" {
					if err := conn.WriteMessage(websocket.TextMessage, []byte(resData)); err != nil {
						fmt.Println("发送消息时出错:", err)
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

					wsfile := utils.NewFileQueue("private/websocket/server.n")
					wsfileData, err := wsfile.ReadFromFile()
					if err != nil {
						fmt.Println("读取文件时出错:", err)
						conn.Close() // 关闭连接
						break
					}
					d := NewDic("private/websocket/server.n", wsfileData)
					d.Val.G.Set("_WS连接_", conn)
					d.Val.G.Set("访问数据", string(responseJSON))
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
		}
		return
	}

	// 输出运行结果
	var send []byte
	responseData := &RequestInfo{
		Path:        path,
		Type:        getType,
		QueryParams: queryParams,
		Headers:     r.Header,
		IP:          ip,
		Host:        r.Host,
	}

	if getType == "POST" {
		if err := r.ParseMultipartForm(32 << 20); r.MultipartForm != nil && err == nil {
			var resFileData = make(map[string][]*PostFile)
			// fmt.Println(r.MultipartForm.File)
			for fieldName := range r.MultipartForm.File {
				file, h, err := r.FormFile(fieldName)
				if err == nil {
					defer file.Close()
					content, err := io.ReadAll(file)
					if err == nil {
						fileContent := base64.StdEncoding.EncodeToString(content)
						resFileData[fieldName] = append(resFileData[fieldName], &PostFile{
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
		routerFile.WriteFileByte(appfiles.GetFile("dic/router.n"))
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

	dic := NewDic("private/system/router.n", FileData)
	defer dic.Close()

	dic.SetGlobal_v(globalV).
		// 终止服务器
		SetFunc("终止服务器", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
			if !inputs.LenOk(0) {
				return "参数错误", nil
			}
			if srv, ok := dto.GV.Get("_监听线程_").(*http.Server); ok && srv != nil {
				srv.Close()
				return "true", nil
			}
			return "false", nil
		}).
		// 设置头部
		SetFunc("设置头部", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
			if !inputs.LenOk(2) {
				return "", errors.New("设置头部参数错误")
			}
			if r, ok := inputs.Get(1).(string); ok {
				if r2, ok := inputs.Get(2).(string); ok {
					w.Header().Set(r, r2)
					return "", nil
				}
				return "参数错误2", nil
			}
			return "参数错误1", nil
		}).
		// GET处理
		SetFunc("G", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
			if !inputs.LenOk(1, 2) {
				return "参数错误", nil
			}
			if r, ok := inputs.Get(1).(string); ok {
				if s := queryParams.Get(r); s != "" {
					return s, nil
				}
				if r, ok := inputs.Get(2).(string); ok {
					return r, nil
				}
			}
			return "", nil
		}).
		// POST处理
		SetFunc("P", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
			if !inputs.LenOk(1, 2) {
				return "参数错误", nil
			}
			key, ok := inputs.Get(1).(string)
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
			if inputs.LenOk(2) {
				if def, ok := inputs.Get(2).(string); ok {
					return def, nil
				}
			}
			return "", nil
		})

	RunData = dic.Run(path)

	SendHeade := globalV.Get("输出头部").(string)
	SendCOOKIE := globalV.Get("COOKIE").(string)

	var headerMap map[string]string
	var cookieMap []*SetCookie

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

	if num, err := strconv.Atoi(HeadInt); err == nil {
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
