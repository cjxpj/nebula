package dic_server

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cjxpj/nebula/debugLog"
	dic_funcs "github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
	"github.com/gorilla/websocket"
)

// ---------- 云工具（NebulaCloudTool）后端代理状态 ----------

var (
	cloudToolMu            sync.Mutex
	cloudToolConn          *websocket.Conn
	cloudToolWriteMu       sync.Mutex
	cloudToolDebug         bool
	cloudToolSeq           atomic.Uint64
	cloudToolPending       sync.Map // id(string) -> chan cloudToolMsg
	cloudToolUsername      string
	cloudToolToken         string
	cloudToolWantConn      bool        // 是否期望保持云工具连接（登录成功后 true，登出/主动断开 false）
	cloudToolReconnecting  atomic.Bool // 是否正在断线重连中，防止并发重连
	cloudToolOfflineNotify = true      // 云工具断开时是否推送离线警告（来自云工具端，默认开启）
	cloudToolMaxDevices    = 1         // 当前账号最大在线设备数（来自云工具端，默认 1，0 表示无限制）
	cloudToolEmail         = ""        // 当前账号绑定的邮箱（来自云工具端，空表示未绑定）

	cloudToolGen        atomic.Uint64 // 连接代次，每次成功连接递增，用于断开时回收对应注入的云函数
	cloudToolInjectedMu sync.Mutex
	cloudToolInjected   = make(map[string]uint64) // 已注入的云函数名 -> 注入时的连接代次

	cloudToolFuncsMu  sync.RWMutex
	cloudToolFuncsMap = map[string]cloudFuncInfo{} // 云函数完整信息缓存（连接成功后由 list_funcs 写入，供前端复用）
)

// 云工具（NebulaCloudTool）后端代理 API。
func init() {
	registerOpuiApi(opuiHandleCloudAPI,
		"get_cloud_tool", "get_cloud_funcs", "cloud_account_info", "save_cloud_tool",
		"get_cloudtool_server", "save_cloudtool_server", "reload_cloudtool_dic", "get_cloudtool_accounts",
		"disconnect_cloudtool_account", "delete_cloudtool_account", "get_cloudtool_whitelist", "add_cloudtool_whitelist",
		"remove_cloudtool_whitelist", "rename_cloudtool_account", "reset_cloudtool_password", "set_cloudtool_balance",
		"set_cloudtool_reviewer", "clear_cloudtool_accounts", "cloud_connect", "cloud_resume",
		"cloud_disconnect", "cloud_cancel_reconnect", "cloud_exec", "cloud_shop_list",
		"cloud_review_list", "cloud_review_action", "cloud_shop_buy", "cloud_shop_download",
		"cloud_shop_publish", "cloud_image_upload", "cloud_list_resources", "cloud_review_resource",
		"cloud_resource_content", "cloud_change_password", "cloud_change_username", "cloud_set_offline_notify",
		"cloud_set_max_devices", "cloud_bind_email", "cloud_confirm_email", "cloud_delete_account",
		"cloud_logout",
	)
}

