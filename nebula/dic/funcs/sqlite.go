package funcs

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"

	_ "github.com/mattn/go-sqlite3"
)

type SqliteRes struct {
	Error        string           `json:"error"`
	LastInsertId int64            `json:"last_insert_id"`
	RowsAffected int64            `json:"rows_affected"`
	Data         []map[string]any `json:"data"`
}

func (r *SqliteRes) r() string {
	// 转换为 JSON 字符串
	j, err := json.Marshal(r)
	if err != nil {
		return err.Error()
	}

	return string(j)
}

func sqliteOpen(d *dto.DicInputs) (any, error) {
	dbPath := d.Inputs.String(1)
	if dbPath == "" || dbPath == ":内存:" {
		dbPath = ":memory:"
	}
	dbf := utils.NewFileQueue(dbPath)
	db, err := dbf.OpenSqlite()
	if err == nil {
		return db, nil
	}
	return d.Inputs.StringDefault(2, "打开失败"), nil
}

func sqliteWrite(d *dto.DicInputs) (any, error) {
	db, ok := d.Inputs.Get(1).(*sql.DB)
	if !ok || db == nil {
		return nil, errors.New("未打开数据库")
	}

	rawTable := d.Inputs.String(2)
	key := d.Inputs.String(3)
	data := d.Inputs.Bytes(4)

	if key == "" {
		return nil, errors.New("文件名不能为空")
	}

	table, err := normalizeTableName(rawTable)
	if err != nil {
		return nil, err
	}

	// ★ 关键：确保表存在
	if err := ensureFsTable(db, table); err != nil {
		return nil, err
	}

	sqlWrite := fmt.Sprintf(`
	INSERT INTO %s (key, data, updated_at)
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

func sqliteRead(d *dto.DicInputs) (any, error) {
	db, ok := d.Inputs.Get(1).(*sql.DB)
	if !ok || db == nil {
		return nil, errors.New("未打开数据库")
	}

	rawTable := d.Inputs.String(2)
	key := d.Inputs.String(3)
	defaultValue := d.Inputs.String(4)

	table, err := normalizeTableName(rawTable)
	if err != nil {
		return defaultValue, nil
	}

	var data []byte
	sqlRead := fmt.Sprintf(
		`SELECT data FROM %s WHERE key=?`,
		table,
	)

	err = db.QueryRow(sqlRead, key).Scan(&data)
	if err != nil {
		return defaultValue, nil
	}

	return string(data), nil
}

func sqliteExec(d *dto.DicInputs) (any, error) {
	db, ok := d.Inputs.Get(1).(*sql.DB)
	if !ok || db == nil {
		return nil, errors.New("未打开数据库")
	}

	Output := SqliteRes{}
	sqlr := d.Inputs.String(2)
	// 判断是否是查询语句
	isQuery := strings.HasPrefix(strings.ToUpper(strings.TrimSpace(sqlr))[:6], "SELECT")

	var stmt *sql.Stmt
	var err error
	if stmt, err = db.Prepare(sqlr); err != nil {
		Output.Error = err.Error()
		return Output.r(), nil
	}
	defer stmt.Close()

	if isQuery {
		// 如果是查询语句，使用 Query

		var rows *sql.Rows
		if d.Inputs.LenOk("3..") {
			args := d.Inputs.List[3:] // 获取参数列表

			rows, err = stmt.Query(args...)
			if err != nil {
				Output.Error = err.Error()
				return Output.r(), nil
			}
			defer rows.Close()
		} else {
			rows, err = stmt.Query()
			if err != nil {
				Output.Error = err.Error()
				return Output.r(), nil
			}
			defer rows.Close()
		}

		// 处理查询结果
		columns, _ := rows.Columns()
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}

		for rows.Next() {
			err = rows.Scan(pointers...)
			if err != nil {
				Output.Error = err.Error()
				return Output.r(), nil
			}

			rowData := make(map[string]any)
			for i, col := range columns {
				rowData[col] = values[i]
			}
			Output.Data = append(Output.Data, rowData)
		}
	} else {
		// 如果是其他语句（如 INSERT, UPDATE, DELETE 等），使用 Exec
		var res sql.Result
		if d.Inputs.LenOk("3..") {
			args := d.Inputs.List[3:]

			res, err = stmt.Exec(args...)
		} else {
			res, err = stmt.Exec()
		}
		if err != nil {
			Output.Error = err.Error()
			return Output.r(), nil
		}

		// 获取受影响的行数和最后插入的 ID
		if Output.RowsAffected, err = res.RowsAffected(); err != nil {
			Output.Error = err.Error()
			return Output.r(), nil
		}
		if Output.LastInsertId, err = res.LastInsertId(); err != nil {
			Output.Error = err.Error()
			return Output.r(), nil
		}
	}

	return Output.r(), nil
}
