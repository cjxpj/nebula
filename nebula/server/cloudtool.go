//go:build !js

package dic_server

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"path/filepath"
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
	cloudUsersTable      = "cloud_tool_users"
	cloudSessionsTable   = "cloud_tool_sessions"
	cloudShopOrdersTable = "cloud_tool_shop_orders"
	cloudShopItemsTable  = "cloud_tool_shop_items" // 词库商品表（内容存数据库）
	cloudImagesTable     = "cloud_tool_images"     // 图床图片表（内容存数据库）
	cloudImageMaxSize    = 10 << 20                // 图床单张图片最大 10MB
)

// cloudShopPublishMaxSize 发布词库时单个文件的内容上限。
const cloudShopPublishMaxSize = 8 << 20

// 资源审核状态：0 待审核，1 已通过，2 已拒绝。
const (
	cloudReviewPending  = 0
	cloudReviewApproved = 1
	cloudReviewRejected = 2
)

// cloudShopIDRe 商品 ID 允许的字符：任意语言文字/数字及 _ - . @ 空格，其余统一替换为下划线。
var cloudShopIDRe = regexp.MustCompile(`[^\p{L}\p{N}_\-\.@ ]`)

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

// 编译后的云工具词库（启动时加载，可通过「更新词库」热替换）。
var (
	cloudServerDataMu  sync.RWMutex
	cloudServerData    *dto.BuildValue
	cloudServerDicPath string
)

// cloudServerDic 返回当前云工具词库快照，供连接处理与热更新并发安全地读取。
func cloudServerDic() (*dto.BuildValue, string) {
	cloudServerDataMu.RLock()
	defer cloudServerDataMu.RUnlock()
	return cloudServerData, cloudServerDicPath
}

// cloudConnState 云工具单个在线连接的计时状态。
type cloudConnState struct {
	start   time.Time        // 连接建立时间
	flushed int64            // 已结算并落库的整秒数
	wc      *cloudServerConn // 该连接的写串行化封装，供服务端主动推送通知
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
	cloudServerDataMu.Lock()
	cloudServerData = data
	cloudServerDicPath = dicPath
	cloudServerDataMu.Unlock()

	if cfg.Debug {
		debugLog.Infof("[云工具] 内置服务已启动: 路径=%s 词库=%s 任意注册=%v 白名单=%v",
			cfg.Addr, cfg.DicDir, cfg.AllowRegister, cfg.Whitelist)
	}
}

// CloudToolReloadDic 重新编译云工具词库并热替换全局词库，随后向所有在线连接推送「词库已更新」通知。
// 供面板「更新词库」按钮使用：新增或修改云函数后无需重启服务、无需断开连接即可生效。
func CloudToolReloadDic() error {
	cfg := dto.ServerConfig.CloudTool
	if cfg == nil || !cfg.Open {
		return errors.New("云工具服务未开启")
	}
	if err := ensureCloudToolDicFiles(cfg.DicDir); err != nil {
		return fmt.Errorf("初始化词库目录失败: %w", err)
	}
	data, dicPath, err := buildCloudToolDic(cfg.DicDir)
	if err != nil {
		return err
	}

	cloudServerDataMu.Lock()
	cloudServerData = data
	cloudServerDicPath = dicPath
	cloudServerDataMu.Unlock()

	cloudServerNotifyDicUpdated()
	if cfg.Debug {
		debugLog.Infof("[云工具] 词库已热更新: 词库=%s", cfg.DicDir)
	}
	return nil
}

