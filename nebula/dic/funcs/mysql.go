package funcs

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"

	_ "github.com/go-sql-driver/mysql"
)

// MysqlConn MySQL 连接对象，保存账号密码地址等信息
type MysqlConn struct {
	mu     sync.RWMutex
	User   string
	Pass   string
	Host   string
	Port   string
	DBName string
	db     *sql.DB
}

// MysqlRes 执行结果
type MysqlRes struct {
	Error        string                   `json:"error"`
	LastInsertId int64                    `json:"last_insert_id"`
	RowsAffected int64                    `json:"rows_affected"`
	Data         []map[string]any `json:"data"`
}

func (r *MysqlRes) r() string {
	j, err := json.Marshal(r)
	if err != nil {
		return err.Error()
	}
	return string(j)
}

// getMysqlConn 从参数中获取 MysqlConn
func getMysqlConn(d *dto.DicInputs) *MysqlConn {
	if v := d.Inputs.Get(1); v != nil {
		if conn, ok := v.(*MysqlConn); ok {
			return conn
		}
	}
	return nil
}

func (c *MysqlConn) DSN(dbName string) string {
	c.mu.RLock()
	defaultDB := c.DBName
	c.mu.RUnlock()
	if dbName == "" {
		dbName = defaultDB
	}
	if dbName == "" {
		return fmt.Sprintf("%s:%s@tcp(%s:%s)/", c.User, c.Pass, c.Host, c.Port)
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", c.User, c.Pass, c.Host, c.Port, dbName)
}

// setDBName 安全写入 DBName（并发安全）
func (c *MysqlConn) setDBName(name string) {
	c.mu.Lock()
	c.DBName = name
	c.mu.Unlock()
}

// hasDBName 安全读取 DBName 是否已设置（并发安全）
func (c *MysqlConn) hasDBName() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.DBName != ""
}

// mysqlNew 创建 MySQL 连接对象
// 用法: $mysql.新建 账号 密码 地址$
func mysqlNew(d *dto.DicInputs) (any, error) {
	user := d.Inputs.String(1)
	pass := d.Inputs.String(2)
	host := d.Inputs.String(3)

	if user == "" || host == "" {
		return nil, fmt.Errorf("MySQL参数未设置：账号或地址为空")
	}

	dbPort := "3306"
	if strings.Contains(host, ":") {
		parts := strings.Split(host, ":")
		host = parts[0]
		dbPort = parts[1]
	}

	return &MysqlConn{
		User: user,
		Pass: pass,
		Host: host,
		Port: dbPort,
	}, nil
}

// mysqlPing 检测 MySQL 连接是否可达
// 用法: $mysql.PING 连接$
func mysqlPing(d *dto.DicInputs) (any, error) {
	conn := getMysqlConn(d)
	if conn == nil {
		return "false", fmt.Errorf("未找到MySQL连接")
	}

	db, err := sql.Open("mysql", conn.DSN(""))
	if err != nil {
		return "false", err
	}
	defer db.Close()

	if err = db.Ping(); err != nil {
		return "false", err
	}
	return "true", nil
}

