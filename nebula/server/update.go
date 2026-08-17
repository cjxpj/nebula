package dic_server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/mobile"
	"github.com/cjxpj/nebula/utils"
)

// UpdateCheckResult 更新检测结果
type UpdateCheckResult struct {
	Status  string `json:"status"`   // ok / error
	Current string `json:"current"`  // 当前版本
	Latest  string `json:"latest"`   // 最新版本
	Update  bool   `json:"update"`   // 是否有新版本
	URL     string `json:"url"`      // 下载页地址
	Notes   string `json:"notes"`    // 更新说明
	DownURL string `json:"down_url"` // 当前平台二进制直链
	Error   string `json:"error"`    // 检测失败原因
}

// checkUpdate 检测最新版本：优先 Gitee（国内快），失败回退 GitHub
func checkUpdate() UpdateCheckResult {
	result := UpdateCheckResult{Status: "ok", Current: appfiles.Version}
	client := &http.Client{Timeout: 8 * time.Second}
	tag, releaseURL, downURL, notes, ok := fetchReleaseWithPlatformBinary(client, "https://gitee.com/api/v5/repos/cjxpj/nebula/releases/latest")
	if !ok {
		tag, releaseURL, downURL, notes, ok = fetchReleaseWithPlatformBinary(client, "https://api.github.com/repos/cjxpj/nebula/releases/latest")
	}
	if !ok {
		result.Status = "error"
		result.Error = "无法获取最新版本，请检查网络后重试"
		return result
	}
	result.Latest = strings.TrimPrefix(tag, "v")
	result.URL = releaseURL
	if result.URL == "" {
		result.URL = "https://github.com/cjxpj/nebula/releases"
	}
	result.DownURL = downURL
	result.Notes = notes
	result.Update = compareVersions(result.Latest, appfiles.Version) > 0
	return result
}

// fetchReleaseWithPlatformBinary 获取 release 信息，downURL 根据当前平台匹配
func fetchReleaseWithPlatformBinary(client *http.Client, url string) (tag, htmlURL, downURL, notes string, ok bool) {
	tag, htmlURL, body, assets := fetchReleaseAssets(client, url)
	if tag == "" {
		return
	}
	notes = body
	var downOK bool
	downURL, downOK = pickBinaryForPlatform(assets)

	// Gitee 接口不返回 html_url，按源补下载页地址
	if htmlURL == "" {
		if strings.Contains(url, "gitee.com") {
			htmlURL = "https://gitee.com/cjxpj/nebula/releases"
		} else {
			htmlURL = "https://github.com/cjxpj/nebula/releases"
		}
	}
	ok = downOK
	return
}

// compareVersions 比较版本号：a>b 返回 1，a<b 返回 -1，相等返回 0
func compareVersions(a, b string) int {
	pa, pb := parseVersion(a), parseVersion(b)
	for i := range len(pa) {
		if pa[i] > pb[i] {
			return 1
		}
		if pa[i] < pb[i] {
			return -1
		}
	}
	return 0
}

// parseVersion 解析形如 16.18.2 / v16.18.2 的版本号，最多取前 3 段，缺省补 0
func parseVersion(v string) [3]int {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".")
	for i := 0; i < len(parts) && i < len(out); i++ {
		if n, err := strconv.Atoi(strings.TrimSpace(parts[i])); err == nil {
			out[i] = n
		}
	}
	return out
}

// 平台/架构关键词 — 包级常量，避免每次调用 pickBinaryForPlatform 重新分配
var osKeywords = map[string][]string{
	"windows": {"windows", "win64", "win32"},
	"linux":   {"linux"},
	"android": {"android", "apk"},
}

var archKeywords = map[string][]string{
	"amd64": {"amd64", "x86_64", "x64"},
	"arm64": {"arm64", "aarch64"},
	"arm":   {"arm", "armv7"},
	"386":   {"386", "i386"},
}

// 预构建的精确匹配模式（os_arch 和 os-arch），在 init 中一次性填充
var exactPatterns []string

func init() {
	for _, osList := range osKeywords {
		for _, os := range osList {
			for _, archList := range archKeywords {
				for _, ak := range archList {
					exactPatterns = append(exactPatterns, os+"_"+ak, os+"-"+ak)
				}
			}
		}
	}
}

// UpdateProgress 在线更新下载进度（返回给前端 / 通过 WS 推送）
type UpdateProgress struct {
	Status     string `json:"status"`     // idle / downloading / paused / completed / failed / installing
	Total      int64  `json:"total"`      // 文件总大小，0 表示未知
	Downloaded int64  `json:"downloaded"` // 已下载字节数
	Percent    int    `json:"percent"`    // 0-100
	Error      string `json:"error"`      // 失败原因
}

