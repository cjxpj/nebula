package utils

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"io"
	"math/big"
	mrand "math/rand/v2"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cjxpj/nebula/debugLog"
	"gopkg.in/ini.v1"
	"gopkg.in/yaml.v3"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

// androidDataDir Android 端固定数据目录（Documents/Nebula）。
// 启动词库 start.n 位于该目录下，实际数据目录由启动词库头部「$设置工作目录$」切换到 Nebula/NebulaData 下。
const androidDataDir = "/storage/emulated/0/Documents/Nebula"

// Error 将文本写入文件
func Error(text string) {
	PrintLog(false, "系统", text)
}

// ErrorStop 写入错误信息文件并停止程序
func ErrorStop(text string) {
	Error(text)
	os.Exit(0)
}

// Log 将文本写入到 log.txt 文件
func Log(text string) {
	PrintLog(true, "系统", text)
}

func PrintLog(code bool, head, text string) {
	text = strings.ReplaceAll(text, "\r\n", `\r\n`)
	text = strings.ReplaceAll(text, "\n", `\n`)
	currentTime := time.Now().Format("20060102/15")
	currentTime2 := time.Now().Format("04m05s")
	file := NewFileQueue(path.Join("database", "log", currentTime+".txt"))
	resCode := "No"
	if code {
		resCode = "Yes"
	}
	file.AppendToFile(fmt.Sprintf("[%s]%s|%s>%s\n", head, resCode, currentTime2, text))
}

// LogStop 将文本写入到 log.txt 文件并停止程序
func LogStop(text string) {
	Log(text)
	os.Exit(0)
}

var appDirOverride string

// SetAppDir 设置应用数据目录。
// 鸿蒙端通过 NAPI 注入沙箱目录；GOOS=linux 下默认返回的相对路径不可写，会触发 ErrorStop 退出。
func SetAppDir(dir string) {
	appDirOverride = dir
}

// GetAppDir 获取应用目录。
// 桌面端默认返回空串，相对路径直接基于进程当前工作目录解析（由「设置工作目录」函数切换）；
// Android 返回固定数据目录，鸿蒙等移动端由 SetAppDir 注入沙箱目录。
func GetAppDir() string {
	if appDirOverride != "" {
		return appDirOverride
	}
	if runtime.GOOS == "android" {
		return androidDataDir
	}
	return ""
}

// startupMode 启动阶段标志：启动期间不落盘日志与编译缓存，避免启动时自动创建 database/log、private/.dic_cache 目录。
var startupMode atomic.Bool

// SetStartupMode 设置启动阶段标志。
func SetStartupMode(v bool) { startupMode.Store(v) }

// InStartupMode 返回是否处于启动阶段。
func InStartupMode() bool { return startupMode.Load() }

// 随机数
func RandNum(min, max int) int {
	if min == max {
		return min
	}
	if min > max {
		min, max = max, min
	}
	// 生成一个安全的随机数
	randomNumber, err := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
	if err != nil {
		return min - 1
	}
	// 将随机数转换为min和max范围内的数字
	randomNumberInt := int(randomNumber.Int64()) + min
	return randomNumberInt
}

// 获取唯一编号
func GetUid(uid string) string {
	var result []byte
	str := base64.StdEncoding.EncodeToString([]byte(uid))
	for i := 0; i < len(str); i++ {
		if i > 0 && i%8 == 0 {
			result = append(result, '/')
		}
		result = append(result, str[i])
	}
	return string(result) + "D"
}

// NewFileQueue 创建一个新的文件队列实例
func NewFileQueue(FileName string) *FileQueue {
	file := &FileQueue{}
	file.SetPath(FileName)
	return file
}

// NewFileQueue 创建一个新的文件队列实例
func NewFile() *FileQueue {
	file := &FileQueue{}
	return file
}

// 重新设置文件路径
func (fq *FileQueue) SetPath(FileName string) *FileQueue {
	appDir := GetAppDir()
	setfilePath := filepath.Join(appDir, FileName)
	if FileName == "/" {
		setfilePath = appDir
	}
	if filepath.IsAbs(FileName) {
		setfilePath = FileName
	}
	fq.FileName = setfilePath
	return fq
}

// 打开sqlite文件
func (fq *FileQueue) OpenSqlite() (*sql.DB, error) {
	conn, err := sql.Open("sqlite3", fq.FileName)
	return conn, err
}

