package dic

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

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
	// 设置头部
	dic.SetFunc("设置头部", dto.DicFunc{
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
			}})
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