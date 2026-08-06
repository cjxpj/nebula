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
				req.Header.Add(key, value)
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
				req.Header.Add(key, value)
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
		return nil, err
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
// 内部工具函数
// =======================

func getAccess(d *dto.DicInputs) *AccessRequest {
	if v := d.Inputs.Get(1); v != nil {
		if req, ok := v.(*AccessRequest); ok {
			return req
		}
	}
	return nil
}

// =======================
// 访问.新建
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

	return req, nil
}

// =======================
// 访问.切换GET / POST
// =======================

func changeRequestGet(d *dto.DicInputs) (any, error) {
	req := getAccess(d)
	if req == nil {
		return nil, errors.New("未新建请求")
	}
	req.Type = "get"
	return "", nil
}

func changeRequestPost(d *dto.DicInputs) (any, error) {
	req := getAccess(d)
	if req == nil {
		return nil, errors.New("未新建请求")
	}
	req.Type = "post"
	if d.Inputs.LenOk(2) {
		req.Body = d.Inputs.String(2)
	}
	return "", nil
}

// =======================
// 访问.禁用跳转
// =======================

func requestDisableRedirects(d *dto.DicInputs) (any, error) {
	req := getAccess(d)
	if req == nil {
		return nil, errors.New("未新建请求")
	}
	req.StopRedirect = true
	return "", nil
}

// =======================
// 访问.启用跳转
// =======================

func requestEnableRedirects(d *dto.DicInputs) (any, error) {
	req := getAccess(d)
	if req == nil {
		return nil, errors.New("未新建请求")
	}
	req.StopRedirect = false
	return "", nil
}

// =======================
// 访问.设置头部
// =======================

func requestSetHeader(d *dto.DicInputs) (any, error) {
	req := getAccess(d)
	if req == nil {
		return nil, errors.New("未新建请求")
	}

	var headers map[string]string
	if err := json.Unmarshal([]byte(d.Inputs.String(2)), &headers); err == nil {
		req.Headers = headers
	}
	return "", nil
}

// =======================
// 访问.设置超时
// =======================

func requestSetTimeout(d *dto.DicInputs) (any, error) {
	req := getAccess(d)
	if req == nil {
		return nil, errors.New("未新建请求")
	}
	req.Timeout = d.Inputs.Int(2)
	return "", nil
}

// =======================
// 访问.POST
// =======================

func requestPost(d *dto.DicInputs) (any, error) {
	req := getAccess(d)
	if req == nil {
		return nil, errors.New("未新建请求")
	}
	req.Type = "post"
	req.Body = d.Inputs.String(2)
	return "", nil
}

// =======================
// 访问.POST文件
// =======================

func requestPostFile(d *dto.DicInputs) (any, error) {
	req := getAccess(d)
	if req == nil {
		return nil, errors.New("未新建请求")
	}

	req.Type = "post"

	field := d.Inputs.String(2)
	if req.Files[field] == nil {
		req.Files[field] = make(map[string][]byte)
	}

	if d.Inputs.LenOk(3) {
		req.Files[field][field] = []byte(d.Inputs.String(3))
	} else {
		req.Files[field][d.Inputs.String(4)] = []byte(d.Inputs.String(5))
	}
	return "", nil
}

// =======================
// 访问.发送
// =======================

func requestSend(d *dto.DicInputs) (any, error) {
	req := getAccess(d)
	if req == nil {
		return nil, errors.New("未新建请求")
	}

	client := &http.Client{
		Timeout: time.Duration(req.Timeout) * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	if req.StopRedirect {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	var httpReq *http.Request
	var err error

	if req.Type == "get" {
		httpReq, err = http.NewRequest("GET", req.Host, nil)
	} else {
		httpReq, err = buildPostRequest(req)
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
// POST 构建
// =======================

func buildPostRequest(req *AccessRequest) (*http.Request, error) {
	if len(req.Files) == 0 {
		body := bytes.NewBufferString(req.Body)
		r, err := http.NewRequest("POST", req.Host, body)
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

	r, err := http.NewRequest("POST", req.Host, &buf)
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", writer.FormDataContentType())
	return r, nil
}

// =======================
// 访问.内容 / 全部内容
// =======================

func requestContent(d *dto.DicInputs) (any, error) {
	req := getAccess(d)
	if req == nil {
		return nil, errors.New("未新建请求")
	}
	if req.Res == nil {
		return "", nil
	}
	return string(req.Res.Data), nil
}

func requestAllContent(d *dto.DicInputs) (any, error) {
	req := getAccess(d)
	if req == nil {
		return nil, errors.New("未新建请求")
	}

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
