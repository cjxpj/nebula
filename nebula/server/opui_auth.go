package dic_server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/cjxpj/nebula/debugLog"
	dic_funcs "github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
	"golang.org/x/crypto/bcrypt"
)

type HttpOpUiConfig_opui struct {
	Open          bool   `json:"open"`
	Path          string `json:"path"`
	Password      string `json:"password"`        // 保存时提交的新密码（明文，仅用于生成哈希）
	AuthPassword  string `json:"auth_password"`   // 二次验证：更改密码/快捷码时提交的当前登录密码
	HasPassword   bool   `json:"has_password"`    // 读取时返回：是否已设置登录密码
	HasQuickToken bool   `json:"has_quick_token"` // 读取时返回：是否已设置快捷登录码
	Cors          bool   `json:"cors"`
}

const opuiAuthTable = "opui_auth"

// opuiPasswordKey 管理面板登录密码哈希在 opui_auth 表中的键名
const opuiPasswordKey = "password"

// opuiQuickTokenKey 管理面板快捷登录码哈希在 opui_auth 表中的键名
const opuiQuickTokenKey = "quick_token"

// 当前快捷登录码明文：仅驻留内存，供启动打印的 WebUI 地址拼接 ?key= 快捷登录；不落库。
var (
	opuiQuickTokenMu    sync.Mutex
	opuiQuickTokenPlain string
)

// opuiPasswordHash 读取管理面板登录密码的 bcrypt 哈希。
// 返回 (hash, 是否已设置, 错误)；数据库异常时返回错误，未设置密码时 set=false。
func opuiPasswordHash() (string, bool, error) {
	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return "", false, err
	}
	if err = dic_funcs.EnsureFsTable(db, opuiAuthTable); err != nil {
		return "", false, err
	}
	var hash string
	err = db.QueryRow(`SELECT data FROM "`+opuiAuthTable+`" WHERE key=?`, opuiPasswordKey).Scan(&hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return hash, true, nil
}

// opuiHasPassword 是否已设置管理面板登录密码（数据库异常时视为未设置）。
func opuiHasPassword() bool {
	_, set, err := opuiPasswordHash()
	return err == nil && set
}

