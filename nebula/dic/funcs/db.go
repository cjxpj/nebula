package funcs

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

var tableNameRe = regexp.MustCompile(`^[a-zA-Z0-9_]{1,32}$`)

// resolveSqlitePath 解析「读sqlite / 写sqlite」的数据库文件路径：
// 只接受相对路径，且解析后必须仍位于当前账号的 database 目录内，防止用 .. 越界读写其它位置。
func resolveSqlitePath(d *dto.DicInputs, raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("数据库名不能为空")
	}
	if filepath.IsAbs(raw) || filepath.VolumeName(raw) != "" {
		return "", fmt.Errorf("只允许相对路径（不支持绝对路径）：%s", raw)
	}
	absBase, err := filepath.Abs(dicDatabaseDir(d))
	if err != nil {
		return "", fmt.Errorf("数据库目录不可用，拒绝访问：%s", raw)
	}
	target := filepath.Join(absBase, raw)
	rel, err := filepath.Rel(absBase, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("路径越界（超出数据库目录）：%s", raw)
	}
	return target, nil
}

// openDBByDir 打开指定目录下 sqlite（data.db）的句柄；调用方用完必须 Close。
func openDBByDir(dir string) (*sql.DB, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(abs, "data.db"))
	if err != nil {
		return nil, err
	}
	// 单连接 + 忙碌等待，避免与词库写入并发时出现 database is locked
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err = db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

var (
	globalMu    sync.Mutex
	globalCache *sql.DB
)

// GetGlobalDB 返回引擎应用数据目录下 database/data.db 的进程级共享句柄，
// 供无词库上下文（如服务端内置工具）使用；句柄常驻，调用方不要关闭。
func GetGlobalDB() (*sql.DB, error) {
	globalMu.Lock()
	defer globalMu.Unlock()
	if globalCache != nil {
		return globalCache, nil
	}
	db, err := openDBByDir(filepath.Join(utils.GetAppDir(), "database"))
	if err != nil {
		return nil, err
	}
	globalCache = db
	return db, nil
}

// GetDicDB 打开当前词库所属账号的数据库句柄（bots/<账号>/database/data.db），
// 各账号数据相互隔离。调用方用完必须 Close，避免连接按账号常驻占用资源。
func GetDicDB(d *dto.DicInputs) (*sql.DB, error) {
	return openDBByDir(dicDatabaseDir(d))
}

func normalizeTableName(name string) (string, error) {
	if !tableNameRe.MatchString(name) {
		return "", errors.New("非法表名：" + name)
	}
	return name, nil
}

func dbClose(d *dto.DicInputs) (any, error) {
	db, ok := d.Inputs.Get(1).(*sql.DB)
	if !ok || db == nil {
		return "false", nil
	}
	if err := db.Close(); err != nil {
		return "false", nil
	}
	return "true", nil
}

