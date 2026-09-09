//go:build !js

package dic_server

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/debugLog"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	dic_funcs "github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"
	"github.com/cjxpj/nebula/utils"
	"github.com/gorilla/websocket"
)

// 内置云工具服务端：账号存全局数据库，余额用经济系统（类型固定为“云工具余额”），
// 云函数由词库目录下的 [函数] 定义，客户端连接 /{访问路径}/{账号} 后执行词库代码并返回结果。

// cloudToolMoneyType 内置云工具余额在经济系统中的类型标识。
const cloudToolMoneyType = "云工具余额"

const (
	cloudUsersTable    = "cloud_tool_users"
	cloudSessionsTable = "cloud_tool_sessions"
)

// cloudUsernameRuleErr 账号格式校验失败的错误提示（前端同文案）。
const cloudUsernameRuleErr = "账号格式不合法：仅支持字母、数字及 _ - . @，长度 1~32 位"

// cloudUsernameRe 账号格式：1~32 位，允许任意语言文字/数字及 _ - . @。
// 该集合天然排除空白、逗号/分号等白名单分隔符，以及 / ? # % 等 URL 特殊字符。
var cloudUsernameRe = regexp.MustCompile(`^[\p{L}\p{N}_\-\.@]{1,32}$`)

// cloudValidUsername 校验账号格式是否合法。
func cloudValidUsername(username string) bool {
	return cloudUsernameRe.MatchString(username)
}

// cloudServerPasswordHash 计算密码的 SHA-256 十六进制小写（与云工具客户端认证协议一致）。
func cloudServerPasswordHash(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}

// CloudToolSplitWhitelist 将白名单字符串按逗号/分号/换行/空格/制表符拆分为账号集合。
func CloudToolSplitWhitelist(s string) map[string]bool {
	set := make(map[string]bool)
	for _, name := range cloudToolSplitWhitelistList(s) {
		set[name] = true
	}
	return set
}

// cloudToolSplitWhitelistList 按分隔符拆分白名单字符串并保留顺序（用于重建白名单字符串）。
func cloudToolSplitWhitelistList(s string) []string {
	var list []string
	for _, name := range strings.FieldsFunc(s, func(r rune) bool {
		switch r {
		case ',', ';', '\n', '\r', ' ', '\t':
			return true
		}
		return false
	}) {
		if name = strings.TrimSpace(name); name != "" {
			list = append(list, name)
		}
	}
	return list
}

// cloudServerMsg 云工具 WebSocket 消息结构（与外部 NebulaCloudTool 协议兼容的精简版）。
type cloudServerMsg struct {
	Type     string   `json:"type"`
	ID       string   `json:"id,omitempty"`
	Func     string   `json:"func,omitempty"`
	Data     string   `json:"data,omitempty"`
	Args     []string `json:"args,omitempty"`
	Msg      string   `json:"msg,omitempty"`
	Username string   `json:"username,omitempty"`
}

var cloudServerUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// 编译后的云工具词库（启动时加载一次，之后只读）。
var (
	cloudServerData    *dto.BuildValue
	cloudServerDicPath string
)

// cloudConnState 云工具单个在线连接的计时状态。
type cloudConnState struct {
	start   time.Time // 连接建立时间
	flushed int64     // 已结算并落库的整秒数
}

// 云工具在线连接：账号 -> (连接 -> 计时状态)，用于定时落库在线时长。
var (
	cloudConnMu sync.Mutex
	cloudConns  = map[string]map[*websocket.Conn]*cloudConnState{}
)

// cloudOnlineFlushInterval 在线时长定时落库间隔。
const cloudOnlineFlushInterval = 30 * time.Second

// 云工具心跳保活参数：周期发送 ping 探测对端存活，pong 未在等待期内到达即判定连接断开。
// 保活只探测存活、不因空闲断开，与「连接就一直连接」的目标一致。
const (
	cloudPingPeriod = 25 * time.Second
	cloudPongWait   = 40 * time.Second
)