// FileExists 检查文件是否存在且是文件
func (fq *FileQueue) FileExists() bool {
	info, err := os.Stat(fq.FileName)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// DirExists 检查文件夹是否存在且是目录
func (fq *FileQueue) DirExists() bool {
	info, err := os.Stat(fq.FileName)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// 检测文件跟文件夹是否存在
func (fq *FileQueue) FileOrDirExists() bool {
	_, err := os.Stat(fq.FileName)
	return !os.IsNotExist(err)
}

// 获取绝对路径
func (fq *FileQueue) GetAbsPath() string {
	filepath, _ := filepath.Abs(fq.FileName)
	return filepath
}

// GetFileSize 返回文件的大小
func (fq *FileQueue) GetFileSize() (int64, error) {
	// 确保文件存在
	fileInfo, err := os.Stat(fq.FileName)
	if os.IsNotExist(err) {
		return 0, fmt.Errorf("文件 '%s' 不存在", fq.FileName)
	}
	if err != nil {
		return 0, err
	}

	// 如果是目录，则返回错误，因为我们只想要文件大小
	if fileInfo.IsDir() {
		return 0, fmt.Errorf("'%s' 是一个目录，不是文件", fq.FileName)
	}

	// 返回文件大小
	return fileInfo.Size(), nil
}

// GetDirSize 返回目录的大小
func (fq *FileQueue) GetDirSize() (int64, error) {
	var size int64

	// 确保路径存在
	if _, err := os.Stat(fq.FileName); os.IsNotExist(err) {
		return 0, fmt.Errorf("目录 '%s' 不存在", fq.FileName)
	}

	// 使用 filepath.Walk 遍历目录
	err := filepath.Walk(fq.FileName, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			debugLog.Infof("遍历 \"%s\" 时出错: %v\n", path, err)
			return err
		}
		if !info.IsDir() {
			// 累加文件大小
			size += info.Size()
		}
		return nil
	})

	if err != nil {
		return 0, err
	}

	return size, nil
}

// WriteToFile 向文件写入数据
func (fq *FileQueue) WriteToFile(data string) {
	l := fileLock(fq.FileName)
	l.Lock()
	defer l.Unlock()

	// 检查文件夹是否存在，不存在则创建
	dir := filepath.Dir(fq.FileName)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			Error("创建文件夹失败")
			return
		}
	}

	// 创建文件，如果不存在
	file, err := os.OpenFile(fq.FileName, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		Error("创建文件失败")
		return
	}
	defer file.Close()

	_, err = file.WriteString(data)
	if err != nil {
		Error("写入数据失败")
	}
}

func (fq *FileQueue) Download(url string) bool {
	l := fileLock(fq.FileName)
	l.Lock()
	defer l.Unlock()

	// 确保目标文件夹存在
	dir := filepath.Dir(fq.FileName)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			Error("创建文件夹失败")
			return false
		}
	}

	// 拼接目标文件路径
	filePath := filepath.Join(dir, filepath.Base(url))

	// 创建目标文件
	out, err := os.Create(filePath)
	if err != nil {
		Error("创建文件失败")
		return false
	}
	defer out.Close()

	// 发送 HTTP GET 请求
	resp, err := httpClient.Get(url)
	if err != nil {
		Error("访问失败")
		return false
	}
	defer resp.Body.Close()

	// 将 HTTP 响应的主体写入目标文件
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		Error("写入数据失败")
		return false
	}

	return true
}

// DownloadWithDynamicThreads 分段多线程下载（无取消能力，供普通调用方使用）。
// maxThreads > 0 时使用指定线程数；maxThreads = 0 时按文件大小自适应（2~8）。
func (fq *FileQueue) DownloadWithDynamicThreads(url string, maxThreads int, showProgress bool, progressFn func(percent float64)) error {
	var onProgress func(percent float64, downloaded, total int64)
	if progressFn != nil {
		onProgress = func(percent float64, downloaded, total int64) {
			progressFn(percent)
		}
	}
	return fq.downloadWithDynamicThreads(context.Background(), url, maxThreads, showProgress, onProgress)
}

// DownloadWithDynamicThreadsCtx 分段多线程下载，支持 ctx 取消（暂停）与 .part.N 断点续传。
// maxThreads > 0 时使用指定线程数；maxThreads = 0 时按文件大小自适应（2~8）。
// onProgress 每变化 1% 及完成时回调一次（percent 0-100，downloaded/total 为字节数）。
func (fq *FileQueue) DownloadWithDynamicThreadsCtx(ctx context.Context, url string, maxThreads int, showProgress bool, onProgress func(percent float64, downloaded, total int64)) error {
	return fq.downloadWithDynamicThreads(ctx, url, maxThreads, showProgress, onProgress)
}

// autoDownloadThreads 根据文件大小自适应计算下载并发线程数（2~8）。
// 小文件多线程的连接建立开销大于收益，用少量线程；大文件用更多线程提升吞吐。
func autoDownloadThreads(length int64) int {
	mb := length / (1024 * 1024)
	switch {
	case mb <= 8:
		return 2
	case mb <= 32:
		return 4
	case mb <= 128:
		return 6
	default:
		return 8
	}
}

