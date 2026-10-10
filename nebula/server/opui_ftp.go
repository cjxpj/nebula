package dic_server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

type HttpOpUiConfig_ftp struct {
	Open          bool   `json:"open"`
	Port          int    `json:"port"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	Debug         bool   `json:"debug"`
	Tls           bool   `json:"tls"`
	PasvPortStart int    `json:"pasv_port_start"`
	PasvPortEnd   int    `json:"pasv_port_end"`
}

type HttpOpUiConfig_sftp struct {
	Open     bool   `json:"open"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Debug    bool   `json:"debug"`
}

// ---------- 安全检测：登录事件追踪 ----------

var (
	// FTP 服务管理
	ftpListener    net.Listener
	ftpCancel      context.CancelFunc
	ftpUser        string // 配置的用户名
	ftpPass        string // 配置的密码
	ftpPasvPortMin int    // PASV 被动模式端口范围起始
	ftpPasvPortMax int    // PASV 被动模式端口范围结束

	// SFTP 服务管理
	sftpListener net.Listener
	sftpCancel   context.CancelFunc
	sftpUser     string // 配置的用户名
	sftpPass     string // 配置的密码
)

// StartFtp 启动 FTP 服务并打印局域网链接
func StartFtp(port int, debug bool, username, password string, tlsEnabled bool, pasvPortStart, pasvPortEnd int) {
	// 先停止旧的 FTP 服务
	StopFtp()

	prefix := "[FTP]"
	if tlsEnabled {
		prefix = "[FTPS]"
	}

	// 配置 TLS
	if tlsEnabled {
		tlsCfg, err := utils.GenerateSelfSignedTLS()
		if err != nil {
			debugLog.Errorf(prefix+" TLS 证书生成失败: %v", err)
		} else {
			ftpTlsConfig = tlsCfg
		}
	} else {
		ftpTlsConfig = nil
	}

	// 打印局域网链接
	lanIP := getLanIP()
	if lanIP != "127.0.0.1" {
		fmt.Printf("%s 局域网链接: ftp://%s:%d\n", prefix, lanIP, port)
	}

	ftpUser = username
	ftpPass = password

	// 设置 PASV 端口范围
	if pasvPortStart > 0 && pasvPortEnd > 0 && pasvPortStart <= pasvPortEnd {
		ftpPasvPortMin = pasvPortStart
		ftpPasvPortMax = pasvPortEnd
	} else {
		ftpPasvPortMin = 32000
		ftpPasvPortMax = 32005
	}

	if debug {
		debugLog.Infof(prefix+" FTP 服务已启动，端口: %d, PASV 端口范围: %d-%d", port, ftpPasvPortMin, ftpPasvPortMax)
		debugLog.Infof(prefix+" 根目录映射: %s", utils.FtpDir())
	}

	ctx, cancel := context.WithCancel(context.Background())
	ftpCancel = cancel

	// 异步启动 FTP 服务端
	go func() {
		if err := runFtpServer(ctx, port, debug); err != nil {
			debugLog.Errorf(prefix+" 服务异常: %v", err)
		}
	}()
}

// StopFtp 停止 FTP 服务
func StopFtp() {
	if ftpCancel != nil {
		ftpCancel()
		ftpCancel = nil
	}
	if ftpListener != nil {
		ftpListener.Close()
		ftpListener = nil
	}
}

// StartSftp 启动 SFTP 服务并打印局域网链接
func StartSftp(port int, debug bool, username, password string) {
	// 先停止旧的 SFTP 服务
	StopSftp()

	// 打印局域网链接
	lanIP := getLanIP()
	if lanIP != "127.0.0.1" {
		fmt.Printf("[SFTP] 局域网链接: sftp://%s:%d\n", lanIP, port)
	}

	sftpUser = username
	sftpPass = password

	if debug {
		debugLog.Infof("[SFTP] SFTP 服务已启动，端口: %d", port)
		debugLog.Infof("[SFTP] 根目录映射: %s", utils.FtpDir())
	}

	ctx, cancel := context.WithCancel(context.Background())
	sftpCancel = cancel

	// 异步启动 SFTP 服务端
	go func() {
		if err := runSftpServer(ctx, port, debug); err != nil {
			debugLog.Errorf("[SFTP] 服务异常: %v", err)
		}
	}()
}

