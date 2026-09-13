package funcs

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

var tableNameRe = regexp.MustCompile(`^[a-zA-Z0-9_]{1,32}$`)

var (
	globalDB     *sql.DB
	globalDBOnce sync.Once
	globalDBErr  error
)

func GetGlobalDB() (*sql.DB, error) {
	globalDBOnce.Do(func() {
		dir := path.Join(utils.GetAppDir(), "database")
		if err := os.MkdirAll(dir, 0755); err != nil {
			globalDBErr = err
			return
		}
		db, err := sql.Open("sqlite", path.Join(dir, "data.db"))
		if err != nil {
			globalDBErr = err
			return
		}
		// 单连接 + 忙碌等待，避免与词库写入并发时出现 database is locked
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		if _, err = db.Exec("PRAGMA busy_timeout=5000"); err != nil {
			db.Close()
			globalDBErr = err
			return
		}
		globalDB = db
	})
	return globalDB, globalDBErr
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
	p := path.Join("database", d.Inputs.String(1))
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
	db, err := GetGlobalDB()
	if err != nil {
		return nil, fmt.Errorf("全局数据库初始化失败: %w", err)
	}

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
	db, err := GetGlobalDB()
	if err != nil {
		return nil, fmt.Errorf("全局数据库初始化失败: %w", err)
	}
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
	db, err := GetGlobalDB()
	if err != nil {
		return nil, fmt.Errorf("全局数据库初始化失败: %w", err)
	}
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
	p := path.Join("database", d.Inputs.String(1))
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
	db, err := GetGlobalDB()
	if err != nil {
		return nil, fmt.Errorf("全局数据库初始化失败: %w", err)
	}

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
	db, err := GetGlobalDB()
	if err != nil {
		return nil, fmt.Errorf("全局数据库初始化失败: %w", err)
	}

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