// cloudServerNotifyDicUpdated 向全部在线连接推送「词库已更新」通知，客户端收到后自动重新拉取云函数。
func cloudServerNotifyDicUpdated() {
	cloudConnMu.Lock()
	conns := make([]*cloudServerConn, 0, len(cloudConns))
	for _, set := range cloudConns {
		for _, st := range set {
			if st.wc != nil {
				conns = append(conns, st.wc)
			}
		}
	}
	cloudConnMu.Unlock()

	for _, wc := range conns {
		_ = wc.write(cloudServerMsg{Type: "dic_updated"})
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
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS "` + cloudShopOrdersTable + `" (
			username   TEXT NOT NULL,
			item_id    TEXT NOT NULL,
			created_at INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (username, item_id)
		)
	`); err != nil {
		return err
	}
	// 词库商品表：内容与元数据全部存库
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS "` + cloudShopItemsTable + `" (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL DEFAULT '',
			price       INTEGER NOT NULL DEFAULT 0,
			description TEXT NOT NULL DEFAULT '',
			content     BLOB NOT NULL,
			created_at  INTEGER NOT NULL DEFAULT 0,
			status      INTEGER NOT NULL DEFAULT 1,
			uploader    TEXT NOT NULL DEFAULT '',
			reviewed_at INTEGER NOT NULL DEFAULT 0,
			reviewer    TEXT NOT NULL DEFAULT ''
		)
	`); err != nil {
		return err
	}
	// 兼容旧表：补充审核相关列（列已存在时忽略报错，旧数据默认视为已通过）
	for _, col := range []string{
		`status INTEGER NOT NULL DEFAULT 1`,
		`uploader TEXT NOT NULL DEFAULT ''`,
		`reviewed_at INTEGER NOT NULL DEFAULT 0`,
		`reviewer TEXT NOT NULL DEFAULT ''`,
	} {
		_, _ = db.Exec(`ALTER TABLE "` + cloudShopItemsTable + `" ADD COLUMN ` + col)
	}
	// 图床图片表：图片二进制存库
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS "` + cloudImagesTable + `" (
			name        TEXT PRIMARY KEY,
			ext         TEXT NOT NULL DEFAULT '',
			data        BLOB NOT NULL,
			created_at  INTEGER NOT NULL DEFAULT 0,
			status      INTEGER NOT NULL DEFAULT 1,
			uploader    TEXT NOT NULL DEFAULT '',
			reviewed_at INTEGER NOT NULL DEFAULT 0,
			reviewer    TEXT NOT NULL DEFAULT ''
		)
	`); err != nil {
		return err
	}
	for _, col := range []string{
		`status INTEGER NOT NULL DEFAULT 1`,
		`uploader TEXT NOT NULL DEFAULT ''`,
		`reviewed_at INTEGER NOT NULL DEFAULT 0`,
		`reviewer TEXT NOT NULL DEFAULT ''`,
	} {
		_, _ = db.Exec(`ALTER TABLE "` + cloudImagesTable + `" ADD COLUMN ` + col)
	}
	// 兼容旧表：账号表补充审核员列
	_, _ = db.Exec(`ALTER TABLE "` + cloudUsersTable + `" ADD COLUMN reviewer INTEGER NOT NULL DEFAULT 0`)
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

// CloudToolSetBalance 直接设置账号的云工具余额（不存在则创建），供 OPUI 面板手动修改余额。
func CloudToolSetBalance(username string, balance int64) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("账号不能为空")
	}
	if balance < 0 {
		return errors.New("余额不能为负数")
	}
	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return err
	}
	if err := dic_funcs.EnsureMoneyTable(db); err != nil {
		return err
	}
	res, err := db.Exec(
		`UPDATE "money" SET balance=? WHERE type=? AND account=?`,
		balance, cloudToolMoneyType, username,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		_, err = db.Exec(
			`INSERT INTO "money" (type, account, balance) VALUES (?, ?, ?)`,
			cloudToolMoneyType, username, balance,
		)
	}
	return err
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

// ============ 词库商城 ============

// cloudShopItem 词库商城单个在售词库（内容字段不随列表返回，仅下载时单独读取）。
type cloudShopItem struct {
	ID    string `json:"id"`    // 商品 ID
	Name  string `json:"name"`  // 名称
	Price int64  `json:"price"` // 价格（云工具余额，0 表示免费）
	Desc  string `json:"desc"`  // 描述
}

// cloudShopListItems 返回全部在售词库（仅已审核通过的，读取数据库）。
func cloudShopListItems(db *sql.DB) []cloudShopItem {
	rows, err := db.Query(
		`SELECT id, name, price, description FROM "`+cloudShopItemsTable+`" WHERE status=? ORDER BY id`,
		cloudReviewApproved,
	)
	if err != nil {
		return []cloudShopItem{}
	}
	defer rows.Close()
	items := make([]cloudShopItem, 0)
	for rows.Next() {
		var it cloudShopItem
		if err := rows.Scan(&it.ID, &it.Name, &it.Price, &it.Desc); err != nil {
			continue
		}
		items = append(items, it)
	}
	return items
}

// cloudShopFind 按商品 ID 查找在售词库（仅已审核通过的可见）。
func cloudShopFind(db *sql.DB, itemID string) (cloudShopItem, bool) {
	var it cloudShopItem
	err := db.QueryRow(
		`SELECT id, name, price, description FROM "`+cloudShopItemsTable+`" WHERE id=? AND status=?`,
		itemID, cloudReviewApproved,
	).Scan(&it.ID, &it.Name, &it.Price, &it.Desc)
	if err != nil {
		return cloudShopItem{}, false
	}
	return it, true
}

// cloudShopExists 判断商品 ID 是否已被占用（不限审核状态，发布时用于同名检测）。
func cloudShopExists(db *sql.DB, itemID string) bool {
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM "`+cloudShopItemsTable+`" WHERE id=?`, itemID,
	).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// cloudShopPurchased 判断账号是否已购买指定词库。
