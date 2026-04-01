package qqbot_msg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

func (b *QQBot) Send(path string, body any, respObj any) error {
	if err := b.ensureToken(); err != nil {
		return err
	}
	headers := GetQQBotAuthHeader(b.Key.AccessToken)
	return postJson(APIURL+path, body, headers, respObj)
}

func (b *QQBot) SendChannelImage(path string, imgData []byte, body any, respObj any) error {
	if err := b.ensureToken(); err != nil {
		return err
	}
	headers := GetQQBotAuthHeader(b.Key.AccessToken)
	return postImageWithJsonDataAsFormFields(APIURL+path, imgData, "NebulaImage", body, headers, respObj)
}

// postImageWithJsonMultipart 上传图片同时传 JSON 参数
func postImageWithJsonDataAsFormFields(
	url string,
	fileData []byte,
	fileName string,
	jsonData any,
	headers http.Header,
	respObj any,
) error {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 写入文件字段
	part, err := writer.CreateFormFile("file_image", fileName)
	if err != nil {
		return fmt.Errorf("创建文件字段失败: %w", err)
	}
	if _, err := part.Write(fileData); err != nil {
		return fmt.Errorf("写入文件内容失败: %w", err)
	}

	// 将 jsonData 转成 map[string]any
	fieldsMap := make(map[string]any)

	if jsonData != nil {
		// 先 Marshal 成 JSON，再 Unmarshal 到 map
		jsonBytes, err := json.Marshal(jsonData)
		if err != nil {
			return fmt.Errorf("编码 JSON 失败: %w", err)
		}
		err = json.Unmarshal(jsonBytes, &fieldsMap)
		if err != nil {
			return fmt.Errorf("解析 JSON 到 map 失败: %w", err)
		}
	}

	// 遍历 map，写入普通表单字段
	for key, val := range fieldsMap {
		strVal := fmt.Sprintf("%v", val)
		fmt.Println("key:", key, "val:", strVal)
		if err := writer.WriteField(key, strVal); err != nil {
			return fmt.Errorf("写入字段 %s 失败: %w", key, err)
		}
	}

	// 结束 multipart
	if err := writer.Close(); err != nil {
		return fmt.Errorf("关闭 multipart 写入器失败: %w", err)
	}

	// 创建请求
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	for k, vv := range headers {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}

	// 执行请求
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		content, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("状态码 %d，响应: %s", resp.StatusCode, string(content))
	}

	if respObj != nil {
		if err := json.NewDecoder(resp.Body).Decode(respObj); err != nil {
			return fmt.Errorf("解析响应失败: %w", err)
		}
	}

	return nil
}

// POST 请求，发送 JSON 并解析响应
func postJson(url string, body any, headers http.Header, respObj any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("编码 JSON 请求失败: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("构造请求失败: %w", err)
	}

	if headers != nil {
		req.Header = headers
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		// 内容字符串
		if c, err := io.ReadAll(resp.Body); err == nil {
			return fmt.Errorf("请求失败，状态码: %d, 内容: %s", resp.StatusCode, string(c))
		}
		return fmt.Errorf("请求失败，状态码: %d", resp.StatusCode)
	}

	// content, _ := io.ReadAll(resp.Body)
	// fmt.Println(string(content))
	if respObj != nil {
		if err := json.NewDecoder(resp.Body).Decode(respObj); err != nil {
			return fmt.Errorf("解析响应失败: %w", err)
		}
	}

	return nil
}