// StartCloudToolServer 读取 [云工具服务端] 配置，初始化账号表、词库目录并编译云函数词库。
// 在 loadConfig 阶段调用一次。
func StartCloudToolServer() {
	cfg := dto.ServerConfig.CloudTool
	if cfg == nil || !cfg.Open {
		return
	}

	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		debugLog.Errorf("[云工具] 全局数据库初始化失败: %v", err)
		return
	}
	if err := ensureCloudToolTables(db); err != nil {
		debugLog.Errorf("[云工具] 初始化数据表失败: %v", err)
		return
	}

	if err := ensureCloudToolDicFiles(cfg.DicDir); err != nil {
		debugLog.Errorf("[云工具] 初始化词库目录失败: %v", err)
		return
	}

	data, dicPath, err := buildCloudToolDic(cfg.DicDir)
	if err != nil {
		debugLog.Errorf("[云工具] 编译词库失败: %v", err)
		return
	}
	cloudServerData = data
	cloudServerDicPath = dicPath

	if cfg.Debug {
		debugLog.Infof("[云工具] 内置服务已启动: 路径=%s 词库=%s 任意注册=%v 白名单=%v",
			cfg.Addr, cfg.DicDir, cfg.AllowRegister, cfg.Whitelist)
	}
}

// ensureCloudToolTables 创建账号表与会话表，并确保经济系统表存在。
func ensureCloudToolTables(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS "` + cloudUsersTable + `" (
			username       TEXT PRIMARY KEY,
			password_hash  TEXT NOT NULL,
			created_at     INTEGER NOT NULL DEFAULT 0,
			online_seconds INTEGER NOT NULL DEFAULT 0
		)
	`); err != nil {
		return err
	}
	// 兼容旧表：补充 online_seconds 列（列已存在时忽略报错）
	_, _ = db.Exec(`ALTER TABLE "` + cloudUsersTable + `" ADD COLUMN online_seconds INTEGER NOT NULL DEFAULT 0`)
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS "` + cloudSessionsTable + `" (
			token      TEXT PRIMARY KEY,
			username   TEXT NOT NULL,
			created_at INTEGER NOT NULL DEFAULT 0
		)
	`); err != nil {
		return err
	}
	return dic_funcs.EnsureMoneyTable(db)
}

// ensureCloudToolDicFiles 首次运行时创建云工具词库目录并写入示例词库。
func ensureCloudToolDicFiles(dicDir string) error {
	dir := path.Join("private", dicDir)
	fq := utils.NewFileQueue(dir)
	if fq.DirExists() {
		return nil
	}
	example := utils.NewFileQueue(path.Join(dir, "示例.n"))
	if data, err := appfiles.GetFile("dic/cloudtool/示例.n"); err == nil {
		example.WriteFileByte(data)
	} else {
		// 示例文件缺失不阻断启动，只写一个占位说明文件
		example.WriteToFile("// 云工具词库目录：此目录下的 .n 文件都会被自动加载\n")
	}
	return nil
}

// buildCloudToolDic 编译云工具词库目录：通过 #引入= 目录/* 聚合目录下全部 .n 文件。
func buildCloudToolDic(dicDir string) (*dto.BuildValue, string, error) {
	entryPath := path.Join("private", dicDir, "main.n")
	entryText := "#引入=" + dicDir + "/*\n"
	dic := dic_dto.NewDic(entryPath, entryText)
	if dic == nil || dic.Data == nil {
		return nil, "", errors.New("云工具词库编译失败")
	}
	return dic.Data, entryPath, nil
}

// cloudServerLoginOrRegister 账号登录或注册。
// 白名单配置非空时，仅白名单内账号可登录/连接；账号不存在时，仅当开启“任意账号注册”才自动创建。
// 返回 registered 表示本次是否新建了账号。
func cloudServerLoginOrRegister(db *sql.DB, username, passwordHash string) (registered bool, err error) {
	username = strings.TrimSpace(username)
	if username == "" || passwordHash == "" {
		return false, errors.New("账号和密码不能为空")
	}
	if !cloudValidUsername(username) {
		return false, errors.New(cloudUsernameRuleErr)
	}

	cfg := dto.ServerConfig.CloudTool
	if cfg == nil {
		return false, errors.New("云工具服务未开启")
	}

	// 白名单：配置了白名单时，只有白名单内账号才能登录/连接
	if len(cfg.Whitelist) > 0 && !cfg.Whitelist[username] {
		return false, errors.New("账号不在白名单")
	}

	var stored string
	err = db.QueryRow(`SELECT password_hash FROM "`+cloudUsersTable+`" WHERE username=?`, username).Scan(&stored)
	if err == nil {
		if stored != passwordHash {
			return false, errors.New("账号或密码错误")
		}
		return false, nil
	}

	// 账号不存在：仅当开启任意账号注册时才自动创建
	if !cfg.AllowRegister {
		return false, errors.New("账号未注册且未开启任意账号注册")
	}

	_, err = db.Exec(
		`INSERT INTO "`+cloudUsersTable+`" (username, password_hash, created_at) VALUES (?, ?, ?)`,
		username, passwordHash, time.Now().Unix(),
	)
	if err != nil {
		return false, errors.New("注册失败")
	}
	return true, nil
}

