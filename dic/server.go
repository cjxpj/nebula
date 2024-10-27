package dic

import (
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
	"github.com/gorilla/websocket"
)

type ServeRouter struct {
	path     string
	Ws       bool
	WsConfig *websocket.Upgrader
	Cache    bool
}

type RequestInfo struct {
	Path        string                 `json:"路径"`
	Type        string                 `json:"来源"`
	QueryParams url.Values             `json:"GET,omitempty"`
	Headers     http.Header            `json:"请求头"`
	IP          string                 `json:"IP"`
	Host        string                 `json:"Host"`
	Post        interface{}            `json:"POST,omitempty"`
	PostFile    map[string][]*PostFile `json:"POSTFile,omitempty"`
}

type PostFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Data string `json:"data"`
}

type SetCookie struct {
	Name     string `json:"命名"`
	Value    string `json:"数据"`
	Path     string `json:"路径"`
	HttpOnly bool   `json:"禁止JS"`
	MaxAge   int    `json:"存活"`
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

	// 运行结果
	var RunData string

	// 访问路径
	var path string = r.URL.Path

	// 访问类型
	getType := r.Method

	queryParams := r.URL.Query()

	ip := getClientIP(r)

	if s.Ws && path == "/ws" {
		// 检查是否为 WebSocket 升级请求
		conn, err := s.WsConfig.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

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
			utils.Error("访问数据异常", s.path)
			return
		}

		global_v := dto.NewVal().
			Set("访问数据", string(responseJSON)).
			Set("WebSocket", conn)

		// websocket_connect = conn

		// 运行词库
		if wsFileData, err := utils.NewFileQueue(s.path + "/private/websocket/connect.n").ReadFromFile(); err == nil {
			RunData = NewDic(wsFileData, "Main", s.path).
				SetGlobal_v(global_v).
				Run()
			utils.Log(RunData, s.path)
		}

		for {
			messageType, p, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					if wsFileData, err := utils.NewFileQueue(s.path + "/private/websocket/close.n").ReadFromFile(); err == nil {
						RunData = NewDic(wsFileData, "Main", s.path).
							SetGlobal_v(global_v).
							Run()
						utils.Log(RunData, s.path)
					}
				} else {
					if wsFileData, err := utils.NewFileQueue(s.path + "/private/websocket/close.n").ReadFromFile(); err == nil {
						RunData = NewDic(wsFileData, "Error", s.path).
							SetGlobal_v(global_v).
							Run()
						utils.Log(RunData, s.path)
					}
				}
				break
			}

			if wsFileData, err := utils.NewFileQueue(s.path + "/private/websocket/msg.n").ReadFromFile(); err == nil {
				v := dto.NewVal().
					Set("消息", string(p))
				RunData = NewDic(wsFileData, "Main", s.path).
					SetGlobal_v(global_v).
					Set_v(v).
					Run()
				if RunData != "" {
					err = conn.WriteMessage(messageType, []byte(RunData))
					if err != nil {
						break
					}
				}
			}
		}

	}

	if s.Ws && path == "/ws" {
		return
	}

	// 输出运行结果
	var send []byte
	// 输出类型
	var SendType string

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

		var body_map map[string]interface{}
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
		utils.Error("访问数据异常", s.path)
		return
	}
	responseJSON := string(resS)

	var FileData string

	if !s.Cache {
		routerFile := utils.NewFileQueue(s.path + "/private/system/router.n")
		if !routerFile.FileExists() {
			routerFile.WriteFileByte(appfiles.DicRouter)
		}

		FileData, err = routerFile.ReadFromFile()
		if err != nil {
			utils.Error("读取路由词库出错", s.path)
			return
		}
	} else {
		if _, ok := dto.GV.Get("cache_路由").(*dto.BuildValue); !ok {
			routerFile := utils.NewFileQueue(s.path + "/private/system/router.n")
			if !routerFile.FileExists() {
				routerFile.WriteFileByte(appfiles.DicRouter)
			}

			FileData, err = routerFile.ReadFromFile()
			if err != nil {
				utils.Error("读取路由词库出错", s.path)
				return
			}
		}
	}

	// 运行词库
	global_v := dto.NewVal().
		Set("响应状态", "200").
		Set("输出类型", "text/plain; charset=UTF-8").
		Set("输出头部", "{}").
		Set("COOKIE", "[]").
		Set("访问数据", string(responseJSON))

	dic := NewDic(FileData, path, s.path).
		SetGlobal_v(global_v)
	if s.Cache {
		dic.Cache = true
		dic.CacheName = "路由"
	}
	RunData = dic.Run()

	SendType = global_v.Get("输出类型").(string)

	// 设置头部并统一输出UTF-8编码
	w.Header().Set("Content-Type", SendType)

	SendHeade := global_v.Get("输出头部").(string)
	SendCOOKIE := global_v.Get("COOKIE").(string)

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

	HeadInt := global_v.Get("响应状态").(string)

	if num, err := strconv.Atoi(HeadInt); err == nil {
		w.WriteHeader(num)
	}

	send = []byte(RunData)

	// 输出内容到响应
	_, err = w.Write(send)
	if err != nil {
		errMsg := fmt.Sprintf("服务器输出Error: %s", err)
		utils.Error(errMsg, s.path)
	}
}
