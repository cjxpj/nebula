package dic_server

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/build"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	dic_funcs "github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"
	"github.com/cjxpj/nebula/utils"
)

type respHeader struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// localRunResult 本地执行网页词库的结果：渲染输出 + 词库设置的响应状态 / 响应头。
type localRunResult struct {
	Output  string       // 模板渲染后的整页输出
	Status  string       // 响应状态（全局变量 响应状态，未设置时为 200）
	Headers []respHeader // 设置头部 + 输出头部 合并后的最终响应头（同名后者覆盖）
}

// setRespHeader 记录 / 覆盖一个响应头：同名覆盖值并保持首次出现的顺序，
// 与 HTTP 链路的 w.Header().Set 语义一致（头名按标准格式规范化）。
func setRespHeader(headers *[]respHeader, key, value string) {
	if key == "" {
		return
	}
	key = textproto.CanonicalMIMEHeaderKey(key)
	for i := range *headers {
		if (*headers)[i].Key == key {
			(*headers)[i].Value = value
			return
		}
	}
	*headers = append(*headers, respHeader{Key: key, Value: value})
}

// localHTTPFuncs 本地运行词库时注入的 HTTP 链路内置函数实现（.n 与 .wn 共用）。
// 词库调试与 AI 工具没有真实请求上下文：GET / POST 取默认值；
// 设置头部 不再忽略，而是记录进 headers 随运行结果返回——
// 前端据此按 Content-Type 决定输出区域的展示方式（HTML 渲染预览 / JSON / 文本）。
func localHTTPFuncs(headers *[]respHeader) map[string]dto.DicFunc {
	getDefault := func(d *dto.DicInputs) (any, error) {
		if d.Inputs.LenOk(2) {
			if s, ok := d.Inputs.Get(2).(string); ok {
				return s, nil
			}
		}
		return "", nil
	}
	return map[string]dto.DicFunc{
		"设置头部": {L: "2", Fn: func(d *dto.DicInputs) (any, error) {
			key, ok := d.Inputs.Get(1).(string)
			if !ok {
				return "参数错误1", nil
			}
			value, ok := d.Inputs.Get(2).(string)
			if !ok {
				return "参数错误2", nil
			}
			setRespHeader(headers, key, value)
			return "", nil
		}},
		"GET":  {L: "1|2", Fn: getDefault},
		"POST": {L: "1|2", Fn: getDefault},
	}
}

// ensureLocalRespDefaults 补齐与 HTTP 链路（serveHTTP）一致的响应全局变量默认值，
// 词库已自行设置时以词库为准。
func ensureLocalRespDefaults(val *dto.Val) {
	if val.Get("响应状态") == nil {
		val.Set("响应状态", "200")
	}
	if val.Get("输出头部") == nil {
		val.Set("输出头部", "{}")
	}
}

// collectLocalResp 运行结束后收集词库设置的响应信息：
// 全局变量 输出头部（JSON 对象形式的额外响应头）在 HTTP 链路于输出阶段写入响应，
// 这里解析后合并进收集结果（同名覆盖 设置头部）；响应状态未设置时按 200。
func collectLocalResp(val *dto.Val, headers []respHeader) (string, []respHeader) {
	if raw, ok := val.Get("输出头部").(string); ok && raw != "" && raw != "{}" {
		var extra map[string]string
		if err := json.Unmarshal([]byte(raw), &extra); err == nil {
			// JSON 对象无序，按 key 排序保证展示顺序稳定
			keys := make([]string, 0, len(extra))
			for k := range extra {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				setRespHeader(&headers, k, extra[k])
			}
		}
	}
	status := "200"
	if s, ok := val.Get("响应状态").(string); ok && s != "" {
		status = s
	}
	return status, headers
}

