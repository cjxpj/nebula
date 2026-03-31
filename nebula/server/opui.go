package dic_server

import (
	"encoding/json"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
	"github.com/gorilla/websocket"
)

type HttpOpUiData struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type HttpOpUiConfig_server struct {
	Server      string `json:"server"`
	CORS        bool   `json:"cors"`
	CORSOrigins string `json:"cors_origins"`
}

type HttpOpUiConfig_websocket struct {
	Open      bool   `json:"open"`
	Cors      bool   `json:"cors"`
	WebSocket string `json:"websocket"`
}

type HttpOpUiConfig_ngrok struct {
	Open   bool   `json:"open"`
	Token  string `json:"token"`
	Domain string `json:"domain"`
}

type HttpOpUiConfig_qq struct {
	Open   bool   `json:"open"`
	Dic    string `json:"dic"`
	Path   string `json:"path"`
	Appid  string `json:"appid"`
	Secret string `json:"secret"`
}

type HttpOpUiConfig_napcat struct {
	Open   bool   `json:"open"`
	Dic    string `json:"dic"`
	Path   string `json:"path"`
	Api    string `json:"api"`
	Secret string `json:"secret"`
}

type HttpOpUiConfig_yunhu struct {
	Open   bool   `json:"open"`
	Dic    string `json:"dic"`
	Path   string `json:"path"`
	Secret string `json:"secret"`
}

type HttpOpUiConfig_feishu struct {
	Open   bool   `json:"open"`
	Dic    string `json:"dic"`
	Path   string `json:"path"`
	Appid  string `json:"appid"`
	Secret string `json:"secret"`
}

type HttpOpUiConfig_EncryptDic struct {
	Text string `json:"text"`
}