func cloudShopPurchased(db *sql.DB, username, itemID string) bool {
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM "`+cloudShopOrdersTable+`" WHERE username=? AND item_id=?`,
		username, itemID,
	).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// cloudShopList 返回词库商城商品列表 JSON（含账号是否已购买标记）。
func cloudShopList(db *sql.DB, username string) string {
	type out struct {
		cloudShopItem
		Purchased bool `json:"purchased"`
	}
	items := cloudShopListItems(db)
	list := make([]out, 0, len(items))
	for _, it := range items {
		list = append(list, out{cloudShopItem: it, Purchased: cloudShopPurchased(db, username, it.ID)})
	}
	b, _ := json.Marshal(list)
	return string(b)
}

// cloudShopBuy 购买词库：价格大于 0 时原子扣减余额并记录订单，免费词库直接成功。
func cloudShopBuy(db *sql.DB, username, itemID string) error {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return errors.New("商品不能为空")
	}
	it, ok := cloudShopFind(db, itemID)
	if !ok {
		return errors.New("商品不存在")
	}
	if cloudShopPurchased(db, username, itemID) {
		return nil // 已购买，幂等
	}
	if it.Price > 0 {
		res, err := db.Exec(
			`UPDATE "money" SET balance = balance - ? WHERE type=? AND account=? AND balance >= ?`,
			it.Price, cloudToolMoneyType, username, it.Price,
		)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return errors.New("余额不足")
		}
	}
	_, err := db.Exec(
		`INSERT OR IGNORE INTO "`+cloudShopOrdersTable+`" (username, item_id, created_at) VALUES (?, ?, ?)`,
		username, itemID, time.Now().Unix(),
	)
	return err
}

// cloudShopDownload 返回已购（或免费）词库的原始内容文本。
func cloudShopDownload(db *sql.DB, username, itemID string) (string, error) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return "", errors.New("商品不能为空")
	}
	it, ok := cloudShopFind(db, itemID)
	if !ok {
		return "", errors.New("商品不存在")
	}
	if it.Price > 0 && !cloudShopPurchased(db, username, itemID) {
		return "", errors.New("尚未购买该词库")
	}
	var content []byte
	if err := db.QueryRow(
		`SELECT content FROM "`+cloudShopItemsTable+`" WHERE id=?`, itemID,
	).Scan(&content); err != nil {
		return "", errors.New("读取词库失败")
	}
	return string(content), nil
}

// cloudShopSafeID 由上传文件名（去扩展名）生成商品 ID，过滤路径分隔符等非法字符。
func cloudShopSafeID(name string) string {
	name = strings.TrimSuffix(strings.TrimSpace(name), filepath.Ext(strings.TrimSpace(name)))
	name = strings.TrimSpace(cloudShopIDRe.ReplaceAllString(name, "_"))
	name = strings.Trim(name, ". ")
	if name == "" || name == "." || name == ".." {
		return ""
	}
	if r := []rune(name); len(r) > 60 {
		name = string(r[:60])
	}
	return name
}

// cloudShopStripMeta 去掉内容开头已有的 //@ 元数据注释行，避免与发布时填写的元数据冲突。
func cloudShopStripMeta(text string) string {
	lines := strings.Split(text, "\n")
	i := 0
	for ; i < len(lines) && i < 50; i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" || strings.HasPrefix(t, "//@") {
			continue
		}
		break
	}
	return strings.Join(lines[i:], "\n")
}

// cloudShopPublish 发布词库：解码 base64 内容并写入数据库，元数据同时存表。
// Args: [0]文件名 [1]名称 [2]价格 [3]描述；商品 ID 取自文件名（去扩展名），同名商品不允许覆盖。
// username 为上传者；开启词库审核时新词库先进入待审核状态，通过后才上架。
func cloudShopPublish(db *sql.DB, username string, msg cloudServerMsg) (string, error) {
	if strings.TrimSpace(msg.Data) == "" {
		return "", errors.New("词库内容不能为空")
	}
	raw, err := base64.StdEncoding.DecodeString(msg.Data)
	if err != nil {
		if raw2, err2 := base64.RawStdEncoding.DecodeString(msg.Data); err2 == nil {
			raw = raw2
		} else {
			return "", errors.New("词库内容不是合法的 base64")
		}
	}
	if len(raw) == 0 {
		return "", errors.New("词库内容为空")
	}
	if len(raw) > cloudShopPublishMaxSize {
		return "", errors.New("词库过大（最大 8MB）")
	}

	arg := func(i int) string {
		if i < len(msg.Args) {
			return strings.TrimSpace(msg.Args[i])
		}
		return ""
	}
	id := cloudShopSafeID(arg(0))
	if id == "" {
		return "", errors.New("词库文件名不合法")
	}
	if cloudShopExists(db, id) {
		return "", errors.New("同名词库已存在，请修改文件名后重试")
	}

	// 元数据行内的换行会破坏头部解析，统一压成空格
	oneLine := strings.NewReplacer("\r", " ", "\n", " ")
	name := oneLine.Replace(arg(1))
	if name == "" {
		name = id
	}
	desc := oneLine.Replace(arg(3))
	price := int64(0)
	if p := arg(2); p != "" {
		v, perr := strconv.ParseInt(p, 10, 64)
		if perr != nil || v < 0 {
			return "", errors.New("价格必须是不小于 0 的整数")
		}
		price = v
	}

	status := cloudReviewApproved
	if cfg := dto.ServerConfig.CloudTool; cfg != nil && cfg.ShopReview {
		status = cloudReviewPending
	}

	content := "//@名称: " + name + "\n//@价格: " + strconv.FormatInt(price, 10) + "\n//@描述: " + desc + "\n" + cloudShopStripMeta(string(raw))
	if _, err := db.Exec(
		`INSERT INTO "`+cloudShopItemsTable+`" (id, name, price, description, content, created_at, status, uploader) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, name, price, desc, []byte(content), time.Now().Unix(), status, username,
	); err != nil {
		return "", errors.New("保存词库失败")
	}
	if status == cloudReviewPending {
		return "发布成功，等待审核通过后上架", nil
	}
	return "发布成功", nil
}