func EnsureFsTable(db *sql.DB, table string) error {
	sql := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s" (
			key TEXT PRIMARY KEY,
			data BLOB NOT NULL,
			updated_at INTEGER NOT NULL
		)
	`, table)

	_, err := db.Exec(sql)
	return err
}

func readSqlite(d *dto.DicInputs) (any, error) {
	p, err := resolveSqlitePath(d, d.Inputs.String(1))
	if err != nil {
		return "", err
	}
	db, err := utils.NewFileQueue(p).OpenSqlite()
	if err != nil {
		return d.Inputs.String(3), nil
	}
	defer db.Close()

	// 仅传入数据库名：返回全部 key
	if d.Inputs.LenOk(1) {
		rows, err2 := db.Query(`SELECT key FROM "fs_files"`)
		if err2 != nil {
			return "[]", nil
		}
		defer rows.Close()

		keys := make([]string, 0)
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err == nil {
				keys = append(keys, k)
			}
		}
		if err = rows.Err(); err != nil {
			return "[]", nil
		}
		return utils.AnyToString(keys), nil
	}

	// 读取指定 key
	key := d.Inputs.String(2)
	defaultValue := d.Inputs.String(3)

	var data string
	err = db.QueryRow(
		`SELECT data FROM "fs_files" WHERE key=?`,
		key,
	).Scan(&data)

	if err != nil {
		return defaultValue, nil
	}

	return data, nil
}

func dbDelete(d *dto.DicInputs) (any, error) {
	db, err := GetDicDB(d)
	if err != nil {
		return nil, fmt.Errorf("数据库初始化失败: %w", err)
	}
	defer db.Close()

	rawTable := d.Inputs.String(1)
	key := d.Inputs.String(2)

	table, err := normalizeTableName(rawTable)
	if err != nil {
		return nil, err
	}

	if err = EnsureFsTable(db, table); err != nil {
		return nil, err
	}

	_, err = db.Exec(
		fmt.Sprintf(`DELETE FROM "%s" WHERE key=?`, table),
		key,
	)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func dbDeleteFile(d *dto.DicInputs) (any, error) {
	db, err := GetDicDB(d)
	if err != nil {
		return nil, fmt.Errorf("数据库初始化失败: %w", err)
	}
	defer db.Close()
	table := "fs_files"
	if err = EnsureFsTable(db, table); err != nil {
		return nil, err
	}
	_, err = db.Exec(fmt.Sprintf(`DELETE FROM "%s" WHERE key=?`, table), d.Inputs.String(1))
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func dbDeleteDir(d *dto.DicInputs) (any, error) {
	db, err := GetDicDB(d)
	if err != nil {
		return nil, fmt.Errorf("数据库初始化失败: %w", err)
	}
	defer db.Close()
	table := "fs_files"
	if err = EnsureFsTable(db, table); err != nil {
		return nil, err
	}
	_, err = db.Exec(fmt.Sprintf(`DELETE FROM "%s" WHERE key=?`, table), d.Inputs.String(1))
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func writeSqlite(d *dto.DicInputs) (any, error) {
	p, err := resolveSqlitePath(d, d.Inputs.String(1))
	if err != nil {
		return nil, err
	}
	db, err := utils.NewFileQueue(p).OpenSqlite()
	if err != nil {
		return nil, nil
	}
	defer db.Close()

	// 确保表存在
	if err = EnsureFsTable(db, "fs_files"); err != nil {
		return 0, nil
	}

	key := d.Inputs.String(2)
	data := d.Inputs.String(3)

	if key == "" {
		return nil, nil
	}

	_, err = db.Exec(`
		INSERT INTO "fs_files" (key, data, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			data = excluded.data,
			updated_at = excluded.updated_at
	`,
		key,
		data,
		time.Now().Unix(),
	)

	if err != nil {
		return nil, err
	}
	return nil, nil
}

func dbWrite(d *dto.DicInputs) (any, error) {
	db, err := GetDicDB(d)
	if err != nil {
		return nil, fmt.Errorf("数据库初始化失败: %w", err)
	}
	defer db.Close()

	// 与 写sqlite 一致，固定写入 fs_files 表
	table := "fs_files"
	if err = EnsureFsTable(db, table); err != nil {
		return nil, err
	}

	// 首参数为命名空间，实际 key 为「命名空间/key」
	namespace := d.Inputs.String(1)
	rawKey := d.Inputs.String(2)
	if rawKey == "" {
		return nil, errors.New("key不能为空")
	}
	key := joinKey(namespace, rawKey)

	data := ""
	if d.Inputs.LenOk(3) {
		data = d.Inputs.String(3)
	}

	sqlWrite := fmt.Sprintf(`
	INSERT INTO "%s" (key, data, updated_at)
	VALUES (?, ?, ?)
	ON CONFLICT(key) DO UPDATE SET
		data = excluded.data,
		updated_at = excluded.updated_at
	`, table)

	_, err = db.Exec(
		sqlWrite,
		key,
		data,
		time.Now().Unix(),
	)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

// joinKey 将命名空间与 key 组合为 fs_files 中的实际键
func joinKey(namespace, key string) string {
	if namespace == "" {
		return key
	}
	return namespace + "/" + key
}

func dbRead(d *dto.DicInputs) (any, error) {
	db, err := GetDicDB(d)
	if err != nil {
		return nil, fmt.Errorf("数据库初始化失败: %w", err)
	}
	defer db.Close()

	// 与 读sqlite 一致，固定读取 fs_files 表
	table := "fs_files"
	if err = EnsureFsTable(db, table); err != nil {
		return nil, err
	}

	namespace := d.Inputs.String(1)

	// 仅传入命名空间：返回该命名空间下的全部 key
	if d.Inputs.LenOk(1) {
		rows, err := db.Query(fmt.Sprintf(`SELECT key FROM "%s"`, table))
		if err != nil {
			return "[]", nil
		}
		defer rows.Close()

		prefix := namespace + "/"
		keys := make([]string, 0)
		for rows.Next() {
			var k string
			if err2 := rows.Scan(&k); err2 == nil {
				if namespace == "" {
					keys = append(keys, k)
					continue
				}
				if strings.HasPrefix(k, prefix) {
					keys = append(keys, strings.TrimPrefix(k, prefix))
				}
			}
		}
		if err = rows.Err(); err != nil {
			return "[]", nil
		}
		return utils.AnyToString(keys), nil
	}

	// 读取指定 key
	key := joinKey(namespace, d.Inputs.String(2))
	defaultValue := d.Inputs.String(3)

	var data string
	err = db.QueryRow(
		fmt.Sprintf(`SELECT data FROM "%s" WHERE key=?`, table),
		key,
	).Scan(&data)

	if err != nil {
		return defaultValue, nil
	}

	return data, nil
}