// opuiHandleCloudAPI 处理云工具相关的前后端代理请求。
func opuiHandleCloudAPI(w http.ResponseWriter, r *http.Request, h *HttpOpUiData) {
	switch h.Type {
	case "get_cloud_tool":
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			w.Write([]byte(`{"addr":"","connected":false,"debug":false}`))
			return
		}
		var j HttpOpUiConfig_cloud_tool
		j.Addr = cloudToolAddr()
		j.Connected = cloudToolConnected()
		j.Reconnecting = cloudToolReconnecting.Load()
		j.Debug = cfg.Section("云工具").Key("调试").MustBool(false)
		j.OfflineNotify = cloudToolGetOfflineNotify()
		j.MaxDevices = cloudToolGetMaxDevices()
		j.Email = cloudToolGetEmail()
		cloudToolDebug = j.Debug
		r, err := json.Marshal(j)
		if err != nil {
			w.Write([]byte(`{"addr":"","connected":false,"debug":false}`))
			return
		}
		w.Write(r)
		return

	case "get_cloud_funcs":
		infos := getCloudFuncs()
		if infos == nil {
			// 缓存未就绪（连接刚建立尚未完成 list_funcs），兜底拉取一次并缓存
			var ok bool
			infos, ok = fetchCloudFuncs()
			if !ok {
				resp, _ := json.Marshal(map[string]string{"status": "error", "error": "获取云函数列表失败"})
				w.Write(resp)
				return
			}
		}
		resp, _ := json.Marshal(map[string]any{"status": "ok", "funcs": infos})
		w.Write(resp)
		return

	case "cloud_account_info":
		res, err := cloudToolCall(cloudToolMsg{Type: "account_info"}, 15*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		var info struct {
			DiskSize      int64  `json:"disk_size"`
			Money         string `json:"money"`
			Coupon        string `json:"coupon"`
			TotalMoney    string `json:"total_money"`
			OnlineSeconds int64  `json:"online_seconds"`
			Reviewer      bool   `json:"reviewer"`
			Devices       []struct {
				IP    string `json:"ip"`
				Start int64  `json:"start"`
			} `json:"devices"`
		}
		if err := json.Unmarshal([]byte(res.Data), &info); err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]any{
			"status":         "ok",
			"disk_size":      info.DiskSize,
			"money":          info.Money,
			"coupon":         info.Coupon,
			"total_money":    info.TotalMoney,
			"online_seconds": info.OnlineSeconds,
			"reviewer":       info.Reviewer,
			"devices":        info.Devices,
		})
		w.Write(resp)
		return

	case "save_cloud_tool":
		var j HttpOpUiConfig_cloud_tool
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		newAddr := strings.TrimSpace(j.Addr)
		oldAddr := strings.TrimSpace(cfg.Section("云工具").Key("连接地址").String())
		cfg.Section("云工具").Key("连接地址").SetValue(newAddr)
		cfg.Section("云工具").Key("调试").SetValue(strconv.FormatBool(j.Debug))
		cloudToolDebug = j.Debug
		cfg.Save()
		if newAddr != oldAddr {
			// 地址变化：断开旧连接，等待下次登录/断线重连使用新地址
			cloudToolSetWantConn(false)
			cloudToolDisconnect()
			broadcastCloudToolStatus(false)
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "get_cloudtool_server":
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := cfg.Section("云工具服务端")
		var j HttpOpUiConfig_cloudtool_server
		j.Open = d.Key("启用").MustBool(false)
		j.Addr = d.Key("访问路径").String()
		if j.Addr == "" {
			j.Addr = "cloudtool"
		}
		j.AllowRegister = d.Key("任意账号注册").MustBool(true)
		j.DicDir = d.Key("词库目录").String()
		if j.DicDir == "" {
			j.DicDir = "cloudtool"
		}
		j.LogoutSec = d.Key("断开注销时长").MustInt(30)
		j.Debug = d.Key("调试").MustBool(false)
		j.ShopOpen = d.Key("词库商城").MustBool(false)
		j.ImageHost = d.Key("图床").MustBool(false)
		j.ShopReview = d.Key("词库审核").MustBool(true)
		j.ImageReview = d.Key("图床审核").MustBool(true)
		r, _ := json.Marshal(j)
		w.Write(r)
		return

	case "save_cloudtool_server":
		var j HttpOpUiConfig_cloudtool_server
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		addr := strings.TrimPrefix(strings.TrimSpace(j.Addr), "/")
		if addr == "" {
			addr = "cloudtool"
		}
		dicDir := strings.TrimSpace(j.DicDir)
		if dicDir == "" {
			dicDir = "cloudtool"
		}
		if j.LogoutSec < 0 {
			j.LogoutSec = 0
		}
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := cfg.Section("云工具服务端")
		d.Key("启用").SetValue(strconv.FormatBool(j.Open))
		d.Key("访问路径").SetValue(addr)
		d.Key("任意账号注册").SetValue(strconv.FormatBool(j.AllowRegister))
		d.Key("词库目录").SetValue(dicDir)
		d.Key("断开注销时长").SetValue(strconv.Itoa(j.LogoutSec))
		d.Key("调试").SetValue(strconv.FormatBool(j.Debug))
		d.Key("词库商城").SetValue(strconv.FormatBool(j.ShopOpen))
		d.Key("图床").SetValue(strconv.FormatBool(j.ImageHost))
		d.Key("词库审核").SetValue(strconv.FormatBool(j.ShopReview))
		d.Key("图床审核").SetValue(strconv.FormatBool(j.ImageReview))
		cfg.Save()

		// 更新内存配置并即时生效（Open=false 时仅拒绝新连接）；白名单由白名单配置页单独管理，不在此覆盖
		dto.ServerConfig.CloudTool = &dto.CloudTool{
			Open:          j.Open,
			Addr:          "/" + addr,
			AllowRegister: j.AllowRegister,
			Whitelist:     CloudToolSplitWhitelist(d.Key("白名单").String()),
			DicDir:        dicDir,
			LogoutSec:     j.LogoutSec,
			Debug:         j.Debug,
			ShopOpen:      j.ShopOpen,
			ImageHost:     j.ImageHost,
			ShopReview:    j.ShopReview,
			ImageReview:   j.ImageReview,
		}
		StartCloudToolServer()
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "reload_cloudtool_dic":
		// 热更新云工具词库：重新编译词库目录并通知全部在线连接重新拉取云函数，无需重启或断开连接
		if err := CloudToolReloadDic(); err != nil {
			b, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			http.Error(w, string(b), http.StatusBadRequest)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "get_cloudtool_accounts":
		// 云工具服务端账号列表（分页 + 账号模糊搜索，含实时连接状态）
		var req struct {
			Page     int    `json:"page"`
			PageSize int    `json:"page_size"`
			Keyword  string `json:"keyword"`
		}
		if err := json.Unmarshal(h.Data, &req); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if req.Page < 1 {
			req.Page = 1
		}
		if req.PageSize <= 0 {
			req.PageSize = 20
		}
		if req.PageSize > 100 {
			req.PageSize = 100
		}
		items, total, err := CloudToolListAccounts(req.Keyword, req.Page, req.PageSize)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]any{"status": "ok", "items": items, "total": total})
		w.Write(resp)
		return

	case "disconnect_cloudtool_account":
		// 强制断开云工具账号（远程下线）
		var j struct {
			Username string `json:"username"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if err := CloudToolDisconnectAccount(j.Username); err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "delete_cloudtool_account":
		// 删除云工具账号（可附带删除云工具余额，默认删除）
		var j struct {
			Username      string `json:"username"`
			DeleteBalance bool   `json:"delete_balance"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if err := CloudToolDeleteAccount(j.Username, j.DeleteBalance); err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "get_cloudtool_whitelist":
		// 云工具服务端白名单列表（分页 + 账号模糊搜索）
		var req struct {
			Page     int    `json:"page"`
			PageSize int    `json:"page_size"`
			Keyword  string `json:"keyword"`
		}
		if err := json.Unmarshal(h.Data, &req); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if req.Page < 1 {
			req.Page = 1
		}
		if req.PageSize <= 0 {
			req.PageSize = 20
		}
		if req.PageSize > 100 {
			req.PageSize = 100
		}
		items, total, err := CloudToolListWhitelist(req.Keyword, req.Page, req.PageSize)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]any{"status": "ok", "items": items, "total": total})
		w.Write(resp)
		return

	case "add_cloudtool_whitelist":
		// 把账号加入云工具服务端白名单
		var j struct {
			Username string `json:"username"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		whitelisted, err := CloudToolAddWhitelist(j.Username)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]any{"status": "ok", "whitelisted": whitelisted})
		w.Write(resp)
		return

	case "remove_cloudtool_whitelist":
		// 把账号移出云工具服务端白名单
		var j struct {
			Username string `json:"username"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		whitelisted, err := CloudToolRemoveWhitelist(j.Username)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]any{"status": "ok", "whitelisted": whitelisted})
		w.Write(resp)
		return

	case "rename_cloudtool_account":
		// 重命名云工具服务端账号（在线连接将先被断开，白名单自动迁移）
		var j struct {
			Username string `json:"username"`
			NewName  string `json:"new_name"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if err := CloudToolRenameAccount(j.Username, j.NewName); err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "reset_cloudtool_password":
		// 重置云工具服务端账号密码（在线连接将先被断开）
		var j struct {
			Username    string `json:"username"`
			NewPassword string `json:"new_password"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if err := CloudToolResetPassword(j.Username, j.NewPassword); err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "set_cloudtool_balance":
		// 直接设置云工具账号余额（面板点击余额编辑，不存在则创建）
		var j struct {
			Username string `json:"username"`
			Balance  int64  `json:"balance"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if err := CloudToolSetBalance(j.Username, j.Balance); err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "set_cloudtool_reviewer":
		// 授予/取消云工具账号的审核权限（拥有审核权限的账号可在客户端审核他人上传的内容）
		var j struct {
			Username string `json:"username"`
			Reviewer bool   `json:"reviewer"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if err := CloudToolSetReviewer(j.Username, j.Reviewer); err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]any{"status": "ok", "reviewer": j.Reviewer})
		w.Write(resp)
		return

	case "clear_cloudtool_accounts":
		// 清空全部云工具账号（白名单内账号保留）
		deleted, kept, err := CloudToolClearAccounts()
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]any{"status": "ok", "deleted": deleted, "kept": kept})
		w.Write(resp)
		return

	case "cloud_connect":
		var j struct {
			Username string `json:"username"`
			Password string `json:"password"` // 已 SHA-256 加密后的密码哈希
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if j.Username == "" || j.Password == "" {
			http.Error(w, `{"status":"error","error":"账号、密码均不能为空"}`, http.StatusBadRequest)
			return
		}
		token, offlineNotify, email, debug, err := cloudToolAuth(j.Username, j.Password, "")
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]any{
			"status":         "ok",
			"connected":      true,
			"token":          token,
			"offline_notify": offlineNotify,
			"debug":          debug,
			"max_devices":    cloudToolGetMaxDevices(),
			"email":          email,
		})
		w.Write(resp)
		return

	case "cloud_resume":
		var j struct {
			Username string `json:"username"`
			Token    string `json:"token"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if j.Username == "" || j.Token == "" {
			http.Error(w, `{"status":"error","error":"账号、token 均不能为空"}`, http.StatusBadRequest)
			return
		}
		token, offlineNotify, email, debug, err := cloudToolAuth(j.Username, "", j.Token)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]any{
			"status":         "ok",
			"connected":      true,
			"token":          token,
			"offline_notify": offlineNotify,
			"debug":          debug,
			"max_devices":    cloudToolGetMaxDevices(),
			"email":          email,
		})
		w.Write(resp)
		return

	case "cloud_disconnect":
		cloudToolSetWantConn(false)
		cloudToolDisconnect()
		broadcastCloudToolStatus(false)
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "cloud_cancel_reconnect":
		cloudToolSetWantConn(false)
		cloudToolDisconnect()
		cloudToolClearAuth()
		broadcastCloudToolStatus(false)
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "cloud_exec":
		var j struct {
			Func string `json:"func"`
			Data string `json:"data"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		res, err := cloudToolCall(cloudToolMsg{Type: "exec", Func: j.Func, Data: j.Data}, 15*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]string{"status": "ok", "result": res.Data})
		w.Write(resp)
		return

	case "cloud_shop_list":
		res, err := cloudToolCall(cloudToolMsg{Type: "shop_list"}, 15*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]string{"status": "ok", "result": res.Data})
		w.Write(resp)
		return

	case "cloud_review_list":
		// 审核员查看待审资源列表（返回 JSON 字符串）
		res, err := cloudToolCall(cloudToolMsg{Type: "review_list"}, 15*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]string{"status": "ok", "result": res.Data})
		w.Write(resp)
		return

	case "cloud_review_action":
		// 审核员提交审核动作：kind: shop|image，id: 资源标识，action: approve|reject
		var j struct {
			Kind   string `json:"kind"`
			ID     string `json:"id"`
			Action string `json:"action"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		res, err := cloudToolCall(cloudToolMsg{Type: "review_action", Func: j.Kind, ID: j.ID, Args: []string{j.Action}}, 15*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]string{"status": "ok", "result": res.Data})
		w.Write(resp)
		return

	case "cloud_shop_buy":
		var j struct {
			Item string `json:"item"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		res, err := cloudToolCall(cloudToolMsg{Type: "shop_buy", Func: j.Item}, 15*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]string{"status": "ok", "result": res.Data})
		w.Write(resp)
		return

	case "cloud_shop_download":
		var j struct {
			Item string `json:"item"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		res, err := cloudToolCall(cloudToolMsg{Type: "shop_download", Func: j.Item}, 15*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]string{"status": "ok", "result": res.Data})
		w.Write(resp)
		return

	case "cloud_shop_publish":
		// 发布词库：文件已在服务端，读取应用目录内的词库文件转 base64 后转发给云工具落盘
		var j struct {
			Path string   `json:"path"`
			Args []string `json:"args"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !checkFilePath(j.Path) {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": "词库路径不合法"})
			w.Write(resp)
			return
		}
		full := filepath.Join(opuiAppDir(), filepath.FromSlash(j.Path))
		raw, err := os.ReadFile(full)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": "读取词库文件失败: " + err.Error()})
			w.Write(resp)
			return
		}
		args := append([]string{filepath.Base(full)}, j.Args...)
		res, err := cloudToolCall(cloudToolMsg{Type: "shop_publish", Data: base64.StdEncoding.EncodeToString(raw), Args: args}, 30*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]string{"status": "ok", "result": res.Data})
		w.Write(resp)
		return

	case "cloud_image_upload":
		// 上传图床：文件已在服务端，读取应用目录内的图片文件转 base64 后转发给云工具
		var j struct {
			Path string   `json:"path"`
			Args []string `json:"args"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !checkFilePath(j.Path) {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": "图片路径不合法"})
			w.Write(resp)
			return
		}
		full := filepath.Join(opuiAppDir(), filepath.FromSlash(j.Path))
		raw, err := os.ReadFile(full)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": "读取图片文件失败: " + err.Error()})
			w.Write(resp)
			return
		}
		res, err := cloudToolCall(cloudToolMsg{Type: "image_upload", Data: base64.StdEncoding.EncodeToString(raw), Args: j.Args}, 30*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		// 待审核时 result 为空串，message 给出「等待审核」提示，前端据此展示
		resp, _ := json.Marshal(map[string]string{"status": "ok", "result": res.Data, "message": res.Msg})
		w.Write(resp)
		return

	case "cloud_list_resources":
		// 云工具资源管理列表（kind: shop|image；status: -1 全部，0 待审，1 通过，2 拒绝）
		var req struct {
			Kind     string `json:"kind"`
			Keyword  string `json:"keyword"`
			Status   int    `json:"status"`
			Page     int    `json:"page"`
			PageSize int    `json:"page_size"`
		}
		if err := json.Unmarshal(h.Data, &req); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if req.Page < 1 {
			req.Page = 1
		}
		if req.PageSize <= 0 {
			req.PageSize = 20
		}
		if req.PageSize > 100 {
			req.PageSize = 100
		}
		items, total, err := CloudToolListResources(req.Kind, req.Keyword, req.Status, req.Page, req.PageSize)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]any{"status": "ok", "items": items, "total": total})
		w.Write(resp)
		return

	case "cloud_review_resource":
		// 审核云工具资源：kind: shop|image，id: 资源标识，action: approve|reject|delete
		var j struct {
			Kind   string `json:"kind"`
			ID     string `json:"id"`
			Action string `json:"action"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if err := CloudToolReviewResource(j.Kind, j.ID, j.Action, "面板"); err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "cloud_resource_content":
		// 预览云工具资源内容：kind: shop|image，id: 资源标识
		var j struct {
			Kind string `json:"kind"`
			ID   string `json:"id"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		content, err := CloudToolResourceContent(j.Kind, j.ID)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]string{"status": "ok", "content": content})
		w.Write(resp)
		return

	case "cloud_change_password":
		var j struct {
			OldPassword string `json:"old_password"`
			NewPassword string `json:"new_password"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		res, err := cloudToolCall(cloudToolMsg{Type: "change_password", OldPassword: j.OldPassword, NewPassword: j.NewPassword}, 15*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		resp, _ := json.Marshal(map[string]string{"status": "ok", "result": res.Data})
		w.Write(resp)
		return

	case "cloud_change_username":
		var j struct {
			OldPassword string `json:"old_password"`
			NewUsername string `json:"new_username"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		j.NewUsername = strings.TrimSpace(j.NewUsername)
		res, err := cloudToolCall(cloudToolMsg{Type: "change_username", OldPassword: j.OldPassword, NewUsername: j.NewUsername}, 15*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		// 更新本地保存的用户名，保证后续断线重连使用新用户名
		cloudToolMu.Lock()
		cloudToolUsername = j.NewUsername
		token := cloudToolToken
		cloudToolMu.Unlock()
		cloudToolSaveAuth(j.NewUsername, token)
		resp, _ := json.Marshal(map[string]string{"status": "ok", "result": res.Data})
		w.Write(resp)
		return

	case "cloud_set_offline_notify":
		var j struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		data := "0"
		if j.Enabled {
			data = "1"
		}
		res, err := cloudToolCall(cloudToolMsg{Type: "set_offline_notify", Data: data}, 15*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		cloudToolMu.Lock()
		cloudToolOfflineNotify = j.Enabled
		cloudToolMu.Unlock()
		resp, _ := json.Marshal(map[string]string{"status": "ok", "result": res.Data})
		w.Write(resp)
		return

	case "cloud_set_max_devices":
		var j struct {
			MaxDevices int `json:"max_devices"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if j.MaxDevices < 0 {
			j.MaxDevices = 0
		}
		res, err := cloudToolCall(cloudToolMsg{Type: "set_max_devices", Data: strconv.Itoa(j.MaxDevices)}, 15*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		cloudToolMu.Lock()
		cloudToolMaxDevices = j.MaxDevices
		cloudToolMu.Unlock()
		resp, _ := json.Marshal(map[string]string{"status": "ok", "result": res.Data})
		w.Write(resp)
		return

	case "cloud_bind_email":
		var j struct {
			Email string `json:"email"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		j.Email = strings.TrimSpace(j.Email)
		res, err := cloudToolCall(cloudToolMsg{Type: "send_email_code", Data: j.Email}, 15*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		// 仅发送验证码，绑定邮箱在验证码校验通过后完成
		resp, _ := json.Marshal(map[string]string{"status": "ok", "result": res.Data})
		w.Write(resp)
		return

	case "cloud_confirm_email":
		var j struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		res, err := cloudToolCall(cloudToolMsg{Type: "confirm_email", Data: j.Code}, 15*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		cloudToolMu.Lock()
		if res.Email != "" {
			cloudToolEmail = res.Email
		}
		cloudToolMu.Unlock()
		resp, _ := json.Marshal(map[string]string{"status": "ok", "result": res.Data, "email": res.Email})
		w.Write(resp)
		return

	case "cloud_delete_account":
		var j struct {
			Password string `json:"password"` // 已 SHA-256 加密后的密码哈希
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		res, err := cloudToolCall(cloudToolMsg{Type: "delete_account", Data: j.Password}, 15*time.Second)
		if err != nil {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(resp)
			return
		}
		if res.Type == "error" {
			resp, _ := json.Marshal(map[string]string{"status": "error", "error": res.Msg})
			w.Write(resp)
			return
		}
		// 注销成功：清除本地认证与连接，前端随之回到未登录状态
		cloudToolSetWantConn(false)
		cloudToolDisconnect()
		cloudToolClearAuth()
		broadcastCloudToolStatus(false)
		resp, _ := json.Marshal(map[string]string{"status": "ok", "result": res.Data})
		w.Write(resp)
		return

	case "cloud_logout":
		cloudToolMu.Lock()
		username := cloudToolUsername
		cloudToolMu.Unlock()
		if username != "" {
			// logout 触发云工具端删除 token 并断开，这里只需发送即可，随后关闭本地代理
			_ = cloudToolSend(cloudToolMsg{Type: "logout"})
		}
		cloudToolSetWantConn(false)
		cloudToolDisconnect()
		cloudToolClearAuth()
		broadcastCloudToolStatus(false)
		w.Write([]byte(`{"status":"ok"}`))
		return
	}
}

// ---------- 云工具（NebulaCloudTool）后端代理 ----------
// 前端（OPUI）不再直连云工具，而是通过后端转发的这套 JSON 协议与 NebulaCloudTool 通信。

// cloudToolMsg 云工具 WebSocket 消息结构，与 NebulaCloudTool 端保持一致。
type cloudToolMsg struct {
	Type          string   `json:"type"`
	ID            string   `json:"id,omitempty"`
	Func          string   `json:"func,omitempty"`
	Data          string   `json:"data,omitempty"`
	Args          []string `json:"args,omitempty"`
	Msg           string   `json:"msg,omitempty"`
	OldPassword   string   `json:"old_password,omitempty"`
	NewPassword   string   `json:"new_password,omitempty"`
	NewUsername   string   `json:"new_username,omitempty"`
	OfflineNotify bool     `json:"offline_notify,omitempty"`
	Debug         string   `json:"debug,omitempty"`
	MaxDevices    int      `json:"max_devices,omitempty"`
	Email         string   `json:"email,omitempty"`
}

// broadcastCloudToolStatus 把云工具连接状态推送给所有面板客户端
func broadcastCloudToolStatus(connected bool) {
	data, _ := json.Marshal(map[string]any{
		"type": "cloud_tool_status",
		"data": map[string]any{
			"connected":    connected,
			"reconnecting": cloudToolReconnecting.Load(),
		},
	})
	broadcastOpuiNotify(data)
}

// broadcastCloudToolOffline 云工具异常断开时向所有面板客户端推送离线警告
func broadcastCloudToolOffline() {
	data, _ := json.Marshal(map[string]any{
		"type": "cloud_tool_offline",
		"data": map[string]any{"msg": "云工具连接已断开"},
	})
	broadcastOpuiNotify(data)
}

// cloudToolSelfAddr 返回本机内置云工具服务端的 WebSocket 地址（默认连接自己）。
// 地址 = ws://127.0.0.1:{核心服务器端口}/{云工具服务端访问路径}。
func cloudToolSelfAddr() string {
	host := "127.0.0.1"
	port := "8080"
	if addr := dto.FuncServers.CoreAddr(); addr != "" {
		if i := strings.LastIndex(addr, ":"); i != -1 {
			port = addr[i+1:]
		}
	}
	path := "/cloudtool"
	if dto.ServerConfig.CloudTool != nil && dto.ServerConfig.CloudTool.Addr != "" {
		path = dto.ServerConfig.CloudTool.Addr
	}
	return "ws://" + host + ":" + port + path
}

// cloudToolAddr 读取合并配置中 [云工具] 连接地址，未配置时默认连接本机内置云工具服务端。
func cloudToolAddr() string {
	cfg, err := dto.LoadConfigFile()
	if err != nil {
		return cloudToolSelfAddr()
	}
	addr := strings.TrimSpace(cfg.Section("云工具").Key("连接地址").String())
	if addr == "" {
		return cloudToolSelfAddr()
	}
	return addr
}

// cloudToolAuthTable 云工具登录凭据在全局数据库中的表名
const cloudToolAuthTable = "cloud_tool"

// cloudToolSaveAuth 把登录账号与 token 持久化到全局数据库，供后端重启后自动恢复连接
func cloudToolSaveAuth(username, token string) {
	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return
	}
	if err := dic_funcs.EnsureFsTable(db, cloudToolAuthTable); err != nil {
		return
	}
	now := time.Now().Unix()
	_, _ = db.Exec(`
		INSERT INTO "cloud_tool" (key, data, updated_at)
		VALUES ('username', ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			data = excluded.data,
			updated_at = excluded.updated_at
	`, username, now)
	_, _ = db.Exec(`
		INSERT INTO "cloud_tool" (key, data, updated_at)
		VALUES ('token', ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			data = excluded.data,
			updated_at = excluded.updated_at
	`, token, now)
}

// cloudToolClearAuth 清除持久化的账号与 token（登出或 token 失效时）
func cloudToolClearAuth() {
	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return
	}
	if err := dic_funcs.EnsureFsTable(db, cloudToolAuthTable); err != nil {
		return
	}
	_, _ = db.Exec(`DELETE FROM "cloud_tool" WHERE key IN ('username', 'token')`)
}

// cloudToolSavedAuth 读取持久化的账号与 token，供启动或断线重连时恢复连接
func cloudToolSavedAuth() (string, string) {
	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return "", ""
	}
	if err := dic_funcs.EnsureFsTable(db, cloudToolAuthTable); err != nil {
		return "", ""
	}

	var username, token string
	_ = db.QueryRow(`SELECT data FROM "cloud_tool" WHERE key='username'`).Scan(&username)
	_ = db.QueryRow(`SELECT data FROM "cloud_tool" WHERE key='token'`).Scan(&token)
	return strings.TrimSpace(username), strings.TrimSpace(token)
}

// cloudToolSetWantConn 设置是否期望保持云工具连接（主动断开时应设为 false，避免触发断线重连）
func cloudToolSetWantConn(want bool) {
	cloudToolMu.Lock()
	cloudToolWantConn = want
	cloudToolMu.Unlock()
}

// cloudToolBuildURL 将连接地址规范为完整 WebSocket 地址，账号作为路径 /:username。
// 若地址已包含路径前缀（如 /cloudtool），账号会拼到该前缀之后（/cloudtool/:username）。
func cloudToolBuildURL(addr, username string) string {
	if !strings.Contains(addr, "://") {
		addr = "ws://" + addr
	}
	// https/http 地址统一转为 wss/ws，保证 WebSocket 可连接
	if after, ok := strings.CutPrefix(addr, "https://"); ok {
		addr = "wss://" + after
	} else if after, ok := strings.CutPrefix(addr, "http://"); ok {
		addr = "ws://" + after
	}
	u, err := url.Parse(addr)
	if err != nil {
		return addr
	}
	base := strings.TrimRight(u.Path, "/")
	if base == "" {
		u.Path = "/" + url.PathEscape(username)
	} else {
		u.Path = base + "/" + url.PathEscape(username)
	}
	return u.String()
}

// cloudToolConnected 返回当前是否已连接云工具
func cloudToolConnected() bool {
	cloudToolMu.Lock()
	defer cloudToolMu.Unlock()
	return cloudToolConn != nil
}

// cloudToolGetOfflineNotify 返回云工具断开时是否推送离线警告（来自云工具端，默认开启）
func cloudToolGetOfflineNotify() bool {
	cloudToolMu.Lock()
	defer cloudToolMu.Unlock()
	return cloudToolOfflineNotify
}

// cloudToolGetMaxDevices 返回当前账号允许的最大在线设备数（来自云工具端，默认 1，0 表示无限制）
func cloudToolGetMaxDevices() int {
	cloudToolMu.Lock()
	defer cloudToolMu.Unlock()
	return cloudToolMaxDevices
}

// cloudToolGetEmail 返回当前账号绑定的邮箱（来自云工具端，空表示未绑定）
func cloudToolGetEmail() string {
	cloudToolMu.Lock()
	defer cloudToolMu.Unlock()
	return cloudToolEmail
}

// cloudToolDisconnect 断开与云工具的连接（不广播，由调用方决定是否广播）
func cloudToolDisconnect() {
	cloudToolMu.Lock()
	conn := cloudToolConn
	cloudToolConn = nil
	cloudToolUsername = ""
	cloudToolToken = ""
	cloudToolEmail = ""
	cloudToolMu.Unlock()
	if conn != nil {
		conn.Close()
	}
}

// cloudToolReadLoop 持续读取云工具响应，按 id 路由到等待中的请求；连接断开时清理状态并触发断线重连
func cloudToolReadLoop(conn *websocket.Conn, gen uint64) {
	// 客户端心跳：周期发送 ping 探测服务端存活，pong 到达即刷新读超时
	pingStop := make(chan struct{})
	defer close(pingStop)
	go func() {
		ticker := time.NewTicker(cloudPingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); err != nil {
					return
				}
			case <-pingStop:
				return
			}
		}
	}()

	defer func() {
		cloudToolMu.Lock()
		isCurrent := cloudToolConn == conn
		if isCurrent {
			cloudToolConn = nil
			cloudToolUsername = ""
			cloudToolToken = ""
			cloudToolEmail = ""
		}
		wantReconnect := isCurrent && cloudToolWantConn
		offlineNotify := cloudToolOfflineNotify
		cloudToolMu.Unlock()
		conn.Close()
		cloudToolPending.Range(func(key, value any) bool {
			if ch, ok := value.(chan cloudToolMsg); ok {
				ch <- cloudToolMsg{Type: "error", Msg: "云工具连接已断开"}
			}
			return true
		})
		broadcastCloudToolStatus(false)
		if wantReconnect && offlineNotify {
			broadcastCloudToolOffline()
		}
		clearInjectedCloudFuncs(gen) // 回收这条连接注入的云函数，断开后应不再存在
		if wantReconnect {
			go cloudToolReconnectLoop()
		}
	}()
	for {
		var msg cloudToolMsg
		if err := conn.ReadJSON(&msg); err != nil {
			// 连接断开（含服务端关闭）：交由断线重连循环持续重试恢复连接
			debugLog.Errorf("[云工具] 连接断开: %v", err)
			return
		}
		if cloudToolDebug {
			debugLog.Infof("[云工具] 收到: type=%s id=%s", msg.Type, msg.ID)
		}
		if msg.Type == "logout_ok" {
			// 服务端主动注销（显式登出或在线时间过短被注销）：停止重连并清除本地登录态，回到未登录状态
			cloudToolSetWantConn(false)
			cloudToolClearAuth()
			return
		}
		if msg.Type == "dic_updated" {
			// 服务端热更新了云函数词库：无需断开连接，重新拉取并注入最新的云函数
			go injectAllCloudFuncs(gen)
			continue
		}
		if msg.Type == "result" || msg.Type == "error" {
			if ch, ok := cloudToolPending.Load(msg.ID); ok {
				ch.(chan cloudToolMsg) <- msg
			}
		}
	}
}

// cloudToolReconnectLoop 断线后自动用持久化凭据重连，带指数退避，直到重连成功或主动停止
func cloudToolReconnectLoop() {
	if !cloudToolReconnecting.CompareAndSwap(false, true) {
		return // 已有重连协程在运行
	}
	// 进入重连状态，立即通知前端
	broadcastCloudToolStatus(false)
	defer func() {
		cloudToolReconnecting.Store(false)
		// 重连结束（成功或停止），同步最终连接状态给前端
		broadcastCloudToolStatus(cloudToolConnected())
	}()

	delay := time.Second
	for {
		cloudToolMu.Lock()
		want := cloudToolWantConn
		cloudToolMu.Unlock()
		if !want {
			return
		}

		username, token := cloudToolSavedAuth()
		if username == "" || token == "" {
			return
		}

		if _, _, _, _, err := cloudToolAuth(username, "", token); err == nil {
			return // 重连成功
		} else if cloudToolDebug {
			debugLog.Errorf("[云工具] 断线重连失败: %v", err)
		}

		if !cloudToolReconnectSleep(delay) {
			return // 等待期间被主动中断
		}
		if delay < 30*time.Second {
			delay *= 2
		}
	}
}

// cloudToolReconnectSleep 可中断的退避等待：期间若期望连接被置为 false 则立即返回 false。
func cloudToolReconnectSleep(d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		cloudToolMu.Lock()
		want := cloudToolWantConn
		cloudToolMu.Unlock()
		if !want {
			return false
		}
		time.Sleep(200 * time.Millisecond)
	}
	return true
}

// cloudToolAuth 建立到云工具的连接并完成认证（passwordHash 密码登录 / token 恢复登录）。
// 成功时返回 token（恢复登录原样返回传入 token）。
func cloudToolAuth(username, passwordHash, token string) (newToken string, offlineNotify bool, email string, debug string, err error) {
	cloudToolDisconnect()

	addr := cloudToolAddr()
	if addr == "" {
		return "", false, "", "", errors.New("云工具连接地址未配置")
	}

	conn, _, err := websocket.DefaultDialer.Dial(cloudToolBuildURL(addr, username), nil)
	if err != nil {
		return "", false, "", "", fmt.Errorf("连接云工具失败: %w", err)
	}

	var authMsg cloudToolMsg
	if token != "" {
		authMsg = cloudToolMsg{Type: "resume", Data: token}
	} else {
		authMsg = cloudToolMsg{Type: "auth", Data: passwordHash}
	}

	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := conn.WriteJSON(authMsg); err != nil {
		conn.Close()
		return "", false, "", "", fmt.Errorf("云工具认证发送失败: %w", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	var resp cloudToolMsg
	if err := conn.ReadJSON(&resp); err != nil {
		conn.Close()
		return "", false, "", "", fmt.Errorf("云工具认证读取失败: %w", err)
	}
	if resp.Type != "auth_ok" {
		conn.Close()
		if token != "" {
			// 恢复登录失败（token 过期/失效），清除持久化凭据
			cloudToolClearAuth()
		}
		return "", false, "", "", errors.New("云工具认证失败: " + resp.Msg)
	}
	// 连接后保持在线：以心跳保活探测服务端存活（收到 ping/pong 即刷新读超时），不做空闲超时断开
	conn.SetReadDeadline(time.Now().Add(cloudPongWait))
	conn.SetPingHandler(func(appData string) error {
		conn.SetReadDeadline(time.Now().Add(cloudPongWait))
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(10*time.Second))
	})
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(cloudPongWait))
	})
	_ = conn.SetWriteDeadline(time.Time{})

	notify := resp.OfflineNotify
	cloudToolMu.Lock()
	cloudToolConn = conn
	cloudToolUsername = username
	cloudToolToken = resp.Data
	cloudToolWantConn = true
	cloudToolOfflineNotify = notify
	cloudToolMaxDevices = resp.MaxDevices
	cloudToolEmail = resp.Email
	gen := cloudToolGen.Add(1)
	cloudToolMu.Unlock()

	cloudToolSaveAuth(username, resp.Data)

	if cloudToolDebug {
		debugLog.Infof("[云工具] 已连接并认证: %s (%s)", addr, username)
	}
	go cloudToolReadLoop(conn, gen)
	broadcastCloudToolStatus(true)
	go injectAllCloudFuncs(gen)

	if token != "" {
		return token, notify, resp.Email, resp.Debug, nil
	}
	return resp.Data, notify, resp.Email, resp.Debug, nil
}

// cloudToolCall 发送一条带 id 的请求并等待对应 id 的响应
func cloudToolCall(msg cloudToolMsg, timeout time.Duration) (cloudToolMsg, error) {
	cloudToolMu.Lock()
	conn := cloudToolConn
	cloudToolMu.Unlock()
	if conn == nil {
		return cloudToolMsg{}, errors.New("云工具未连接")
	}

	id := fmt.Sprintf("%d", cloudToolSeq.Add(1))
	msg.ID = id
	ch := make(chan cloudToolMsg, 1)
	cloudToolPending.Store(id, ch)
	defer cloudToolPending.Delete(id)

	cloudToolWriteMu.Lock()
	err := conn.WriteJSON(msg)
	cloudToolWriteMu.Unlock()
	if err != nil {
		return cloudToolMsg{}, fmt.Errorf("云工具发送失败: %w", err)
	}

	select {
	case res := <-ch:
		return res, nil
	case <-time.After(timeout):
		// 收不到响应视为连接堵塞：关闭当前连接触发断线重连，并返回错误中断后续执行
		conn.Close()
		return cloudToolMsg{}, errors.New("云工具等待返回超时")
	}
}

// cloudToolSend 发送一条无需响应的消息（如 logout）
func cloudToolSend(msg cloudToolMsg) error {
	cloudToolMu.Lock()
	conn := cloudToolConn
	cloudToolMu.Unlock()
	if conn == nil {
		return errors.New("云工具未连接")
	}
	cloudToolWriteMu.Lock()
	defer cloudToolWriteMu.Unlock()
	return conn.WriteJSON(msg)
}

// injectCloudFunc 把一个云工具云函数注入为 nebula 字典函数，供词库直接 $函数名$ 调用。
// rule 为该云函数声明的参数数量规则，注入后字典函数按同一规则校验并原样传递位置参数。
func injectCloudFunc(gen uint64, name, rule string) error {
	err := dic_funcs.Register(name, rule, func(d *dto.DicInputs) (any, error) {
		args := d.Inputs.StringAfterList(1) // 去掉第 0 位函数名占位，得到全部位置参数
		res, err := cloudToolCall(cloudToolMsg{Type: "exec", Func: name, Args: args}, 15*time.Second)
		if err != nil {
			return "", err
		}
		if res.Type == "error" {
			return "", errors.New(res.Msg)
		}
		return res.Data, nil
	})
	if err != nil {
		return err
	}
	cloudToolInjectedMu.Lock()
	cloudToolInjected[name] = gen
	cloudToolInjectedMu.Unlock()
	return nil
}

// cloudFuncInfo 云函数信息：参数规则、说明与每次扣除金额（与云工具 list_funcs 返回结构一致）。
type cloudFuncInfo struct {
	Rule  string `json:"rule"`
	Desc  string `json:"desc"`
	Price string `json:"price,omitempty"`
}

// fetchCloudFuncs 拉取云工具已注册的云函数完整信息并写入缓存，返回最新列表。
func fetchCloudFuncs() (map[string]cloudFuncInfo, bool) {
	res, err := cloudToolCall(cloudToolMsg{Type: "list_funcs"}, 15*time.Second)
	if err != nil {
		return nil, false
	}
	if res.Type != "result" {
		return nil, false
	}
	var infos map[string]cloudFuncInfo
	if err := json.Unmarshal([]byte(res.Data), &infos); err != nil {
		return nil, false
	}
	cloudToolFuncsMu.Lock()
	cloudToolFuncsMap = infos
	cloudToolFuncsMu.Unlock()
	return infos, true
}

// getCloudFuncs 返回缓存的云函数完整信息；缓存为空时返回 nil。
func getCloudFuncs() map[string]cloudFuncInfo {
	cloudToolFuncsMu.RLock()
	defer cloudToolFuncsMu.RUnlock()
	if len(cloudToolFuncsMap) == 0 {
		return nil
	}
	cp := make(map[string]cloudFuncInfo, len(cloudToolFuncsMap))
	maps.Copy(cp, cloudToolFuncsMap)
	return cp
}

// injectAllCloudFuncs 拉取云工具已注册的云函数并逐个注入为字典函数，同时缓存完整信息供前端复用。
// 每次连接成功（含断线重连）与服务端热更新词库后都会调用：先注销本连接已注入的旧函数再按最新列表重新注入，
// 使新增与修改的云函数都能生效，无需断开连接。
func injectAllCloudFuncs(gen uint64) {
	infos, ok := fetchCloudFuncs()
	if !ok {
		return // 拉取失败时保留现有注入，避免云函数整体失效
	}
	cloudToolInjectedMu.Lock()
	for name, g := range cloudToolInjected {
		if g == gen {
			dic_funcs.Unregister(name)
			delete(cloudToolInjected, name)
		}
	}
	cloudToolInjectedMu.Unlock()
	for name, info := range infos {
		if name == "" {
			continue
		}
		_ = injectCloudFunc(gen, name, info.Rule)
	}
}

// clearInjectedCloudFuncs 注销指定代次连接注入的全部云函数，断开后这些函数应不再存在。
func clearInjectedCloudFuncs(gen uint64) {
	cloudToolInjectedMu.Lock()
	for name, g := range cloudToolInjected {
		if g == gen {
			dic_funcs.Unregister(name)
			delete(cloudToolInjected, name)
		}
	}
	cloudToolInjectedMu.Unlock()

	// 断开后清空云函数信息缓存，避免残留上一连接的数据
	cloudToolFuncsMu.Lock()
	cloudToolFuncsMap = map[string]cloudFuncInfo{}
	cloudToolFuncsMu.Unlock()
}

// StartCloudTool 启动时恢复云工具调试开关，并用上次登录持久化的账号与 token 自动恢复连接
func StartCloudTool() {
	// 注入状态查询回调，供字典函数 $云工具状态$ 判断是否已连接
	dto.CloudToolConnected = cloudToolConnected

	cfg, err := dto.LoadConfigFile()
	if err != nil {
		return
	}
	cloudToolDebug = cfg.Section("云工具").Key("调试").MustBool(false)

	username, token := cloudToolSavedAuth()
	if username != "" && token != "" {
		cloudToolSetWantConn(true) // 期望保持连接，失败后交由断线重连循环持续重试
		go func() {
			if _, _, _, _, err := cloudToolAuth(username, "", token); err != nil {
				if cloudToolDebug {
					debugLog.Errorf("[云工具] 启动自动连接失败: %v", err)
				}
				go cloudToolReconnectLoop() // 失败后进入退避重试，直到云工具可用或凭据失效
			}
		}()
	}
}

type HttpOpUiConfig_cloud_tool struct {
	Addr          string `json:"addr"`           // 星云云工具 WebSocket 连接地址（基础地址，账号会拼到路径 /:username）
	Connected     bool   `json:"connected"`      // 后端当前是否已连接云工具（get 返回）
	Reconnecting  bool   `json:"reconnecting"`   // 后端当前是否正在断线重连云工具（get 返回）
	Debug         bool   `json:"debug"`          // 是否开启云工具调试日志
	OfflineNotify bool   `json:"offline_notify"` // 云工具断开时是否向面板推送离线警告（来自云工具端，默认开启）
	MaxDevices    int    `json:"max_devices"`    // 当前账号最大在线设备数（来自云工具端，默认 1，0 表示无限制）
	Email         string `json:"email"`          // 当前账号绑定的邮箱（来自云工具端，空表示未绑定）
}

type HttpOpUiConfig_cloudtool_server struct {
	Open          bool   `json:"open"`           // 是否开启内置云工具服务端
	Addr          string `json:"addr"`           // 访问路径（不含前导 /）
	AllowRegister bool   `json:"allow_register"` // 是否允许任意账号注册
	DicDir        string `json:"dic_dir"`        // 云工具词库目录（相对 private/）
	LogoutSec     int    `json:"logout_sec"`     // 断开自动注销时长（秒），0 表示关闭
	Debug         bool   `json:"debug"`          // 调试打印
	ShopOpen      bool   `json:"shop_open"`      // 词库商城开关
	ImageHost     bool   `json:"image_host"`     // 图床开关
	ShopReview    bool   `json:"shop_review"`    // 词库上传审核开关
	ImageReview   bool   `json:"image_review"`   // 图床上传审核开关
}
