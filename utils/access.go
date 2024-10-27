package utils

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
)

func AccessGet(inputs []string) string {
	inputsLen := len(inputs)
	if inputsLen == 1 || inputsLen == 2 {
		url := inputs[0]
		if !regexp.MustCompile(`^https?://`).MatchString(url) {
			url = "http://" + url
		}

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			Error(err.Error(), "system")
			return "新建访问报错"
		}

		if inputsLen == 2 {
			var headers map[string]string
			if err := json.Unmarshal([]byte(inputs[1]), &headers); err == nil {
				for key, value := range headers {
					req.Header.Add(key, value)
					req.Header.Set(key, value)
				}
			}
		}

		req.Header.Set("User-Agent", "Nebula-Client/1.0")

		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
		resp, err := client.Do(req)
		if err != nil {
			return "访问报错"
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			Error(err.Error(), "system")
			return "获取错误"
		}

		res := string(body)

		return res

	}
	return ""
}

func AccessPost(inputs []string) string {
	inputsLen := len(inputs)
	if inputsLen == 2 || inputsLen == 3 {
		url := inputs[0]
		if !regexp.MustCompile(`^https?://`).MatchString(url) {
			url = "http://" + url
		}

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

		if inputsLen == 3 {
			var headers map[string]string
			if err := json.Unmarshal([]byte(inputs[2]), &headers); err == nil {
				for key, value := range headers {
					req.Header.Add(key, value)
					req.Header.Set(key, value)
				}
			}
		}

		req.Header.Set("User-Agent", "Nebula-Client/1.0")

		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
		resp, err := client.Do(req)
		if err != nil {
			return "访问报错"
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			Error(err.Error(), "system")
			return "获取错误"
		}

		res := string(body)

		return res

	}
	return ""
}