// cloudServerCreateToken 生成登录 token 并写入会话表。
func cloudServerCreateToken(db *sql.DB, username string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	_, err := db.Exec(
		`INSERT INTO "`+cloudSessionsTable+`" (token, username, created_at) VALUES (?, ?, ?)`,
		token, username, time.Now().Unix(),
	)
	if err != nil {
		return "", err
	}
	return token, nil
}

// cloudServerValidateToken 校验 token，返回对应账号。
func cloudServerValidateToken(db *sql.DB, token string) (string, bool) {
	if token == "" {
		return "", false
	}
	var username string
	err := db.QueryRow(`SELECT username FROM "`+cloudSessionsTable+`" WHERE token=?`, token).Scan(&username)
	if err != nil {
		return "", false
	}
	return username, true
}

// cloudServerDeleteToken 删除 token（退出登录）。
func cloudServerDeleteToken(db *sql.DB, token string) error {
	_, err := db.Exec(`DELETE FROM "`+cloudSessionsTable+`" WHERE token=?`, token)
	return err
}

// cloudServerMoney 读取账号余额（返回字符串，不存在为 "0"）。
func cloudServerMoney(db *sql.DB, username string) string {
	var balance int64
	if err := db.QueryRow(
		`SELECT balance FROM "money" WHERE type=? AND account=?`,
		cloudToolMoneyType, username,
	).Scan(&balance); err != nil {
		return "0"
	}
	return strconv.FormatInt(balance, 10)
}

// cloudServerOnlineSeconds 读取账号累计在线秒数（账号不存在返回 0）。
func cloudServerOnlineSeconds(db *sql.DB, username string) int64 {
	var secs int64
	if err := db.QueryRow(
		`SELECT online_seconds FROM "`+cloudUsersTable+`" WHERE username=?`,
		username,
	).Scan(&secs); err != nil {
		return 0
	}
	return secs
}

// cloudServerAddOnlineSeconds 累加账号在线秒数（连接结束时调用）。
func cloudServerAddOnlineSeconds(db *sql.DB, username string, secs int64) {
	if secs <= 0 {
		return
	}
	_, _ = db.Exec(
		`UPDATE "`+cloudUsersTable+`" SET online_seconds = online_seconds + ? WHERE username=?`,
		secs, username,
	)
}

// CloudToolAccountInfo 云工具服务端账号信息（OPUI 账号管理用）。
type CloudToolAccountInfo struct {
	Username    string `json:"username"`   // 账号
	CreatedAt   int64  `json:"created_at"` // 注册时间（Unix 秒）
	OnlineSec   int64  `json:"online_seconds"`
	Online      bool   `json:"online"` // 是否在线
	ConnCount   int    `json:"conn_count"`
	Whitelisted bool   `json:"whitelisted"` // 是否在服务端白名单
	Balance     string `json:"balance"`     // 云工具余额（不存在为 "0"）
}

