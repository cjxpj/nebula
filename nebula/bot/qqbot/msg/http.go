package qqbot_msg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/cjxpj/nebula/debugLog"
)

func (b *QQBot) Send(path string, body any, respObj any) error {
	if err := b.EnsureToken(); err != nil {
		return err
	}
	headers := GetQQBotAuthHeader(b.Key.AccessToken)

	if b.Debug {
		bodyJson, _ := json.Marshal(body)
		debugLog.Infof("[QQBot 发送] %s%s\n", APIURL, path)
		debugLog.Infof("[QQBot 请求] %s\n", string(bodyJson))
	}

	err := postJson(APIURL+path, body, headers, respObj)

	if b.Debug {
		if err != nil {
			debugLog.Infof("[QQBot 错误] %v\n", err)
		} else if respObj != nil {
			respJson, _ := json.Marshal(respObj)
			debugLog.Infof("[QQBot 返回] %s\n", string(respJson))
		}
	}
	return err
}

func (b *QQBot) Get(path string, respObj any) error {
	if err := b.EnsureToken(); err != nil {
		return err
	}
	headers := GetQQBotAuthHeader(b.Key.AccessToken)

	if b.Debug {
		debugLog.Infof("[QQBot GET] %s%s\n", APIURL, path)
	}

	err := getJson(APIURL+path, headers, respObj)

	if b.Debug {
		if err != nil {
			debugLog.Infof("[QQBot 错误] %v\n", err)
		} else if respObj != nil {
			respJson, _ := json.Marshal(respObj)
			debugLog.Infof("[QQBot GET返回] %s\n", string(respJson))
		}
	}
	return err
}

func (b *QQBot) Patch(path string, body any, respObj any) error {
	if err := b.EnsureToken(); err != nil {
		return err
	}
	headers := GetQQBotAuthHeader(b.Key.AccessToken)

	if b.Debug {
		bodyJson, _ := json.Marshal(body)
		debugLog.Infof("[QQBot PATCH] %s%s\n", APIURL, path)
		debugLog.Infof("[QQBot 请求] %s\n", string(bodyJson))
	}

	err := patchJson(APIURL+path, body, headers, respObj)

	if b.Debug {
		if err != nil {
			debugLog.Infof("[QQBot 错误] %v\n", err)
		} else if respObj != nil {
			respJson, _ := json.Marshal(respObj)
			debugLog.Infof("[QQBot PATCH返回] %s\n", string(respJson))
		}
	}
	return err
}

func (b *QQBot) Put(path string, body any, respObj any) error {
	if err := b.EnsureToken(); err != nil {
		return err
	}
	headers := GetQQBotAuthHeader(b.Key.AccessToken)

	if b.Debug {
		bodyJson, _ := json.Marshal(body)
		debugLog.Infof("[QQBot PUT] %s%s\n", APIURL, path)
		debugLog.Infof("[QQBot 请求] %s\n", string(bodyJson))
	}

	err := putJson(APIURL+path, body, headers, respObj)

	if b.Debug {
		if err != nil {
			debugLog.Infof("[QQBot 错误] %v\n", err)
		} else if respObj != nil {
			respJson, _ := json.Marshal(respObj)
			debugLog.Infof("[QQBot PUT返回] %s\n", string(respJson))
		}
	}
	return err
}

func (b *QQBot) SendChannelImage(path string, imgData []byte, body any, respObj any) error {
	if err := b.EnsureToken(); err != nil {
		return err
	}
	headers := GetQQBotAuthHeader(b.Key.AccessToken)

	if b.Debug {
		bodyJson, _ := json.Marshal(body)
		debugLog.Infof("[QQBot 发送图片] %s%s\n", APIURL, path)
		debugLog.Infof("[QQBot 请求] %s | 图片大小: %d bytes\n", string(bodyJson), len(imgData))
	}

	err := postImageWithJsonDataAsFormFields(APIURL+path, imgData, "NebulaImage", body, headers, respObj)

	if b.Debug {
		if err != nil {
			debugLog.Infof("[QQBot 错误] %v\n", err)
		} else if respObj != nil {
			respJson, _ := json.Marshal(respObj)
			debugLog.Infof("[QQBot 返回] %s\n", string(respJson))
		}
	}
	return err
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
		debugLog.Infof("key:%v val:%v", key, strVal)
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

func getJson(url string, headers http.Header, respObj any) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("构造请求失败: %w", err)
	}

	if headers != nil {
		req.Header = headers
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if c, err := io.ReadAll(resp.Body); err == nil {
			return fmt.Errorf("请求失败，状态码: %d, 内容: %s", resp.StatusCode, string(c))
		}
		return fmt.Errorf("请求失败，状态码: %d", resp.StatusCode)
	}

	if respObj != nil {
		if err := json.NewDecoder(resp.Body).Decode(respObj); err != nil {
			return fmt.Errorf("解析响应失败: %w", err)
		}
	}

	return nil
}

func patchJson(url string, body any, headers http.Header, respObj any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("编码 JSON 请求失败: %w", err)
	}

	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(data))
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
		if c, err := io.ReadAll(resp.Body); err == nil {
			return fmt.Errorf("请求失败，状态码: %d, 内容: %s", resp.StatusCode, string(c))
		}
		return fmt.Errorf("请求失败，状态码: %d", resp.StatusCode)
	}

	if respObj != nil {
		if err := json.NewDecoder(resp.Body).Decode(respObj); err != nil {
			return fmt.Errorf("解析响应失败: %w", err)
		}
	}

	return nil
}

func putJson(url string, body any, headers http.Header, respObj any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("编码 JSON 请求失败: %w", err)
	}

	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(data))
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
		if c, err := io.ReadAll(resp.Body); err == nil {
			return fmt.Errorf("请求失败，状态码: %d, 内容: %s", resp.StatusCode, string(c))
		}
		return fmt.Errorf("请求失败，状态码: %d", resp.StatusCode)
	}

	if respObj != nil {
		if err := json.NewDecoder(resp.Body).Decode(respObj); err != nil {
			return fmt.Errorf("解析响应失败: %w", err)
		}
	}

	return nil
}

// GetGroupMemberRole 获取群成员角色
func (b *QQBot) GetGroupMemberRole(groupOpenID, memberOpenID string) string {
	if groupOpenID == "" || memberOpenID == "" {
		return "null"
	}
	var r GroupMemberRole
	if err := b.Get("/v2/groups/"+groupOpenID+"/members/"+memberOpenID, &r); err != nil {
		return "null"
	}
	switch r.Role {
	case "owner":
		return "1"
	case "admin":
		return "2"
	default:
		return "null"
	}
}
