package dic_server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

type HttpOpUiConfig_ngrok struct {
	Open       bool   `json:"open"`
	Token      string `json:"token"`
	Domain     string `json:"domain"`
	ServerAddr string `json:"server_addr"` // 要转发的本地 HTTP 服务器监听地址（空则核心服务器）
}

// Ngrok 内网穿透 API。
func init() {
	registerOpuiApi(opuiHandleNgrokAPI,
		"get_ngrok", "save_ngrok", "toggle_ngrok",
	)
}

// opuiHandleNgrokAPI 处理本模块注册的 OPUI API 类型。
func opuiHandleNgrokAPI(w http.ResponseWriter, r *http.Request, h *HttpOpUiData) {
	switch h.Type {
	case "get_ngrok":
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := cfg.Section("Ngrok")
		var j HttpOpUiConfig_ngrok
		j.Open = d.Key("启用").MustBool(false)
		j.Token = d.Key("密钥").String()
		j.Domain = d.Key("访问链接").String()
		j.ServerAddr = d.Key("服务器").String()
		r, _ := json.Marshal(j)
		w.Write(r)
		return

	case "save_ngrok":
		var j HttpOpUiConfig_ngrok
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := cfg.Section("Ngrok")
		d.Key("启用").SetValue(strconv.FormatBool(j.Open))
		d.Key("密钥").SetValue(j.Token)
		d.Key("访问链接").SetValue(j.Domain)
		d.Key("服务器").SetValue(j.ServerAddr)
		cfg.Save()

		// 保存即生效：按启用状态启停隧道（服务器地址变更时先停旧隧道再重连）
		if j.Open {
			if dto.ServerConfig.NgrokListener != nil || dto.ServerConfig.NgrokCancel != nil {
				StopNgrok()
			}
			dto.ServerConfig.Ngrok = &dto.NgrokConfig{
				Addr:       j.Domain,
				Token:      j.Token,
				ServerAddr: j.ServerAddr,
			}
			url, err := StartNgrok(j.Token, j.Domain)
			if err != nil {
				w.Write([]byte(`{"status":"error","error":"` + err.Error() + `"}`))
				return
			}
			w.Write([]byte(`{"status":"ok","url":"` + url + `"}`))
		} else {
			StopNgrok()
			dto.ServerConfig.Ngrok = nil
			w.Write([]byte(`{"status":"ok"}`))
		}
		return

	case "toggle_ngrok":
		var j struct {
			Open bool `json:"open"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := cfg.Section("Ngrok")
		d.Key("启用").SetValue(strconv.FormatBool(j.Open))
		cfg.Save()

		if j.Open {
			token := d.Key("密钥").String()
			domain := d.Key("访问链接").String()
			serverAddr := d.Key("服务器").String()
			dto.ServerConfig.Ngrok = &dto.NgrokConfig{
				Addr:       domain,
				Token:      token,
				ServerAddr: serverAddr,
			}
			url, err := StartNgrok(token, domain)
			if err != nil {
				w.Write([]byte(`{"status":"error","error":"` + err.Error() + `"}`))
				return
			}
			w.Write([]byte(`{"status":"ok","url":"` + url + `"}`))
		} else {
			StopNgrok()
			dto.ServerConfig.Ngrok = nil
			w.Write([]byte(`{"status":"ok"}`))
		}
		return

	}
}
