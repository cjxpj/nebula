package funcs

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/cjxpj/nebula/dto"

	_ "github.com/go-sql-driver/mysql"
)

type MysqlRes struct {
	Error        string                   `json:"error"`
	LastInsertId int64                    `json:"last_insert_id"`
	RowsAffected int64                    `json:"rows_affected"`
	Data         []map[string]interface{} `json:"data"`
}

func (r *MysqlRes) r() string {
	// 转换为 JSON 字符串
	j, err := json.Marshal(r)
	if err != nil {
		return err.Error()
	}

	return string(j)
}

func mysqlConn(d *dto.DicInputs) (any, error) {
	Output := MysqlRes{}
	dbUser, _ := d.V.Get("_MYSQL_账号").(string)
	dbPass, _ := d.V.Get("_MYSQL_密码").(string)
	dbHost, _ := d.V.Get("_MYSQL_地址").(string)
	if dbUser == "" || dbHost == "" {
		return "", fmt.Errorf("MySQL参数未设置")
	}
	// 从地址解析端口
	dbPort := "3306" // 默认端口为 3306
	if strings.Contains(dbHost, ":") {
		urlPath := strings.Split(dbHost, ":")
		dbHost = urlPath[0]
		dbPort = urlPath[1]
	}

	dbName := d.Inputs.String(1) // 数据库名称

	dbPath := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", dbUser, dbPass, dbHost, dbPort, dbName)

	db, err := sql.Open("mysql", dbPath)
	if err != nil {
		Output.Error = err.Error()
		return Output.r(), nil
	}
	defer db.Close()

	sqlr := d.Inputs.String(2)

	// 检查数据库连接是否成功
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
			args := make([]any, len(d.Inputs.List[3:]))
			copy(args, d.Inputs.List[3:])

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
		values := make([]interface{}, len(columns))
		pointers := make([]interface{}, len(columns))
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
			args := make([]any, len(d.Inputs.List[3:]))
			copy(args, d.Inputs.List[3:])

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