// opuiVerifyPassword 校验管理面板登录密码。
// 未设置密码时返回 true；数据库异常时返回错误（调用方应拒绝访问）。
func opuiVerifyPassword(password string) (bool, error) {
	hash, set, err := opuiPasswordHash()
	if err != nil {
		return false, err
	}
	if !set {
		return true, nil // 未设置密码，放行
	}
	if password == "" {
		return false, nil
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil, nil
}

// opuiQuickTokenHash 读取管理面板快捷登录码的 bcrypt 哈希。
// 返回 (hash, 是否已设置, 错误)；数据库异常时返回错误，未设置时 set=false。
func opuiQuickTokenHash() (string, bool, error) {
	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return "", false, err
	}
	if err = dic_funcs.EnsureFsTable(db, opuiAuthTable); err != nil {
		return "", false, err
	}
	var hash string
	err = db.QueryRow(`SELECT data FROM "`+opuiAuthTable+`" WHERE key=?`, opuiQuickTokenKey).Scan(&hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return hash, true, nil
}

// opuiHasQuickToken 是否已设置快捷登录码（数据库异常时视为未设置）。
func opuiHasQuickToken() bool {
	_, set, err := opuiQuickTokenHash()
	return err == nil && set
}

// opuiSetQuickToken 设置（或重置）快捷登录码，明文仅用于生成 bcrypt 哈希，不落库。
func opuiSetQuickToken(token string) error {
	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return err
	}
	if err = dic_funcs.EnsureFsTable(db, opuiAuthTable); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(token), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		INSERT INTO "`+opuiAuthTable+`" (key, data, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			data = excluded.data,
			updated_at = excluded.updated_at
	`, opuiQuickTokenKey, string(hash), time.Now().Unix())
	return err
}

// opuiVerifyQuickToken 校验快捷登录码。
func opuiVerifyQuickToken(token string) (bool, error) {
	if token == "" {
		return false, nil
	}
	hash, set, err := opuiQuickTokenHash()
	if err != nil {
		return false, err
	}
	if !set {
		return false, nil
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(token)) == nil, nil
}

// opuiVerifyKey 校验管理面板登录凭证：先匹配登录密码，再匹配快捷登录码。
// 未设置密码时放行；数据库异常时返回错误（调用方应拒绝访问）。
func opuiVerifyKey(key string) (bool, error) {
	if ok, err := opuiVerifyPassword(key); err != nil || ok {
		return ok, err
	}
	return opuiVerifyQuickToken(key)
}

// opuiVerifyAuthPassword 二次验证当前登录密码（仅匹配登录密码，不匹配快捷登录码）。
// 用于更改密码/快捷登录码前确认身份。
func opuiVerifyAuthPassword(password string) bool {
	if password == "" {
		return false
	}
	ok, err := opuiVerifyPassword(password)
	return err == nil && ok
}

// EnsureOpuiQuickToken 生成一个新的随机快捷登录码：哈希入库，明文保存在内存供 WebUI 地址拼接。
// 管理面板未启用或生成失败时返回空串。
func EnsureOpuiQuickToken() string {
	if dto.ServerConfig.OPUI == nil {
		return ""
	}
	token := utils.RandomString("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789", 32)
	if token == "" {
		return ""
	}
	if err := opuiSetQuickToken(token); err != nil {
		return ""
	}
	opuiQuickTokenMu.Lock()
	opuiQuickTokenPlain = token
	opuiQuickTokenMu.Unlock()
	return token
}

// GetOpuiQuickToken 返回当前内存中保存的快捷登录码明文（用于 WebUI 地址拼接），无则返回空串。
func GetOpuiQuickToken() string {
	opuiQuickTokenMu.Lock()
	defer opuiQuickTokenMu.Unlock()
	return opuiQuickTokenPlain
}

// opuiSetPassword 设置（或更新）管理面板登录密码，明文仅用于生成 bcrypt 哈希，不落库。
func opuiSetPassword(password string) error {
	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return err
	}
	if err = dic_funcs.EnsureFsTable(db, opuiAuthTable); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		INSERT INTO "`+opuiAuthTable+`" (key, data, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			data = excluded.data,
			updated_at = excluded.updated_at
	`, opuiPasswordKey, string(hash), time.Now().Unix())
	return err
}

// EnsureOpuiInitialPassword 管理面板启用但未设置登录密码时，自动生成随机初始密码并返回明文。
// 未启用管理面板、已设置密码或数据库异常时返回空串与 false。
func EnsureOpuiInitialPassword() (string, bool) {
	if dto.ServerConfig.OPUI == nil {
		return "", false
	}
	// 直接读取密码哈希，区分「未设置」与「数据库异常」：异常时不生成，避免误覆盖用户已设置的密码
	_, set, err := opuiPasswordHash()
	if err != nil {
		return "", false
	}
	if set {
		return "", false
	}
	pwd := utils.RandomString("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789", 16)
	if pwd == "" {
		return "", false
	}
	if err := opuiSetPassword(pwd); err != nil {
		return "", false
	}
	return pwd, true
}

func opuiCheckKey(r *http.Request, hType string) bool {
	if hType == "check_opui_key" || hType == "get_opui" || hType == "get_bg" {
		return true // 密码校验和查询配置状态免鉴权
	}
	valid, err := opuiVerifyKey(r.Header.Get("X-OPUI-Key"))
	if err != nil {
		return false // 数据库异常时拒绝访问，避免误放行
	}
	return valid
}

// toDataURI 把图片字节转为 base64 data URI（带浏览器可识别的图片类型）

// 管理面板鉴权 / 密码 / 快捷码 API。
func init() {
	registerOpuiApi(opuiHandleAuthAPI,
		"get_opui", "save_opui", "reset_opui_password", "generate_quick_token",
		"check_opui_key",
	)
}