// CloudToolListAccounts 分页返回云工具服务端账号及其实时连接状态。
// keyword 按账号模糊搜索（空串不过滤）；page 从 1 开始，pageSize 每页条数；返回 (账号列表, 总条数, 错误)。
func CloudToolListAccounts(keyword string, page, pageSize int) ([]CloudToolAccountInfo, int, error) {
	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return nil, 0, err
	}
	// 服务端可能从未启用过，先确保数据表存在，避免查询报错
	if err := ensureCloudToolTables(db); err != nil {
		return nil, 0, err
	}

	// 实时连接数：账号 -> 在线连接数
	cloudConnMu.Lock()
	connCounts := make(map[string]int, len(cloudConns))
	for name, set := range cloudConns {
		connCounts[name] = len(set)
	}
	cloudConnMu.Unlock()

	// 白名单：用于标识账号是否在白名单内
	whitelist := map[string]bool{}
	if cfg := dto.ServerConfig.CloudTool; cfg != nil && cfg.Whitelist != nil {
		whitelist = cfg.Whitelist
	}

	// 账号模糊搜索
	where := ""
	var args []any
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		where = ` WHERE username LIKE ?`
		args = append(args, "%"+keyword+"%")
	}

	// 总条数
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM "`+cloudUsersTable+`"`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	rows, err := db.Query(
		`SELECT username, created_at, online_seconds FROM "`+cloudUsersTable+`"`+where+` ORDER BY created_at DESC, username ASC LIMIT ? OFFSET ?`,
		append(args, pageSize, (page-1)*pageSize)...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := make([]CloudToolAccountInfo, 0, pageSize)
	for rows.Next() {
		var it CloudToolAccountInfo
		if err := rows.Scan(&it.Username, &it.CreatedAt, &it.OnlineSec); err != nil {
			return nil, 0, err
		}
		it.ConnCount = connCounts[it.Username]
		it.Online = it.ConnCount > 0
		it.Whitelisted = whitelist[it.Username]
		it.Balance = cloudServerMoney(db, it.Username)
		list = append(list, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// CloudToolDisconnectAccount 强制断开账号：删除其全部登录 token 并关闭其全部在线连接。
// token 失效后客户端断线无法恢复连接，实现远程下线。
func CloudToolDisconnectAccount(username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("账号不能为空")
	}
	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return err
	}
	_, _ = db.Exec(`DELETE FROM "`+cloudSessionsTable+`" WHERE username=?`, username)

	// 关闭全部在线连接并结算未落库的在线时长
	cloudConnMu.Lock()
	set := cloudConns[username]
	var conns []*websocket.Conn
	var seconds int64
	for conn, st := range set {
		conns = append(conns, conn)
		seconds += elapsedCloudOnlineSeconds(st)
	}
	delete(cloudConns, username)
	cloudConnMu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
	if seconds > 0 {
		cloudServerAddOnlineSeconds(db, username, seconds)
	}
	return nil
}

// CloudToolDeleteAccount 删除账号：强制断开连接并删除账号及其全部登录 token；
// deleteBalance 为 true 时同时删除该账号的“云工具余额”。
func CloudToolDeleteAccount(username string, deleteBalance bool) error {
	if err := CloudToolDisconnectAccount(username); err != nil {
		return err
	}
	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return err
	}
	username = strings.TrimSpace(username)
	if deleteBalance {
		// 确保 money 表存在（服务端可能从未启用过，避免删除时报错）
		if err := dic_funcs.EnsureMoneyTable(db); err != nil {
			return err
		}
		if _, err := db.Exec(`DELETE FROM "money" WHERE type=? AND account=?`, cloudToolMoneyType, username); err != nil {
			return err
		}
	}
	_, err = db.Exec(`DELETE FROM "`+cloudUsersTable+`" WHERE username=?`, username)
	return err
}

// CloudToolClearAccounts 清空全部账号，白名单内的账号保留（不含则全部删除）。
// 返回 (删除数, 白名单保留数, 错误)。
func CloudToolClearAccounts() (int, int, error) {
	var whitelist map[string]bool
	if cfg := dto.ServerConfig.CloudTool; cfg != nil {
		whitelist = cfg.Whitelist
	}

	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return 0, 0, err
	}
	// 服务端可能从未启用过，先确保数据表存在，避免查询报错
	if err := ensureCloudToolTables(db); err != nil {
		return 0, 0, err
	}

	rows, err := db.Query(`SELECT username FROM "` + cloudUsersTable + `"`)
	if err != nil {
		return 0, 0, err
	}
	var toDelete []string
	var kept int
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			rows.Close()
			return 0, 0, err
		}
		if whitelist[u] {
			kept++
			continue
		}
		toDelete = append(toDelete, u)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	for _, u := range toDelete {
		if err := CloudToolDeleteAccount(u, true); err != nil {
			return 0, 0, err
		}
	}
	return len(toDelete), kept, nil
}

// CloudToolAddWhitelist 把账号加入服务端白名单并即时生效（写回 config.yaml 并同步内存配置）。
// 返回操作后该账号是否在白名单内。
func CloudToolAddWhitelist(username string) (bool, error) {
	return cloudToolSetWhitelist(username, true)
}

// CloudToolRemoveWhitelist 把账号移出服务端白名单并即时生效。
// 返回操作后该账号是否在白名单内。
func CloudToolRemoveWhitelist(username string) (bool, error) {
	return cloudToolSetWhitelist(username, false)
}

// cloudToolSetWhitelist 把账号加入（add=true）或移出（add=false）服务端白名单，
// 写回 config.yaml 的 [云工具服务端] 白名单并同步内存配置，立即生效。
// 返回操作后该账号是否在白名单内。
func cloudToolSetWhitelist(username string, add bool) (bool, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return false, errors.New("账号不能为空")
	}
	if !cloudValidUsername(username) {
		return false, errors.New(cloudUsernameRuleErr)
	}

	cfg, err := dto.LoadConfigFile()
	if err != nil {
		return false, errors.New("系统配置不存在")
	}
	d := cfg.Section("云工具服务端")
	current := d.Key("白名单").String()
	in := CloudToolSplitWhitelist(current)[username]
	if in == add {
		return in, nil // 状态无需变化
	}

	// 保序重建白名单字符串（逗号分隔）
	parts := cloudToolSplitWhitelistList(current)
	out := make([]string, 0, len(parts)+1)
	for _, name := range parts {
		if !add && name == username {
			continue // 移出：跳过目标账号
		}
		out = append(out, name)
	}
	if add {
		out = append(out, username)
	}
	d.Key("白名单").SetValue(strings.Join(out, ","))
	if err := cfg.Save(); err != nil {
		return in, err
	}

	// 同步内存配置，立即生效
	if cfg := dto.ServerConfig.CloudTool; cfg != nil {
		if cfg.Whitelist == nil {
			cfg.Whitelist = map[string]bool{}
		}
		if add {
			cfg.Whitelist[username] = true
		} else {
			delete(cfg.Whitelist, username)
		}
	}
	return add, nil
}

// CloudToolListWhitelist 分页返回白名单账号（按账号名排序，来自内存配置）。
// keyword 按账号模糊搜索（空串不过滤）；page 从 1 开始；返回 (账号列表, 总条数, 错误)。
func CloudToolListWhitelist(keyword string, page, pageSize int) ([]string, int, error) {
	whitelist := map[string]bool{}
	if cfg := dto.ServerConfig.CloudTool; cfg != nil && cfg.Whitelist != nil {
		whitelist = cfg.Whitelist
	}
	keyword = strings.TrimSpace(keyword)
	names := make([]string, 0, len(whitelist))
	for name := range whitelist {
		if keyword == "" || strings.Contains(name, keyword) {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	total := len(names)
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	start := (page - 1) * pageSize
	if start >= total {
		return []string{}, total, nil
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return names[start:end], total, nil
}

// CloudToolRenameAccount 重命名账号：先断开其在线连接并清理 token（需用新账号重新登录），再改名。
// 若旧账号在白名单内，白名单同步迁移到新账号。
func CloudToolRenameAccount(oldName, newName string) error {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if oldName == "" || newName == "" {
		return errors.New("账号不能为空")
	}
	if !cloudValidUsername(newName) {
		return errors.New(cloudUsernameRuleErr)
	}
	if oldName == newName {
		return nil
	}

	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return err
	}
	// 服务端可能从未启用过，先确保数据表存在，避免查询报错
	if err := ensureCloudToolTables(db); err != nil {
		return err
	}

	var exists int
	if err := db.QueryRow(`SELECT COUNT(*) FROM "`+cloudUsersTable+`" WHERE username=?`, oldName).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return errors.New("账号不存在")
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM "`+cloudUsersTable+`" WHERE username=?`, newName).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		return errors.New("新账号名已存在")
	}

	// 先断开在线连接并清理旧 token
	if err := CloudToolDisconnectAccount(oldName); err != nil {
		return err
	}

	// 白名单迁移：旧账号在白名单则迁移到新账号
	var wasWhitelisted bool
	if cfg := dto.ServerConfig.CloudTool; cfg != nil && cfg.Whitelist != nil {
		wasWhitelisted = cfg.Whitelist[oldName]
	}
	if wasWhitelisted {
		if _, err := cloudToolSetWhitelist(oldName, false); err != nil {
			return err
		}
		if _, err := cloudToolSetWhitelist(newName, true); err != nil {
			return err
		}
	}

	// 云工具余额跟随账号迁移到新账号名
	if _, err := db.Exec(`UPDATE "money" SET account=? WHERE type=? AND account=?`, newName, cloudToolMoneyType, oldName); err != nil {
		return err
	}

	_, err = db.Exec(`UPDATE "`+cloudUsersTable+`" SET username=? WHERE username=?`, newName, oldName)
	return err
}

