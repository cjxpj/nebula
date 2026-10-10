package dic_server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

const ipBlacklistFile = "private/system/ip_blacklist.json"

var (
	ipBlacklist   = make(map[string]bool)
	ipBlacklistMu sync.Mutex
)

func LoadIPBlacklist() {
	ff := utils.NewFileQueue(ipBlacklistFile)
	data, err := ff.ReadFromFile()
	if err != nil {
		return
	}
	var list []string
	if json.Unmarshal([]byte(data), &list) == nil {
		ipBlacklistMu.Lock()
		ipBlacklist = make(map[string]bool, len(list))
		for _, ip := range list {
			ipBlacklist[strings.TrimSpace(ip)] = true
		}
		ipBlacklistMu.Unlock()
	}
}

func saveIPBlacklist() {
	ipBlacklistMu.Lock()
	list := make([]string, 0, len(ipBlacklist))
	for ip := range ipBlacklist {
		list = append(list, ip)
	}
	ipBlacklistMu.Unlock()
	data, _ := json.Marshal(list)
	utils.NewFileQueue(ipBlacklistFile).WriteToFile(string(data))
}

// ---------- 防火墙（词库实现） ----------

// CheckFirewall IP黑名单 + 防火墙词库检查，返回 true 表示已拦截（已写入响应）
func CheckFirewall(w http.ResponseWriter, r *http.Request) bool {
	ip := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		ip = strings.Split(fwd, ",")[0]
	}
	if real := r.Header.Get("X-Real-IP"); real != "" {
		ip = real
	}
	ip = strings.TrimSpace(ip)
	if colon := strings.LastIndex(ip, ":"); colon > 0 && !strings.Contains(ip, ".") {
		// IPv6: [::1]:1234, 处理纯 IPv6
	} else if colon > 0 {
		ip = ip[:colon] // strip port
	}

	// IP 黑名单检查
	ipBlacklistMu.Lock()
	blocked := ipBlacklist[ip]
	ipBlacklistMu.Unlock()
	if blocked {
		http.Error(w, "403 Forbidden: IP is blacklisted", http.StatusForbidden)
		return true
	}

	// 防火墙词库检查
	cfg, err := dto.LoadConfigFile()
	if err != nil {
		return false
	}
	fwSec := cfg.Section("防火墙")
	if !fwSec.Key("启用").MustBool(false) {
		return false
	}
	dicPath := fwSec.Key("词库").String()
	if dicPath == "" {
		return false
	}

	fileData, err := utils.NewFileQueue(dicPath).ReadFromFile()
	if err != nil {
		return false
	}

	dic := dic_dto.NewDic(dicPath, fileData)
	dic.Val.G.Set("IP", ip)
	dic.Val.G.Set("路径", r.URL.Path)
	dic.Val.G.Set("方法", r.Method)
	dic.Val.G.Set("UA", r.UserAgent())
	dic.Val.G.Set("请求头", fmt.Sprintf("%v", r.Header))

	result := dic_api.Api.DicRun(dic, "检查")
	if result != "" {
		// 对输出结果做最终变量插值，确保 %IP%、%路径% 等变量被正确替换
		result = utils.AnyToString(dic.Val.Text(result))
		if strings.HasPrefix(result, "放行") {
			return false
		}
		http.Error(w, result, http.StatusForbidden)
		return true
	}

	return false
}

// FTP / SFTP 服务管理

// IP 黑名单 / 防火墙 API。
func init() {
	registerOpuiApi(opuiHandleNetAPI,
		"ip_blacklist_list", "ip_blacklist_add", "ip_blacklist_remove", "firewall_get_config",
		"firewall_save_config",
	)
}

// opuiHandleNetAPI 处理本模块注册的 OPUI API 类型。
func opuiHandleNetAPI(w http.ResponseWriter, r *http.Request, h *HttpOpUiData) {
	switch h.Type {
	case "ip_blacklist_list":
		ipBlacklistMu.Lock()
		list := make([]string, 0, len(ipBlacklist))
		for ip := range ipBlacklist {
			list = append(list, ip)
		}
		ipBlacklistMu.Unlock()
		r, _ := json.Marshal(list)
		w.Write(r)
		return

	case "ip_blacklist_add":
		var j struct {
			IP string `json:"ip"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil || j.IP == "" {
			http.Error(w, `{"status":"error","error":"invalid ip"}`, http.StatusBadRequest)
			return
		}
		ipBlacklistMu.Lock()
		ipBlacklist[strings.TrimSpace(j.IP)] = true
		ipBlacklistMu.Unlock()
		saveIPBlacklist()
		// 广播安全事件
		notifyData, _ := json.Marshal(map[string]string{
			"type":   "ip_blacklist",
			"action": "add",
			"ip":     j.IP,
		})
		broadcastOpuiNotify(notifyData)
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "ip_blacklist_remove":
		var j struct {
			IP string `json:"ip"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil || j.IP == "" {
			http.Error(w, `{"status":"error","error":"invalid ip"}`, http.StatusBadRequest)
			return
		}
		ipBlacklistMu.Lock()
		delete(ipBlacklist, strings.TrimSpace(j.IP))
		ipBlacklistMu.Unlock()
		saveIPBlacklist()
		notifyData, _ := json.Marshal(map[string]string{
			"type":   "ip_blacklist",
			"action": "remove",
			"ip":     j.IP,
		})
		broadcastOpuiNotify(notifyData)
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "firewall_get_config":
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			w.Write([]byte(`{"enabled":false,"dic_path":""}`))
			return
		}
		fwSec := cfg.Section("防火墙")
		enabled := fwSec.Key("启用").MustBool(false)
		dicPath := fwSec.Key("词库").String()
		r, _ := json.Marshal(map[string]any{
			"enabled":  enabled,
			"dic_path": dicPath,
		})
		w.Write(r)
		return

	case "firewall_save_config":
		var j struct {
			Enabled bool   `json:"enabled"`
			DicPath string `json:"dic_path"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			http.Error(w, `{"status":"error","error":"config load failed"}`, http.StatusInternalServerError)
			return
		}
		cfg.Section("防火墙").Key("启用").SetValue(strconv.FormatBool(j.Enabled))
		cfg.Section("防火墙").Key("词库").SetValue(j.DicPath)
		cfg.Save()
		w.Write([]byte(`{"status":"ok"}`))
		return
	}
}
