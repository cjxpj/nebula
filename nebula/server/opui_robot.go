package dic_server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cjxpj/nebula/bot/qqbot"
	qqbot_msg "github.com/cjxpj/nebula/bot/qqbot/msg"
	"github.com/cjxpj/nebula/bot/secludedbot"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

type HttpOpUiConfig_qq struct {
	Open        bool   `json:"open"`
	Dic         string `json:"dic"`
	Path        string `json:"path"`
	Appid       string `json:"appid"`
	Secret      string `json:"secret"`
	AtCompat    bool   `json:"at_compat"`
	FilterSlash bool   `json:"filter_slash"`
	Debug       bool   `json:"debug"`
	Ws          bool   `json:"ws"`
	WsIntents   int    `json:"ws_intents"`
	Remark      string `json:"remark"`
	BotName     string `json:"bot_name"`
	BotAvatar   string `json:"bot_avatar"`
	Robot       string `json:"robot"`
	Connected   bool   `json:"connected"`
}

type HttpOpUiConfig_qq_instance struct {
	Section string            `json:"section"`
	Config  HttpOpUiConfig_qq `json:"config"`
}

type HttpOpUiConfig_qq_list struct {
	Instances []HttpOpUiConfig_qq_instance `json:"instances"`
}

// qqBotInfoCache 是 botinfo.json 的最小字段，用于重启后兜底恢复机器人头像昵称。
type qqBotInfoCache struct {
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
}