// attachLocalHTTPFuncs 为本地运行的普通词库（.n）注入 HTTP 链路内置函数（设置头部 / GET / POST），
// 并补齐响应全局变量默认值，行为与 serveHTTP 的 .n 分支一致。
// MyFunc 先复制再注入：编译缓存命中时词库实例与缓存共享同一函数表，就地写入会污染后续加载。
// 返回的指针在整个运行期间有效，运行结束后交给 collectLocalResp 读取收集结果。
func attachLocalHTTPFuncs(dic *dic_dto.Dic) *[]respHeader {
	merged := make(map[string]dto.DicFunc, len(dic.MyFunc)+3)
	maps.Copy(merged, dic.MyFunc)
	dic.MyFunc = merged
	headers := &[]respHeader{}
	maps.Copy(dic.MyFunc, localHTTPFuncs(headers))
	ensureLocalRespDefaults(dic.Val.G)
	return headers
}

// runWebDicLocal 本地执行 .wn 网页词库：读文件 → 注入 HTTP 函数与全局变量 →
// 执行脚本块并完成模板渲染。词库调试与 AI 的 run_web_dic 工具共用。
// 真实 HTTP 链路里 设置头部 / 响应状态 会写进响应对象，本地没有响应对象，
// 改为收集后随结果返回；输出头部（JSON 自定义头）在 HTTP 链路于输出阶段写入，这里同样在运行结束后合并。
func runWebDicLocal(path string, g map[string]string) (*localRunResult, error) {
	data, err := utils.NewFileQueue(path).ReadFileByte()
	if err != nil {
		return nil, err
	}
	webdic := dic_dto.NewWebDic(path, string(data))
	headers := []respHeader{}
	webdic.MyFunc = localHTTPFuncs(&headers)
	for k, v := range g {
		webdic.Val.G.Set(k, v)
	}
	ensureLocalRespDefaults(webdic.Val.G)
	output := dic_api.Api.WebDicRun(webdic)
	status, headers := collectLocalResp(webdic.Val.G, headers)
	return &localRunResult{Output: output, Status: status, Headers: headers}, nil
}