// 下载任务状态
const (
	dlStatusRunning int32 = iota // 0 下载中
	dlStatusDone                 // 1 完成
	dlStatusFailed               // 2 失败
)

// DownloadTask 异步下载任务，实时记录下载进度/速度/状态，供调用方查询。
type DownloadTask struct {
	downloaded int64 // 已下载字节数
	total      int64 // 文件总字节数
	status     int32 // 下载状态

	mu     sync.Mutex
	errMsg string    // 失败原因
	lastDL int64     // 上次查询速度时的已下载字节数
	lastAt time.Time // 上次查询速度的时间
}

// Progress 返回下载进度百分比（0-100）。
func (t *DownloadTask) Progress() float64 {
	d := atomic.LoadInt64(&t.downloaded)
	total := atomic.LoadInt64(&t.total)
	if total <= 0 {
		return 0
	}
	p := float64(d) * 100 / float64(total)
	if p > 100 {
		p = 100
	}
	return p
}

// Downloaded 返回已下载字节数。
func (t *DownloadTask) Downloaded() int64 {
	return atomic.LoadInt64(&t.downloaded)
}

// Total 返回文件总字节数。
func (t *DownloadTask) Total() int64 {
	return atomic.LoadInt64(&t.total)
}

// Speed 返回最近一次查询间隔内的平均下载速度（MB/s）。
func (t *DownloadTask) Speed() float64 {
	now := time.Now()
	d := atomic.LoadInt64(&t.downloaded)
	t.mu.Lock()
	last := t.lastDL
	lastAt := t.lastAt
	t.lastDL = d
	t.lastAt = now
	t.mu.Unlock()
	dt := now.Sub(lastAt).Seconds()
	if dt <= 0 || d < last {
		return 0
	}
	return float64(d-last) / dt / 1048576
}

// Status 返回下载状态："下载中"、"完成"、"失败"。
func (t *DownloadTask) Status() string {
	switch atomic.LoadInt32(&t.status) {
	case dlStatusDone:
		return "完成"
	case dlStatusFailed:
		return "失败"
	default:
		return "下载中"
	}
}

// Error 返回失败原因，未失败时返回空字符串。
func (t *DownloadTask) Error() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.errMsg
}

// DownloadAsync 异步分段多线程下载，立即返回 *DownloadTask，可实时查询进度/速度/状态。
func (fq *FileQueue) DownloadAsync(url string, maxThreads int, showProgress bool) *DownloadTask {
	task := &DownloadTask{lastAt: time.Now()}
	go func() {
		err := fq.DownloadWithDynamicThreadsCtx(context.Background(), url, maxThreads, showProgress, func(_ float64, downloaded, total int64) {
			atomic.StoreInt64(&task.downloaded, downloaded)
			atomic.StoreInt64(&task.total, total)
		})
		if err != nil {
			atomic.StoreInt32(&task.status, dlStatusFailed)
			task.mu.Lock()
			task.errMsg = err.Error()
			task.mu.Unlock()
			return
		}
		atomic.StoreInt32(&task.status, dlStatusDone)
		atomic.StoreInt64(&task.downloaded, atomic.LoadInt64(&task.total))
	}()
	return task
}