// loadQQBotInfoCache 从机器人词库目录下的 botinfo.json 读取缓存的头像昵称。
func loadQQBotInfoCache(filePath string) *qqBotInfoCache {
	if filePath == "" {
		return nil
	}
	p := filepath.Join(filePath, "botinfo.json")
	if !filepath.IsAbs(p) {
		p = filepath.Join(utils.GetAppDir(), p)
	}
	var info qqBotInfoCache
	if data, err := os.ReadFile(p); err == nil && json.Unmarshal(data, &info) == nil {
		return &info
	}
	return nil
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

type HttpOpUiConfig_secluded struct {
	Open    bool   `json:"open"`
	Dic     string `json:"dic"`
	Address string `json:"address"`
	Token   string `json:"token"`
	Debug   bool   `json:"debug"`
}

// 机器人平台配置 API（QQ / NapCat / 云湖 / 飞书 / SEC）。
func init() {
	registerOpuiApi(opuiHandleRobotAPI,
		"get_qq", "get_qq_list", "save_qq", "toggle_qq_debug",
		"add_qq", "del_qq", "qq_sandbox_run", "get_napcat",
		"save_napcat", "get_yunhu", "save_yunhu", "get_feishu",
		"save_feishu", "get_secluded", "save_secluded",
	)
}

// opuiHandleRobotAPI 处理本模块注册的 OPUI API 类型。
func opuiHandleRobotAPI(w http.ResponseWriter, r *http.Request, h *HttpOpUiData) {
	switch h.Type {
	case "get_qq", "get_qq_list":
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		var list HttpOpUiConfig_qq_list
		for _, secName := range cfg.Sections() {
			if secName == "QQ" || (strings.HasPrefix(secName, "QQ") && len(secName) > 2) {
				d := cfg.Section(secName)
				var j HttpOpUiConfig_qq
				j.Open = d.Key("启用").MustBool(false)
				j.Dic = d.Key("词库").String()
				j.Path = d.Key("访问路径").String()
				j.Appid = d.Key("APPID").String()
				j.Secret = d.Key("密钥").String()
				j.AtCompat = d.Key("全量艾特兼容").MustBool(true)
				j.FilterSlash = d.Key("过滤开头斜杠").MustBool(true)
				j.Debug = d.Key("调试打印").MustBool(false)
				j.Ws = d.Key("WebSocket").MustBool(false)
				j.WsIntents = d.Key("监听码").MustInt(0)
				j.Remark = d.Key("备注").String()
				j.Robot = d.Key("Robot").String()
				if dto.ServerConfig.QQBots != nil {
					if bot := dto.ServerConfig.QQBots[secName]; bot != nil {
						j.Connected = bot.WsConn != nil
						if bot.API != nil {
							j.BotName = bot.API.BotUsername
							j.BotAvatar = bot.API.BotAvatar
						}
					}
				}
				// 未启用或尚未上线时，从本地 botinfo.json 兜底恢复头像昵称
				if j.BotName == "" && j.BotAvatar == "" {
					if info := loadQQBotInfoCache(j.Dic); info != nil {
						j.BotName = info.Username
						j.BotAvatar = info.Avatar
					}
				}
				list.Instances = append(list.Instances, HttpOpUiConfig_qq_instance{
					Section: secName,
					Config:  j,
				})
			}
		}
		r, _ := json.Marshal(list)
		w.Write(r)
		return

	case "save_qq":
		var j HttpOpUiConfig_qq_instance
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		sectionName := j.Section
		if sectionName == "" {
			sectionName = "QQ"
		}
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		// 备注唯一性检查
		if j.Config.Remark != "" {
			for _, secName := range cfg.Sections() {
				if secName != sectionName && (secName == "QQ" || (strings.HasPrefix(secName, "QQ") && len(secName) > 2)) {
					if cfg.Section(secName).Key("备注").String() == j.Config.Remark {
						http.Error(w, `{"status":"error","error":"备注名已存在"}`, http.StatusConflict)
						return
					}
				}
			}
		}
		d := cfg.Section(sectionName)
		d.Key("启用").SetValue(strconv.FormatBool(j.Config.Open))
		d.Key("词库").SetValue(j.Config.Dic)
		d.Key("访问路径").SetValue(j.Config.Path)
		d.Key("APPID").SetValue(j.Config.Appid)
		d.Key("密钥").SetValue(j.Config.Secret)
		d.Key("全量艾特兼容").SetValue(strconv.FormatBool(j.Config.AtCompat))
		d.Key("过滤开头斜杠").SetValue(strconv.FormatBool(j.Config.FilterSlash))
		d.Key("调试打印").SetValue(strconv.FormatBool(j.Config.Debug))
		d.Key("WebSocket").SetValue(strconv.FormatBool(j.Config.Ws))
		d.Key("监听码").SetValue(strconv.Itoa(j.Config.WsIntents))
		d.Key("备注").SetValue(j.Config.Remark)
		d.Key("Robot").SetValue(j.Config.Robot)
		dto.LoadConfig_qq(d, sectionName)
		cfg.Save()
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "toggle_qq_debug":
		var j struct {
			Section string `json:"section"`
			Debug   bool   `json:"debug"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		// 更新配置文件
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := cfg.Section(j.Section)
		d.Key("调试打印").SetValue(strconv.FormatBool(j.Debug))
		cfg.Save()
		// 仅更新运行中 bot 的 Debug 标志，不重连
		if dto.ServerConfig.QQBots != nil {
			if bot := dto.ServerConfig.QQBots[j.Section]; bot != nil {
				bot.Debug = j.Debug
				if bot.API != nil {
					bot.API.Debug = j.Debug
				}
			}
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "add_qq":
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		// 找到下一个可用的编号
		maxNum := 0
		for _, name := range cfg.Sections() {
			if strings.HasPrefix(name, "QQ") {
				if name == "QQ" {
					if maxNum < 1 {
						maxNum = 1
					}
				} else {
					numStr := name[2:]
					if num, err := strconv.Atoi(numStr); err == nil && num > maxNum {
						maxNum = num
					}
				}
			}
		}
		newNum := maxNum + 1
		newSection := "QQ" + strconv.Itoa(newNum)
		d := cfg.Section(newSection)
		d.Key("启用").SetValue("false")
		d.Key("词库").SetValue("private/bot/qq" + strconv.Itoa(newNum))
		d.Key("访问路径").SetValue("qq-bot" + strconv.Itoa(newNum))
		d.Key("APPID").SetValue("")
		d.Key("密钥").SetValue("")
		d.Key("全量艾特兼容").SetValue("true")
		d.Key("过滤开头斜杠").SetValue("true")
		d.Key("调试打印").SetValue("false")
		d.Key("WebSocket").SetValue("true")
		d.Key("监听码").SetValue("0")
		d.Key("备注").SetValue("")
		d.Key("Robot").SetValue("")
		cfg.Save()
		j := HttpOpUiConfig_qq_instance{
			Section: newSection,
			Config: HttpOpUiConfig_qq{
				Open:        false,
				Dic:         "private/bot/qq" + strconv.Itoa(newNum),
				Path:        "qq-bot" + strconv.Itoa(newNum),
				Appid:       "",
				Secret:      "",
				AtCompat:    true,
				FilterSlash: true,
				Debug:       false,
				Ws:          true,
				WsIntents:   0,
				Remark:      "",
				Robot:       "",
			},
		}
		r, _ := json.Marshal(j)
		w.Write(r)
		return

	case "del_qq":
		var j struct {
			Section string `json:"section"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if j.Section == "" {
			http.Error(w, `{"status":"error","error":"section is empty"}`, http.StatusBadRequest)
			return
		}
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		cfg.DeleteSection(j.Section)
		// 从运行中移除
		if dto.ServerConfig.QQBots != nil {
			delete(dto.ServerConfig.QQBots, j.Section)
		}
		cfg.Save()
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "qq_sandbox_run":
		var j struct {
			Section string `json:"section"`
			// Dic 自定义词库路径（优先于 section，无需选择实例即可测试）
			Dic string `json:"dic"`
			// DicFile 只执行该词库文件（沙箱单文件测试），空则执行 Dic/dic 目录下全部词库
			DicFile string `json:"dic_file"`
			Msg     string `json:"msg"`
			// Private 按群私聊模拟（词库 #私聊# 触发词生效），默认群聊
			Private bool `json:"private"`
			// ReplyID 前端右键回复指定的被引用消息 ID，空则回退为本次消息自身
			ReplyID string `json:"reply_id"`
			// GroupID 自定义模拟群号（写入 %群号%），空则使用默认 sandbox_group
			GroupID string `json:"group_id"`
			// Images 用户发送的图片（data URL 列表），注入词库 $IMG$ 附件
			Images []string `json:"images"`
			// User 自定义模拟用户（ID 写入 QQ/qq 变量，Name 写入 昵称 变量）
			User struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"user"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}

		dicPath := j.Dic
		appid, secret := "", ""
		atCompat, filterSlash := true, true
		debug := false
		robot := ""

		// 未直接传词库路径时，回退到按 section 从配置读取（兼容旧调用）
		if dicPath == "" {
			if j.Section == "" {
				http.Error(w, `{"status":"error","error":"dic或section不能为空"}`, http.StatusBadRequest)
				return
			}
			cfg, err := dto.LoadConfigFile()
			if err != nil {
				utils.ErrorStop("系统配置不存在")
			}
			d := cfg.Section(j.Section)
			dicPath = d.Key("词库").String()
			appid = d.Key("APPID").String()
			secret = d.Key("密钥").String()
			atCompat = d.Key("全量艾特兼容").MustBool(true)
			filterSlash = d.Key("过滤开头斜杠").MustBool(true)
			debug = d.Key("调试打印").MustBool(false)
			robot = d.Key("Robot").String()
		}
		if dicPath == "" {
			http.Error(w, `{"status":"error","error":"词库路径不能为空"}`, http.StatusBadRequest)
			return
		}

		// 单文件测试：仅执行前端选中的词库文件（校验路径合法性，避免越权读取）
		dicFile := strings.TrimSpace(j.DicFile)
		if dicFile != "" && !checkDicPath(dicFile) {
			http.Error(w, `{"status":"error","error":"测试词库路径不合法"}`, http.StatusBadRequest)
			return
		}

		capture := qqbot_msg.NewSandboxCapture()
		api := qqbot_msg.NewQQBot(appid, secret)
		api.Debug = debug
		api.Sandbox = capture

		bot := &qqbot_msg.RouterQQBot{
			FilePath:    dicPath,
			DicFile:     dicFile,
			API:         api,
			AtCompat:    atCompat,
			FilterSlash: filterSlash,
			Debug:       debug,
			Robot:       robot,
		}

		msgID, messages := qqbot.SandboxRun(bot, j.Msg, qqbot.SandboxUser{ID: j.User.ID, Name: j.User.Name}, j.Private, j.ReplyID, j.GroupID, j.Images)
		if messages == nil {
			messages = []qqbot_msg.SandboxMessage{}
		}
		resp, _ := json.Marshal(map[string]any{"status": "ok", "msg_id": msgID, "messages": messages})
		w.Write(resp)
		return

	case "get_napcat":
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := cfg.Section("NapCat")
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
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := cfg.Section("NapCat")
		d.Key("启用").SetValue(strconv.FormatBool(j.Open))
		d.Key("词库").SetValue(j.Dic)
		d.Key("访问路径").SetValue(j.Path)
		d.Key("密钥").SetValue(j.Secret)
		d.Key("发送消息接口").SetValue(j.Api)
		dto.LoadConfig_napcat(d)
		cfg.Save()
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "get_yunhu":
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := cfg.Section("云湖")
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
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := cfg.Section("云湖")
		d.Key("启用").SetValue(strconv.FormatBool(j.Open))
		d.Key("词库").SetValue(j.Dic)
		d.Key("访问路径").SetValue(j.Path)
		d.Key("密钥").SetValue(j.Secret)
		dto.LoadConfig_yunhu(d)
		cfg.Save()
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "get_feishu":
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := cfg.Section("飞书")
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
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := cfg.Section("飞书")
		d.Key("启用").SetValue(strconv.FormatBool(j.Open))
		d.Key("词库").SetValue(j.Dic)
		d.Key("访问路径").SetValue(j.Path)
		d.Key("APPID").SetValue(j.Appid)
		d.Key("密钥").SetValue(j.Secret)
		dto.LoadConfig_feishu(d)
		cfg.Save()
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "get_secluded":
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := cfg.Section("Secluded")
		var j HttpOpUiConfig_secluded
		j.Open = d.Key("启用").MustBool(false)
		j.Dic = d.Key("词库").String()
		j.Address = d.Key("对接地址").String()
		j.Token = d.Key("令牌").String()
		j.Debug = d.Key("调试打印").MustBool(false)
		r, _ := json.Marshal(j)
		w.Write(r)
		return

	case "save_secluded":
		var j HttpOpUiConfig_secluded
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		cfg, err := dto.LoadConfigFile()
		if err != nil {
			utils.ErrorStop("系统配置不存在")
		}
		d := cfg.Section("Secluded")
		d.Key("启用").SetValue(strconv.FormatBool(j.Open))
		d.Key("词库").SetValue(j.Dic)
		d.Key("对接地址").SetValue(j.Address)
		d.Key("令牌").SetValue(j.Token)
		d.Key("调试打印").SetValue(strconv.FormatBool(j.Debug))
		dto.LoadConfig_secluded(d)
		cfg.Save()
		if j.Open {
			if dto.ServerConfig.SecludedBot != nil && dto.ServerConfig.SecludedBot.Addr != "" {
				secludedbot.Start(dto.ServerConfig.SecludedBot.Addr, dto.ServerConfig.SecludedBot.Token)
			}
		} else {
			secludedbot.Stop()
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	}
}
