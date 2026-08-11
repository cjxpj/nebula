package dic_server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cjxpj/nebula/appfiles"
)

// UpdateCheckResult 更新检测结果
type UpdateCheckResult struct {
	Status  string `json:"status"`   // ok / error
	Current string `json:"current"`  // 当前版本
	Latest  string `json:"latest"`   // 最新版本
	Update  bool   `json:"update"`   // 是否有新版本
	URL     string `json:"url"`      // 下载页地址
	Notes   string `json:"notes"`    // 更新说明
	DownURL string `json:"down_url"` // 客户端安装包直链（.exe）
	Error   string `json:"error"`    // 检测失败原因
}

// checkUpdate 检测最新版本：优先 GitHub，失败则回退 Gitee（国内节点）
func checkUpdate() UpdateCheckResult {
	result := UpdateCheckResult{Status: "ok", Current: appfiles.Version}
	client := &http.Client{Timeout: 8 * time.Second}
	latest, releaseURL, downURL, notes, ok := fetchLatestRelease(client, "https://api.github.com/repos/cjxpj/nebula/releases/latest")
	if !ok {
		// GitHub 不可用，回退 Gitee（国内直连更快）
		latest, releaseURL, downURL, notes, ok = fetchLatestRelease(client, "https://gitee.com/api/v5/repos/cjxpj/nebula/releases/latest")
	}
	if !ok {
		result.Status = "error"
		result.Error = "无法获取最新版本，请检查网络后重试"
		return result
	}
	result.Latest = strings.TrimPrefix(latest, "v")
	result.URL = releaseURL
	if result.URL == "" {
		result.URL = "https://github.com/cjxpj/nebula/releases"
	}
	result.DownURL = downURL
	result.Notes = notes
	result.Update = compareVersions(result.Latest, appfiles.Version) > 0
	return result
}

// fetchLatestRelease 请求 release 接口，返回 tag_name / 下载页 / 安装包直链 / 更新说明
func fetchLatestRelease(client *http.Client, url string) (tag, htmlURL, downURL, notes string, ok bool) {
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
	htmlURL = data.HTMLURL
	// 优先取 Windows 客户端安装包（.exe），其次取第一个非源码包（非 .zip/.tar.gz）
	for _, a := range data.Assets {
		lower := strings.ToLower(a.Name)
		if strings.HasSuffix(lower, ".exe") {
			downURL = a.BrowserDownloadURL
			break
		}
	}
	if downURL == "" {
		for _, a := range data.Assets {
			lower := strings.ToLower(a.Name)
			if !strings.HasSuffix(lower, ".zip") && !strings.HasSuffix(lower, ".tar.gz") {
				downURL = a.BrowserDownloadURL
				break
			}
		}
	}
	// Gitee 接口不返回 html_url，按源补下载页地址
	if htmlURL == "" {
		if strings.Contains(url, "gitee.com") {
			htmlURL = "https://gitee.com/cjxpj/nebula/releases"
		} else {
			htmlURL = "https://github.com/cjxpj/nebula/releases"
		}
	}
	return data.TagName, htmlURL, downURL, data.Body, data.TagName != ""
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