// ============ 图床 ============

// cloudImageExtByType 按图片内容类型返回扩展名，不支持的返回空串。
func cloudImageExtByType(contentType string) string {
	switch contentType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/bmp":
		return ".bmp"
	case "image/svg+xml":
		return ".svg"
	}
	return ""
}

// cloudImageExtByName 按上传文件名返回已知图片扩展名（用于覆盖自动识别结果）。
func cloudImageExtByName(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg":
		return strings.ToLower(filepath.Ext(name))
	}
	return ""
}

// cloudImageUpload 解码 base64 图片并存入库，返回外链 URL。
// username 为上传者；开启图床审核时新图片先进入待审核状态，通过后外链才可访问。
func cloudImageUpload(db *sql.DB, r *http.Request, msg cloudServerMsg, username string) (url string, pending bool, err error) {
	if msg.Data == "" {
		return "", false, errors.New("图片数据不能为空")
	}
	raw, err := base64.StdEncoding.DecodeString(msg.Data)
	if err != nil {
		if raw2, err2 := base64.RawStdEncoding.DecodeString(msg.Data); err2 == nil {
			raw = raw2
		} else {
			return "", false, errors.New("图片数据不是合法的 base64")
		}
	}
	if len(raw) == 0 {
		return "", false, errors.New("图片数据为空")
	}
	if len(raw) > cloudImageMaxSize {
		return "", false, errors.New("图片过大（最大 10MB）")
	}
	ext := cloudImageExtByType(http.DetectContentType(raw))
	if len(msg.Args) > 0 {
		if e := cloudImageExtByName(msg.Args[0]); e != "" {
			ext = e
		}
	}
	if ext == "" {
		return "", false, errors.New("不支持的图片类型")
	}

	status := cloudReviewApproved
	if cfg := dto.ServerConfig.CloudTool; cfg != nil && cfg.ImageReview {
		status = cloudReviewPending
	}

	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), hex.EncodeToString(b), ext)
	if _, err := db.Exec(
		`INSERT INTO "`+cloudImagesTable+`" (name, ext, data, created_at, status, uploader) VALUES (?, ?, ?, ?, ?, ?)`,
		name, ext, raw, time.Now().Unix(), status, username,
	); err != nil {
		return "", false, errors.New("保存图片失败")
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	addr := "/cloudtool"
	if cfg := dto.ServerConfig.CloudTool; cfg != nil && cfg.Addr != "" {
		addr = cfg.Addr
	}
	return fmt.Sprintf("%s://%s%s/image/%s", scheme, r.Host, addr, name), status == cloudReviewPending, nil
}

// cloudImageServe 处理图床图片 GET 请求并从数据库返回图片内容（仅已审核通过的图片可访问）。
func cloudImageServe(db *sql.DB, w http.ResponseWriter, r *http.Request, name string) {
	var data []byte
	if err := db.QueryRow(
		`SELECT data FROM "`+cloudImagesTable+`" WHERE name=? AND status=?`, name, cloudReviewApproved,
	).Scan(&data); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", http.DetectContentType(data))
	_, _ = w.Write(data)
}

// ============ 资源管理与审核 ============

// 资源类型：词库商城 / 图床。
const (
	cloudResourceShop  = "shop"
	cloudResourceImage = "image"
)

// cloudResourceTable 返回资源类型对应的数据表名与主键列名。
func cloudResourceTable(kind string) (table, idColumn string, ok bool) {
	switch kind {
	case cloudResourceShop:
		return cloudShopItemsTable, "id", true
	case cloudResourceImage:
		return cloudImagesTable, "name", true
	}
	return "", "", false
}

// CloudToolShopResource 词库商城资源（面板资源管理与审核用）。
type CloudToolShopResource struct {
	ID         string `json:"id"`    // 商品 ID（文件名）
	Name       string `json:"name"`  // 名称
	Price      int64  `json:"price"` // 价格
	Desc       string `json:"desc"`  // 描述
	Uploader   string `json:"uploader"`
	CreatedAt  int64  `json:"created_at"`
	Status     int    `json:"status"` // 0 待审 1 通过 2 拒绝
	Reviewer   string `json:"reviewer"`
	ReviewedAt int64  `json:"reviewed_at"`
}

// CloudToolImageResource 图床资源（面板资源管理与审核用）。
type CloudToolImageResource struct {
	Name       string `json:"name"`
	Path       string `json:"path"` // 相对访问路径（/访问路径/image/文件名）
	Size       int64  `json:"size"`
	Uploader   string `json:"uploader"`
	CreatedAt  int64  `json:"created_at"`
	Status     int    `json:"status"`
	Reviewer   string `json:"reviewer"`
	ReviewedAt int64  `json:"reviewed_at"`
}

