package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

func installPHP(destDir string, output *[]string) error {
	url := "https://windows.php.net/downloads/releases/php-7.4.33-Win32-vc15-x64.zip"

	zipPath := utils.NewFileQueue("php_download.zip")
	defer zipPath.DeleteFile() // 确保下载文件最终被删除

	*output = append(*output, "正在分段下载 PHP ...")
	if err := zipPath.DownloadWithDynamicThreads(url, 8, true); err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}

	*output = append(*output, "下载完成，正在解压...")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	if !zipPath.UnZip(destDir) {
		return fmt.Errorf("解压失败")
	}

	*output = append(*output, "✅ PHP 安装成功，路径：" + destDir)
	return nil
}

func parseRequestToMap(r *http.Request) (getMap, postMap, fileMap map[string]any, err error) {
	getMap = make(map[string]any)
	postMap = make(map[string]any)
	fileMap = make(map[string]any)

	// 解析 GET
	query := r.URL.Query()
	for k := range query {
		getMap[k] = query.Get(k)
	}

	// 解析 POST & 文件
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		err = r.ParseMultipartForm(32 << 20) // 32MB
		if err != nil {
			return
		}
		for k, vals := range r.MultipartForm.Value {
			postMap[k] = vals[0]
		}
		for k, files := range r.MultipartForm.File {
			if len(files) > 0 {
				file, err := files[0].Open()
				if err != nil {
					continue
				}
				tmp, _ := os.CreateTemp("", "upload-*")
				io.Copy(tmp, file)
				file.Close()
				tmp.Close()
				fileMap[k] = tmp.Name()
			}
		}
	} else {
		r.ParseForm()
		for k := range r.PostForm {
			postMap[k] = r.PostForm.Get(k)
		}
	}

	return
}

var (
	phpServerRunning bool
	phpServerMutex   sync.Mutex
	phpServerCancel  context.CancelFunc
	lastUseTime      time.Time
	phpCmd           *exec.Cmd
)

// 启动 PHP 内置服务器，10 秒内无请求自动关闭，ctx 控制生命周期
func ensurePHPServerRunning(ctx context.Context, phpDir string) error {
	phpServerMutex.Lock()
	defer phpServerMutex.Unlock()

	if phpServerRunning {
		lastUseTime = time.Now()
		return nil
	}

	cmdCtx, cancel := context.WithCancel(context.Background())
	phpExec := dto.GV.GetStr("_PhpPath_")
	cmd := exec.CommandContext(cmdCtx, phpExec, "-S", "127.0.0.1:8800", "-t", phpDir)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("PHP 启动失败: %v", err)
	}

	phpCmd = cmd
	phpServerRunning = true
	phpServerCancel = cancel
	lastUseTime = time.Now()

	// 监听外部 ctx 取消时，关闭 PHP 服务器
	go func() {
		<-ctx.Done()
		phpServerMutex.Lock()
		defer phpServerMutex.Unlock()
		if phpServerRunning {
			if phpCmd.Process != nil {
				phpCmd.Process.Kill()
			}
			phpServerCancel()
			phpServerRunning = false
			phpServerCancel = nil
		}
	}()

	// 5秒无请求自动关闭逻辑
	go func() {
		for {
			time.Sleep(2 * time.Second)
			phpServerMutex.Lock()
			if time.Since(lastUseTime) > 5*time.Second {
				if phpCmd.Process != nil {
					phpCmd.Process.Kill()
				}
				phpServerCancel()
				phpServerRunning = false
				phpServerCancel = nil
				phpServerMutex.Unlock()
				break
			}
			phpServerMutex.Unlock()
		}
	}()

	time.Sleep(200 * time.Millisecond) // 等待 PHP 启动
	return nil
}

// 执行临时 PHP 脚本，并传递 GET/POST/文件数据
func runTempPHP(
	ctx context.Context,
	code string,
	getData, postData, fileData *map[string]any,
	w http.ResponseWriter,
) (string, error) {
	appDir := utils.GetAppDir()
	phpDir := filepath.Join(appDir, "public")
	_ = os.MkdirAll(phpDir, 0755)

	// 写入临时 PHP 文件
	tmpFile, err := os.CreateTemp(phpDir, "tempscript-*.php")
	if err != nil {
		return "", fmt.Errorf("创建 PHP 文件失败: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(code)
	if err != nil {
		return "", fmt.Errorf("写入 PHP 脚本失败: %v", err)
	}
	tmpFile.Close()

	// 启动或复用 PHP 内置服务器
	if err := ensurePHPServerRunning(ctx, phpDir); err != nil {
		return "", err
	}

	// 构建 URL（含 GET 参数）
	scriptName := filepath.Base(tmpFile.Name())
	urlStr := "http://127.0.0.1:8800/" + scriptName
	if getData != nil && len(*getData) > 0 {
		query := url.Values{}
		for k, v := range *getData {
			query.Set(k, fmt.Sprint(v))
		}
		urlStr += "?" + query.Encode()
	}

	// 构建请求体
	var body io.Reader
	var contentType string

	if fileData != nil && len(*fileData) > 0 {
		var b bytes.Buffer
		writer := multipart.NewWriter(&b)
		for k, v := range *fileData {
			path, ok := v.(string)
			if !ok {
				continue
			}
			part, _ := writer.CreateFormFile(k, filepath.Base(path))
			file, _ := os.Open(path)
			io.Copy(part, file)
			file.Close()
		}
		if postData != nil {
			for k, v := range *postData {
				writer.WriteField(k, fmt.Sprint(v))
			}
		}
		writer.Close()
		body = &b
		contentType = writer.FormDataContentType()
	} else if postData != nil && len(*postData) > 0 {
		var parts []string
		for k, v := range *postData {
			parts = append(parts, fmt.Sprintf("%s=%s",
				url.QueryEscape(k), url.QueryEscape(fmt.Sprint(v))))
		}
		body = strings.NewReader(strings.Join(parts, "&"))
		contentType = "application/x-www-form-urlencoded"
	}

	// 构造请求
	method := "GET"
	if body != nil {
		method = "POST"
	}
	req, _ := http.NewRequest(method, urlStr, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	// 执行请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("访问失败: %v", err)
	}
	defer resp.Body.Close()

	// 将响应头写入 w
	if w != nil {
		for key, values := range resp.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
	}

	respBody, _ := io.ReadAll(resp.Body)
	return string(respBody), nil
}
