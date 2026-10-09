package dic

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// setCORS 设置跨域响应头。若为 OPTIONS 预检请求则处理后返回 true。
func setCORS(w http.ResponseWriter, r *http.Request, origin string) bool {
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-OPUI-Key")
	if origin != "*" {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	if r.Method == http.MethodOptions {
		if reqMethod := r.Header.Get("Access-Control-Request-Method"); reqMethod != "" {
			w.Header().Set("Access-Control-Allow-Methods", reqMethod)
		}
		if reqHeaders := r.Header.Get("Access-Control-Request-Headers"); reqHeaders != "" {
			w.Header().Set("Access-Control-Allow-Headers", reqHeaders)
		}
		w.Header().Set("Access-Control-Max-Age", "86400")
		w.WriteHeader(http.StatusOK)
		return true
	}
	return false
}

// newHTTPRequestInfo 构建当前请求的访问信息（路径/类型/GET/POST/文件等）
func newHTTPRequestInfo(r *http.Request) *dto.HTTPRequestInfo {
	info := &dto.HTTPRequestInfo{
		Path:        r.URL.Path,
		Type:        r.Method,
		QueryParams: r.URL.Query(),
		Headers:     r.Header,
		IP:          utils.GetClientIP(r),
		Host:        r.Host,
	}

	if r.Method != "POST" {
		return info
	}

	if err := r.ParseMultipartForm(32 << 20); r.MultipartForm != nil && err == nil {
		resFileData := make(map[string][]*dto.PostFile)
		for fieldName := range r.MultipartForm.File {
			file, h, err := r.FormFile(fieldName)
			if err == nil {
				content, err := io.ReadAll(file)
				file.Close()
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
		info.PostFile = resFileData
	}

	var bodyMap map[string]any
	body, err := io.ReadAll(r.Body)
	if err == nil {
		defer r.Body.Close()
		if err := json.Unmarshal(body, &bodyMap); err == nil {
			info.Post = bodyMap
		} else {
			strBody := string(body)
			if strBody == "" {
				// 获取POST参数
				r.ParseForm()
				info.Post = r.PostForm
			} else {
				info.Post = strBody
			}
		}
	} else {
		// 获取POST参数
		r.ParseForm()
		info.Post = r.PostForm
	}
	return info
}

// setDicWebFuncs 为网页词库设置 设置头部/GET/POST 等内置函数
func setDicWebFuncs(dic *dic_dto.Dic, w http.ResponseWriter, queryParams url.Values, info *dto.HTTPRequestInfo) {
	dic.AddFuncs(dicWebFuncs(w, queryParams, info))
}

// dicWebFuncs 构造网页词库执行期注入的内置函数：设置头部 / GET / POST。
// 引擎自有 HTTP 服务器（setDicWebFuncs）与宿主下发的公开访问（WebHTTPRun）共用同一实现。
func dicWebFuncs(w http.ResponseWriter, queryParams url.Values, info *dto.HTTPRequestInfo) map[string]dto.DicFunc {
	return map[string]dto.DicFunc{
		// 设置头部
		"设置头部": {
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
			}},
		// GET处理
		"GET": {
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
			}},
		// POST处理
		"POST": {
			L: "1|2",
			Fn: func(d *dto.DicInputs) (any, error) {
				key, ok := d.Inputs.Get(1).(string)
				if !ok {
					return "参数必须是字符串", nil
				}

				// 处理不同类型的 Post
				switch post := info.Post.(type) {
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
			}},
	}
}

// capWriter 捕获词库执行期设置的响应头/状态码，替代真实 http.ResponseWriter 跨 DLL 传递。
// 正文不落缓冲——由词库返回值（runData）承载并作为 WebHTTPResult.Body 回传宿主。
type capWriter struct {
	header http.Header
	status int
}

func newCapWriter() *capWriter {
	return &capWriter{header: make(http.Header), status: http.StatusOK}
}

func (c *capWriter) Header() http.Header         { return c.header }
func (c *capWriter) Write(b []byte) (int, error) { return len(b), nil }
func (c *capWriter) WriteHeader(code int)        { c.status = code }

// WebHTTPRun 执行一次由宿主下发的网页词库公开访问。
//
// 职责边界：宿主负责用真实请求构建「访问数据」（Access）并负责响应回写；
// 引擎只做执行期编排——按访问数据重建 _请求数据_、挂载通用内置函数（设置头部/GET/POST）、
// 运行词库并收集输出头部/COOKIE/响应状态与正文。
func (m *dicImpl) WebHTTPRun(req *dic_dto.WebHTTPRequest) *dic_dto.WebHTTPResult {
	info := &dto.HTTPRequestInfo{}
	if strings.TrimSpace(req.Access) != "" {
		_ = json.Unmarshal([]byte(req.Access), info)
	}

	webRoot := req.WebRoot
	if webRoot == "" {
		webRoot = "."
	}

	cw := newCapWriter()
	globalV := dto.NewVal().
		Set("响应状态", "200").
		Set("输出头部", "{}").
		Set("COOKIE", "[]").
		Set("网站根目录", webRoot).
		SetRaw("_请求数据_", buildHTTPRequest(info, req.Body)).
		SetRaw("_响应数据_", cw).
		Set("访问数据", req.Access)

	funcs := dicWebFuncs(cw, info.QueryParams, info)

	var out string
	if req.Ext == ".wn" {
		wd := dic_dto.NewWebDic(req.Path, req.Content)
		wd.SetGlobal_v(globalV)
		wd.MyFunc = funcs
		out = m.WebDicRun(wd)
	} else {
		// .n 的触发词：默认 Main（多文件路由下一文件即一路由）；
		// 单文件路由时宿主传站内 URL 路径，使其按路由词库方式执行。
		trigger := req.Trigger
		if trigger == "" {
			trigger = "Main"
		}
		d := dic_dto.NewDic(req.Path, req.Content)
		d.SetGlobal_v(globalV).AddFuncs(funcs)
		out = m.DicRun(d, trigger)
	}

	writeDicWebOutput(cw, globalV, out)
	return &dic_dto.WebHTTPResult{Status: cw.status, Headers: cw.header, Body: out}
}

// buildHTTPRequest 依据宿主下发的访问数据与原始请求体重建 *http.Request，
// 供 _请求数据_（访问转发、router.n 等）使用。
func buildHTTPRequest(info *dto.HTTPRequestInfo, body string) *http.Request {
	method := info.Type
	if method == "" {
		method = http.MethodGet
	}
	target := info.Path
	if target == "" {
		target = "/"
	}
	if q := info.QueryParams.Encode(); q != "" {
		target += "?" + q
	}

	r, err := http.NewRequest(method, target, io.NopCloser(strings.NewReader(body)))
	if err != nil {
		r, _ = http.NewRequest(http.MethodGet, "/", nil)
	}
	if info.Host != "" {
		r.Host = info.Host
	}
	if info.IP != "" {
		r.RemoteAddr = info.IP
	}
	for k, vs := range info.Headers {
		for _, v := range vs {
			r.Header.Add(k, v)
		}
	}
	return r
}

// writeDicWebOutput 按网页词库规范输出响应（自定义头部/COOKIE/响应状态/内容）
func writeDicWebOutput(w http.ResponseWriter, globalV *dto.Val, runData string) {
	sendHeade, _ := globalV.Get("输出头部").(string)
	sendCOOKIE, _ := globalV.Get("COOKIE").(string)

	var headerMap map[string]string
	var cookieMap []*dto.SetCookie

	if sendHeade != "{}" {
		if err := json.Unmarshal([]byte(sendHeade), &headerMap); err == nil {
			for key, value := range headerMap {
				w.Header().Set(key, value)
			}
		}
	}

	if sendCOOKIE != "[]" {
		if err := json.Unmarshal([]byte(sendCOOKIE), &cookieMap); err == nil {
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

	headInt := "200"
	if rH, ok := globalV.Get("响应状态").(string); ok {
		headInt = rH
	}

	if num, e := strconv.Atoi(headInt); e == nil {
		w.WriteHeader(num)
	}

	send := []byte(runData)

	// 输出内容到响应
	w.Header().Set("Content-Length", strconv.Itoa(len(send)))
	if _, err := w.Write(send); err != nil {
		errMsg := fmt.Sprintf("服务器输出Error: %s", err)
		utils.Error(errMsg)
	}
}