// mysqlExec 在指定数据库上执行 SQL 语句
// 用法: $mysql.执行 连接 SQL [绑定参数...]$
//       $mysql.执行 连接 数据库名 SQL [绑定参数...]$
func mysqlExec(d *dto.DicInputs) (any, error) {
	Output := MysqlRes{}

	conn := getMysqlConn(d)
	if conn == nil {
		Output.Error = "未找到MySQL连接"
		return Output.r(), nil
	}

	var dbName, sqlr string
	var bindStart int

	inputLen := d.Inputs.Len()
	if inputLen >= 3 {
		// 连接 数据库名 SQL [绑定参数...]
		dbName = d.Inputs.String(2)
		conn.setDBName(dbName)
		sqlr = d.Inputs.String(3)
		bindStart = 4
	} else {
		// 连接 SQL
		sqlr = d.Inputs.String(2)
		bindStart = 3
	}

	if sqlr == "" {
		Output.Error = "SQL语句为空"
		return Output.r(), nil
	}

	// 判断是否是查询语句
	if len(sqlr) <= 6 {
		Output.Error = "sql is too short"
		return Output.r(), nil
	}
	isQuery := strings.ToUpper(strings.TrimSpace(sqlr))[:6] == "SELECT"

	db, err := sql.Open("mysql", conn.DSN(dbName))
	if err != nil {
		Output.Error = err.Error()
		return Output.r(), nil
	}
	defer db.Close()

	stmt, err := db.Prepare(sqlr)
	if err != nil {
		Output.Error = err.Error()
		return Output.r(), nil
	}
	defer stmt.Close()

	if isQuery {
		var rows *sql.Rows
		if bindStart < len(d.Inputs.List) {
			args := make([]any, len(d.Inputs.List[bindStart:]))
			copy(args, d.Inputs.List[bindStart:])
			rows, err = stmt.Query(args...)
		} else {
			rows, err = stmt.Query()
		}
		if err != nil {
			Output.Error = err.Error()
			return Output.r(), nil
		}
		defer rows.Close()

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
		var res sql.Result
		if bindStart < len(d.Inputs.List) {
			args := make([]any, len(d.Inputs.List[bindStart:]))
			copy(args, d.Inputs.List[bindStart:])
			res, err = stmt.Exec(args...)
		} else {
			res, err = stmt.Exec()
		}
		if err != nil {
			Output.Error = err.Error()
			return Output.r(), nil
		}
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

// mysqlSwitchDB 切换当前连接的默认数据库
// 用法: $mysql.切换数据库 连接 数据库名$
func mysqlSwitchDB(d *dto.DicInputs) (any, error) {
	conn := getMysqlConn(d)
	if conn == nil {
		return nil, fmt.Errorf("未找到MySQL连接")
	}
	conn.setDBName(d.Inputs.String(2))
	return "", nil
}

// mysqlWrite 写入 key-value 数据到指定表单
// 用法: $mysql.写 连接 表单 键 值$
func mysqlWrite(d *dto.DicInputs) (any, error) {
	conn := getMysqlConn(d)
	if conn == nil {
		return nil, fmt.Errorf("未找到MySQL连接")
	}
	if !conn.hasDBName() {
		return nil, fmt.Errorf("未设置数据库，请先使用 mysql.切换数据库 或 mysql.执行 指定数据库名")
	}

	rawTable := d.Inputs.String(2)
	key := d.Inputs.String(3)
	data := d.Inputs.String(4)

	if rawTable == "" {
		return nil, fmt.Errorf("表单名不能为空")
	}
	if key == "" {
		return nil, fmt.Errorf("键不能为空")
	}

	table, err := normalizeTableName(rawTable)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("mysql", conn.DSN(""))
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if err = ensureMySQLTable(db, table); err != nil {
		return nil, err
	}

	sqlWrite := fmt.Sprintf(
		"INSERT INTO `%s` (`key`, `data`, `updated_at`) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE `data`=VALUES(`data`), `updated_at`=VALUES(`updated_at`)",
		table,
	)
	_, err = db.Exec(sqlWrite, key, data, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	return nil, nil
}

// mysqlRead 读取 key-value 数据
// 用法: $mysql.读 连接$                — 列出所有表单
//       $mysql.读 连接 表单$            — 列出表单所有键
//       $mysql.读 连接 表单 键$         — 读取指定键的值
//       $mysql.读 连接 表单 键 默认值$   — 读取，不存在返回默认值
func mysqlRead(d *dto.DicInputs) (any, error) {
	conn := getMysqlConn(d)
	if conn == nil {
		return nil, fmt.Errorf("未找到MySQL连接")
	}
	if !conn.hasDBName() {
		return nil, fmt.Errorf("未设置数据库，请先使用 mysql.切换数据库 或 mysql.执行 指定数据库名")
	}

	db, err := sql.Open("mysql", conn.DSN(""))
	if err != nil {
		return nil, err
	}
	defer db.Close()

	// 仅1个参数：列出所有表单
	if d.Inputs.LenOk(1) {
		rows, err := db.Query("SHOW TABLES")
		if err != nil {
			return "[]", nil
		}
		defer rows.Close()

		tables := make([]string, 0)
		for rows.Next() {
			var name string
			if err = rows.Scan(&name); err == nil {
				tables = append(tables, name)
			}
		}
		return utils.AnyToString(tables), nil
	}

	rawTable := d.Inputs.String(2)

	// 2个参数：列出表单所有键
	if d.Inputs.LenOk(2) {
		table, err := normalizeTableName(rawTable)
		if err != nil {
			return "[]", nil
		}
		rows, err := db.Query(fmt.Sprintf("SELECT `key` FROM `%s`", table))
		if err != nil {
			return "[]", nil
		}
		defer rows.Close()

		keys := make([]string, 0)
		for rows.Next() {
			var k string
			if err = rows.Scan(&k); err == nil {
				keys = append(keys, k)
			}
		}
		return utils.AnyToString(keys), nil
	}

	key := d.Inputs.String(3)
	defaultValue := d.Inputs.String(4)

	table, err := normalizeTableName(rawTable)
	if err != nil {
		return defaultValue, nil
	}

	var data string
	sqlRead := fmt.Sprintf("SELECT `data` FROM `%s` WHERE `key`=?", table)
	err = db.QueryRow(sqlRead, key).Scan(&data)
	if err != nil {
		return defaultValue, nil
	}
	return data, nil
}

// mysqlDeleteFile 删除指定表单中指定 key 的数据
// 用法: $mysql.删除文件 连接 键$
func mysqlDeleteFile(d *dto.DicInputs) (any, error) {
	conn := getMysqlConn(d)
	if conn == nil {
		return nil, fmt.Errorf("未找到MySQL连接")
	}
	if !conn.hasDBName() {
		return nil, fmt.Errorf("未设置数据库，请先使用 mysql.切换数据库 或 mysql.执行 指定数据库名")
	}

	db, err := sql.Open("mysql", conn.DSN(""))
	if err != nil {
		return nil, err
	}
	defer db.Close()

	table := "fs_files"
	if err = ensureMySQLTable(db, table); err != nil {
		return nil, err
	}
	_, err = db.Exec(fmt.Sprintf("DELETE FROM `%s` WHERE `key`=?", table), d.Inputs.String(2))
	if err != nil {
		return nil, err
	}
	return nil, nil
}

// mysqlDeleteDir 删除指定表单中指定 key 的数据（与删除文件同）
// 用法: $mysql.删除文件夹 连接 键$
func mysqlDeleteDir(d *dto.DicInputs) (any, error) {
	return mysqlDeleteFile(d)
}

// ensureMySQLTable 确保 MySQL 表存在
func ensureMySQLTable(db *sql.DB, table string) error {
	sql := fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS `%s` (`key` VARCHAR(255) PRIMARY KEY, `data` LONGTEXT NOT NULL, `updated_at` BIGINT NOT NULL) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",
		table,
	)
	_, err := db.Exec(sql)
	return err
}

// mysqlClose 关闭 MySQL 连接
// 用法: $mysql.关闭 连接$
func mysqlClose(d *dto.DicInputs) (any, error) {
	conn := getMysqlConn(d)
	if conn == nil {
		return "false", nil
	}
	if conn.db != nil {
		conn.db.Close()
		conn.db = nil
	}
	return "true", nil
}
