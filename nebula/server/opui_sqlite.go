package dic_server

import (
	"database/sql"
	"encoding/hex"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

func openSqliteOpui(full string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", full)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// opuiOpenSqlite 校验相对路径并打开应用目录下的 SQLite 数据库，失败时写出错误响应并返回 nil
func opuiOpenSqlite(w http.ResponseWriter, path string) *sql.DB {
	if !checkFilePath(path) {
		http.Error(w, `{"status":"error","error":"文件路径不合法"}`, http.StatusBadRequest)
		return nil
	}
	full := filepath.Join(opuiAppDir(), filepath.FromSlash(path))
	db, err := openSqliteOpui(full)
	if err != nil {
		http.Error(w, `{"status":"error","error":"数据库打开失败: `+err.Error()+`"}`, http.StatusBadRequest)
		return nil
	}
	return db
}

// sqliteCellValue 将 SQLite 扫描值转换为可 JSON 序列化的展示值：
// BLOB 可打印时按文本展示，否则转 hex；时间转为固定格式字符串
func sqliteCellValue(v any) any {
	switch t := v.(type) {
	case nil:
		return nil
	case []byte:
		if utf8.Valid(t) {
			return string(t)
		}
		return "0x" + hex.EncodeToString(t)
	case time.Time:
		return t.Format("2006-01-02 15:04:05")
	default:
		return v
	}
}

// isSimpleIdent 校验 SQLite 标识符（表名/列名）仅为字母数字下划线，
// 用于内联编辑时安全拼接 UPDATE 语句，避免 SQL 注入
func isSimpleIdent(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}

// stripLeadingSQLComments 去掉 SQL 开头的注释与空白，用于准确判断是否为查询语句，
// 避免带注释（如 "-- 说明\nSELECT ..."）的查询被误判为写语句而丢弃结果
func stripLeadingSQLComments(s string) string {
	s = strings.TrimSpace(s)
	for {
		if strings.HasPrefix(s, "--") {
			if i := strings.IndexByte(s, '\n'); i >= 0 {
				s = strings.TrimSpace(s[i+1:])
				continue
			}
			return ""
		}
		if strings.HasPrefix(s, "/*") {
			if i := strings.Index(s, "*/"); i >= 0 {
				s = strings.TrimSpace(s[i+2:])
				continue
			}
			return ""
		}
		return s
	}
}

// sqlAggRe 匹配聚合函数调用（要求函数名前为标识符边界，避免误伤 my_count() 等自定义函数）
var sqlAggRe = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_])(COUNT|SUM|AVG|MIN|MAX|TOTAL|GROUP_CONCAT)\s*\(`)

// isIdentByte 判断字节是否为 SQL 标识符字符（字母/数字/下划线）
func isIdentByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}

// findTopLevelKeyword 在跳过字符串、引号标识符、方括号标识符、注释与括号嵌套的前提下，
// 定位首个位于最外层的关键词（不区分大小写），返回其字节偏移，未找到返回 -1
func findTopLevelKeyword(s, kw string) int {
	depth := 0
	for i := 0; i < len(s); {
		c := s[i]
		switch c {
		case '\'', '"', '`':
			// 跳过字符串/引号标识符，处理双写转义
			q := c
			i++
			for i < len(s) {
				if s[i] == q {
					if q == '\'' && i+1 < len(s) && s[i+1] == '\'' {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
		case '[':
			if j := strings.IndexByte(s[i+1:], ']'); j >= 0 {
				i += j + 2
			} else {
				i++
			}
		case '(':
			depth++
			i++
		case ')':
			depth--
			i++
		case '-':
			if i+1 < len(s) && s[i+1] == '-' {
				for i < len(s) && s[i] != '\n' {
					i++
				}
			} else {
				i++
			}
		case '/':
			if i+1 < len(s) && s[i+1] == '*' {
				i += 2
				for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
					i++
				}
				i += 2
			} else {
				i++
			}
		default:
			if depth == 0 && i+len(kw) <= len(s) {
				beforeOK := i == 0 || !isIdentByte(s[i-1])
				afterOK := i+len(kw) >= len(s) || !isIdentByte(s[i+len(kw)])
				if beforeOK && afterOK && strings.EqualFold(s[i:i+len(kw)], kw) {
					return i
				}
			}
			i++
		}
	}
	return -1
}

// extractLeadingIdent 提取字符串开头的第一个标识符（支持裸标识符及引号/反引号/方括号包裹），
// 用于读取 FROM 子句中的表名
func extractLeadingIdent(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	switch s[0] {
	case '"', '`':
		q := s[0]
		for i := 1; i < len(s); i++ {
			if s[i] == q {
				if i+1 < len(s) && s[i+1] == q {
					i++
					continue
				}
				return s[1:i]
			}
		}
		return ""
	case '[':
		if i := strings.IndexByte(s[1:], ']'); i >= 0 {
			return s[1 : 1+i]
		}
		return ""
	default:
		i := 0
		for i < len(s) && isIdentByte(s[i]) {
			i++
		}
		return s[:i]
	}
}

// detectEditableTable 判断 SQL 是否为可安全附加 rowid 的「单表 SELECT」并返回表名。
// 多表连接、分组、去重、联合、聚合等无法精确定位行的查询返回空。
func detectEditableTable(sql string) (string, bool) {
	s := stripLeadingSQLComments(sql)
	upper := strings.ToUpper(s)
	if !strings.HasPrefix(upper, "SELECT") {
		return "", false
	}
	if sqlAggRe.MatchString(upper) ||
		strings.Contains(upper, " JOIN ") ||
		strings.Contains(upper, " GROUP BY") ||
		strings.Contains(upper, " HAVING") ||
		strings.Contains(upper, " UNION ") ||
		strings.Contains(upper, " DISTINCT") {
		return "", false
	}
	from := findTopLevelKeyword(s, "FROM")
	if from < 0 {
		return "", false
	}
	table := extractLeadingIdent(s[from+len("FROM"):])
	if table == "" || !isSimpleIdent(table) {
		return "", false
	}
	return table, true
}

// prependRowid 在 SELECT（及可选 DISTINCT/ALL 修饰符）之后插入 rowid 别名列，
// 供内联编辑按 rowid 精确定位行
func prependRowid(sql string) string {
	from := findTopLevelKeyword(sql, "FROM")
	head := strings.TrimSpace(sql[:from])
	tail := " " + strings.TrimSpace(sql[from:])
	pos := len("SELECT")
	rest := strings.TrimLeft(head[pos:], " \t")
	pos += len(head[pos:]) - len(rest)
	up := strings.ToUpper(rest)
	for _, kw := range []string{"DISTINCT", "ALL"} {
		if strings.HasPrefix(up, kw) && (len(rest) == len(kw) || !isIdentByte(rest[len(kw)])) {
			pos += len(kw)
			break
		}
	}
	return head[:pos] + ` rowid AS "_nbid",` + head[pos:] + tail
}

// isRowidTable 判断给定表名是否为可编辑的 rowid 表（排除视图与 WITHOUT ROWID 表）
func isRowidTable(db *sql.DB, name string) bool {
	var typ string
	var ddl *string
	if err := db.QueryRow(`SELECT type, sql FROM sqlite_master WHERE name = ?`, name).Scan(&typ, &ddl); err != nil {
		return false
	}
	if typ != "table" {
		return false
	}
	return ddl == nil || !strings.Contains(strings.ToLower(*ddl), "without rowid")
}

// copyPath 递归复制文件或目录，目标路径不存在时创建并保留原权限
