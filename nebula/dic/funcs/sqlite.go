package funcs

import (
	"database/sql"
	"path"
	"strings"

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

func sqliteConn(d *dto.DicInputs) (any, error) {

	Output := SqliteRes{}
	dbPath := path.Join(utils.GetAppDir(), d.Inputs.String(1))
	dbPathName := ":memory:"
	if d.Inputs.String(1) == ":内存:" || d.Inputs.String(1) == dbPathName {
		dbPath = dbPathName
	}
	var db *sql.DB
	var err error
	// 内存数据库
	if dbPath == dbPathName {
		// 检查全局变量中是否已经存在数据库连接
		if dbs, ok := dto.GV.Get("_DB_SQLITE").(*sql.DB); ok {
			db = dbs
		} else {
			db, err = sql.Open("sqlite3", dbPath)
			if err != nil {
				Output.Error = err.Error()
				return Output.r(), nil
			}
			dto.GV.Set("_DB_SQLITE", db)
		}
	} else {
		db, err = sql.Open("sqlite3", dbPath)
		if err != nil {
			Output.Error = err.Error()
			return Output.r(), nil
		}
	}

	if dbPath != dbPathName && db != nil {
		defer db.Close()
	}

	sqlr := d.Inputs.String(2)

	if sqlr == "PING" {
		// 检查数据库连接是否成功
		if err = db.Ping(); err != nil {
			return "false", nil
		}
		return "true", nil
	}

	// 判断是否是查询语句
	if len(sqlr) <= 6 {
		Output.Error = "sql is too short"
		return Output.r(), nil
	}
	isQuery := strings.ToUpper(strings.TrimSpace(sqlr))[:6] == "SELECT"

	var stmt *sql.Stmt
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

			rowData := make(map[string]interface{})
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