// StopSftp 停止 SFTP 服务
func StopSftp() {
	if sftpCancel != nil {
		sftpCancel()
		sftpCancel = nil
	}
	if sftpListener != nil {
		sftpListener.Close()
		sftpListener = nil
	}
}

// FTP / SFTP 服务 API。
func init() {
	registerOpuiApi(opuiHandleFTPAPI,
		"get_ftp", "save_ftp", "get_sftp", "save_sftp",
	)
}

// opuiHandleFTPAPI 处理本模块注册的 OPUI API 类型。
func opuiHandleFTPAPI(w http.ResponseWriter, r *http.Request, h *HttpOpUiData) {
	switch h.Type {
	case "get_ftp":
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := cfg.Section("FTP")
		var j HttpOpUiConfig_ftp
		j.Open = d.Key("启用").MustBool(false)
		j.Port = d.Key("端口").MustInt(21)
		j.Username = d.Key("用户名").String()
		j.Password = d.Key("密码").String()
		j.Debug = d.Key("调试").MustBool(false)
		j.Tls = d.Key("TLS").MustBool(false)
		j.PasvPortStart = d.Key("PASV端口起始").MustInt(32000)
		j.PasvPortEnd = d.Key("PASV端口结束").MustInt(32005)
		r, _ := json.Marshal(j)
		w.Write(r)
		return

	case "save_ftp":
		var j HttpOpUiConfig_ftp
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		// 校验数据端口范围
		if j.PasvPortStart < 1 || j.PasvPortStart > 65535 || j.PasvPortEnd < 1 || j.PasvPortEnd > 65535 {
			http.Error(w, `{"status":"error","error":"数据端口范围必须在 1-65535 之间"}`, http.StatusBadRequest)
			return
		}
		if j.PasvPortStart > j.PasvPortEnd {
			http.Error(w, `{"status":"error","error":"起始端口不能大于结束端口"}`, http.StatusBadRequest)
			return
		}
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := cfg.Section("FTP")
		d.Key("启用").SetValue(strconv.FormatBool(j.Open))
		d.Key("端口").SetValue(strconv.Itoa(j.Port))
		d.Key("用户名").SetValue(j.Username)
		d.Key("密码").SetValue(j.Password)
		d.Key("调试").SetValue(strconv.FormatBool(j.Debug))
		d.Key("TLS").SetValue(strconv.FormatBool(j.Tls))
		d.Key("PASV端口起始").SetValue(strconv.Itoa(j.PasvPortStart))
		d.Key("PASV端口结束").SetValue(strconv.Itoa(j.PasvPortEnd))
		cfg.Save()

		if j.Open {
			StartFtp(j.Port, j.Debug, j.Username, j.Password, j.Tls, j.PasvPortStart, j.PasvPortEnd)
		} else {
			StopFtp()
		}

		w.Write([]byte(`{"status":"ok"}`))
		return

	case "get_sftp":
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := cfg.Section("SFTP")
		var j HttpOpUiConfig_sftp
		j.Open = d.Key("启用").MustBool(false)
		j.Port = d.Key("端口").MustInt(22)
		j.Username = d.Key("用户名").String()
		j.Password = d.Key("密码").String()
		j.Debug = d.Key("调试").MustBool(false)
		r, _ := json.Marshal(j)
		w.Write(r)
		return

	case "save_sftp":
		var j HttpOpUiConfig_sftp
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := cfg.Section("SFTP")
		d.Key("启用").SetValue(strconv.FormatBool(j.Open))
		d.Key("端口").SetValue(strconv.Itoa(j.Port))
		d.Key("用户名").SetValue(j.Username)
		d.Key("密码").SetValue(j.Password)
		d.Key("调试").SetValue(strconv.FormatBool(j.Debug))
		cfg.Save()

		if j.Open {
			StartSftp(j.Port, j.Debug, j.Username, j.Password)
		} else {
			StopSftp()
		}

		w.Write([]byte(`{"status":"ok"}`))
		return

	}
}