// CloudToolResetPassword 重置账号密码（明文密码按协议做 SHA-256 后落库）。
// 重置后强制断开在线连接并清理 token，客户端需用新密码重新登录。
func CloudToolResetPassword(username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("账号不能为空")
	}
	password = strings.TrimSpace(password)
	if len(password) < 3 || len(password) > 64 {
		return errors.New("密码长度需为 3~64 位")
	}

	if err := CloudToolDisconnectAccount(username); err != nil {
		return err
	}
	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return err
	}
	if err := ensureCloudToolTables(db); err != nil {
		return err
	}
	res, err := db.Exec(
		`UPDATE "`+cloudUsersTable+`" SET password_hash=? WHERE username=?`,
		cloudServerPasswordHash(password), username,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("账号不存在")
	}
	return nil
}

// init 启动云工具在线时长定时落库协程，避免服务重启丢失进行中的时长。
func init() {
	go func() {
		ticker := time.NewTicker(cloudOnlineFlushInterval)
		defer ticker.Stop()
		for range ticker.C {
			flushCloudOnlineSeconds()
		}
	}()
}

// elapsedCloudOnlineSeconds 计算自上次结算以来尚未落库的整秒数（调用方需持有 cloudConnMu）。
func elapsedCloudOnlineSeconds(st *cloudConnState) int64 {
	total := int64(time.Since(st.start).Seconds())
	if total <= st.flushed {
		return 0
	}
	return total - st.flushed
}