// checkFilePath 校验文件管理路径：仅允许应用目录内的相对路径，
// 拒绝绝对路径、包含 .. 的越权路径
func checkFilePath(path string) bool {
	if path == "" || filepath.IsAbs(path) {
		return false
	}
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "\\") {
		return false
	}
	if strings.Contains(path, "..") {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

// opuiAppDir 返回 OPUI 文件管理的根目录。
// 桌面端 utils.GetAppDir() 返回空串（相对路径基于进程当前工作目录），此时回退到当前工作目录，
// 避免 os.ReadDir("") / filepath.Walk("") 等接口因空路径失败。

// 词库管理 API（列表 / 内容 / 格式化 / IR / 积木 / 加密）。
func init() {
	registerOpuiApi(opuiHandleDicAPI,
		"dic_encrypt_file", "get_dic_list", "dic_get_content", "dic_save_content",
		"dic_format", "get_dic_funcs", "gen_dic_from_ir", "gen_ir_from_dic",
		"gen_dic_from_blocks", "gen_blocks_from_dic", "gen_blocks_from_code",
	)
}

// opuiHandleDicAPI 处理本模块注册的 OPUI API 类型。
func opuiHandleDicAPI(w http.ResponseWriter, r *http.Request, h *HttpOpUiData) {
	switch h.Type {
	case "dic_encrypt_file":
		// 生成加密词库：读取 .n 词库文件，加密后在原目录生成同名 .bak 文件
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
		src := filepath.Join(opuiAppDir(), filepath.FromSlash(j.Path))
		data, err := os.ReadFile(src)
		if err != nil {
			http.Error(w, `{"status":"error","error":"文件读取失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		encodeDic, err := utils.Encrypt(string(data), appfiles.Key)
		if err != nil {
			http.Error(w, `{"status":"error","error":"加密失败"}`, http.StatusBadRequest)
			return
		}
		dst := j.Path + ".bak"
		full := filepath.Join(opuiAppDir(), filepath.FromSlash(dst))
		if err := os.WriteFile(full, []byte("// "+appfiles.Version+"\n"+encodeDic), 0o644); err != nil {
			http.Error(w, `{"status":"error","error":"文件写入失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		var rj struct {
			Status string `json:"status"`
			Path   string `json:"path"`
		}
		rj.Status = "ok"
		rj.Path = dst
		r, _ := json.Marshal(rj)
		w.Write(r)
		return

	case "get_dic_list":
		// 词库打开：读取指定目录下的直接子项（文件夹 + .n 文件），逐层浏览，不递归扫描
		var j struct {
			Path string `json:"path"` // 当前目录，相对应用目录，空表示应用目录根
		}
		_ = json.Unmarshal(h.Data, &j)
		dirPath := strings.TrimSpace(j.Path)
		root := opuiAppDir()
		if dirPath != "" {
			if !checkFilePath(dirPath) {
				jsonResp, _ := json.Marshal(map[string]any{"entries": []any{}})
				w.Write(jsonResp)
				return
			}
			root = filepath.Join(opuiAppDir(), filepath.FromSlash(dirPath))
		}
		items, err := os.ReadDir(root)
		if err != nil {
			jsonResp, _ := json.Marshal(map[string]any{"entries": []any{}})
			w.Write(jsonResp)
			return
		}
		type dicEntry struct {
			Name string `json:"name"`
			Path string `json:"path"`
			Dir  bool   `json:"dir"`
		}
		entries := make([]dicEntry, 0, len(items))
		for _, it := range items {
			name := it.Name()
			if !it.IsDir() && !strings.HasSuffix(strings.ToLower(name), ".n") && !checkWebDicPath(name) {
				continue // 只显示文件夹与 .n / .wn 词库文件
			}
			entries = append(entries, dicEntry{
				Name: name,
				Path: filepath.ToSlash(filepath.Join(dirPath, name)),
				Dir:  it.IsDir(),
			})
		}
		// 文件夹在前，文件夹/文件各自按名称排序
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].Dir != entries[j].Dir {
				return entries[i].Dir
			}
			return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
		})
		jsonResp, _ := json.Marshal(map[string]any{"entries": entries})
		w.Write(jsonResp)
		return

	case "dic_get_content":
		var j struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if j.Path == "" {
			http.Error(w, `{"status":"error","error":"词库路径不能为空"}`, http.StatusBadRequest)
			return
		}
		if !checkDicOrWebPath(j.Path) {
			http.Error(w, `{"status":"error","error":"词库路径不合法"}`, http.StatusBadRequest)
			return
		}
		content, err := utils.NewFileQueue(j.Path).ReadFromFile()
		if err != nil {
			if os.IsNotExist(err) {
				// 默认调试词库（如 private/debug.n）首次打开时自动创建空文件
				if filepath.ToSlash(filepath.Clean(j.Path)) == defaultDebugDic() {
					utils.NewFileQueue(j.Path).WriteToFile("")
					content = ""
				} else {
					w.Write([]byte(`{"status":"not_found","error":"词库文件不存在"}`))
					return
				}
			} else {
				http.Error(w, `{"status":"error","error":"词库读取失败: `+err.Error()+`"}`, http.StatusBadRequest)
				return
			}
		}

		// 编译检测：打开词库时也编译一次，返回编译问题（error 红色 / warning 黄色）供前端高亮。
		// 网页词库（.wn）是 HTML，走脚本块变量检查 + 模板键核对。
		resp := map[string]any{"content": content}
		if checkWebDicPath(j.Path) {
			if warnings := run.WebDicCheck(content); len(warnings) > 0 {
				resp["warnings"] = warnings
			}
		} else if dicCheck, err := dic_dto.RunDicNoCache(j.Path); err == nil && dicCheck != nil {
			defer dicCheck.Close()
			if len(dicCheck.Data.Warnings) > 0 {
				resp["warnings"] = dicCheck.Data.Warnings
			}
		}
		jsonResp, _ := json.Marshal(resp)
		w.Write(jsonResp)
		return

	case "dic_save_content":
		var j struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if j.Path == "" {
			http.Error(w, `{"status":"error","error":"词库路径不能为空"}`, http.StatusBadRequest)
			return
		}
		if !checkDicOrWebPath(j.Path) {
			http.Error(w, `{"status":"error","error":"词库路径不合法"}`, http.StatusBadRequest)
			return
		}
		// 网页词库（.wn）：走 HTML 排版 + 脚本块检查，不参与 .n 的块结构格式化与编译
		if checkWebDicPath(j.Path) {
			content := j.Content
			formatted := false
			if dicAutoFormatEnabled() {
				if f := build.FormatWebDic(content); f != content {
					content = f
					formatted = true
				}
			}
			utils.NewFileQueue(j.Path).WriteToFile(content)

			resp := map[string]any{"status": "ok"}
			if formatted {
				// 回传格式化结果，供前端同步编辑器内容（避免界面与实际文件不一致）
				resp["content"] = content
			}
			if warnings := run.WebDicCheck(content); len(warnings) > 0 {
				resp["warnings"] = warnings
			}
			jsonResp, _ := json.Marshal(resp)
			w.Write(jsonResp)
			return
		}
		// 保存自动格式化（运行配置开关，默认启用）：落盘前按块结构重新缩进
		content := j.Content
		formatted := false
		if dicAutoFormatEnabled() {
			if f := build.FormatDic(content); f != content {
				content = f
				formatted = true
			}
		}
		utils.NewFileQueue(j.Path).WriteToFile(content)

		// 编译检测：保存后重新编译词库，返回编译问题（error 红色 / warning 黄色）供前端高亮
		resp := map[string]any{"status": "ok"}
		if formatted {
			// 回传格式化结果，供前端同步编辑器内容（避免界面与实际文件不一致）
			resp["content"] = content
		}
		if dicCheck, err := dic_dto.RunDicNoCache(j.Path); err == nil && dicCheck != nil {
			defer dicCheck.Close()
			if len(dicCheck.Data.Warnings) > 0 {
				resp["warnings"] = dicCheck.Data.Warnings
			}
		}
		jsonResp, _ := json.Marshal(resp)
		w.Write(jsonResp)
		return

	case "dic_format":
		// 词库格式化：.n 按块结构自动缩进（build.FormatDic），.wn 只缩进 HTML 结构
		// 并逐字节保留 <script type="nebula"> 正文（build.FormatWebDic），与命令行 -format 共用实现
		var j struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if j.Path == "" {
			http.Error(w, `{"status":"error","error":"词库路径不能为空"}`, http.StatusBadRequest)
			return
		}
		if !checkDicOrWebPath(j.Path) {
			http.Error(w, `{"status":"error","error":"词库路径不合法"}`, http.StatusBadRequest)
			return
		}
		var formatted string
		if checkWebDicPath(j.Path) {
			formatted = build.FormatWebDic(j.Content)
		} else {
			formatted = build.FormatDic(j.Content)
		}
		jsonResp, _ := json.Marshal(map[string]any{
			"status":  "ok",
			"content": formatted,
			"changed": formatted != j.Content,
		})
		w.Write(jsonResp)
		return

	case "get_dic_funcs":
		// 返回全部已注册词库函数（实时读取注册表），供前端代码补全
		jsonResp, _ := json.Marshal(map[string]any{"cmds": dic_funcs.ListFuncs()})
		w.Write(jsonResp)
		return

	case "gen_dic_from_ir":
		// 接收结构化 JSON IR，统一转成 .n 源码返回。
		// 积木编程与词库调试 AI 协作共用：前端 / AI 只产出 IR，格式由后端保证。
		var ir build.IR
		if err := json.Unmarshal(h.Data, &ir); err != nil {
			http.Error(w, `{"status":"error","error":"invalid ir json"}`, http.StatusBadRequest)
			return
		}
		// 与保存/格式化（dic_save_content、dic_format）共用 FormatDic，
		// 保证「生成出来的代码」与「保存后落盘/回显的代码」逐字一致。
		jsonResp, _ := json.Marshal(map[string]any{"code": build.FormatDic(build.IRToNebula(ir))})
		w.Write(jsonResp)
		return

	case "gen_ir_from_dic":
		// 读取磁盘上的词库文件，反向解析为结构化 JSON IR 返回。
		// 积木编程打开已有 .n 文件时用：以文件内容为准转成积木显示。
		var j struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if j.Path == "" {
			http.Error(w, `{"status":"error","error":"词库路径不能为空"}`, http.StatusBadRequest)
			return
		}
		if !checkDicOrWebPath(j.Path) {
			http.Error(w, `{"status":"error","error":"词库路径不合法"}`, http.StatusBadRequest)
			return
		}
		// 网页词库（.wn）是 HTML，不参与 .n 的积木解析，返回空 IR
		if checkWebDicPath(j.Path) {
			jsonResp, _ := json.Marshal(map[string]any{"ir": build.IR{}})
			w.Write(jsonResp)
			return
		}
		content, err := utils.NewFileQueue(j.Path).ReadFromFile()
		if err != nil {
			if !os.IsNotExist(err) {
				http.Error(w, `{"status":"error","error":"词库读取失败: `+err.Error()+`"}`, http.StatusBadRequest)
				return
			}
			content = ""
		}
		jsonResp, _ := json.Marshal(map[string]any{"ir": build.NebulaToIR(content)})
		w.Write(jsonResp)
		return

	case "gen_dic_from_blocks":
		// 接收 Blockly 工作区 JSON（积木 ↔ IR 的映射也在后端做），统一转成 .n 源码返回。
		// 积木编程页保存时用：前端只负责渲染与序列化工作区，格式与语义由后端保证。
		ir, err := build.BlocksToIR(h.Data)
		if err != nil {
			http.Error(w, `{"status":"error","error":"invalid workspace json"}`, http.StatusBadRequest)
			return
		}
		// 与保存/格式化（dic_save_content、dic_format）共用 FormatDic，
		// 避免「积木生成」与「保存」两套缩进约定不一致，导致编辑器内容在保存时被拉平跳变。
		jsonResp, _ := json.Marshal(map[string]any{"code": build.FormatDic(build.IRToNebula(ir))})
		w.Write(jsonResp)
		return

	case "gen_blocks_from_dic":
		// 读取磁盘上的词库文件，后端直接解析为 Blockly 工作区载入状态返回。
		// 积木编程页打开已有 .n 文件时用：前端把 data.blocks 直接交给 workspaces.load。
		var j struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if j.Path == "" {
			http.Error(w, `{"status":"error","error":"词库路径不能为空"}`, http.StatusBadRequest)
			return
		}
		if !checkDicOrWebPath(j.Path) {
			http.Error(w, `{"status":"error","error":"词库路径不合法"}`, http.StatusBadRequest)
			return
		}
		// 网页词库（.wn）是 HTML，不参与 .n 的积木解析，返回空工作区
		if checkWebDicPath(j.Path) {
			jsonResp, _ := json.Marshal(map[string]any{"blocks": build.IRToBlocks(build.IR{})})
			w.Write(jsonResp)
			return
		}
		content, err := utils.NewFileQueue(j.Path).ReadFromFile()
		if err != nil {
			if !os.IsNotExist(err) {
				http.Error(w, `{"status":"error","error":"词库读取失败: `+err.Error()+`"}`, http.StatusBadRequest)
				return
			}
			content = ""
		}
		jsonResp, _ := json.Marshal(map[string]any{"blocks": build.IRToBlocks(build.NebulaToIR(content))})
		w.Write(jsonResp)
		return

	case "gen_blocks_from_code":
		// 接收编辑器里的 .n 代码文本，后端直接解析为 Blockly 工作区载入状态返回。
		// 积木编程页「应用到积木」用：编辑器内容可能尚未落盘，不能复用 gen_blocks_from_dic。
		var j struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		jsonResp, _ := json.Marshal(map[string]any{"blocks": build.IRToBlocks(build.NebulaToIR(j.Content))})
		w.Write(jsonResp)
		return

	}
}