// downloadWithDynamicThreads 分段多线程下载核心实现。
// maxThreads <= 0 时在拿到文件大小后按大小自适应计算线程数（2~8）。
func (fq *FileQueue) downloadWithDynamicThreads(ctx context.Context, url string, maxThreads int, showProgress bool, onProgress func(percent float64, downloaded, total int64)) error {
	autoThreads := maxThreads <= 0
	if autoThreads {
		maxThreads = 8 // 先按上限预分配连接池，拿到文件大小后再按需下调
	}
	// ------------------------  可调常量区 ------------------------
	const (
		chunkSize     = 4 * 1024 * 1024 // 4 MB
		dialTimeout   = 10 * time.Second
		headerTimeout = 10 * time.Second
		maxRetries    = 3
		userAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"
	)
	// 运行时配置（依赖 maxThreads）
	idleConns := maxThreads * 4
	idlePerHost := maxThreads * 4
	idleTimeout := 90 * time.Second
	// -------------------------------------------------------------

	// 1. HEAD 拿大小与 Range 支持
	client := &http.Client{
		Timeout: 30 * time.Minute,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			MaxIdleConns:          idleConns,
			MaxIdleConnsPerHost:   idlePerHost,
			IdleConnTimeout:       idleTimeout,
			TLSHandshakeTimeout:   dialTimeout,
			DisableCompression:    true,
			ResponseHeaderTimeout: 30 * time.Second, // 首字节 30 s 足够
			DialContext: (&net.Dialer{
				Timeout:   dialTimeout,
				KeepAlive: 30 * time.Second,
				DualStack: true,
			}).DialContext,
		},
	}
	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HEAD 失败，状态码 %d", resp.StatusCode)
	}
	length := resp.ContentLength
	if length <= 0 {
		return errors.New("无法获取 Content-Length")
	}
	if autoThreads {
		maxThreads = autoDownloadThreads(length)
	}
	acceptRanges := strings.EqualFold(resp.Header.Get("Accept-Ranges"), "bytes")
	if !acceptRanges {
		if showProgress {
			debugLog.Infof("⚠️  服务器不支持 Range，强制单线程")
		}
		maxThreads = 1
	}

	// 2. 分片任务生成
	chunks := int((length + chunkSize - 1) / chunkSize)
	type task struct {
		idx   int
		start int64
		end   int64
	}
	tasks := make(chan task, chunks)
	for i := range chunks {
		start := int64(i) * chunkSize
		end := start + chunkSize - 1
		if end >= length {
			end = length - 1
		}
		tasks <- task{idx: i, start: start, end: end}
	}
	close(tasks)

	// 3. 目录 & 临时文件
	dir := filepath.Dir(fq.FileName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmpFiles := make([]string, chunks)

	// 4. 进度回调/进度条
	var downloaded int64
	var mu sync.Mutex
	startT := time.Now()
	stopBar := make(chan struct{})
	showProgressBar := showProgress || onProgress != nil
	if showProgressBar {
		go func() {
			tick := time.NewTicker(100 * time.Millisecond)
			defer tick.Stop()
			lastPct := -1
			for {
				select {
				case <-tick.C:
					d := atomic.LoadInt64(&downloaded)
					percent := float64(d) * 100 / float64(length)
					if percent > 100 {
						percent = 100
					}
					if onProgress != nil {
						if pct := int(percent); pct != lastPct {
							lastPct = pct
							onProgress(percent, d, length)
						}
					} else {
						speed := float64(d) / 1048576 / time.Since(startT).Seconds()
						eta := time.Duration(float64(length-d)/1048576/speed) * time.Second
						fmt.Printf("\r[%.1f%%] %.2f MB/s  eta %v ", percent, speed, eta.Round(time.Second))
					}
				case <-stopBar:
					if onProgress == nil {
						debugLog.Infof("[100.0%%] %.2f MB/s  total %s \n",
							float64(length)/1048576/time.Since(startT).Seconds(),
							time.Since(startT).Round(time.Millisecond))
					}
					return
				}
			}
		}()
	}

	// 5. 并发下载（支持 .part.N 断点续传与 ctx 取消）
	var wg sync.WaitGroup
	var firstErr error
	sem := make(chan struct{}, maxThreads)
	for t := 0; t < maxThreads; t++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for tk := range tasks {
				if ctx.Err() != nil {
					continue
				}
				mu.Lock()
				hasErr := firstErr != nil
				mu.Unlock()
				if hasErr {
					continue
				}

				tmp := fmt.Sprintf("%s.part.%d", fq.FileName, tk.idx)
				// 断点续传：该分块已完整下载则跳过，只累加进度
				if fi, err := os.Stat(tmp); err == nil && fi.Size() == tk.end-tk.start+1 {
					tmpFiles[tk.idx] = tmp
					atomic.AddInt64(&downloaded, fi.Size())
					continue
				}

				sem <- struct{}{}
				for attempt := range maxRetries {
					if ctx.Err() != nil {
						break
					}
					if err := func() error {
						req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
						req.Header.Set("User-Agent", userAgent)
						if maxThreads > 1 {
							req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", tk.start, tk.end))
						}
						resp, err := client.Do(req)
						if err != nil {
							return err
						}
						defer resp.Body.Close()
						if maxThreads > 1 && resp.StatusCode != http.StatusPartialContent {
							return fmt.Errorf("range 请求返回 %d", resp.StatusCode)
						}
						if maxThreads == 1 && resp.StatusCode != http.StatusOK {
							return fmt.Errorf("GET 请求返回 %d", resp.StatusCode)
						}

						f, err := os.Create(tmp)
						if err != nil {
							return err
						}
						defer f.Close()

						buf := make([]byte, 256*1024)
						for {
							if ctx.Err() != nil {
								return ctx.Err()
							}
							n, err := resp.Body.Read(buf)
							if n > 0 {
								f.Write(buf[:n])
								atomic.AddInt64(&downloaded, int64(n))
							}
							if err != nil {
								if err == io.EOF {
									break
								}
								return err
							}
						}
						tmpFiles[tk.idx] = tmp
						return nil
					}(); err == nil {
						break
					} else {
						_ = os.Remove(tmp) // 清理不完整分块，避免续传时误判为完整
						if ctx.Err() != nil {
							break
						}
						if attempt == maxRetries-1 {
							mu.Lock()
							if firstErr == nil {
								firstErr = err
							}
							mu.Unlock()
						}
					}
				}
				<-sem
			}
		}()
	}
	wg.Wait()
	close(stopBar)
	if firstErr != nil {
		return firstErr
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// 6. 零拷贝合并
	out, err := os.Create(fq.FileName)
	if err != nil {
		return err
	}
	defer out.Close()
	for _, p := range tmpFiles {
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, f)
		f.Close()
		os.Remove(p)
		if err != nil {
			return err
		}
	}
	// 完成时回调一次 100%，确保调用方拿到最终进度
	if onProgress != nil {
		onProgress(100, length, length)
	}
	return nil
}