func OpUI(w http.ResponseWriter, r *http.Request, getpath string) {
	if getpath == "" {
		http.Redirect(w, r, dto.ServerConfig.OPUI.Addr+"/", http.StatusFound)
		return
	}

	if getpath == "/" {
		getpath = "/index.html"
	}

	if r.Method == http.MethodPost &&
		strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var h *HttpOpUiData
		if err := json.NewDecoder(r.Body).Decode(&h); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		switch h.Type {
		case "get_server":
			ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
			f, err := ff.LoadIni()
			if err != nil {
				utils.ErrorStop("系统配置不存在")
			}
			d := f.Section("HTTP")
			var j HttpOpUiConfig_server
			j.Server = d.Key("server").String()
			j.CORS = d.Key("跨域").MustBool(false)
			j.CORSOrigins = d.Key("跨域白名单").String()
			if r, err := json.Marshal(j); err == nil {
				w.Write(r)
			}
			return

		case "save_server":
			var j HttpOpUiConfig_server
			if err := json.Unmarshal(h.Data, &j); err != nil {
				http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
				return
			}
			ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
			f, err := ff.LoadIni()
			if err != nil {
				utils.ErrorStop("系统配置不存在")
			}
			d := f.Section("HTTP")
			d.Key("server").SetValue(j.Server)
			d.Key("跨域").SetValue(strconv.FormatBool(j.CORS))
			d.Key("跨域白名单").SetValue(j.CORSOrigins)
			if err := ff.SaveIni(f); err != nil {
				utils.ErrorStop("系统配置保存失败")
			}
			dto.ServerConfig.Router.Cors = j.CORS
			dto.ServerConfig.Router.CorsOrigins = j.CORSOrigins
			// 处理配置请求
			w.Write([]byte(`{"status":"ok"}`))
			return

		case "get_websocket":
			ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
			f, err := ff.LoadIni()
			if err != nil {
				utils.ErrorStop("系统配置不存在")
			}
			d := f.Section("WebSocket")
			var j HttpOpUiConfig_websocket
			j.Open = d.Key("启用").MustBool(false)
			j.Cors = d.Key("跨域").MustBool(false)
			j.WebSocket = d.Key("访问路径").String()
			r, _ := json.Marshal(j)
			w.Write(r)
			return

		case "save_websocket":
			var j HttpOpUiConfig_websocket
			if err := json.Unmarshal(h.Data, &j); err != nil {
				http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
				return
			}
			ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
			f, err := ff.LoadIni()
			if err != nil {
				utils.ErrorStop("系统配置不存在")
			}
			d := f.Section("WebSocket")
			dto.ServerConfig.Ws = &dto.ServerRouterWebSocket{
				Open: j.Open,
				Addr: "/" + j.WebSocket,
				Conn: &websocket.Upgrader{
					CheckOrigin: func(r *http.Request) bool {
						// 跨域连接
						return j.Cors
					},
				},
			}
			d.Key("启用").SetValue(strconv.FormatBool(j.Open))
			d.Key("跨域").SetValue(strconv.FormatBool(j.Cors))
			d.Key("访问路径").SetValue(j.WebSocket)
			ff.SaveIni(f)
			w.Write([]byte(`{"status":"ok"}`))
			return

		case "get_ngrok":
			ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
			f, err := ff.LoadIni()
			if err != nil {
				utils.ErrorStop("系统配置不存在")
			}
			d := f.Section("Ngrok")
			var j HttpOpUiConfig_ngrok
			j.Open = d.Key("启用").MustBool(false)
			j.Token = d.Key("密钥").String()
			j.Domain = d.Key("访问链接").String()
			r, _ := json.Marshal(j)
			w.Write(r)
			return

		case "save_ngrok":
			var j HttpOpUiConfig_ngrok
			if err := json.Unmarshal(h.Data, &j); err != nil {
				http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
				return
			}
			ff := utils.NewFileQueue(dto.CONFIG_SYSTEM_PATH)
			f, err := ff.LoadIni()
			if err != nil {
				utils.ErrorStop("系统配置不存在")
			}
			d := f.Section("Ngrok")
			d.Key("启用").SetValue(strconv.FormatBool(j.Open))
			d.Key("密钥").SetValue(j.Token)
			d.Key("访问链接").SetValue(j.Domain)
			ff.SaveIni(f)
			w.Write([]byte(`{"status":"ok"}`))
			return

		case "get_qq":
			ff := utils.NewFileQueue(dto.CONFIG_PATH)
			f, err := ff.LoadIni()
			if err != nil {
				utils.ErrorStop("系统配置不存在")
			}
			d := f.Section("QQ")
			var j HttpOpUiConfig_qq
			j.Open = d.Key("启用").MustBool(false)
			j.Dic = d.Key("词库").String()
			j.Path = d.Key("访问路径").String()
			j.Appid = d.Key("APPID").String()
			j.Secret = d.Key("密钥").String()
			r, _ := json.Marshal(j)
			w.Write(r)
			return

		case "save_qq":
			var j HttpOpUiConfig_qq
			if err := json.Unmarshal(h.Data, &j); err != nil {
				http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
				return
			}
			ff := utils.NewFileQueue(dto.CONFIG_PATH)
			f, err := ff.LoadIni()
			if err != nil {
				utils.ErrorStop("系统配置不存在")
			}
			d := f.Section("QQ")
			d.Key("启用").SetValue(strconv.FormatBool(j.Open))
			d.Key("词库").SetValue(j.Dic)
			d.Key("访问路径").SetValue(j.Path)
			d.Key("APPID").SetValue(j.Appid)
			d.Key("密钥").SetValue(j.Secret)
			dto.LoadConfig_qq(d)
			ff.SaveIni(f)
			w.Write([]byte(`{"status":"ok"}`))
			return

		case "get_napcat":
			ff := utils.NewFileQueue(dto.CONFIG_PATH)
			f, err := ff.LoadIni()
			if err != nil {
				utils.ErrorStop("系统配置不存在")
			}
			d := f.Section("NapCat")
			var j HttpOpUiConfig_napcat
			j.Open = d.Key("启用").MustBool(false)
			j.Dic = d.Key("词库").String()
			j.Path = d.Key("访问路径").String()
			j.Secret = d.Key("密钥").String()
			j.Api = d.Key("发送消息接口").String()
			r, _ := json.Marshal(j)
			w.Write(r)
			return

		case "save_napcat":
			var j HttpOpUiConfig_napcat
			if err := json.Unmarshal(h.Data, &j); err != nil {
				http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
				return
			}
			ff := utils.NewFileQueue(dto.CONFIG_PATH)
			f, err := ff.LoadIni()
			if err != nil {
				utils.ErrorStop("系统配置不存在")
			}
			d := f.Section("NapCat")
			d.Key("启用").SetValue(strconv.FormatBool(j.Open))
			d.Key("词库").SetValue(j.Dic)
			d.Key("访问路径").SetValue(j.Path)
			d.Key("密钥").SetValue(j.Secret)
			d.Key("发送消息接口").SetValue(j.Api)
			dto.LoadConfig_napcat(d)
			ff.SaveIni(f)
			w.Write([]byte(`{"status":"ok"}`))
			return

		case "get_yunhu":
			ff := utils.NewFileQueue(dto.CONFIG_PATH)
			f, err := ff.LoadIni()
			if err != nil {
				utils.ErrorStop("系统配置不存在")
			}
			d := f.Section("云湖")
			var j HttpOpUiConfig_yunhu
			j.Open = d.Key("启用").MustBool(false)
			j.Dic = d.Key("词库").String()
			j.Path = d.Key("访问路径").String()
			j.Secret = d.Key("密钥").String()
			r, _ := json.Marshal(j)
			w.Write(r)
			return

		case "save_yunhu":
			var j HttpOpUiConfig_yunhu
			if err := json.Unmarshal(h.Data, &j); err != nil {
				http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
				return
			}
			ff := utils.NewFileQueue(dto.CONFIG_PATH)
			f, err := ff.LoadIni()
			if err != nil {
				utils.ErrorStop("系统配置不存在")
			}
			d := f.Section("云湖")
			d.Key("启用").SetValue(strconv.FormatBool(j.Open))
			d.Key("词库").SetValue(j.Dic)
			d.Key("访问路径").SetValue(j.Path)
			d.Key("密钥").SetValue(j.Secret)
			dto.LoadConfig_yunhu(d)
			ff.SaveIni(f)
			w.Write([]byte(`{"status":"ok"}`))
			return

		case "get_feishu":
			ff := utils.NewFileQueue(dto.CONFIG_PATH)
			f, err := ff.LoadIni()
			if err != nil {
				utils.ErrorStop("系统配置不存在")
			}
			d := f.Section("飞书")
			var j HttpOpUiConfig_feishu
			j.Open = d.Key("启用").MustBool(false)
			j.Dic = d.Key("词库").String()
			j.Path = d.Key("访问路径").String()
			j.Appid = d.Key("APPID").String()
			j.Secret = d.Key("密钥").String()
			r, _ := json.Marshal(j)
			w.Write(r)
			return

		case "save_feishu":
			var j HttpOpUiConfig_feishu
			if err := json.Unmarshal(h.Data, &j); err != nil {
				http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
				return
			}
			ff := utils.NewFileQueue(dto.CONFIG_PATH)
			f, err := ff.LoadIni()
			if err != nil {

				utils.ErrorStop("系统配置不存在")
			}
			d := f.Section("飞书")
			d.Key("启用").SetValue(strconv.FormatBool(j.Open))
			d.Key("词库").SetValue(j.Dic)
			d.Key("访问路径").SetValue(j.Path)
			d.Key("APPID").SetValue(j.Appid)
			d.Key("密钥").SetValue(j.Secret)
			dto.LoadConfig_feishu(d)
			ff.SaveIni(f)
			w.Write([]byte(`{"status":"ok"}`))
			return

		case "encrypt_dic":
			var j HttpOpUiConfig_EncryptDic
			if err := json.Unmarshal(h.Data, &j); err != nil {
				http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
				return
			}
			encodeDic, err := utils.Encrypt(j.Text, appfiles.Key)
			if err != nil {
				http.Error(w, `{"status":"error","error":"加密失败"}`, http.StatusBadRequest)
				return
			}
			var rj struct {
				Status string `json:"status"`
				Text   string `json:"text"`
			}
			rj.Status = "ok"
			rj.Text = encodeDic
			r, _ := json.Marshal(rj)
			w.Write(r)
			return

		default:
			http.Error(w, `{"status":"error","error":"invalid type"}`, http.StatusBadRequest)
			return
		}
	}

	fullPath := "dic/public/opui" + getpath

	// ===== 设置 Content-Type（关键）=====
	ext := filepath.Ext(fullPath)
	if ct := mime.TypeByExtension(ext); ct != "" {
		w.Header().Set("Content-Type", ct)
	}

	data := appfiles.GetFile(fullPath)
	if data == nil {
		http.NotFound(w, r)
		return
	}

	if _, err := w.Write(data); err != nil {
		utils.Error("服务器输出 Error: " + err.Error())
	}
}