// flushCloudOnlineSeconds 把当前所有活跃连接的进行中时长结算并落库。
func flushCloudOnlineSeconds() {
	cloudConnMu.Lock()
	type pending struct {
		username string
		seconds  int64
	}
	var batch []pending
	for username, set := range cloudConns {
		var seconds int64
		for _, st := range set {
			d := elapsedCloudOnlineSeconds(st)
			st.flushed += d
			seconds += d
		}
		if seconds > 0 {
			batch = append(batch, pending{username: username, seconds: seconds})
		}
	}
	cloudConnMu.Unlock()

	if len(batch) == 0 {
		return
	}
	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return
	}
	for _, p := range batch {
		cloudServerAddOnlineSeconds(db, p.username, p.seconds)
	}
}

// cloudAddConn 注册云工具在线连接，用于在线时长定时落库。
func cloudAddConn(username string, conn *websocket.Conn, start time.Time) {
	cloudConnMu.Lock()
	defer cloudConnMu.Unlock()
	set := cloudConns[username]
	if set == nil {
		set = map[*websocket.Conn]*cloudConnState{}
		cloudConns[username] = set
	}
	set[conn] = &cloudConnState{start: start}
}

// cloudRemoveConn 移除云工具在线连接，返回该连接尚未落库的在线秒数。
func cloudRemoveConn(username string, conn *websocket.Conn) int64 {
	cloudConnMu.Lock()
	defer cloudConnMu.Unlock()
	set := cloudConns[username]
	if set == nil {
		return 0
	}
	st, ok := set[conn]
	if !ok {
		return 0
	}
	delete(set, conn)
	if len(set) == 0 {
		delete(cloudConns, username)
	}
	return elapsedCloudOnlineSeconds(st)
}

// cloudElapsedOnline 返回指定连接尚未落库的在线秒数（account_info 用于返回实时在线时长）。
func cloudElapsedOnline(username string, conn *websocket.Conn) int64 {
	cloudConnMu.Lock()
	defer cloudConnMu.Unlock()
	set := cloudConns[username]
	if set == nil {
		return 0
	}
	st, ok := set[conn]
	if !ok {
		return 0
	}
	return elapsedCloudOnlineSeconds(st)
}

// cloudServerListFuncs 扫描词库全部 [函数]，返回 函数名 -> {rule, desc} 的 JSON。
func cloudServerListFuncs(data *dto.BuildValue) string {
	infos := make(map[string]map[string]string)
	for _, item := range data.DicFuncs["函数"] {
		rule := item.ParamRule
		if rule == "" {
			rule = "0"
		}
		info := map[string]string{"rule": rule}
		if item.Desc != "" {
			info["desc"] = item.Desc
		}
		infos[item.Trigger] = info
	}
	b, _ := json.Marshal(infos)
	return string(b)
}