// DownloadWithMirrors 依次尝试多个 URL 下载，首个成功即返回
func (fq *FileQueue) DownloadWithMirrors(urls []string, maxThreads int, showProgress bool, progressFn func(percent float64)) error {
	var lastErr error
	for i, url := range urls {
		if i > 0 {
			debugLog.Infof("回退至镜像: %s", url)
		}
		if err := fq.DownloadWithDynamicThreads(url, maxThreads, showProgress, progressFn); err != nil {
			lastErr = err
			_ = os.Remove(fq.FileName) // 清理失败残留
			continue
		}
		return nil
	}
	return lastErr
}

// ZipFolder 将文件夹压缩成 ZIP 文件
func (fq *FileQueue) ZipFolder(destZip string) bool {
	if absDest, err := filepath.Abs(destZip); err == nil {
		destZip = NewFileQueue(absDest).FileName
	} else {
		destZip = NewFileQueue(destZip).FileName
	}
	// 创建 ZIP 文件
	zipFile, err := os.Create(destZip)
	if err != nil {
		Error("创建 ZIP 文件失败: " + err.Error())
		return false
	}
	defer zipFile.Close()

	// 创建 ZIP 写入器
	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// 处理文件夹及其文件
	err = filepath.WalkDir(fq.FileName, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("遍历文件夹失败: %w", err)
		}

		// 跳过根文件夹
		if path == fq.FileName {
			return nil
		}

		// 获取相对路径
		relPath, err := filepath.Rel(fq.FileName, path)
		if err != nil {
			return fmt.Errorf("获取相对路径失败: %w", err)
		}

		// 如果是目录，则创建目录项
		if d.IsDir() {
			return nil
		}

		// 创建 ZIP 文件项
		zipFile, err := zipWriter.Create(relPath)
		if err != nil {
			return fmt.Errorf("创建 ZIP 文件项失败: %w", err)
		}

		// 打开源文件
		srcFile, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("打开源文件失败: %w", err)
		}
		func() {
			defer srcFile.Close()
			_, err = io.Copy(zipFile, srcFile)
		}()
		if err != nil {
			return fmt.Errorf("复制文件内容失败: %w", err)
		}

		return nil
	})

	if err != nil {
		Error("压缩文件夹失败: " + err.Error())
		return false
	}

	return true
}

// 解压zip
func (fq *FileQueue) UnZip(dest string) bool {
	if absDest, err := filepath.Abs(dest); err == nil {
		dest = NewFileQueue(absDest).FileName
	} else {
		dest = NewFileQueue(dest).FileName
	}

	r, err := zip.OpenReader(fq.FileName)
	if err != nil {
		Error("打开ZIP文件失败")
		return false
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)

		// 防止 Zip Slip（目录穿越攻击）
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			Error(fmt.Sprintf("非法文件路径: %s", fpath))
			return false
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fpath, os.ModePerm); err != nil {
				Error("创建目录失败")
				return false
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			Error("创建目录失败")
			return false
		}

		dstFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			Error("创建解压文件失败")
			return false
		}

		fileInArchive, err := f.Open()
		if err != nil {
			dstFile.Close()
			Error("打开压缩文件中的文件失败")
			return false
		}

		_, err = io.Copy(dstFile, fileInArchive)

		// 先关文件再判断错误
		dstFile.Close()
		fileInArchive.Close()

		if err != nil {
			Error("复制文件内容失败")
			return false
		}
	}

	return true
}

// DeleteFile 删除文件，并返回是否成功删除
func (fq *FileQueue) DeleteFile() bool {
	l := fileLock(fq.FileName)
	l.Lock()
	defer l.Unlock()

	// 检查文件夹是否存在
	if p, err := os.Stat(fq.FileName); os.IsNotExist(err) || p.IsDir() {
		return false
	}

	// 删除文件
	if err := os.Remove(fq.FileName); err != nil {
		return false
	}

	return true
}

// 删除文件夹
func (fq *FileQueue) DeleteFolder() bool {
	l := fileLock(fq.FileName)
	l.Lock()
	defer l.Unlock()

	// 检查文件夹是否存在
	if p, err := os.Stat(fq.FileName); os.IsNotExist(err) || !p.IsDir() {
		return false
	}

	// 删除文件夹及其所有内容
	if err := os.RemoveAll(fq.FileName); err != nil {
		return false
	}
	return true
}

