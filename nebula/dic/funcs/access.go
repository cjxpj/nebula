package funcs

import (
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
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
		if err := file.DownloadWithDynamicThreads(d.Inputs.String(1), d.Inputs.Int(3), printOpen); err != nil {
			return "false", err
		}
		return "true", nil

	}
	return "", errors.New("参数数量错误")
}

func (f *DicFunc) AccessGet() (string, error) {
	if f.Len == 1 || f.Len == 2 {
		url := f.Inputs.String(1)
		if !regexp.MustCompile(`^https?://`).MatchString(url) {
			url = "http://" + url
		}

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			utils.Error(err.Error())
			return "新建访问报错", nil
		}

		req.Header.Set("User-Agent", "Nebula-Client/1.0")

		if f.Len == 2 {
			var headers map[string]string
			if err := json.Unmarshal([]byte(f.Inputs.String(2)), &headers); err == nil {
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
			return "访问报错", nil
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			utils.Error(err.Error())
			return "获取错误", nil
		}

		res := string(body)

		return res, nil

	}
	return "", errors.New("参数数量错误")
}

func (f *DicFunc) AccessPost() (string, error) {
	if f.Len == 2 || f.Len == 3 {
		url := f.Inputs.String(1)
		if !regexp.MustCompile(`^https?://`).MatchString(url) {
			url = "http://" + url
		}

		bodys := f.Inputs.String(2)
		reqBody := bytes.NewBufferString(bodys)
		req, err := http.NewRequest("POST", url, reqBody)
		if err != nil {
			return "新建访问报错", nil
		}

		if utils.IsJSON(bodys) {
			req.Header.Set("Content-Type", "application/json")
		} else {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}

		req.Header.Set("User-Agent", "Nebula-Client/1.0")

		if f.Len == 3 {
			var headers map[string]string
			if err := json.Unmarshal([]byte(f.Inputs.String(3)), &headers); err == nil {
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
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
		resp, err := client.Do(req)
		if err != nil {
			return "访问报错", nil
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			utils.Error(err.Error())
			return "获取错误", nil
		}

		res := string(body)

		return res, nil

	}
	return "", errors.New("参数数量错误")
}

func (f *DicFunc) AccessSet(sysVal *dto.LocalDicValue) (string, error) {
	if f.Len != 1 {
		return "", errors.New("参数数量错误")
	}
	setUrl := f.Inputs.String(1)
	regex := regexp.MustCompile(`^https?://`)
	if !regex.MatchString(setUrl) {
		setUrl = "http://" + setUrl
	}
	setobj := map[string]interface{}{
		"type":      "get",
		"host":      setUrl,
		"headers":   map[string]string{},
		"times_out": 0,
		"file":      make(map[string]map[string][]byte),
		"body":      "",
		"res":       map[string]interface{}{},
	}
	sysVal.Access = setobj
	return "", nil
}

func (f *DicFunc) AccessSetTimes(sysVal *dto.LocalDicValue) (string, error) {
	if f.Len != 1 {
		return "", errors.New("参数数量错误")
	}
	obj, ok := sysVal.Access.(map[string]interface{})
	if !ok {
		obj = map[string]interface{}{}
	}
	// if err := json.Unmarshal([]byte(f.Inputs[1]), &headers); err == nil {
	// 转int
	if times, err := strconv.Atoi(f.Inputs.String(1)); err == nil {
		obj["times_out"] = times
	}
	sysVal.Access = obj
	return "", nil
}

func (f *DicFunc) AccessSetHeader(sysVal *dto.LocalDicValue) (string, error) {
	if f.Len != 1 {
		return "", errors.New("参数数量错误")
	}
	obj, ok := sysVal.Access.(map[string]interface{})
	if !ok {
		obj = map[string]interface{}{}
	}
	// 尝试解析 JSON 字符串
	var headers map[string]string
	if err := json.Unmarshal([]byte(f.Inputs.String(1)), &headers); err == nil {
		obj["headers"] = headers
	} else {
		obj["headers"] = headers
	}
	sysVal.Access = obj
	return "", nil
}

func (f *DicFunc) AccessSetGet(sysVal *dto.LocalDicValue) (string, error) {
	if f.Len != 1 {
		return "", errors.New("参数数量错误")
	}
	obj, ok := sysVal.Access.(map[string]interface{})
	if !ok {
		obj = map[string]interface{}{}
	}
	obj["type"] = "get"
	sysVal.Access = obj
	return "", nil
}

func (f *DicFunc) AccessSetPost(sysVal *dto.LocalDicValue) (string, error) {
	if !(f.Len == 0 || f.Len == 1) {
		return "", errors.New("参数数量错误")
	}
	obj, ok := sysVal.Access.(map[string]interface{})
	if !ok {
		obj = map[string]interface{}{}
	}
	obj["type"] = "post"
	if f.Len == 1 {
		obj["body"] = f.Inputs.String(1) // 请求主体
	}
	sysVal.Access = obj
	return "", nil
}

func (f *DicFunc) AccessSetPostFile(sysVal *dto.LocalDicValue) (string, error) {
	if !(f.Len == 0 || f.Len == 2 || f.Len == 3) {
		return "", errors.New("参数数量错误")
	}
	obj, ok := sysVal.Access.(map[string]interface{})
	if !ok {
		obj = map[string]interface{}{}
	}
	obj["type"] = "post"
	if f.Len == 2 {
		if files, ok := obj["file"].(map[string]map[string][]byte); ok {
			if files[f.Inputs.String(1)] == nil {
				files[f.Inputs.String(1)] = make(map[string][]byte)
			}
			files[f.Inputs.String(1)][f.Inputs.String(1)] = []byte(f.Inputs.String(2))
		}
	}
	if f.Len == 3 {
		if files, ok := obj["file"].(map[string]map[string][]byte); ok {
			if files[f.Inputs.String(1)] == nil {
				files[f.Inputs.String(1)] = make(map[string][]byte)
			}
			files[f.Inputs.String(1)][f.Inputs.String(2)] = []byte(f.Inputs.String(3))
		}
	}
	sysVal.Access = obj
	return "", nil
}

func (f *DicFunc) AccessSend(sysVal *dto.LocalDicValue) (string, error) {
	if f.Len != 0 {
		return "", errors.New("参数数量错误")
	}
	obj, ok := sysVal.Access.(map[string]interface{})
	if !ok {
		obj = map[string]interface{}{}
	}
	switch obj["type"] {
	case "get":
		req, err := http.NewRequest("GET", obj["host"].(string), nil)
		if err != nil {
			utils.Error(err.Error())
			return "新建通信报错", nil
		}

		req.Header.Set("User-Agent", "Nebula-Client/1.0")

		for key, value := range obj["headers"].(map[string]string) {
			req.Header.Add(key, value)
			req.Header.Set(key, value)
		}

		times_out := obj["times_out"].(int)

		client := &http.Client{
			// 超时限制
			Timeout: time.Duration(times_out) * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
		resp, err := client.Do(req)
		if err != nil {
			return "发送通信报错", nil
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			utils.Error(err.Error())
			return "通信获取报错", nil
		}

		obj["res"] = map[string]interface{}{
			"statusText": resp.Status,
			"status":     resp.StatusCode,
			"headers":    resp.Header,
			"data":       body,
		}
		sysVal.Access = obj
		return "", nil
	case "post":
		var ok bool
		sendurl, ok := obj["host"].(string)
		if !ok {
			return "未记录", nil
		}
		objStr, ok := obj["body"].(string)
		if !ok {
			return "不存在POST数据", nil
		}
		// 获取文件列表并遍历
		if fileList, ok := obj["file"].(map[string]map[string][]byte); ok && len(fileList) > 0 {
			var reqBody bytes.Buffer
			writer := multipart.NewWriter(&reqBody)
			for fieldName, fileLists := range fileList {
				for fileName, fileContent := range fileLists {
					// 创建一个 form 文件部分
					part, err := writer.CreateFormFile(fieldName, fileName)
					if err != nil {
						utils.Error(err.Error())
						return "新建文件报错", nil
					}

					// 写入文件内容到 part
					_, err = part.Write(fileContent)
					if err != nil {
						utils.Error(err.Error())
						return "写入文件报错", nil
					}
				}
			}

			// 添加表单数据部分
			if objStr != "" {
				// 尝试解析为 JSON
				var formData map[string]string
				objerr := json.Unmarshal([]byte(objStr), &formData)
				if objerr == nil {
					// 解析为 JSON 成功，写入表单字段
					if err := writeFields(formData, writer); err != nil {
						return "添加报错", nil
					}
				} else {
					// 解析 JSON 失败，尝试解析为表单
					parsedFormData, err := url.ParseQuery(objStr)
					if err != nil {
						return "解析POST表单报错", nil
					}
					// 将 url.Values 转换为 map[string]string，取每个键的第一个值
					formDataMap := make(map[string]string)
					for key, values := range parsedFormData {
						if len(values) > 0 {
							formDataMap[key] = values[0]
						}
					}
					// 写入表单字段
					if err := writeFields(formDataMap, writer); err != nil {
						return "添加报错", nil
					}
				}
			}

			// 关闭 writer 来完成请求体
			writer.Close()

			// 创建一个 HTTP POST 请求
			req, err := http.NewRequest("POST", sendurl, &reqBody)
			if err != nil {
				utils.Error(err.Error())
				return "新建通信报错", nil
			}

			req.Header.Set("User-Agent", "juice-requests/1.0")

			// 设置请求头
			req.Header.Set("Content-Type", writer.FormDataContentType())

			for key, value := range obj["headers"].(map[string]string) {
				req.Header.Add(key, value)
				req.Header.Set(key, value)
			}

			times_out := obj["times_out"].(int)

			// 发送请求
			client := &http.Client{
				// 超时限制
				Timeout: time.Duration(times_out) * time.Second,
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				},
			}
			resp, err := client.Do(req)
			if err != nil {
				return "发送通信报错", nil
			}
			defer resp.Body.Close()

			// 使用 io.ReadAll 读取整个响应体
			respBody, err := io.ReadAll(resp.Body)
			if err != nil {
				utils.Error(err.Error())
				return "通信获取报错", nil
			}

			obj["res"] = map[string]interface{}{
				"statusText": resp.Status,
				"status":     resp.StatusCode,
				"headers":    resp.Header,
				"data":       respBody,
			}
			sysVal.Access = obj
			return "", nil
		}

		reqBody := bytes.NewBufferString(objStr) // 使用请求的主体

		req, err := http.NewRequest("POST", sendurl, reqBody)
		if err != nil {
			utils.Error(err.Error())
			return "新建通信报错", nil
		}

		req.Header.Set("User-Agent", "juice-requests/1.0")

		if utils.IsJSON(objStr) {
			req.Header.Set("Content-Type", "application/json")
		} else {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}

		for key, value := range obj["headers"].(map[string]string) {
			req.Header.Add(key, value)
			req.Header.Set(key, value)
		}

		client := &http.Client{
			// 超时限制30秒
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
		resp, err := client.Do(req)
		if err != nil {
			return "发送通信报错", nil
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			utils.Error(err.Error())
			return "通信获取报错", nil
		}
		obj["res"] = map[string]interface{}{
			"statusText": resp.Status,
			"status":     resp.StatusCode,
			"headers":    resp.Header,
			"data":       respBody,
		}
		sysVal.Access = obj
		return "", nil
	}
	return "", errors.New("参数数量错误")
}

// 辅助函数：写入表单字段
func writeFields(data map[string]string, writer *multipart.Writer) error {
	for key, value := range data {
		if err := writer.WriteField(key, value); err != nil {
			return err
		}
	}
	return nil
}

func (f *DicFunc) AccessGetSendAll(sysVal *dto.LocalDicValue) (string, error) {
	if f.Len != 0 {
		return "", errors.New("参数数量错误")
	}

	// 确保 sysVal.Access 是一个 map[string]interface{} 类型
	obj, ok := sysVal.Access.(map[string]interface{})
	if !ok {
		return "未定义", nil
	}

	var result string
	var objData []uint8
	objDatas := make(map[string]map[string][]byte)

	// 将拷贝中的 res 字段设置为 nil（即 null）
	objRes, ok := obj["res"].(map[string]interface{})
	if ok {
		if objDatA, Ok := objRes["data"].([]uint8); Ok {
			objData = objDatA
		}
		objRes["data"] = "已屏蔽"
	}

	objFile, oks := obj["file"].(map[string]map[string][]byte)
	if oks {
		for k, v := range objFile {
			for kk, vv := range v {
				if objDatas[k] == nil {
					objDatas[k] = make(map[string][]byte)
				}
				objDatas[k][kk] = vv
				objFile[k][kk] = nil
			}
		}
	}

	// 将拷贝的 objs 转换为 JSON 字符串并返回
	resultss, err := json.Marshal(obj)
	results := string(resultss)

	// results, err := json.Marshal(obj)
	if err == nil {
		result = string(results)
	}
	if ok {
		objRes["data"] = objData
	}

	if oks {
		obj["file"] = objDatas
	}

	if err != nil {
		return "获取错误", nil
	}

	return result, nil
}

func (f *DicFunc) AccessGetSend(sysVal *dto.LocalDicValue) (string, error) {
	if f.Len != 0 {
		return "", errors.New("参数数量错误")
	}
	obj, ok := sysVal.Access.(map[string]interface{})
	if !ok {
		obj = map[string]interface{}{}
	}
	res, ok := obj["res"].(map[string]interface{})
	if !ok {
		return "", nil
	}
	if result, ok := res["data"].([]byte); ok {
		return string(result), nil
	}
	return "不存在结果", nil
}