// updateManager 在线更新下载状态（单例，支持暂停/续传/断线重连）
type updateManager struct {
	mu      sync.Mutex
	status  string
	downURL string
	tag     string
	total   int64
	done    int64
	errMsg  string
	tmpFile string
	exePath string
	cancel  context.CancelFunc
	running bool
}

var upd = &updateManager{status: "idle"}

// getUpdateStatus 返回当前下载状态快照
func getUpdateStatus() UpdateProgress {
	upd.mu.Lock()
	defer upd.mu.Unlock()
	p := UpdateProgress{
		Status:     upd.status,
		Total:      upd.total,
		Downloaded: upd.done,
		Error:      upd.errMsg,
	}
	if upd.total > 0 {
		p.Percent = int(upd.done * 100 / upd.total)
	}
	if p.Percent < 0 {
		p.Percent = 0
	}
	if p.Percent > 100 {
		p.Percent = 100
	}
	return p
}

// broadcastUpdateProgress 通过 WS 推送下载进度给所有已连接的 OPUI 前端
func broadcastUpdateProgress(p UpdateProgress) {
	data, _ := json.Marshal(struct {
		Type string `json:"type"`
		UpdateProgress
	}{"update_progress", p})
	broadcastOpuiNotify(data)
}

// startOnlineUpdate 启动/继续在线更新下载（幂等：下载中则忽略，暂停则继续）
func startOnlineUpdate() {
	upd.mu.Lock()
	if upd.running {
		upd.mu.Unlock()
		return
	}
	if upd.status == "completed" || upd.status == "installing" {
		upd.mu.Unlock()
		return
	}
	upd.running = true
	ctx, cancel := context.WithCancel(context.Background())
	upd.cancel = cancel
	upd.mu.Unlock()

	go func() {
		defer func() {
			upd.mu.Lock()
			upd.running = false
			upd.mu.Unlock()
		}()
		runUpdateDownload(ctx)
	}()
}

// pauseOnlineUpdate 暂停下载（保留已下载的部分文件，可继续）
func pauseOnlineUpdate() {
	upd.mu.Lock()
	if upd.status == "downloading" || upd.status == "preparing" {
		upd.status = "paused"
	}
	cancel := upd.cancel
	upd.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	broadcastUpdateProgress(getUpdateStatus())
}

// runUpdateDownload 下载主流程：解析目标 → 循环下载（断线自动重连续传）→ 安装/替换重启
func runUpdateDownload(ctx context.Context) {
	// 1. 首次确定下载目标（release、临时文件、exe 路径），暂停续传时复用已有目标
	upd.mu.Lock()
	downURL, tmpFile, exePath := upd.downURL, upd.tmpFile, upd.exePath
	upd.mu.Unlock()

	if downURL == "" || tmpFile == "" {
		if err := prepareUpdateTargets(); err != nil {
			upd.mu.Lock()
			upd.status = "failed"
			upd.errMsg = err.Error()
			upd.mu.Unlock()
			broadcastUpdateProgress(getUpdateStatus())
			return
		}
		upd.mu.Lock()
		downURL, tmpFile, exePath = upd.downURL, upd.tmpFile, upd.exePath
		upd.mu.Unlock()
	}

	// 2. 下载循环：网络中断时保留已下载部分，退避后自动重连续传
	for {
		if ctx.Err() != nil {
			setUpdatePaused()
			return
		}
		err := downloadToFile(ctx, downURL, tmpFile)
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			setUpdatePaused()
			return
		}
		upd.mu.Lock()
		upd.status = "downloading"
		upd.errMsg = err.Error()
		upd.mu.Unlock()
		broadcastUpdateProgress(getUpdateStatus())

		// 退避等待，期间可被暂停中断
		select {
		case <-ctx.Done():
			setUpdatePaused()
			return
		case <-time.After(3 * time.Second):
		}
	}

	// 3. 下载完成
	upd.mu.Lock()
	upd.status = "completed"
	if upd.total > 0 {
		upd.done = upd.total
	}
	upd.mu.Unlock()
	broadcastUpdateProgress(getUpdateStatus())

	// 4. 安装（Android）/ 替换重启（桌面端）
	if runtime.GOOS == "android" && mobile.InstallApkFunc != nil {
		upd.mu.Lock()
		upd.status = "installing"
		upd.mu.Unlock()
		broadcastUpdateProgress(getUpdateStatus())
		if err := mobile.InstallApkFunc(tmpFile); err != nil {
			upd.mu.Lock()
			upd.status = "failed"
			upd.errMsg = err.Error()
			upd.mu.Unlock()
			broadcastUpdateProgress(getUpdateStatus())
		}
		return
	}
	upd.mu.Lock()
	upd.status = "installing"
	upd.mu.Unlock()
	broadcastUpdateProgress(getUpdateStatus())
	_ = replaceAndRestart(exePath, tmpFile)
}