// callCloudToolFunc 执行云工具词库中指定的 [函数]，返回执行结果字符串。
// username 会注入为变量 %账号%，参数按 %参数1%..%参数N% 注入。
func callCloudToolFunc(data *dto.BuildValue, dicPath, username, name string, args []string) (string, error) {
	if data == nil {
		return "", errors.New("云工具词库未就绪")
	}

	str, Tstr, _, errRule, ok := run.RunFuncIndexed(data.GetFuncIndex(), name, len(args))
	if !ok {
		if errRule != "" {
			return "", fmt.Errorf("参数数量错误(需要%s，实际%d)", errRule, len(args))
		}
		return "", fmt.Errorf("未找到云函数: %s", name)
	}

	full := append([]string{name}, args...)
	text := strings.Join(full, " ")

	v := dto.NewDicVal()
	funcv := dto.NewVal().
		Set("触发", Tstr).
		Set("触发词", text).
		Set("账号", username)
	v.G.SetRaw("_词库路径_", dicPath)

	dto.ValRunTrigger(text, Tstr, v.NewDicVal(funcv), v)

	runDic := dic_dto.NewRunDicEntry().
		CloseTrigger().
		SetGlobal_v(v.G).
		Set_v(funcv).
		SetDic_v(data)

	return dic_api.Api.DicRunLine(runDic, str), nil
}

// cloudServerConn 串行化单个连接的写操作。
type cloudServerConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *cloudServerConn) write(msg cloudServerMsg) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.conn.WriteJSON(msg)
}

// CloudToolServerHandler 处理内置云工具的 WebSocket 连接。
func CloudToolServerHandler(w http.ResponseWriter, r *http.Request) {
	cfg := dto.ServerConfig.CloudTool
	if cfg == nil || !cfg.Open {
		http.NotFound(w, r)
		return
	}
	if cloudServerData == nil {
		http.Error(w, "云工具词库未就绪", http.StatusServiceUnavailable)
		return
	}

	conn, err := cloudServerUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(1 << 20)

	pathUsername := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, cfg.Addr+"/"))
	username, token, ok := cloudServerAuth(conn, pathUsername)
	if !ok {
		conn.Close()
		return
	}
	loginAt := time.Now() // 本次连接开始时间
	cloudAddConn(username, conn, loginAt)

	// 断开自动注销阈值（秒）：连接断开时本次在线时长低于该值即注销，0 表示关闭
	logoutAfter := time.Duration(cfg.LogoutSec) * time.Second

	wc := &cloudServerConn{conn: conn}

	// 心跳保活：周期发送 ping 探测连接是否真的存活（不做空闲超时断开）。
	// 存活连接由 ping/pong 持续刷新读超时；探测失败（读超时）按断开处理，由下方错误分支判断是否注销
	conn.SetReadDeadline(time.Now().Add(cloudPongWait))
	conn.SetPingHandler(func(appData string) error {
		conn.SetReadDeadline(time.Now().Add(cloudPongWait))
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(10*time.Second))
	})
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(cloudPongWait))
	})
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

	// 连接后保持在线，不做空闲超时断开；仅在对端断开或显式登出时结束
	defer func() {
		conn.Close()
		// 连接结束时结算剩余在线时长并移除连接
		if db, err := dic_funcs.GetGlobalDB(); err == nil {
			cloudServerAddOnlineSeconds(db, username, cloudRemoveConn(username, conn))
		}
	}()

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			// 连接断开：本次在线时长不足配置阈值视为异常连接，自动注销（删除 token）；
			// 否则保留 token，供客户端 resume 恢复连接
			if logoutAfter > 0 && time.Since(loginAt) < logoutAfter {
				_ = wc.write(cloudServerMsg{Type: "logout_ok", Msg: "在线时间过短已自动注销"})
				if db, dbErr := dic_funcs.GetGlobalDB(); dbErr == nil {
					_ = cloudServerDeleteToken(db, token)
				}
			}
			return
		}

		var msg cloudServerMsg
		if err := json.Unmarshal(raw, &msg); err != nil {
			_ = wc.write(cloudServerMsg{Type: "error", Msg: "消息格式错误"})
			continue
		}

		if cfg.Debug {
			debugLog.Infof("[云工具] 收到: username=%s type=%s func=%s id=%s", username, msg.Type, msg.Func, msg.ID)
		}

		switch msg.Type {
		case "exec":
			res, execErr := callCloudToolFunc(cloudServerData, cloudServerDicPath, username, msg.Func, msg.Args)
			if execErr != nil {
				_ = wc.write(cloudServerMsg{Type: "error", ID: msg.ID, Msg: execErr.Error()})
			} else {
				_ = wc.write(cloudServerMsg{Type: "result", ID: msg.ID, Data: res})
			}
		case "list_funcs":
			_ = wc.write(cloudServerMsg{Type: "result", ID: msg.ID, Data: cloudServerListFuncs(cloudServerData)})
		case "account_info":
			db, dbErr := dic_funcs.GetGlobalDB()
			if dbErr != nil {
				_ = wc.write(cloudServerMsg{Type: "error", ID: msg.ID, Msg: "数据库不可用"})
				continue
			}
			onlineSecs := cloudServerOnlineSeconds(db, username) + cloudElapsedOnline(username, conn)
			b, _ := json.Marshal(map[string]any{
				"username":       username,
				"money":          cloudServerMoney(db, username),
				"type":           cloudToolMoneyType,
				"online_seconds": onlineSecs,
			})
			_ = wc.write(cloudServerMsg{Type: "result", ID: msg.ID, Data: string(b)})
		case "logout":
			if db, err := dic_funcs.GetGlobalDB(); err == nil {
				_ = cloudServerDeleteToken(db, token)
			}
			_ = wc.write(cloudServerMsg{Type: "logout_ok"})
			return
		default:
			_ = wc.write(cloudServerMsg{Type: "error", ID: msg.ID, Msg: "未知消息类型: " + msg.Type})
		}
	}
}