// cloudServerIsReviewer 判断账号是否拥有审核权限。
func cloudServerIsReviewer(db *sql.DB, username string) bool {
	var reviewer int
	if err := db.QueryRow(
		`SELECT reviewer FROM "`+cloudUsersTable+`" WHERE username=?`, username,
	).Scan(&reviewer); err != nil {
		return false
	}
	return reviewer != 0
}

// CloudToolSetReviewer 授予（reviewer=true）或取消（reviewer=false）账号的审核权限。
func CloudToolSetReviewer(username string, reviewer bool) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("账号不能为空")
	}
	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return err
	}
	if err := ensureCloudToolTables(db); err != nil {
		return err
	}
	v := 0
	if reviewer {
		v = 1
	}
	res, err := db.Exec(`UPDATE "`+cloudUsersTable+`" SET reviewer=? WHERE username=?`, v, username)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("账号不存在")
	}
	return nil
}

// cloudResourcePage 规范化分页参数。
func cloudResourcePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	return page, pageSize
}

// cloudResourceWhere 按审核状态（-1 表示全部）与名称关键字拼接查询条件。
func cloudResourceWhere(keyword string, status int) (string, []any) {
	conds := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if status >= 0 {
		conds = append(conds, "status=?")
		args = append(args, status)
	}
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		conds = append(conds, "name LIKE ?")
		args = append(args, "%"+keyword+"%")
	}
	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// cloudToolListShopResources 分页返回词库商城资源（含待审与已拒绝）。
func cloudToolListShopResources(db *sql.DB, keyword string, status, page, pageSize int) ([]CloudToolShopResource, int, error) {
	where, args := cloudResourceWhere(keyword, status)
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM "`+cloudShopItemsTable+`"`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	page, pageSize = cloudResourcePage(page, pageSize)
	rows, err := db.Query(
		`SELECT id, name, price, description, uploader, created_at, status, reviewer, reviewed_at
		 FROM "`+cloudShopItemsTable+`"`+where+` ORDER BY created_at DESC, id ASC LIMIT ? OFFSET ?`,
		append(args, pageSize, (page-1)*pageSize)...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	list := make([]CloudToolShopResource, 0, pageSize)
	for rows.Next() {
		var it CloudToolShopResource
		if err := rows.Scan(&it.ID, &it.Name, &it.Price, &it.Desc, &it.Uploader, &it.CreatedAt, &it.Status, &it.Reviewer, &it.ReviewedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// cloudToolListImageResources 分页返回图床资源（含待审与已拒绝）。
func cloudToolListImageResources(db *sql.DB, keyword string, status, page, pageSize int) ([]CloudToolImageResource, int, error) {
	where, args := cloudResourceWhere(keyword, status)
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM "`+cloudImagesTable+`"`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	page, pageSize = cloudResourcePage(page, pageSize)
	rows, err := db.Query(
		`SELECT name, LENGTH(data), uploader, created_at, status, reviewer, reviewed_at
		 FROM "`+cloudImagesTable+`"`+where+` ORDER BY created_at DESC, name ASC LIMIT ? OFFSET ?`,
		append(args, pageSize, (page-1)*pageSize)...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	addr := "/cloudtool"
	if cfg := dto.ServerConfig.CloudTool; cfg != nil && cfg.Addr != "" {
		addr = cfg.Addr
	}
	list := make([]CloudToolImageResource, 0, pageSize)
	for rows.Next() {
		var it CloudToolImageResource
		if err := rows.Scan(&it.Name, &it.Size, &it.Uploader, &it.CreatedAt, &it.Status, &it.Reviewer, &it.ReviewedAt); err != nil {
			return nil, 0, err
		}
		it.Path = addr + "/image/" + it.Name
		list = append(list, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// CloudToolListResources 分页返回云工具资源（kind: shop|image；status: -1 全部，0 待审，1 通过，2 拒绝）。
func CloudToolListResources(kind, keyword string, status, page, pageSize int) ([]any, int, error) {
	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return nil, 0, err
	}
	// 服务端可能从未启用过，先确保数据表存在，避免查询报错
	if err := ensureCloudToolTables(db); err != nil {
		return nil, 0, err
	}
	var items []any
	var total int
	switch kind {
	case cloudResourceShop:
		list, n, err := cloudToolListShopResources(db, keyword, status, page, pageSize)
		if err != nil {
			return nil, 0, err
		}
		items, total = make([]any, 0, len(list)), n
		for _, it := range list {
			items = append(items, it)
		}
	case cloudResourceImage:
		list, n, err := cloudToolListImageResources(db, keyword, status, page, pageSize)
		if err != nil {
			return nil, 0, err
		}
		items, total = make([]any, 0, len(list)), n
		for _, it := range list {
			items = append(items, it)
		}
	default:
		return nil, 0, errors.New("资源类型不合法")
	}
	return items, total, nil
}

// CloudToolReviewResource 处理资源审核动作（kind: shop|image；action: approve|reject|delete）。
// reviewer 为操作者标识（面板操作固定为“面板”，客户端审核员为其账号）。
func CloudToolReviewResource(kind, id, action, reviewer string) error {
	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return err
	}
	if err := ensureCloudToolTables(db); err != nil {
		return err
	}
	table, idColumn, ok := cloudResourceTable(kind)
	if !ok {
		return errors.New("资源类型不合法")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("资源标识不能为空")
	}
	switch action {
	case "delete":
		_, err := db.Exec(`DELETE FROM "`+table+`" WHERE `+idColumn+`=?`, id)
		return err
	case "approve", "reject":
		status := cloudReviewApproved
		if action == "reject" {
			status = cloudReviewRejected
		}
		res, err := db.Exec(
			`UPDATE "`+table+`" SET status=?, reviewer=?, reviewed_at=? WHERE `+idColumn+`=?`,
			status, reviewer, time.Now().Unix(), id,
		)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return errors.New("资源不存在")
		}
		return nil
	}
	return errors.New("审核动作不合法")
}

// cloudImageMimeByExt 按扩展名返回图片 MIME 类型。
func cloudImageMimeByExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".svg":
		return "image/svg+xml"
	}
	return "application/octet-stream"
}