// AppendToFile 向文件追加数据
func (fq *FileQueue) AppendToFile(data string) {
	l := fileLock(fq.FileName)
	l.Lock()
	defer l.Unlock()

	// 检查文件夹是否存在，不存在则创建
	dir := filepath.Dir(fq.FileName)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			Error("创建文件夹失败")
			return
		}
	}

	// 打开文件以追加模式
	file, err := os.OpenFile(fq.FileName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		Error("打开文件失败")
		return
	}
	defer file.Close()

	// 创建缓存器
	writer := bufio.NewWriter(file)

	// 写入数据到缓存
	if _, err := writer.WriteString(data); err != nil {
		Error("追加数据缓存失败")
		return
	}

	// 刷新缓存，将数据写入文件
	if err := writer.Flush(); err != nil {
		Error("追加数据失败")
	}

	// 写入数据到文件末尾
	// _, err = file.WriteString(data)
	// if err != nil {
	// 	Error("追加数据失败")
	// }
}

// 读取图片
func (fq *FileQueue) ReadImage() (image.Image, error) {
	l := fileLock(fq.FileName)
	l.RLock()
	defer l.RUnlock()

	file, err := os.Open(fq.FileName)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}

	return img, nil
}

// ReadFile 完整从文件读取数据
func (fq *FileQueue) ReadFile() (string, error) {
	l := fileLock(fq.FileName)
	l.RLock()
	defer l.RUnlock()

	file, err := os.Open(fq.FileName)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var result strings.Builder
	buf := make([]byte, 1024)
	for {
		n, err := file.Read(buf)
		if n > 0 {
			result.Write(buf[:n])
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
	}
	return result.String(), nil
}

// 获取文件夹列表
func (fq *FileQueue) GetDirList() ([]string, error) {
	return fq.GetList("dir")
}

// 获取文件列表
func (fq *FileQueue) GetFileList() ([]string, error) {
	return fq.GetList("file")
}

// GetList 函数用于获取指定文件夹中的文件列表
func (fq *FileQueue) GetList(t string) ([]string, error) {
	fileList := []string{}

	entries, err := os.ReadDir(fq.FileName)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		// 根据 t 的值来决定是否添加到 fileList 中
		switch t {
		case "file", "文件":
			if !entry.IsDir() {
				fileList = append(fileList, entry.Name())
			}
		case "dir", "文件夹":
			if entry.IsDir() {
				fileList = append(fileList, entry.Name())
			}
		default: // 默认处理 "all"
			fileList = append(fileList, entry.Name())
		}
	}

	return fileList, nil
}

// ReadFileExt 获取文件后缀
func (fq *FileQueue) ReadFileExt() string {
	return filepath.Ext(fq.FileName)
}

// 加载ini
func (fq *FileQueue) LoadIni() (*ini.File, error) {
	l := fileLock(fq.FileName)
	l.RLock()
	defer l.RUnlock()

	file, err := ini.Load(fq.FileName)
	if err != nil {
		return nil, err
	}
	return file, nil
}

// 保存ini
func (fq *FileQueue) SaveIni(file *ini.File) error {
	l := fileLock(fq.FileName)
	l.Lock()
	defer l.Unlock()

	return file.SaveTo(fq.FileName)
}

