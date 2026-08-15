package funcs

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// 下载文件
func downloadFile(d *dto.DicInputs) (any, error) {
	if d.Inputs.LenOk(2) {
		file := utils.NewFileQueue(d.Inputs.String(2))
		if file.Download(d.Inputs.String(1)) {
			return "true", nil
		}
		return "false", nil
	}
	if d.Inputs.LenOk(3, 4) {
		file := utils.NewFileQueue(d.Inputs.String(2))
		printOpen := false
		if d.Inputs.String(4) == "true" {
			printOpen = true
		}
		if err := file.DownloadWithDynamicThreads(d.Inputs.String(1), d.Inputs.Int(3), printOpen, nil); err != nil {
			return "false", err
		}
		return "true", nil

	}
	return "", errors.New("参数数量错误")
}

func accessGet(d *dto.DicInputs) (any, error) {
	url := d.Inputs.String(1)
	if !regexp.MustCompile(`^https?://`).MatchString(url) {
		url = "http://" + url
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("新建GET请求失败: %w", err)
	}

	req.Header.Set("User-Agent", "Nebula-Client/1.0")

	if d.Inputs.LenOk(2) {
		var headers map[string]string
		if err := json.Unmarshal([]byte(d.Inputs.String(2)), &headers); err == nil {
			for key, value := range headers {
				req.Header.Set(key, value)
			}
		}
	}

	client := &http.Client{
		// 超时限制
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	resp, err := client.Do(req)
	client.CloseIdleConnections()
	if err != nil {
		return "", fmt.Errorf("GET访问失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取GET响应失败: %w", err)
	}

	res := string(body)

	return res, nil
}

func accessPost(d *dto.DicInputs) (any, error) {
	url := d.Inputs.String(1)
	if !regexp.MustCompile(`^https?://`).MatchString(url) {
		url = "http://" + url
	}

	bodys := d.Inputs.String(2)
	reqBody := bytes.NewBufferString(bodys)
	req, err := http.NewRequest("POST", url, reqBody)
	if err != nil {
		return "", fmt.Errorf("新建POST请求失败: %w", err)
	}

	if utils.IsJSON(bodys) {
		req.Header.Set("Content-Type", "application/json")
	} else {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	req.Header.Set("User-Agent", "Nebula-Client/1.0")

	if d.Inputs.LenOk(3) {
		var headers map[string]string
		if err := json.Unmarshal([]byte(d.Inputs.String(3)), &headers); err == nil {
			for key, value := range headers {
				req.Header.Set(key, value)
			}
		}
	}

	client := &http.Client{
		// 超时限制
		Timeout:   15 * time.Second,
		Transport: &http.Transport{},
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("POST访问失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取POST响应失败: %w", err)
	}

	res := string(body)

	return res, nil
}

// =======================
// 访问转发
// 将当前请求的 GET/POST 数据及头部原样转发到指定链接
// =======================

func requestForward(d *dto.DicInputs) (any, error) {
	targetURL := d.Inputs.String(1)
	if !regexp.MustCompile(`^https?://`).MatchString(targetURL) {
		targetURL = "http://" + targetURL
	}

	// 从 dic 线程变量中获取原始请求
	reqVal := d.V.G.Get("_请求数据_")
	if reqVal == nil {
		return "", errors.New("无法获取原始请求")
	}
	origReq, ok := reqVal.(*http.Request)
	if !ok {
		return "", errors.New("原始请求类型错误")
	}

	// 读取原始请求体
	var reqBody []byte
	if origReq.Body != nil {
		var err error
		reqBody, err = io.ReadAll(origReq.Body)
		if err != nil {
			return "", err
		}
		// 重建原始请求体，以便后续处理
		origReq.Body = io.NopCloser(bytes.NewBuffer(reqBody))
	}

	// 创建转发请求
	forwardReq, err := http.NewRequest(origReq.Method, targetURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("新建转发请求失败: %w", err)
	}

	// 复制原始请求头部（排除部分不需要转发的头部）
	skipHeaders := map[string]bool{
		"Host":              true,
		"Content-Length":    true,
		"Transfer-Encoding": true,
		"Connection":        true,
		"Keep-Alive":        true,
		"Proxy-Connection":  true,
		"Upgrade":           true,
	}
	for key, values := range origReq.Header {
		if skipHeaders[key] {
			continue
		}
		for _, value := range values {
			forwardReq.Header.Add(key, value)
		}
	}

	// 复制 URL 查询参数
	q := forwardReq.URL.Query()
	for key, values := range origReq.URL.Query() {
		for _, value := range values {
			q.Add(key, value)
		}
	}
	forwardReq.URL.RawQuery = q.Encode()

	// 发送转发请求
	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Do(forwardReq)
	if err != nil {
		return "", fmt.Errorf("转发请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取转发响应失败: %w", err)
	}

	return string(body), nil
}

// =======================
// 结构体定义
// =======================

type AccessRequest struct {
	Type         string
	Host         string
	Headers      map[string]string
	Timeout      int
	Files        map[string]map[string][]byte
	Body         string
	Res          *AccessResponse
	StopRedirect bool
}

type AccessResponse struct {
	StatusText string
	Status     int
	Headers    http.Header
	Data       []byte
}

// =======================
// 新建访问（返回面对像对象）
// =======================

func newRequest(d *dto.DicInputs) (any, error) {
	setUrl := d.Inputs.String(1)
	if !regexp.MustCompile(`^https?://`).MatchString(setUrl) {
		setUrl = "http://" + setUrl
	}

	req := &AccessRequest{
		Type:         "get",
		Host:         setUrl,
		Headers:      map[string]string{},
		Timeout:      0,
		Files:        make(map[string]map[string][]byte),
		Body:         "",
		Res:          nil,
		StopRedirect: false,
	}

	instance := &dto.DicClass{
		LocalValue: dto.NewVal().
			Set("_访问_", req).
			Set("地址", setUrl),
	}
	instance.Fn = map[string]dto.DicFunc{
		"切换GET":     {L: "0", Fn: func(d *dto.DicInputs) (any, error) { return setAccessMethod(req, d, "get") }},
		"切换POST":    {L: "0|1", Fn: func(d *dto.DicInputs) (any, error) { return setAccessMethod(req, d, "post") }},
		"切换PUT":     {L: "0|1", Fn: func(d *dto.DicInputs) (any, error) { return setAccessMethod(req, d, "put") }},
		"切换DELETE":  {L: "0|1", Fn: func(d *dto.DicInputs) (any, error) { return setAccessMethod(req, d, "delete") }},
		"切换PATCH":   {L: "0|1", Fn: func(d *dto.DicInputs) (any, error) { return setAccessMethod(req, d, "patch") }},
		"切换HEAD":    {L: "0", Fn: func(d *dto.DicInputs) (any, error) { return setAccessMethod(req, d, "head") }},
		"切换OPTIONS": {L: "0", Fn: func(d *dto.DicInputs) (any, error) { return setAccessMethod(req, d, "options") }},
		"禁用跳转": {L: "0", Fn: func(d *dto.DicInputs) (any, error) {
			req.StopRedirect = true
			return "", nil
		}},
		"启用跳转": {L: "0", Fn: func(d *dto.DicInputs) (any, error) {
			req.StopRedirect = false
			return "", nil
		}},
		"设置头部": {L: "1", Fn: func(d *dto.DicInputs) (any, error) {
			var headers map[string]string
			if err := json.Unmarshal([]byte(d.Inputs.String(1)), &headers); err == nil {
				req.Headers = headers
			}
			return "", nil
		}},
		"设置超时": {L: "1", Fn: func(d *dto.DicInputs) (any, error) {
			req.Timeout = d.Inputs.Int(1)
			return "", nil
		}},
		"POST": {L: "1", Fn: func(d *dto.DicInputs) (any, error) {
			req.Type = "post"
			req.Body = d.Inputs.String(1)
			return "", nil
		}},
		"POST文件": {L: "2|3", Fn: func(d *dto.DicInputs) (any, error) {
			req.Type = "post"
			field := d.Inputs.String(1)
			if req.Files[field] == nil {
				req.Files[field] = make(map[string][]byte)
			}
			if d.Inputs.LenOk(2) {
				req.Files[field][field] = []byte(d.Inputs.String(2))
			} else {
				req.Files[field][d.Inputs.String(2)] = []byte(d.Inputs.String(3))
			}
			return "", nil
		}},
		"发送": {L: "0", Fn: func(d *dto.DicInputs) (any, error) {
			return sendRequest(req)
		}},
		"全部内容": {L: "0", Fn: func(d *dto.DicInputs) (any, error) {
			return accessAllContent(req)
		}},
		"内容": {L: "0", Fn: func(d *dto.DicInputs) (any, error) {
			return accessContent(req)
		}},
	}
	return instance, nil
}

// setAccessMethod 切换请求方式，可附带请求体。
func setAccessMethod(req *AccessRequest, d *dto.DicInputs, method string) (any, error) {
	req.Type = method
	if d.Inputs.LenOk(1) {
		req.Body = d.Inputs.String(1)
	}
	return "", nil
}

// sendRequest 发送请求，结果写入 req.Res，不返回数据。
func sendRequest(req *AccessRequest) (any, error) {
	client := &http.Client{
		Timeout: time.Duration(req.Timeout) * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	if client.Timeout <= 0 {
		client.Timeout = 15 * time.Second
	}

	if req.StopRedirect {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	var httpReq *http.Request
	var err error

	method := strings.ToUpper(req.Type)
	if method == "" {
		method = "GET"
	}

	// GET / HEAD / OPTIONS 不携带请求体，其余方式按 body 构建
	if method == "GET" || method == "HEAD" || method == "OPTIONS" {
		httpReq, err = http.NewRequest(method, req.Host, nil)
	} else {
		httpReq, err = buildMethodRequest(method, req)
	}

	if err != nil {
		return "", errors.New("新建请求失败")
	}

	httpReq.Header.Set("User-Agent", "Nebula-Client/1.0")
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("请求发送失败: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	req.Res = &AccessResponse{
		StatusText: resp.Status,
		Status:     resp.StatusCode,
		Headers:    resp.Header,
		Data:       data,
	}

	return "", nil
}

// =======================
// 请求体构建（POST / PUT / DELETE / PATCH 等）
// =======================

func buildMethodRequest(method string, req *AccessRequest) (*http.Request, error) {
	if len(req.Files) == 0 {
		body := bytes.NewBufferString(req.Body)
		r, err := http.NewRequest(method, req.Host, body)
		if err != nil {
			return nil, err
		}
		if utils.IsJSON(req.Body) {
			r.Header.Set("Content-Type", "application/json")
		} else {
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		return r, nil
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	for field, files := range req.Files {
		for name, data := range files {
			part, err := writer.CreateFormFile(field, name)
			if err != nil {
				return nil, err
			}
			part.Write(data)
		}
	}

	if req.Body != "" {
		var m map[string]string
		if json.Unmarshal([]byte(req.Body), &m) == nil {
			for k, v := range m {
				writer.WriteField(k, v)
			}
		} else {
			vals, _ := url.ParseQuery(req.Body)
			for k, v := range vals {
				if len(v) > 0 {
					writer.WriteField(k, v[0])
				}
			}
		}
	}

	writer.Close()

	r, err := http.NewRequest(method, req.Host, &buf)
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", writer.FormDataContentType())
	return r, nil
}

// =======================
// 访问.内容 / 全部内容
// =======================

func accessContent(req *AccessRequest) (any, error) {
	if req.Res == nil {
		return "", nil
	}
	return string(req.Res.Data), nil
}

func accessAllContent(req *AccessRequest) (any, error) {
	copyReq := *req
	if copyReq.Res != nil {
		copyReq.Res = &AccessResponse{
			StatusText: req.Res.StatusText,
			Status:     req.Res.Status,
			Headers:    req.Res.Headers,
			Data:       []byte("已屏蔽"),
		}
	}

	b, _ := json.Marshal(copyReq)
	return string(b), nil
}
