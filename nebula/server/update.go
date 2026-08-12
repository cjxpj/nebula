package dic_server

import (
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
	downURL, _ = pickBinaryForPlatform(assets)

	// Gitee 接口不返回 html_url，按源补下载页地址
	if htmlURL == "" {
		if strings.Contains(url, "gitee.com") {
			htmlURL = "https://gitee.com/cjxpj/nebula/releases"
		} else {
			htmlURL = "https://github.com/cjxpj/nebula/releases"
		}
	}
	ok = true
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

// progressWriter 下载进度写入器，每 10% 回调一次移动端通知栏进度
type progressWriter struct {
	total   int64
	written int64
	lastPct int
}

func (w *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	w.written += int64(n)
	pct := int(w.written * 100 / w.total)
	if pct != w.lastPct && pct%10 == 0 {
		w.lastPct = pct
		mobile.UpdateDownloadProgressFunc(w.written, w.total)
	}
	return n, nil
}

// doOnlineUpdate 下载最新版本并自动重启
func doOnlineUpdate() error {
	// 1. 获取最新 release：优先 Gitee（国内快），失败回退 GitHub
	client := &http.Client{Timeout: 30 * time.Second}
	tag, downURL, ok := fetchPlatformBinary(client, "https://gitee.com/api/v5/repos/cjxpj/nebula/releases/latest")
	if !ok {
		tag, downURL, ok = fetchPlatformBinary(client, "https://api.github.com/repos/cjxpj/nebula/releases/latest")
	}
	if !ok {
		return fmt.Errorf("无法获取最新版本或未找到当前平台的二进制文件")
	}

	// 2. 比较版本，确认有更新
	latest := strings.TrimPrefix(tag, "v")
	if compareVersions(latest, appfiles.Version) <= 0 {
		return fmt.Errorf("当前已是最新版本 (%s)", appfiles.Version)
	}

	// 3. 确定下载路径（Android 直接放 NebulaData，桌面端放 exe 同级 NebulaData）
	var tmpFile string
	var exePath string
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

	// 使用带超时的 client 下载，避免无限挂起
	dlClient := &http.Client{Timeout: 10 * time.Minute}
	req, err := http.NewRequest("GET", downURL, nil)
	if err != nil {
		return fmt.Errorf("创建下载请求失败: %v", err)
	}
	req.Header.Set("User-Agent", "Nebula-Client/1.0")
	resp, err := dlClient.Do(req)
	if err != nil {
		return fmt.Errorf("下载失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败，HTTP 状态码: %d", resp.StatusCode)
	}

	out, err := os.Create(tmpFile)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %v", err)
	}
	defer out.Close()

	total := resp.ContentLength
	var writer io.Writer = out

	// 手机端：通知栏显示下载进度
	if total > 0 && mobile.UpdateDownloadProgressFunc != nil {
		pr := &progressWriter{
			total:   total,
			written: 0,
			lastPct: -1,
		}
		writer = io.MultiWriter(out, pr)
	}

	_, err = io.Copy(writer, resp.Body)
	if err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("写入文件失败: %v", err)
	}
	out.Close()
	resp.Body.Close()

	// 5. 手机端弹出安装，桌面端替换重启
	if runtime.GOOS == "android" && mobile.InstallApkFunc != nil {
		return mobile.InstallApkFunc(tmpFile)
	}
	return replaceAndRestart(exePath, tmpFile)
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