// opuiHandleAuthAPI 处理本模块注册的 OPUI API 类型。
func opuiHandleAuthAPI(w http.ResponseWriter, r *http.Request, h *HttpOpUiData) {
	switch h.Type {
	case "get_opui":
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			debugLog.Errorf("[OPUI] get_opui LoadConfigFile failed: %v", err)
			w.Write([]byte(`{"open":false,"path":"","password":"","has_password":false,"cors":false}`))
			return
		}
		d := cfg.Section("管理面板")
		var j HttpOpUiConfig_opui
		j.Open = d.Key("启用").MustBool(false)
		j.Path = d.Key("访问路径").String()
		j.HasPassword = opuiHasPassword()
		j.HasQuickToken = opuiHasQuickToken()
		j.Cors = d.Key("跨域").MustBool(false)
		r, err := json.Marshal(j)
		if err != nil {
			debugLog.Errorf("[OPUI] get_opui json.Marshal failed: %v", err)
			w.Write([]byte(`{"open":false,"path":"","has_password":false}`))
			return
		}
		w.Write(r)
		return

	case "save_opui":
		var j HttpOpUiConfig_opui
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := cfg.Section("管理面板")
		d.Key("启用").SetValue(strconv.FormatBool(j.Open))
		d.Key("访问路径").SetValue(j.Path)
		d.Key("跨域").SetValue(strconv.FormatBool(j.Cors))
		cfg.Save()

		// 登录密码改为单向加密存储到全局数据库，不再写入配置文件；更改密码需二次验证当前密码
		if j.Password != "" {
			if !opuiVerifyAuthPassword(j.AuthPassword) {
				http.Error(w, `{"status":"error","error":"auth_password_invalid"}`, http.StatusUnauthorized)
				return
			}
			debugLog.Debugf("[OPUI] save_opui: 设置登录密码 (len=%d)", len(j.Password))
			if err := opuiSetPassword(j.Password); err != nil {
				http.Error(w, `{"status":"error","error":"set password failed"}`, http.StatusInternalServerError)
				return
			}
		}

		if j.Open {
			dto.ServerConfig.OPUI = &dto.OPUI{
				Addr: "/" + j.Path,
				Cors: j.Cors,
			}
		} else {
			dto.ServerConfig.OPUI = nil
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "reset_opui_password":
		var j struct {
			AuthPassword string `json:"auth_password"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !opuiVerifyAuthPassword(j.AuthPassword) {
			http.Error(w, `{"status":"error","error":"auth_password_invalid"}`, http.StatusUnauthorized)
			return
		}
		pwd := utils.RandomString("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789", 16)
		if pwd == "" {
			http.Error(w, `{"status":"error","error":"generate password failed"}`, http.StatusInternalServerError)
			return
		}
		if err := opuiSetPassword(pwd); err != nil {
			http.Error(w, `{"status":"error","error":"set password failed"}`, http.StatusInternalServerError)
			return
		}
		debugLog.Infof("[OPUI] reset_opui_password: 重置随机登录密码")
		resp, _ := json.Marshal(map[string]string{"password": pwd})
		w.Write(resp)
		return

	case "generate_quick_token":
		var j struct {
			AuthPassword string `json:"auth_password"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !opuiVerifyAuthPassword(j.AuthPassword) {
			http.Error(w, `{"status":"error","error":"auth_password_invalid"}`, http.StatusUnauthorized)
			return
		}
		token := EnsureOpuiQuickToken()
		if token == "" {
			http.Error(w, `{"status":"error","error":"generate token failed"}`, http.StatusInternalServerError)
			return
		}
		resp, _ := json.Marshal(map[string]string{"token": token})
		w.Write(resp)
		return

	case "check_opui_key":
		var j struct {
			Key string `json:"key"`
		}
		if len(h.Data) == 0 {
			debugLog.Errorf("[OPUI] check_opui_key: h.Data is nil or empty")
			w.Write([]byte(`{"valid":false}`))
			return
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			debugLog.Errorf("[OPUI] check_opui_key: json.Unmarshal failed, data=%s, err=%v", string(h.Data), err)
			w.Write([]byte(`{"valid":false}`))
			return
		}
		clientIP := utils.GetClientIP(r)
		valid, err := opuiVerifyKey(j.Key)
		if err != nil {
			w.Write([]byte(`{"valid":false}`))
			return
		}
		if valid {
			addLoginEvent("admin_login", "OPUI 管理员登录成功", clientIP)
			w.Write([]byte(`{"valid":true}`))
		} else {
			addLoginEvent("admin_login_fail", "OPUI 登录失败: 密码错误", clientIP)
			w.Write([]byte(`{"valid":false}`))
		}
		return

	}
}
