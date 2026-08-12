package utils

import (
	"bytes"
	"crypto/tls"
	"io"
	"net/http"
	"time"
)

var defaultHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
}

func Get(url string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "新建访问报错", err
	}

	req.Header.Set("User-Agent", "Nebula-Client/1.0")

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return "访问报错", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		Error(err.Error())
		return "获取错误", err
	}

	res := string(body)

	return res, nil
}

func Post(inputs []string) string {
	inputsLen := len(inputs)
	if inputsLen == 2 || inputsLen == 3 {
		url := inputs[0]
		// if !HttpHeaderReg.MatchString(url) {
		// 	url = "http://" + url
		// }

		bodys := inputs[1]
		reqBody := bytes.NewBufferString(bodys)
		req, err := http.NewRequest("POST", url, reqBody)
		if err != nil {
			return "新建访问报错"
		}

		if IsJSON(bodys) {
			req.Header.Set("Content-Type", "application/json")
		} else {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}

		req.Header.Set("User-Agent", "Nebula-Client/1.0")
		
		if inputsLen == 3 {
			var headers map[string]string
			if err := Json.Unmarshal([]byte(inputs[2]), &headers); err == nil {
				for key, value := range headers {
					req.Header.Set(key, value)
				}
			}
		}

		resp, err := defaultHTTPClient.Do(req)
		if err != nil {
			return "访问报错"
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			Error(err.Error())
			return "获取错误"
		}

		res := string(body)

		return res

	}
	return ""
}
