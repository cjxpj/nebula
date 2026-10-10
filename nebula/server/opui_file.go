package dic_server

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// 文件管理 API（OPUI 面板的目录 / 文件浏览、读写、上传下载等）。
func init() {
	registerOpuiApi(opuiHandleFileAPI,
		"file_list", "file_search", "file_read", "file_image",
		"file_write", "file_db_tables", "file_db_query", "file_db_update_cell",
		"file_create", "file_delete", "file_upload", "file_upload_status",
		"file_upload_chunk", "file_upload_merge", "file_mkdir", "file_rename",
		"file_copy", "file_move",
	)
}

// opuiHandleFileAPI 处理文件管理相关请求。
func opuiHandleFileAPI(w http.ResponseWriter, r *http.Request, h *HttpOpUiData) {
	switch h.Type {
	case "file_list":
		// 文件管理：列出指定目录（应用目录内）的直接子项（文件夹 + 文件），逐层浏览
		var j struct {
			Path string `json:"path"` // 当前目录，相对应用目录，空表示应用目录根
		}
		_ = json.Unmarshal(h.Data, &j)
		dirPath := strings.TrimSpace(j.Path)
		root := opuiAppDir()
		if dirPath != "" {
			if !checkFilePath(dirPath) {
				http.Error(w, `{"status":"error","error":"目录路径不合法"}`, http.StatusBadRequest)
				return
			}
			root = filepath.Join(opuiAppDir(), filepath.FromSlash(dirPath))
		}
		items, err := os.ReadDir(root)
		if err != nil {
			http.Error(w, `{"status":"error","error":"读取目录失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		type fileEntry struct {
			Name  string `json:"name"`
			Path  string `json:"path"`
			Dir   bool   `json:"dir"`
			Size  int64  `json:"size"`
			Mtime int64  `json:"mtime"`
		}
		entries := make([]fileEntry, 0, len(items))
		for _, it := range items {
			name := it.Name()
			info, ierr := it.Info()
			if ierr != nil {
				continue
			}
			entries = append(entries, fileEntry{
				Name:  name,
				Path:  filepath.ToSlash(filepath.Join(dirPath, name)),
				Dir:   it.IsDir(),
				Size:  info.Size(),
				Mtime: info.ModTime().Unix(),
			})
		}
		// 文件夹在前，文件夹/文件各自按名称排序
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].Dir != entries[j].Dir {
				return entries[i].Dir
			}
			return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
		})
		jsonResp, _ := json.Marshal(map[string]any{
			"entries": entries,
			"root":    filepath.Base(opuiAppDir()), // 应用数据目录名，供前端面包屑根节点显示真实目录名
		})
		w.Write(jsonResp)
		return

	case "file_search":
		// 文件管理搜索：默认搜索当前目录下的直接子项（文件夹+文件），
		// 开启深度搜索则递归子目录；默认按名称包含关键字（忽略大小写），
		// 开启正则则按正则表达式匹配名称（忽略大小写）
		var j struct {
			Path    string `json:"path"`
			Keyword string `json:"keyword"`
			Deep    bool   `json:"deep"`
			Regex   bool   `json:"regex"`
		}
		_ = json.Unmarshal(h.Data, &j)
		dirPath := strings.TrimSpace(j.Path)
		root := opuiAppDir()
		if dirPath != "" {
			if !checkFilePath(dirPath) {
				http.Error(w, `{"status":"error","error":"目录路径不合法"}`, http.StatusBadRequest)
				return
			}
			root = filepath.Join(opuiAppDir(), filepath.FromSlash(dirPath))
		}
		kw := strings.TrimSpace(j.Keyword)
		var re *regexp.Regexp
		if j.Regex && kw != "" {
			var rerr error
			re, rerr = regexp.Compile("(?i)" + kw)
			if rerr != nil {
				http.Error(w, `{"status":"error","error":"正则表达式无效: `+rerr.Error()+`"}`, http.StatusBadRequest)
				return
			}
		}
		matchName := func(name string) bool {
			if kw == "" {
				return true
			}
			if re != nil {
				return re.MatchString(name)
			}
			return strings.Contains(strings.ToLower(name), strings.ToLower(kw))
		}
		type fileEntry struct {
			Name  string `json:"name"`
			Path  string `json:"path"`
			Dir   bool   `json:"dir"`
			Size  int64  `json:"size"`
			Mtime int64  `json:"mtime"`
		}
		entries := make([]fileEntry, 0)
		if j.Deep {
			_ = filepath.Walk(root, func(full string, info os.FileInfo, err error) error {
				if err != nil || full == root {
					return nil
				}
				name := info.Name()
				if !matchName(name) {
					return nil
				}
				rel, _ := filepath.Rel(root, full)
				relPath := filepath.ToSlash(rel)
				if dirPath != "" {
					relPath = filepath.ToSlash(filepath.Join(dirPath, rel))
				}
				entries = append(entries, fileEntry{
					Name:  name,
					Path:  relPath,
					Dir:   info.IsDir(),
					Size:  info.Size(),
					Mtime: info.ModTime().Unix(),
				})
				return nil
			})
		} else {
			items, err := os.ReadDir(root)
			if err != nil {
				http.Error(w, `{"status":"error","error":"读取目录失败: `+err.Error()+`"}`, http.StatusBadRequest)
				return
			}
			for _, it := range items {
				name := it.Name()
				if !matchName(name) {
					continue
				}
				info, ierr := it.Info()
				if ierr != nil {
					continue
				}
				entries = append(entries, fileEntry{
					Name:  name,
					Path:  filepath.ToSlash(filepath.Join(dirPath, name)),
					Dir:   it.IsDir(),
					Size:  info.Size(),
					Mtime: info.ModTime().Unix(),
				})
			}
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].Dir != entries[j].Dir {
				return entries[i].Dir
			}
			return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
		})
		jsonResp, _ := json.Marshal(map[string]any{"entries": entries})
		w.Write(jsonResp)
		return

	case "file_read":
		// 读取应用目录下任意文件（.n 词库请使用 dic_get_content）
		var j struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !checkFilePath(j.Path) {
			http.Error(w, `{"status":"error","error":"文件路径不合法"}`, http.StatusBadRequest)
			return
		}
		full := filepath.Join(opuiAppDir(), filepath.FromSlash(j.Path))
		data, err := os.ReadFile(full)
		if err != nil {
			http.Error(w, `{"status":"error","error":"文件读取失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		// 检测二进制：含 NUL 字节或非 UTF-8 文本，避免前端直接编辑损坏内容
		binary := bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data)
		resp := map[string]any{
			"path":   j.Path,
			"size":   int64(len(data)),
			"binary": binary,
		}
		if !binary {
			resp["content"] = string(data)
		}
		jsonResp, _ := json.Marshal(resp)
		w.Write(jsonResp)
		return

	case "file_image":
		// 读取图片文件并返回 base64，供前端在线预览
		var j struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !checkFilePath(j.Path) {
			http.Error(w, `{"status":"error","error":"文件路径不合法"}`, http.StatusBadRequest)
			return
		}
		full := filepath.Join(opuiAppDir(), filepath.FromSlash(j.Path))
		data, err := os.ReadFile(full)
		if err != nil {
			http.Error(w, `{"status":"error","error":"文件读取失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		mime := "application/octet-stream"
		switch strings.ToLower(filepath.Ext(j.Path)) {
		case ".png":
			mime = "image/png"
		case ".jpg", ".jpeg":
			mime = "image/jpeg"
		case ".gif":
			mime = "image/gif"
		case ".webp":
			mime = "image/webp"
		case ".bmp":
			mime = "image/bmp"
		case ".svg":
			mime = "image/svg+xml"
		case ".ico":
			mime = "image/x-icon"
		case ".avif":
			mime = "image/avif"
		}
		jsonResp, _ := json.Marshal(map[string]any{
			"path": j.Path,
			"mime": mime,
			"data": base64.StdEncoding.EncodeToString(data),
		})
		w.Write(jsonResp)
		return

	case "file_write":
		// 写入应用目录下任意文件（.n 词库请使用 dic_save_content）
		var j struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !checkFilePath(j.Path) {
			http.Error(w, `{"status":"error","error":"文件路径不合法"}`, http.StatusBadRequest)
			return
		}
		full := filepath.Join(opuiAppDir(), filepath.FromSlash(j.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			http.Error(w, `{"status":"error","error":"创建目录失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		if err := os.WriteFile(full, []byte(j.Content), 0o644); err != nil {
			http.Error(w, `{"status":"error","error":"文件写入失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
		w.Write(jsonResp)
		return

	case "file_db_tables":
		// 文件管理：列出 SQLite 数据库中的表/视图及建表语句
		var j struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		db := opuiOpenSqlite(w, j.Path)
		if db == nil {
			return
		}
		defer db.Close()
		rows, err := db.Query(`SELECT name, type, sql FROM sqlite_master WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY name`)
		if err != nil {
			http.Error(w, `{"status":"error","error":"数据库读取失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		defer rows.Close()
		tables := make([]map[string]string, 0)
		for rows.Next() {
			var name, typ string
			var sqlText *string
			if err := rows.Scan(&name, &typ, &sqlText); err != nil {
				continue
			}
			createSQL := ""
			if sqlText != nil {
				createSQL = *sqlText
			}
			tables = append(tables, map[string]string{"name": name, "type": typ, "sql": createSQL})
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok", "tables": tables})
		w.Write(jsonResp)
		return

	case "file_db_query":
		// 文件管理：对 SQLite 数据库执行 SQL（SELECT/WITH/PRAGMA 返回表格，其余为执行语句）
		var j struct {
			Path string `json:"path"`
			Sql  string `json:"sql"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		db := opuiOpenSqlite(w, j.Path)
		if db == nil {
			return
		}
		defer db.Close()
		trimmed := strings.ToUpper(stripLeadingSQLComments(j.Sql))
		isQuery := strings.HasPrefix(trimmed, "SELECT") || strings.HasPrefix(trimmed, "WITH") ||
			strings.HasPrefix(trimmed, "PRAGMA") || strings.HasPrefix(trimmed, "EXPLAIN") ||
			strings.HasPrefix(trimmed, "VALUES") || strings.Contains(trimmed, " RETURNING ")
		if !isQuery {
			res, err := db.Exec(j.Sql)
			if err != nil {
				jsonResp, _ := json.Marshal(map[string]any{"status": "ok", "error": err.Error()})
				w.Write(jsonResp)
				return
			}
			affected, _ := res.RowsAffected()
			id, _ := res.LastInsertId()
			jsonResp, _ := json.Marshal(map[string]any{"status": "ok", "rows_affected": affected, "last_insert_id": id})
			w.Write(jsonResp)
			return
		}
		// 查询路径：单表 SELECT 且为 rowid 表时自动前置 rowid，使结果支持内联编辑
		execSQL := j.Sql
		editable := false
		table := ""
		if t, ok := detectEditableTable(j.Sql); ok && isRowidTable(db, t) {
			execSQL = prependRowid(j.Sql)
			editable = true
			table = t
		}
		rows, err := db.Query(execSQL)
		if err != nil {
			// 改写后的查询失败则回退原始查询（只读）
			if execSQL != j.Sql {
				rows, err = db.Query(j.Sql)
				editable = false
				table = ""
			}
			if err != nil {
				jsonResp, _ := json.Marshal(map[string]any{"status": "ok", "error": err.Error()})
				w.Write(jsonResp)
				return
			}
		}
		defer rows.Close()
		columns, _ := rows.Columns()
		out := make([][]any, 0)
		for rows.Next() {
			// 结果行数上限，避免大表拖垮前端
			if len(out) >= 1000 {
				break
			}
			vals := make([]any, len(columns))
			ptrs := make([]any, len(columns))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				continue
			}
			row := make([]any, len(columns))
			for i, v := range vals {
				row[i] = sqliteCellValue(v)
			}
			out = append(out, row)
		}
		jsonResp, _ := json.Marshal(map[string]any{
			"status": "ok", "columns": columns, "rows": out,
			"editable": editable, "table": table,
		})
		w.Write(jsonResp)
		return

	case "file_db_update_cell":
		// 文件管理：内联编辑表格单元格（通过 rowid 定位行，仅限简单标识符表/列）
		var j struct {
			Path   string `json:"path"`
			Table  string `json:"table"`
			Rowid  int64  `json:"rowid"`
			Column string `json:"column"`
			Value  any    `json:"value"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !isSimpleIdent(j.Table) || !isSimpleIdent(j.Column) {
			http.Error(w, `{"status":"error","error":"参数不合法"}`, http.StatusBadRequest)
			return
		}
		db := opuiOpenSqlite(w, j.Path)
		if db == nil {
			return
		}
		defer db.Close()
		// 值绑定：空字符串视为 NULL；其余原样按字符串绑定，交由 SQLite 列类型亲和性自动转换，
		// 避免后端强转数字导致 TEXT 列前导零（如 "007"）被截断
		var bind any
		if s, ok := j.Value.(string); ok {
			bind = s
			if s == "" {
				bind = nil
			}
		} else {
			bind = j.Value
		}
		updateSQL := fmt.Sprintf(`UPDATE "%s" SET "%s" = ? WHERE rowid = ?`, j.Table, j.Column)
		res, err := db.Exec(updateSQL, bind, j.Rowid)
		if err != nil {
			jsonResp, _ := json.Marshal(map[string]any{"status": "ok", "error": err.Error()})
			w.Write(jsonResp)
			return
		}
		affected, _ := res.RowsAffected()
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok", "rows_affected": affected})
		w.Write(jsonResp)
		return

	case "file_create":
		// 文件管理：在指定目录下创建新文件（可选初始内容，默认空文件）
		var j struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !checkFilePath(j.Path) {
			http.Error(w, `{"status":"error","error":"文件路径不合法"}`, http.StatusBadRequest)
			return
		}
		full := filepath.Join(opuiAppDir(), filepath.FromSlash(j.Path))
		if _, err := os.Stat(full); err == nil {
			http.Error(w, `{"status":"error","error":"文件已存在"}`, http.StatusBadRequest)
			return
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			http.Error(w, `{"status":"error","error":"创建目录失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		if err := os.WriteFile(full, []byte(j.Content), 0o644); err != nil {
			http.Error(w, `{"status":"error","error":"文件创建失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
		w.Write(jsonResp)
		return

	case "file_delete":
		// 文件管理：删除指定文件或目录（递归删除）
		var j struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !checkFilePath(j.Path) {
			http.Error(w, `{"status":"error","error":"文件路径不合法"}`, http.StatusBadRequest)
			return
		}
		full := filepath.Join(opuiAppDir(), filepath.FromSlash(j.Path))
		if err := os.RemoveAll(full); err != nil {
			http.Error(w, `{"status":"error","error":"删除失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
		w.Write(jsonResp)
		return

	case "file_upload":
		// 文件管理：上传文件（内容为 base64 编码），同名文件覆盖
		var j struct {
			Path string `json:"path"`
			Data string `json:"data"` // base64 编码的文件内容
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !checkFilePath(j.Path) {
			http.Error(w, `{"status":"error","error":"文件路径不合法"}`, http.StatusBadRequest)
			return
		}
		raw, err := base64.StdEncoding.DecodeString(j.Data)
		if err != nil {
			http.Error(w, `{"status":"error","error":"文件内容解码失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		full := filepath.Join(opuiAppDir(), filepath.FromSlash(j.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			http.Error(w, `{"status":"error","error":"创建目录失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		if err := os.WriteFile(full, raw, 0o644); err != nil {
			http.Error(w, `{"status":"error","error":"文件写入失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
		w.Write(jsonResp)
		return

	case "file_upload_status":
		// 文件管理：查询分块上传进度（已上传的分块序号）
		var j struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !checkFilePath(j.Path) {
			http.Error(w, `{"status":"error","error":"文件路径不合法"}`, http.StatusBadRequest)
			return
		}
		chunks := []int{}
		if items, err := os.ReadDir(uploadTmpDir(j.Path)); err == nil {
			for _, e := range items {
				name := e.Name()
				if !strings.HasPrefix(name, "chunk_") {
					continue
				}
				idx, err := strconv.Atoi(strings.TrimPrefix(name, "chunk_"))
				if err == nil {
					chunks = append(chunks, idx)
				}
			}
		}
		sort.Ints(chunks)
		jsonResp, _ := json.Marshal(map[string]any{"chunks": chunks})
		w.Write(jsonResp)
		return

	case "file_upload_chunk":
		// 文件管理：上传单个分块（base64）
		var j struct {
			Path  string `json:"path"`
			Index int    `json:"index"`
			Total int    `json:"total"`
			Data  string `json:"data"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !checkFilePath(j.Path) {
			http.Error(w, `{"status":"error","error":"文件路径不合法"}`, http.StatusBadRequest)
			return
		}
		if j.Index < 0 || j.Total <= 0 || j.Index >= j.Total {
			http.Error(w, `{"status":"error","error":"分块参数不合法"}`, http.StatusBadRequest)
			return
		}
		raw, err := base64.StdEncoding.DecodeString(j.Data)
		if err != nil {
			http.Error(w, `{"status":"error","error":"分块内容解码失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		tmpDir := uploadTmpDir(j.Path)
		if err := os.MkdirAll(tmpDir, 0o755); err != nil {
			http.Error(w, `{"status":"error","error":"创建临时目录失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		chunkFile := filepath.Join(tmpDir, fmt.Sprintf("chunk_%d", j.Index))
		if err := os.WriteFile(chunkFile, raw, 0o644); err != nil {
			http.Error(w, `{"status":"error","error":"分块写入失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
		w.Write(jsonResp)
		return

	case "file_upload_merge":
		// 文件管理：合并所有分块为最终文件
		var j struct {
			Path  string `json:"path"`
			Total int    `json:"total"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !checkFilePath(j.Path) {
			http.Error(w, `{"status":"error","error":"文件路径不合法"}`, http.StatusBadRequest)
			return
		}
		if j.Total <= 0 {
			http.Error(w, `{"status":"error","error":"分块总数不合法"}`, http.StatusBadRequest)
			return
		}
		tmpDir := uploadTmpDir(j.Path)
		full := filepath.Join(opuiAppDir(), filepath.FromSlash(j.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			http.Error(w, `{"status":"error","error":"创建目录失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		out, err := os.Create(full)
		if err != nil {
			http.Error(w, `{"status":"error","error":"创建目标文件失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		var mergeErr error
		for i := 0; i < j.Total; i++ {
			chunkFile := filepath.Join(tmpDir, fmt.Sprintf("chunk_%d", i))
			data, err := os.ReadFile(chunkFile)
			if err != nil {
				mergeErr = fmt.Errorf("分块 %d 缺失或读取失败: %v", i, err)
				break
			}
			if _, err := out.Write(data); err != nil {
				mergeErr = fmt.Errorf("写入目标文件失败: %v", err)
				break
			}
		}
		out.Close()
		if mergeErr != nil {
			_ = os.Remove(full)
			http.Error(w, `{"status":"error","error":"`+mergeErr.Error()+`"}`, http.StatusBadRequest)
			return
		}
		_ = os.RemoveAll(tmpDir) // 清理临时分块
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
		w.Write(jsonResp)
		return

	case "file_mkdir":
		// 文件管理：新建文件夹
		var j struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !checkFilePath(j.Path) {
			http.Error(w, `{"status":"error","error":"路径不合法"}`, http.StatusBadRequest)
			return
		}
		full := filepath.Join(opuiAppDir(), filepath.FromSlash(j.Path))
		if _, err := os.Stat(full); err == nil {
			http.Error(w, `{"status":"error","error":"已存在同名项"}`, http.StatusBadRequest)
			return
		}
		if err := os.MkdirAll(full, 0o755); err != nil {
			http.Error(w, `{"status":"error","error":"创建文件夹失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
		w.Write(jsonResp)
		return

	case "file_rename":
		// 文件管理：重命名文件/目录
		var j struct {
			Path    string `json:"path"`
			NewName string `json:"newName"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if !checkFilePath(j.Path) {
			http.Error(w, `{"status":"error","error":"路径不合法"}`, http.StatusBadRequest)
			return
		}
		newName := strings.TrimSpace(j.NewName)
		if newName == "" {
			http.Error(w, `{"status":"error","error":"名称不能为空"}`, http.StatusBadRequest)
			return
		}
		if newName == "." || newName == ".." || strings.ContainsAny(newName, `/\`) {
			http.Error(w, `{"status":"error","error":"名称不合法"}`, http.StatusBadRequest)
			return
		}
		oldFull := filepath.Join(opuiAppDir(), filepath.FromSlash(j.Path))
		if _, err := os.Stat(oldFull); err != nil {
			http.Error(w, `{"status":"error","error":"原文件不存在"}`, http.StatusBadRequest)
			return
		}
		newFull := filepath.Join(filepath.Dir(oldFull), newName)
		if _, err := os.Stat(newFull); err == nil {
			http.Error(w, `{"status":"error","error":"已存在同名项"}`, http.StatusBadRequest)
			return
		}
		if err := os.Rename(oldFull, newFull); err != nil {
			http.Error(w, `{"status":"error","error":"重命名失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
		w.Write(jsonResp)
		return

	case "file_copy":
		// 文件管理：复制多个文件/目录到目标目录
		var j struct {
			Paths  []string `json:"paths"`
			Target string   `json:"target"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if len(j.Paths) == 0 {
			http.Error(w, `{"status":"error","error":"未选择文件"}`, http.StatusBadRequest)
			return
		}
		for _, p := range j.Paths {
			if !checkFilePath(p) {
				http.Error(w, `{"status":"error","error":"源路径不合法"}`, http.StatusBadRequest)
				return
			}
		}
		if j.Target != "" && !checkFilePath(j.Target) {
			http.Error(w, `{"status":"error","error":"目标路径不合法"}`, http.StatusBadRequest)
			return
		}
		appDir := opuiAppDir()
		targetDir := filepath.Join(appDir, filepath.FromSlash(j.Target))
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			http.Error(w, `{"status":"error","error":"创建目标目录失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		for _, p := range j.Paths {
			src := filepath.Join(appDir, filepath.FromSlash(p))
			base := filepath.Base(src)
			dst := filepath.Join(targetDir, base)
			if _, err := os.Stat(dst); err == nil {
				http.Error(w, `{"status":"error","error":"目标已存在同名项: `+base+`"}`, http.StatusBadRequest)
				return
			}
			if err := copyPath(src, dst); err != nil {
				http.Error(w, `{"status":"error","error":"复制失败: `+err.Error()+`"}`, http.StatusBadRequest)
				return
			}
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
		w.Write(jsonResp)
		return

	case "file_move":
		// 文件管理：移动（剪切）多个文件/目录到目标目录
		var j struct {
			Paths  []string `json:"paths"`
			Target string   `json:"target"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if len(j.Paths) == 0 {
			http.Error(w, `{"status":"error","error":"未选择文件"}`, http.StatusBadRequest)
			return
		}
		for _, p := range j.Paths {
			if !checkFilePath(p) {
				http.Error(w, `{"status":"error","error":"源路径不合法"}`, http.StatusBadRequest)
				return
			}
		}
		if j.Target != "" && !checkFilePath(j.Target) {
			http.Error(w, `{"status":"error","error":"目标路径不合法"}`, http.StatusBadRequest)
			return
		}
		appDir := opuiAppDir()
		targetDir := filepath.Join(appDir, filepath.FromSlash(j.Target))
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			http.Error(w, `{"status":"error","error":"创建目标目录失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		for _, p := range j.Paths {
			src := filepath.Join(appDir, filepath.FromSlash(p))
			base := filepath.Base(src)
			dst := filepath.Join(targetDir, base)
			if filepath.Clean(src) == filepath.Clean(dst) {
				continue // 源就在目标目录，无需移动
			}
			if _, err := os.Stat(dst); err == nil {
				http.Error(w, `{"status":"error","error":"目标已存在同名项: `+base+`"}`, http.StatusBadRequest)
				return
			}
			if err := os.Rename(src, dst); err != nil {
				http.Error(w, `{"status":"error","error":"移动失败: `+err.Error()+`"}`, http.StatusBadRequest)
				return
			}
		}
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok"})
		w.Write(jsonResp)
		return
	}
}

func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return copyFile(src, dst, info.Mode().Perm())
	}
	if err = os.MkdirAll(dst, info.Mode().Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := copyPath(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// copyFile 复制单个文件并保留权限
func copyFile(src, dst string, perm os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, perm)
}

// uploadTmpDir 返回分块上传的临时目录（按目标路径取 md5 避免冲突）
func uploadTmpDir(path string) string {
	sum := md5.Sum([]byte(path))
	return filepath.Join(os.TempDir(), "nebula_upload_"+fmt.Sprintf("%x", sum[:]))
}