func setUpdatePaused() {
	upd.mu.Lock()
	upd.status = "paused"
	upd.errMsg = ""
	upd.mu.Unlock()
	broadcastUpdateProgress(getUpdateStatus())
}

// prepareUpdateTargets 解析最新版本与当前平台下载直链，确定临时文件与 exe 路径
func prepareUpdateTargets() error {
	client := &http.Client{Timeout: 30 * time.Second}
	tag, downURL, ok := fetchPlatformBinary(client, "https://gitee.com/api/v5/repos/cjxpj/nebula/releases/latest")
	if !ok {
		tag, downURL, ok = fetchPlatformBinary(client, "https://api.github.com/repos/cjxpj/nebula/releases/latest")
	}
	if !ok {
		return fmt.Errorf("无法获取最新版本或未找到当前平台的二进制文件")
	}
	latest := strings.TrimPrefix(tag, "v")
	if compareVersions(latest, appfiles.Version) <= 0 {
		return fmt.Errorf("当前已是最新版本 (%s)", appfiles.Version)
	}

	var tmpFile, exePath string
	if runtime.GOOS == "android" {
		tmpFile = filepath.Join(utils.GetAppDir(), "nebula_update.apk")
	} else {
		var err error
		exePath, err = exec.LookPath(os.Args[0])
		if err != nil {
			return fmt.Errorf("无法获取程序路径: %v", err)
		}
		exePath, err = filepath.Abs(exePath)
		if err != nil {
			return fmt.Errorf("无法解析程序路径: %v", err)
		}
		tmpFile = filepath.Join(filepath.Dir(exePath), utils.GetAppDir(), filepath.Base(exePath)+".new")
	}
	// 首次下载清掉可能残留的旧文件，保证从零开始（后续暂停/续传/断线重连不再经过此处）
	_ = os.Remove(tmpFile)

	upd.mu.Lock()
	upd.tag = tag
	upd.downURL = downURL
	upd.tmpFile = tmpFile
	upd.exePath = exePath
	upd.status = "downloading"
	upd.mu.Unlock()
	return nil
}

// downloadToFile 单次下载：从已下载的偏移量断点续传（支持 HTTP Range），
// 写入期间持续更新进度；ctx 取消时返回 ctx.Err() 表示用户暂停。
func downloadToFile(ctx context.Context, downURL, tmpFile string) error {
	// 读取已下载大小作为续传偏移
	var offset int64
	if fi, err := os.Stat(tmpFile); err == nil {
		offset = fi.Size()
	}

	out, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开临时文件失败: %v", err)
	}
	defer out.Close()

	req, err := http.NewRequestWithContext(ctx, "GET", downURL, nil)
	if err != nil {
		return fmt.Errorf("创建下载请求失败: %v", err)
	}
	req.Header.Set("User-Agent", "Nebula-Client/1.0")
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载失败: %v", err)
	}
	defer resp.Body.Close()

	// 206 续传 / 200 全量（服务器不支持断点时从头开始）
	if resp.StatusCode == http.StatusPartialContent {
		if _, err := out.Seek(offset, io.SeekStart); err != nil {
			return fmt.Errorf("定位续传位置失败: %v", err)
		}
	} else if resp.StatusCode == http.StatusOK {
		if err := out.Truncate(0); err != nil {
			return fmt.Errorf("重置临时文件失败: %v", err)
		}
		if _, err := out.Seek(0, io.SeekStart); err != nil {
			return err
		}
		offset = 0
	} else {
		return fmt.Errorf("下载失败，HTTP 状态码: %d", resp.StatusCode)
	}

	total := int64(0)
	if resp.ContentLength > 0 {
		total = offset + resp.ContentLength
	}

	upd.mu.Lock()
	upd.status = "downloading"
	upd.total = total
	upd.done = offset
	upd.errMsg = ""
	upd.mu.Unlock()
	broadcastUpdateProgress(getUpdateStatus())

	buf := make([]byte, 64*1024)
	lastMobilePct := -1
	lastFrontPct := -1
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := out.Write(buf[:n]); err != nil {
				return fmt.Errorf("写入文件失败: %v", err)
			}
			upd.mu.Lock()
			upd.done += int64(n)
			done, totalNow := upd.done, upd.total
			upd.mu.Unlock()

			pct := -1
			if totalNow > 0 {
				pct = int(done * 100 / totalNow)
			}

			// 移动端通知栏：每 10% 回调一次
			if totalNow > 0 && mobile.UpdateDownloadProgressFunc != nil && pct != lastMobilePct && pct%10 == 0 {
				lastMobilePct = pct
				_ = mobile.UpdateDownloadProgressFunc(done, totalNow)
			}
			// 前端进度：每 1% 推送一次
			if pct != lastFrontPct {
				lastFrontPct = pct
				broadcastUpdateProgress(getUpdateStatus())
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("读取下载内容失败: %v", readErr)
		}
	}

	// 完整性校验
	upd.mu.Lock()
	done, totalNow := upd.done, upd.total
	upd.mu.Unlock()
	if totalNow > 0 && done < totalNow {
		return fmt.Errorf("下载不完整 (%d/%d)", done, totalNow)
	}
	return nil
}