// 加载yaml
func (fq *FileQueue) LoadYaml() (map[string]any, error) {
	l := fileLock(fq.FileName)
	l.RLock()
	defer l.RUnlock()

	data, err := os.ReadFile(fq.FileName)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	err = yaml.Unmarshal(data, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// 保存yaml
func (fq *FileQueue) SaveYaml(data map[string]any) error {
	l := fileLock(fq.FileName)
	l.Lock()
	defer l.Unlock()

	// 检查文件夹是否存在，不存在则创建
	dir := filepath.Dir(fq.FileName)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	yamlData, err := yaml.Marshal(data)
	if err != nil {
		return err
	}

	return os.WriteFile(fq.FileName, yamlData, 0644)
}

// 打开文件
func (fq *FileQueue) OpenFile() (*os.File, error) {
	return os.Open(fq.FileName)
}

// SplitLines 将文件字节按行切分，去除行尾的 \n 与 \r（兼容 CRLF），
// 并去掉末尾因换行产生的空行。返回的行切片共享同一块底层数据（零拷贝）。
func SplitLines(data []byte) []string {
	s := string(data)
	if s == "" {
		return nil
	}
	// 去掉末尾换行，避免结尾多出一个空行（与 bufio.Scanner 的 ScanLines 行为一致）
	s = strings.TrimSuffix(s, "\n")
	lines := strings.Split(s, "\n")
	// 兼容 Windows CRLF：去掉每行行尾的 \r
	for i := range lines {
		lines[i] = strings.TrimSuffix(lines[i], "\r")
	}
	return lines
}

// ReadFromFileLines 读取文件并按行切分，返回所有行（行尾的 \n 与 \r 已被移除）。
// 采用一次性读入 + strings.Split 切分，行切片共享同一块底层数据，避免逐行分配，兼顾速度与内存。
func (fq *FileQueue) ReadFromFileLines() ([]string, error) {
	l := fileLock(fq.FileName)
	l.RLock()
	defer l.RUnlock()

	data, err := os.ReadFile(fq.FileName)
	if err != nil {
		return nil, err
	}
	return SplitLines(data), nil
}

// ReadFromFile 从文件读取数据（逐行读取后拼接）
func (fq *FileQueue) ReadFromFile() (string, error) {
	lines, err := fq.ReadFromFileLines()
	if err != nil {
		return "", err
	}
	return strings.Join(lines, "\n"), nil
}

// Copy 复制文件或文件夹
func (fq *FileQueue) Copy(newName string) bool {
	l := fileLock(fq.FileName) // 使用写锁，确保线程安全
	l.Lock()
	defer l.Unlock()

	// 新文件名
	newPath := NewFileQueue(newName).FileName

	if newPath == fq.FileName {
		return false
	}

	// 获取文件信息
	fileInfo, err := os.Stat(fq.FileName)
	if err != nil {
		return false
	}

	// 根据文件类型进行不同的处理
	if fileInfo.IsDir() {
		// 处理文件夹
		err = CopyDir(fq.FileName, newPath)
	} else {
		// 处理文件
		err = CopyFile(fq.FileName, newPath)
	}

	return err == nil
}

// copyFile 复制文件
func CopyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return err
	}

	return dstFile.Sync()
}