// CloudToolResourceContent 资源内容预览：词库返回文本内容，图床返回 base64 data URL。
func CloudToolResourceContent(kind, id string) (string, error) {
	db, err := dic_funcs.GetGlobalDB()
	if err != nil {
		return "", err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return "", errors.New("资源标识不能为空")
	}
	switch kind {
	case cloudResourceShop:
		var content []byte
		if err := db.QueryRow(`SELECT content FROM "`+cloudShopItemsTable+`" WHERE id=?`, id).Scan(&content); err != nil {
			return "", errors.New("词库不存在")
		}
		const maxPreview = 256 << 10
		if len(content) > maxPreview {
			return string(content[:maxPreview]) + "\n\n……（内容过长，仅显示前 256KB）", nil
		}
		return string(content), nil
	case cloudResourceImage:
		var data []byte
		var ext string
		if err := db.QueryRow(`SELECT data, ext FROM "`+cloudImagesTable+`" WHERE name=?`, id).Scan(&data, &ext); err != nil {
			return "", errors.New("图片不存在")
		}
		if len(data) > 4<<20 {
			return "", errors.New("图片过大（超过 4MB），无法预览")
		}
		return "data:" + cloudImageMimeByExt(ext) + ";base64," + base64.StdEncoding.EncodeToString(data), nil
	}
	return "", errors.New("资源类型不合法")
}

// cloudServerReviewList 返回待审核资源列表 JSON（审核员在客户端查看）。
func cloudServerReviewList(db *sql.DB, username string) (string, error) {
	if !cloudServerIsReviewer(db, username) {
		return "", errors.New("无审核权限")
	}
	const maxItems = 200
	shop, _, err := cloudToolListShopResources(db, "", cloudReviewPending, 1, maxItems)
	if err != nil {
		return "", err
	}
	image, _, err := cloudToolListImageResources(db, "", cloudReviewPending, 1, maxItems)
	if err != nil {
		return "", err
	}
	b, _ := json.Marshal(map[string]any{"shop": shop, "image": image})
	return string(b), nil
}