// cloudServerAuth 读取首条消息完成认证（auth 密码登录 / resume token 恢复），成功返回账号与 token。
// 新注册成功后会执行词库 [函数] 注册信息，并把其返回作为注册信息回传。
func cloudServerAuth(conn *websocket.Conn, pathUsername string) (username, token string, ok bool) {
	_, raw, err := conn.ReadMessage()
	if err != nil {
		return "", "", false
	}

	var msg cloudServerMsg
	if err := json.Unmarshal(raw, &msg); err != nil {
		_ = conn.WriteJSON(cloudServerMsg{Type: "auth_fail", Msg: "认证消息错误"})
		return "", "", false
	}
	if msg.Username != "" {
		pathUsername = msg.Username
	}

	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		_ = conn.WriteJSON(cloudServerMsg{Type: "auth_fail", Msg: "数据库初始化失败"})
		return "", "", false
	}

	switch msg.Type {
	case "auth":
		username = strings.TrimSpace(pathUsername)
		if username == "" {
			_ = conn.WriteJSON(cloudServerMsg{Type: "auth_fail", Msg: "账号不能为空"})
			return "", "", false
		}
		registered, err := cloudServerLoginOrRegister(db, username, msg.Data)
		if err != nil {
			_ = conn.WriteJSON(cloudServerMsg{Type: "auth_fail", Msg: err.Error()})
			return "", "", false
		}
		token, err = cloudServerCreateToken(db, username)
		if err != nil {
			_ = conn.WriteJSON(cloudServerMsg{Type: "auth_fail", Msg: "登录失败"})
			return "", "", false
		}

		info := ""
		if registered {
			if s, e := callCloudToolFunc(cloudServerData, cloudServerDicPath, username, "注册信息", nil); e == nil {
				info = s
			}
		}
		_ = conn.WriteJSON(cloudServerMsg{Type: "auth_ok", Data: token, Msg: info})
		return username, token, true

	case "resume":
		username, ok = cloudServerValidateToken(db, msg.Data)
		if !ok {
			_ = conn.WriteJSON(cloudServerMsg{Type: "auth_fail", Msg: "登录已过期"})
			return "", "", false
		}
		if pathUsername != "" && username != pathUsername {
			_ = conn.WriteJSON(cloudServerMsg{Type: "auth_fail", Msg: "账号不匹配"})
			return "", "", false
		}
		_ = conn.WriteJSON(cloudServerMsg{Type: "auth_ok", Data: msg.Data})
		return username, msg.Data, true

	default:
		_ = conn.WriteJSON(cloudServerMsg{Type: "auth_fail", Msg: "认证消息错误"})
		return "", "", false
	}
}