// copyDir 递归复制文件夹
func CopyDir(srcDir, dstDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}

	// 创建目标文件夹
	err = os.MkdirAll(dstDir, os.ModePerm)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())

		if entry.IsDir() {
			// 递归复制子文件夹
			err = CopyDir(srcPath, dstPath)
			if err != nil {
				return err
			}
		} else {
			// 复制文件
			err = CopyFile(srcPath, dstPath)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// 文件重命名
func (fq *FileQueue) Rename(newName string) bool {
	l := fileLock(fq.FileName) // 使用写锁，确保线程安全
	l.Lock()
	defer l.Unlock()

	// 获取原文件所在目录
	dir := filepath.Dir(fq.FileName)

	// 构造新文件完整路径
	newPath := filepath.Join(dir, newName)

	// 重命名文件
	err := os.Rename(fq.FileName, newPath)
	if err == nil {
		// 成功重命名后，更新 FileName
		fq.FileName = newPath
		return true
	}
	return false
}

// MoveFile 将文件移动到目标目录（同一分区内）
func (fq *FileQueue) MoveFile(targetDir string) bool {
	l := fileLock(fq.FileName)
	l.Lock()
	defer l.Unlock()

	// 构造目标路径，保持文件名不变
	newPath := filepath.Join(targetDir, filepath.Base(fq.FileName))

	// 使用 os.Rename 移动文件
	err := os.Rename(fq.FileName, newPath)
	if err == nil {
		fq.FileName = newPath // 更新 FileName
		return true
	}

	return false
}

// 获取文件名字
func (fq *FileQueue) GetFileName() (string, error) {
	// 判断文件是否存在
	if p, err := os.Stat(fq.FileName); os.IsNotExist(err) || !p.IsDir() {
		return "", errors.New("文件不存在")
	}
	return fq.FileName, nil
}

// ReadFileByte 从文件完整读取数据并返回字节切片
func (fq *FileQueue) ReadFileByte() ([]byte, error) {
	l := fileLock(fq.FileName)
	l.RLock()
	defer l.RUnlock()

	file, err := os.ReadFile(fq.FileName)
	if err != nil {
		return nil, err
	}

	return file, nil
}

// WriteFileByte 向文件写入数据
func (fq *FileQueue) WriteFileByte(data []byte) {
	l := fileLock(fq.FileName)
	l.Lock()
	defer l.Unlock()

	// 检查文件夹是否存在，不存在则创建
	dir := filepath.Dir(fq.FileName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		debugLog.Infof("创建文件夹失败，可能是权限问题，也有可能是有相同名字的文件存在占用了。")
		// Error("创建文件夹失败")
		return
	}

	// 写入数据到文件中
	if err := os.WriteFile(fq.FileName, data, 0644); err != nil {
		Error("写入数据失败")
	}
}

// escapeFileKeyValue 转义键值中的换行、回车和反斜杠，保证值只占一行
func escapeFileKeyValue(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

// unescapeFileKeyValue 反转义 escapeFileKeyValue 的结果
func unescapeFileKeyValue(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		switch s[i+1] {
		case 'n':
			b.WriteByte('\n')
			i++
		case 'r':
			b.WriteByte('\r')
			i++
		case '\\':
			b.WriteByte('\\')
			i++
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// ReadFileKeyList 从文件中读取所有键值对并返回一个 []map[string]string
func (fq *FileQueue) ReadFileKeyList() ([]map[string]string, error) {
	data, err := fq.ReadFromFile()
	if err != nil {
		return nil, err
	}

	// 将数据拆分成行
	lines := strings.Split(data, "\n")

	// 跳过第一行的标志
	skipFirstLine := true

	var result []map[string]string

	// 解析每一行，并将键值对存储到 map 中
	for _, line := range lines {
		if skipFirstLine {
			skipFirstLine = false
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			entry := map[string]string{
				"key":  parts[0],
				"data": unescapeFileKeyValue(parts[1]),
			}
			result = append(result, entry)
		}
	}

	return result, nil
}

// ReadFileKey 从文件中读取与给定键相关联的值
func (fq *FileQueue) ReadFileKey(key string) (string, error) {
	data, err := fq.ReadFromFile()
	if err != nil {
		return "", err
	}

	// 将数据拆分成行
	lines := strings.Split(data, "\n")

	// 跳过第一行的标志
	skipFirstLine := true

	// 查找具有指定键的行
	for _, line := range lines {
		if skipFirstLine {
			skipFirstLine = false
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 && parts[0] == key {
			return unescapeFileKeyValue(parts[1]), nil
		}
	}

	return "", fmt.Errorf("未找到键 %s", key)
}

// WriteFileKey 将键值对写入文件
func (fq *FileQueue) WriteFileKey(key, value string) error {

	data, err := fq.ReadFromFile()
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// 获取当前时间并格式化为字符串
	currentTime := time.Now().Format("2006-01-02 15:04:05")

	// 将数据拆分成行
	lines := strings.Split(data, "\n")

	// 更新或追加键值对
	var found bool
	for i, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 && parts[0] == key {
			lines[i] = fmt.Sprintf("%s=%s", key, escapeFileKeyValue(value))
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, fmt.Sprintf("%s=%s", key, escapeFileKeyValue(value)))
	}

	// 将第一行更新为当前时间
	lines[0] = fmt.Sprintf("更新时间: %s", currentTime)

	// 将行重新组合
	newData := strings.Join(lines, "\n")

	// 将更新的数据写入文件
	fq.WriteToFile(newData)

	return nil
}

// CreateFolderIfNotExists 检查并创建文件夹，返回是否成功创建
func CreateFolderIfNotExists(folderName string) bool {
	appDir := GetAppDir()
	dirPath := filepath.Join(appDir, folderName)

	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		err := os.MkdirAll(dirPath, 0755)
		if err != nil {
			ErrorStop("无法创建文件夹" + folderName)
			return false
		}
		return true
	} else if err != nil {
		ErrorStop("无法检查文件夹" + folderName)
	}
	return false
}

// GetLineCount 获取文件行数（高效版本）
func (fq *FileQueue) GetLineCount() (int, error) {
	l := fileLock(fq.FileName)
	l.Lock()
	defer l.Unlock()

	file, err := os.Open(fq.FileName)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		count++
	}
	if err := scanner.Err(); err != nil {
		return count, err
	}
	return count, nil
}

// 从文件中随机读取一行（单次扫描，高性能版本）
func (fq *FileQueue) ReadFileRandomLine() (string, error) {
	l := fileLock(fq.FileName)
	l.Lock()
	defer l.Unlock()

	file, err := os.Open(fq.FileName)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var chosen string
	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		count++
		// 蓄水池抽样算法（Reservoir Sampling）
		if mrand.IntN(count) == 0 {
			chosen = scanner.Text()
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if count == 0 {
		return "", errors.New("文件为空")
	}
	return chosen, nil
}

// ReadLines 从指定行开始读取指定数量的行（高效流式版）
func (fq *FileQueue) ReadLines(start, count int) ([]string, error) {
	l := fileLock(fq.FileName)
	l.Lock()
	defer l.Unlock()

	if start < 0 || count <= 0 {
		return nil, errors.New("参数无效")
	}

	file, err := os.Open(fq.FileName)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := make([]string, 0, count)
	index := 0

	for scanner.Scan() {
		if index >= start {
			lines = append(lines, scanner.Text())
			if len(lines) >= count {
				break
			}
		}
		index++
	}
	if err := scanner.Err(); err != nil {
		return lines, err
	}
	return lines, nil
}

// FindFfmpegExe 在 baseDir 下搜索 ffmpeg-*-essentials_build/bin 目录，返回 bin 路径或空串
func FindFfmpegExe(baseDir string) string {
	pattern := filepath.Join(baseDir, "ffmpeg-*essentials_build", "bin")
	matches, _ := filepath.Glob(pattern)
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}