// CloudToolAccountInfo 云工具服务端账号信息（OPUI 账号管理用）。
type CloudToolAccountInfo struct {
	Username    string `json:"username"`   // 账号
	CreatedAt   int64  `json:"created_at"` // 注册时间（Unix 秒）
	OnlineSec   int64  `json:"online_seconds"`
	Online      bool   `json:"online"` // 是否在线
	ConnCount   int    `json:"conn_count"`
	Whitelisted bool   `json:"whitelisted"` // 是否在服务端白名单
	Reviewer    bool   `json:"reviewer"`    // 是否拥有审核权限
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
	var whereArgs []any
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		where = ` WHERE u.username LIKE ?`
		whereArgs = append(whereArgs, "%"+keyword+"%")
	}

	// 总条数
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM "`+cloudUsersTable+`" u`+where, whereArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	// 余额通过 LEFT JOIN money 一次性取出，避免在 rows 遍历中再次查询占用同一连接导致阻塞
	queryArgs := append([]any{cloudToolMoneyType}, whereArgs...)
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	rows, err := db.Query(
		`SELECT u.username, u.created_at, u.online_seconds, u.reviewer, IFNULL(m.balance, 0)
		 FROM "`+cloudUsersTable+`" u
		 LEFT JOIN "money" m ON m.type=? AND m.account=u.username`+where+
			` ORDER BY u.created_at DESC, u.username ASC LIMIT ? OFFSET ?`,
		queryArgs...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := make([]CloudToolAccountInfo, 0, pageSize)
	for rows.Next() {
		var it CloudToolAccountInfo
		var balance int64
		var reviewer int
		if err := rows.Scan(&it.Username, &it.CreatedAt, &it.OnlineSec, &reviewer, &balance); err != nil {
			return nil, 0, err
		}
		it.ConnCount = connCounts[it.Username]
		it.Online = it.ConnCount > 0
		it.Whitelisted = whitelist[it.Username]
		it.Reviewer = reviewer != 0
		it.Balance = strconv.FormatInt(balance, 10)
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

// cloudAddConn 注册云工具在线连接，用于在线时长定时落库，并保留写封装供服务端推送通知。
func cloudAddConn(username string, conn *websocket.Conn, wc *cloudServerConn, start time.Time) {
	cloudConnMu.Lock()
	defer cloudConnMu.Unlock()
	set := cloudConns[username]
	if set == nil {
		set = map[*websocket.Conn]*cloudConnState{}
		cloudConns[username] = set
	}
	set[conn] = &cloudConnState{start: start, wc: wc}
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

// callCloudToolEvent 由服务端内部触发云工具词库的 [系统] 类事件。
// 该路径不经过函数索引，故 [系统] 事件不会出现在 list_funcs，也无法被客户端 exec 调用；
// username 会注入为变量 %账号%。
func callCloudToolEvent(data *dto.BuildValue, dicPath, username, trigger string) (string, error) {
	if data == nil {
		return "", errors.New("云工具词库未就绪")
	}
	d := &dic_dto.Dic{
		Data:   data,
		Val:    dto.NewDicVal(),
		Path:   dicPath,
		MyFunc: data.MyFunc,
	}
	d.Val.P.Set("账号", username)
	return dic_api.Api.DicRunEvent(d, "系统", trigger), nil
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

	// 图床图片服务：GET /{访问路径}/image/{文件名}（开启图床时）
	if cfg.ImageHost {
		if name, ok := strings.CutPrefix(r.URL.Path, cfg.Addr+"/image/"); ok {
			db, dbErr := dic_funcs.GetGlobalDB()
			if dbErr != nil {
				http.NotFound(w, r)
				return
			}
			cloudImageServe(db, w, r, name)
			return
		}
	}

	// 非 WebSocket 升级请求（如图片外的普通 GET）直接返回 404
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.NotFound(w, r)
		return
	}

	if data, _ := cloudServerDic(); data == nil {
		http.Error(w, "云工具词库未就绪", http.StatusServiceUnavailable)
		return
	}

	conn, err := cloudServerUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	// 放大读限制以支持较大的 base64 载荷（图床图片 / 词库商城发布，大小仍在上传侧单独限制）
	conn.SetReadLimit(cloudImageMaxSize * 2)

	pathUsername := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, cfg.Addr+"/"))
	username, token, ok := cloudServerAuth(conn, pathUsername)
	if !ok {
		conn.Close()
		return
	}
	loginAt := time.Now() // 本次连接开始时间

	// 断开自动注销阈值（秒）：连接断开时本次在线时长低于该值即注销，0 表示关闭
	logoutAfter := time.Duration(cfg.LogoutSec) * time.Second

	wc := &cloudServerConn{conn: conn}
	cloudAddConn(username, conn, wc, loginAt)

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
			data, dicPath := cloudServerDic()
			res, execErr := callCloudToolFunc(data, dicPath, username, msg.Func, msg.Args)
			if execErr != nil {
				_ = wc.write(cloudServerMsg{Type: "error", ID: msg.ID, Msg: execErr.Error()})
			} else {
				_ = wc.write(cloudServerMsg{Type: "result", ID: msg.ID, Data: res})
			}
		case "list_funcs":
			data, _ := cloudServerDic()
			_ = wc.write(cloudServerMsg{Type: "result", ID: msg.ID, Data: cloudServerListFuncs(data)})
		case "shop_list":
			if !cfg.ShopOpen {
				_ = wc.write(cloudServerMsg{Type: "error", ID: msg.ID, Msg: "词库商城未开启"})
				continue
			}
			db, dbErr := dic_funcs.GetGlobalDB()
			if dbErr != nil {
				_ = wc.write(cloudServerMsg{Type: "error", ID: msg.ID, Msg: "数据库不可用"})
				continue
			}
			_ = wc.write(cloudServerMsg{Type: "result", ID: msg.ID, Data: cloudShopList(db, username)})
		case "shop_buy":
			if !cfg.ShopOpen {
				_ = wc.write(cloudServerMsg{Type: "error", ID: msg.ID, Msg: "词库商城未开启"})
				continue
			}
			db, dbErr := dic_funcs.GetGlobalDB()
			if dbErr != nil {
				_ = wc.write(cloudServerMsg{Type: "error", ID: msg.ID, Msg: "数据库不可用"})
				continue
			}
			if err := cloudShopBuy(db, username, msg.Func); err != nil {
				_ = wc.write(cloudServerMsg{Type: "error", ID: msg.ID, Msg: err.Error()})
			} else {
				_ = wc.write(cloudServerMsg{Type: "result", ID: msg.ID, Data: "购买成功"})
			}
		case "shop_download":
			if !cfg.ShopOpen {
				_ = wc.write(cloudServerMsg{Type: "error", ID: msg.ID, Msg: "词库商城未开启"})
				continue
			}
			db, dbErr := dic_funcs.GetGlobalDB()
			if dbErr != nil {
				_ = wc.write(cloudServerMsg{Type: "error", ID: msg.ID, Msg: "数据库不可用"})
				continue
			}
			content, err := cloudShopDownload(db, username, msg.Func)
			if err != nil {
				_ = wc.write(cloudServerMsg{Type: "error", ID: msg.ID, Msg: err.Error()})
			} else {
				_ = wc.write(cloudServerMsg{Type: "result", ID: msg.ID, Data: content})
			}
		case "shop_publish":
			if !cfg.ShopOpen {
				_ = wc.write(cloudServerMsg{Type: "error", ID: msg.ID, Msg: "词库商城未开启"})
				continue
			}
			db, dbErr := dic_funcs.GetGlobalDB()
			if dbErr != nil {
				_ = wc.write(cloudServerMsg{Type: "error", ID: msg.ID, Msg: "数据库不可用"})
				continue
			}
			if result, err := cloudShopPublish(db, username, msg); err != nil {
				_ = wc.write(cloudServerMsg{Type: "error", ID: msg.ID, Msg: err.Error()})
			} else {
				_ = wc.write(cloudServerMsg{Type: "result", ID: msg.ID, Data: result})
			}
		case "image_upload":
			if !cfg.ImageHost {
				_ = wc.write(cloudServerMsg{Type: "error", ID: msg.ID, Msg: "图床未开启"})
				continue
			}
			db, dbErr := dic_funcs.GetGlobalDB()
			if dbErr != nil {
				_ = wc.write(cloudServerMsg{Type: "error", ID: msg.ID, Msg: "数据库不可用"})
				continue
			}
			url, pending, err := cloudImageUpload(db, r, msg, username)
			if err != nil {
				_ = wc.write(cloudServerMsg{Type: "error", ID: msg.ID, Msg: err.Error()})
			} else {
				// 待审核时 Data 为空字符串，由前端提示「上传成功，等待审核」
				if pending {
					_ = wc.write(cloudServerMsg{Type: "result", ID: msg.ID, Data: "", Msg: "上传成功，等待审核通过后可访问外链"})
				} else {
					_ = wc.write(cloudServerMsg{Type: "result", ID: msg.ID, Data: url})
				}
			}
		case "account_info":
			db, dbErr := dic_funcs.GetGlobalDB()
			if dbErr != nil {
				_ = wc.write(cloudServerMsg{Type: "error", ID: msg.ID, Msg: "数据库不可用"})
				continue
			}
			onlineSecs := cloudServerOnlineSeconds(db, username) + cloudElapsedOnline(username, conn)
			money := cloudServerMoney(db, username)
			b, _ := json.Marshal(map[string]any{
				"username": username,
				"money":    money,
				// 内置服务端无优惠券概念，优惠券恒为 0，总金额等于余额；
				// 与前端「余额 + 优惠券 = 总金额」的展示保持一致。
				"coupon":         "0",
				"total_money":    money,
				"type":           cloudToolMoneyType,
				"online_seconds": onlineSecs,
				// 是否具备审核权限，客户端据此显示「审核」入口
				"reviewer": cloudServerIsReviewer(db, username),
			})
			_ = wc.write(cloudServerMsg{Type: "result", ID: msg.ID, Data: string(b)})
		case "review_list":
			db, dbErr := dic_funcs.GetGlobalDB()
			if dbErr != nil {
				_ = wc.write(cloudServerMsg{Type: "error", ID: msg.ID, Msg: "数据库不可用"})
				continue
			}
			data, err := cloudServerReviewList(db, username)
			if err != nil {
				_ = wc.write(cloudServerMsg{Type: "error", ID: msg.ID, Msg: err.Error()})
			} else {
				_ = wc.write(cloudServerMsg{Type: "result", ID: msg.ID, Data: data})
			}
		case "review_action":
			db, dbErr := dic_funcs.GetGlobalDB()
			if dbErr != nil {
				_ = wc.write(cloudServerMsg{Type: "error", ID: msg.ID, Msg: "数据库不可用"})
				continue
			}
			if !cloudServerIsReviewer(db, username) {
				_ = wc.write(cloudServerMsg{Type: "error", ID: msg.ID, Msg: "无审核权限"})
				continue
			}
			// msg.Func 为资源类型(shop/image)，msg.ID 为资源标识，msg.Args[0] 为动作(approve/reject)
			action := ""
			if len(msg.Args) > 0 {
				action = msg.Args[0]
			}
			if err := CloudToolReviewResource(msg.Func, msg.ID, action, username); err != nil {
				_ = wc.write(cloudServerMsg{Type: "error", ID: msg.ID, Msg: err.Error()})
			} else {
				_ = wc.write(cloudServerMsg{Type: "result", ID: msg.ID, Data: "操作成功"})
			}
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
// 新注册成功后会触发词库 [系统]用户注册 事件，并把其返回作为注册信息回传。
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
			data, dicPath := cloudServerDic()
			if s, e := callCloudToolEvent(data, dicPath, username, "用户注册"); e == nil {
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
