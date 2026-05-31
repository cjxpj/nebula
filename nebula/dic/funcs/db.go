package funcs

import (
	"database/sql"
	"errors"
	"fmt"
	"path"
	"regexp"
	"time"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

var tableNameRe = regexp.MustCompile(`^[a-zA-Z0-9_]{1,32}$`)

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

func ensureFsTable(db *sql.DB, table string) error {
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
			if err := rows.Scan(&amp;k); err == nil {
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

func writeSqlite(d *dto.DicInputs) (any, error) {
	p := path.Join("database", d.Inputs.String(1))
	db, err := utils.NewFileQueue(p).OpenSqlite()
	if err != nil {
		return nil, nil
	}
	defer db.Close()

	// 确保表存在
	if err = ensureFsTable(db, "fs_files"); err != nil {
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