// fetchPlatformBinary 请求 release 接口，返回 tag / 当前平台对应的二进制下载地址
func fetchPlatformBinary(client *http.Client, url string) (tag, downURL string, ok bool) {
	tag, _, _, assets := fetchReleaseAssets(client, url)
	if tag == "" {
		return "", "", false
	}
	u, ok := pickBinaryForPlatform(assets)
	return tag, u, ok
}

// fetchReleaseAssets 请求 release 接口，返回 tag_name / html_url / body / assets
func fetchReleaseAssets(client *http.Client, url string) (tag, htmlURL, body string, assets []struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", "Nebula-Client/1.0")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var data struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Body    string `json:"body"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return
	}
	return data.TagName, data.HTMLURL, data.Body, data.Assets
}

// pickBinaryForPlatform 从 assets 中选出当前平台对应的二进制文件下载地址
func pickBinaryForPlatform(assets []struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}) (downURL string, ok bool) {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// 仅支持已构建的平台和架构，其余返回 false（仅手动下载）
	osKeys, osOk := osKeywords[goos]
	archKeys, archOk := archKeywords[goarch]
	if !osOk || !archOk {
		return "", false
	}

	// 优先级：精确匹配 os_arch > 同时包含 os+arch > 仅包含 os > 仅包含 arch
	type match struct {
		url  string
		rank int
	}
	var best *match

	for _, a := range assets {
		lower := strings.ToLower(a.Name)
		if strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tar") {
			continue
		}

		osMatch, archMatch := false, false
		for _, k := range osKeys {
			if strings.Contains(lower, k) {
				osMatch = true
				break
			}
		}
		for _, k := range archKeys {
			if strings.Contains(lower, k) {
				archMatch = true
				break
			}
		}

		rank := 0
		if osMatch && archMatch {
			rank = 3
			// 精确匹配（如 "linux_amd64"）加分（预计算模式数组）
			for _, p := range exactPatterns {
				if strings.Contains(lower, p) {
					rank = 5
					break
				}
			}
		} else if osMatch {
			rank = 2
		} else if archMatch {
			rank = 1
		}

		if rank > 0 && (best == nil || rank > best.rank) {
			best = &match{url: a.BrowserDownloadURL, rank: rank}
		}
	}

	if best != nil {
		return best.url, true
	}

	// 关键词匹配失败，回退到扩展名匹配（本项目 Release 命名简洁：nebula / nebula.exe / nebula.apk）
	switch goos {
	case "windows":
		for _, a := range assets {
			if strings.HasSuffix(strings.ToLower(a.Name), ".exe") {
				return a.BrowserDownloadURL, true
			}
		}
	case "linux":
		for _, a := range assets {
			lower := strings.ToLower(a.Name)
			// Linux 二进制：无常见扩展名，排除 apk/exe/zip/tar.gz
			if !strings.HasSuffix(lower, ".exe") &&
				!strings.HasSuffix(lower, ".apk") &&
				!strings.HasSuffix(lower, ".zip") &&
				!strings.HasSuffix(lower, ".tar.gz") &&
				!strings.HasSuffix(lower, ".tar") &&
				!strings.HasPrefix(lower, "source") {
				return a.BrowserDownloadURL, true
			}
		}
	case "android":
		for _, a := range assets {
			if strings.HasSuffix(strings.ToLower(a.Name), ".apk") {
				return a.BrowserDownloadURL, true
			}
		}
	}
	return "", false
}
