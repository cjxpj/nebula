package dic_server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// aiDicToolsPrompt 启用工具能力时追加的系统提示词。
const aiDicToolsPrompt = `
【工具能力】
你可以直接调用工具读写本应用目录下的文件（含 .n 词库），不再只是给建议：
1. 修改文件前先读取最新内容；.n 词库请用专用工具（read_dic / save_dic / check_dic / run_dic），保存后工具会返回编译诊断，存在 error 级诊断时应先修复再保存；
2. 用户要求「修改 / 新增词库内容」时必须真正调用写入工具完成修改，不要只输出代码让用户手动粘贴，也不要先反问「是否需要我帮你修改」——直接执行，完成后汇报结果；
3. 工具调用必须走函数调用通道（tool_calls），禁止把调用写成 <tool_call>工具名<arg_key>参数名</arg_key><arg_value>参数值</arg_value></tool_call> 这类文本混进正文（那样不会真正执行），也不要只回复「已保存 / 已重写」却不调用工具；若本轮提示「工具调用不可用」，说明当前模型或接口不支持 function calling，此时只能给出代码与建议，并明确告知用户「未能直接写入文件」，不得谎称已修改；
4. 修改完成后说明改了哪个文件，并附上修改后的完整内容（Markdown 代码围栏）；前端编辑器会自动同步你写入的内容，无需提醒用户重新打开，仅当编辑器有未保存修改时才说明本次改动尚未同步；
5. 路径一律用相对应用目录的相对路径，词库文件以 .n 结尾，不得访问应用目录之外的路径；工具返回的原始 JSON 无需复述，只汇报结论与必要改动。`

// ============== AI 工具调用（词库调试 AI 的文件/词库能力） ==============
//
// 词库调试的 AI 对话不再只是「聊天 + 给建议」，而是通过 OpenAI function calling 直接读写
// 应用目录下的文件（含 .n 词库），形成「读取 → 修改 → 校验 → 汇报」的闭环。
// 复用 OPUI 文件管理既有的路径校验（checkFilePath / checkDicPath）与根目录（opuiAppDir），
// 所有工具只能在应用目录内操作，禁止绝对路径与 .. 越权。

const (
	// aiToolMaxRounds 单轮对话内最多允许的工具调用轮数，防止模型陷入无限循环
	aiToolMaxRounds = 8
	// aiToolTotalBudget 单轮对话内工具调用的总时间预算，超出后不再允许调用工具，强制模型基于已有信息收尾
	aiToolTotalBudget = 180 * time.Second
	// aiToolReadMaxRunes 工具单次返回的文本最大字符数（超出截断，避免撑爆模型上下文）
	aiToolReadMaxRunes = 60000
	// aiToolOutputMaxRunes 词库运行输出回灌给模型时的最大字符数
	aiToolOutputMaxRunes = 20000
)

// aiNotifyFileChanged 通知前端：AI 已改动磁盘上的文件（写入 / 保存 / 删除 / 重命名 / 移动）。
// 前端据此刷新编辑器内容、同步标签路径与文件树，避免编辑器停留在旧内容、随后又被自动保存覆盖。
func aiNotifyFileChanged(action, path string, extra map[string]any) {
	data := map[string]any{"action": action, "path": path}
	for k, v := range extra {
		data[k] = v
	}
	msg, err := json.Marshal(map[string]any{"type": "ai_file_changed", "data": data})
	if err != nil {
		return
	}
	broadcastOpuiNotify(msg)
}

// aiToolFileEntry 工具返回的文件/目录条目。
type aiToolFileEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Dir  bool   `json:"dir"`
	Size int64  `json:"size,omitempty"`
}

// aiTool 构造一个 OpenAI function calling 工具定义。
func aiTool(name, desc string, props map[string]any, required ...string) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        name,
			"description": desc,
			"parameters": map[string]any{
				"type":       "object",
				"properties": props,
				"required":   required,
			},
		},
	}
}

// aiToolDefinitions 返回词库调试 AI 可用的全部工具。
func aiToolDefinitions() []map[string]any {
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	boolean := func(desc string) map[string]any { return map[string]any{"type": "boolean", "description": desc} }
	num := func(desc string) map[string]any { return map[string]any{"type": "integer", "description": desc} }
	strList := func(desc string) map[string]any {
		return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": desc}
	}

	return []map[string]any{
		aiTool("list_files", "列出应用目录下某个目录的直接子项（文件夹与文件）。path 留空表示应用目录根。",
			map[string]any{"path": str("目录路径（相对应用目录，如 public、private/api）。留空表示应用目录根。")}),
		aiTool("search_files", "按名称搜索应用目录下的文件或文件夹。",
			map[string]any{
				"path":    str("搜索起始目录（相对应用目录）。留空表示应用目录根。"),
				"keyword": str("名称关键字；留空表示匹配全部。"),
				"deep":    boolean("是否递归搜索子目录，默认 false 只搜当前目录。"),
			}, "keyword"),
		aiTool("read_file", "读取应用目录下的文本文件内容。词库（.n）请改用 read_dic 以同时获得编译诊断。",
			map[string]any{"path": str("文件路径（相对应用目录）")}, "path"),
		aiTool("write_file", "创建或覆盖应用目录下的文本文件（自动创建上级目录）。词库（.n）请改用 save_dic。",
			map[string]any{
				"path":    str("文件路径（相对应用目录）"),
				"content": str("要写入的完整文件内容"),
			}, "path", "content"),
		aiTool("delete_file", "删除应用目录下的文件或文件夹（文件夹递归删除），不可恢复，调用前务必确认。",
			map[string]any{"path": str("要删除的路径（相对应用目录）")}, "path"),
		aiTool("rename_file", "重命名应用目录下的文件或文件夹（仅改同目录下的名称）。",
			map[string]any{
				"path":     str("原路径（相对应用目录）"),
				"new_name": str("新名称（不含路径分隔符）"),
			}, "path", "new_name"),
		aiTool("move_file", "把一个或多个文件/文件夹移动到目标目录。",
			map[string]any{
				"paths":  strList("要移动的路径列表（相对应用目录）"),
				"target": str("目标目录（相对应用目录），留空表示应用目录根"),
			}, "paths"),
		aiTool("read_dic", "读取 .n 词库文件的完整代码，并返回编译诊断（error/warning），修改词库前先用它查看最新内容。",
			map[string]any{"path": str("词库路径（相对应用目录，.n 结尾）")}, "path"),
		aiTool("save_dic", "保存 .n 词库文件（覆盖写入）并返回编译诊断。保存前会做语法规范校验（如把 # 当注释、块结构内空行），不合格会拒绝写入并返回问题列表，需修正后重新保存；若返回 error 级诊断，应继续修复后再次保存。",
			map[string]any{
				"path":    str("词库路径（相对应用目录，.n 结尾）"),
				"content": str("要保存的完整词库代码"),
			}, "path", "content"),
		aiTool("check_dic", "编译检查 .n 词库并返回诊断（error/warning），不修改文件。",
			map[string]any{"path": str("词库路径（相对应用目录，.n 结尾）")}, "path"),
		aiTool("run_dic", "运行 .n 词库并返回输出与运行诊断，用于验证修改效果。存在 error 级编译诊断时会拒绝运行。",
			map[string]any{
				"path":    str("词库路径（相对应用目录，.n 结尾）"),
				"trigger": str("触发词，默认 Main"),
				"timeout": num("运行超时秒数，默认 15，最大 60"),
			}, "path"),
	}
}

// aiToolResult 把工具结果序列化为回灌给模型的文本。
func aiToolResult(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"error":"工具结果序列化失败"}`
	}
	return string(b)
}

// aiToolFail 构造一条工具失败结果。
func aiToolFail(msg string) string {
	return aiToolResult(map[string]any{"error": msg})
}

// aiToolDecode 解析工具调用参数（空参数视为零值）。
func aiToolDecode(argsJSON string, dst any) error {
	if strings.TrimSpace(argsJSON) == "" {
		return nil
	}
	return json.Unmarshal([]byte(argsJSON), dst)
}

// aiDicWarnings 编译词库并返回诊断列表（error/warning），失败时以 error 诊断形式返回原因。
func aiDicWarnings(path string) []dto.BuildWarning {
	d, err := dic_dto.RunDicNoCache(path)
	if err != nil {
		return []dto.BuildWarning{{Level: "error", Text: "词库编译失败: " + err.Error()}}
	}
	if d == nil {
		return nil
	}
	defer d.Close()
	return d.Data.Warnings
}

// aiToolExecute 执行一次工具调用，返回回灌给模型的文本结果与展示在思考区的简要说明。
// streamID 为本轮 AI 流式请求 id，运行词库时用于把执行结果推送到前端「运行结果」面板。
func aiToolExecute(name, argsJSON, streamID string) (result string, brief string) {
	switch name {
	case "list_files":
		return aiToolListFiles(argsJSON)
	case "search_files":
		return aiToolSearchFiles(argsJSON)
	case "read_file":
		return aiToolReadFile(argsJSON)
	case "write_file":
		return aiToolWriteFile(argsJSON)
	case "delete_file":
		return aiToolDeleteFile(argsJSON)
	case "rename_file":
		return aiToolRenameFile(argsJSON)
	case "move_file":
		return aiToolMoveFile(argsJSON)
	case "read_dic":
		return aiToolReadDic(argsJSON)
	case "save_dic":
		return aiToolSaveDic(argsJSON)
	case "check_dic":
		return aiToolCheckDic(argsJSON)
	case "run_dic":
		return aiToolRunDic(argsJSON, streamID)
	default:
		return aiToolFail("未知工具: " + name), "未知工具 " + name
	}
}

// aiToolListFiles 列出目录直接子项。
func aiToolListFiles(argsJSON string) (string, string) {
	var a struct {
		Path string `json:"path"`
	}
	if err := aiToolDecode(argsJSON, &a); err != nil {
		return aiToolFail("参数解析失败: " + err.Error()), "参数错误"
	}
	dir := strings.TrimSpace(a.Path)
	root := opuiAppDir()
	if dir != "" {
		if !checkFilePath(dir) {
			return aiToolFail("目录路径不合法，只能使用应用目录内的相对路径"), "路径不合法"
		}
		root = filepath.Join(opuiAppDir(), filepath.FromSlash(dir))
	}
	items, err := os.ReadDir(root)
	if err != nil {
		return aiToolFail("读取目录失败: " + err.Error()), "读取目录失败"
	}
	entries := make([]aiToolFileEntry, 0, len(items))
	for _, it := range items {
		info, ierr := it.Info()
		if ierr != nil {
			continue
		}
		entries = append(entries, aiToolFileEntry{
			Name: it.Name(),
			Path: filepath.ToSlash(filepath.Join(dir, it.Name())),
			Dir:  it.IsDir(),
			Size: info.Size(),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Dir != entries[j].Dir {
			return entries[i].Dir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	show := dir
	if show == "" {
		show = "（应用目录根）"
	}
	return aiToolResult(map[string]any{"path": dir, "entries": entries}),
		fmt.Sprintf("列出 %s，共 %d 项", show, len(entries))
}

// aiToolSearchFiles 按名称搜索文件/目录。
func aiToolSearchFiles(argsJSON string) (string, string) {
	var a struct {
		Path    string `json:"path"`
		Keyword string `json:"keyword"`
		Deep    bool   `json:"deep"`
	}
	if err := aiToolDecode(argsJSON, &a); err != nil {
		return aiToolFail("参数解析失败: " + err.Error()), "参数错误"
	}
	dir := strings.TrimSpace(a.Path)
	root := opuiAppDir()
	if dir != "" {
		if !checkFilePath(dir) {
			return aiToolFail("目录路径不合法，只能使用应用目录内的相对路径"), "路径不合法"
		}
		root = filepath.Join(opuiAppDir(), filepath.FromSlash(dir))
	}
	kw := strings.ToLower(strings.TrimSpace(a.Keyword))
	matchName := func(n string) bool {
		return kw == "" || strings.Contains(strings.ToLower(n), kw)
	}
	entries := make([]aiToolFileEntry, 0)
	if a.Deep {
		_ = filepath.Walk(root, func(full string, info os.FileInfo, err error) error {
			if err != nil || full == root {
				return nil
			}
			if !matchName(info.Name()) {
				return nil
			}
			rel, rerr := filepath.Rel(opuiAppDir(), full)
			if rerr != nil {
				return nil
			}
			entries = append(entries, aiToolFileEntry{
				Name: info.Name(),
				Path: filepath.ToSlash(rel),
				Dir:  info.IsDir(),
				Size: info.Size(),
			})
			return nil
		})
	} else {
		items, err := os.ReadDir(root)
		if err != nil {
			return aiToolFail("读取目录失败: " + err.Error()), "读取目录失败"
		}
		for _, it := range items {
			if !matchName(it.Name()) {
				continue
			}
			info, ierr := it.Info()
			if ierr != nil {
				continue
			}
			entries = append(entries, aiToolFileEntry{
				Name: it.Name(),
				Path: filepath.ToSlash(filepath.Join(dir, it.Name())),
				Dir:  it.IsDir(),
				Size: info.Size(),
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Dir != entries[j].Dir {
			return entries[i].Dir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	return aiToolResult(map[string]any{"keyword": a.Keyword, "entries": entries}),
		fmt.Sprintf("搜索「%s」命中 %d 项", a.Keyword, len(entries))
}

// aiToolReadFile 读取文本文件内容。
func aiToolReadFile(argsJSON string) (string, string) {
	var a struct {
		Path string `json:"path"`
	}
	if err := aiToolDecode(argsJSON, &a); err != nil {
		return aiToolFail("参数解析失败: " + err.Error()), "参数错误"
	}
	p := strings.TrimSpace(a.Path)
	if !checkFilePath(p) {
		return aiToolFail("文件路径不合法，只能使用应用目录内的相对路径"), "路径不合法"
	}
	data, err := os.ReadFile(filepath.Join(opuiAppDir(), filepath.FromSlash(p)))
	if err != nil {
		return aiToolFail("文件读取失败: " + err.Error()), "读取失败 " + p
	}
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		return aiToolResult(map[string]any{"path": p, "size": len(data), "binary": true,
			"note": "这是二进制文件，无法作为文本读取"}), "读取二进制文件 " + p
	}
	content := string(data)
	truncated := false
	if len([]rune(content)) > aiToolReadMaxRunes {
		content = aiClipRunes(content, aiToolReadMaxRunes)
		truncated = true
	}
	return aiToolResult(map[string]any{"path": p, "size": len(data), "content": content, "truncated": truncated}),
		fmt.Sprintf("读取 %s（%d 字节）", p, len(data))
}

// aiToolWriteFile 创建或覆盖文本文件。
func aiToolWriteFile(argsJSON string) (string, string) {
	var a struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := aiToolDecode(argsJSON, &a); err != nil {
		return aiToolFail("参数解析失败: " + err.Error()), "参数错误"
	}
	p := strings.TrimSpace(a.Path)
	if !checkFilePath(p) {
		return aiToolFail("文件路径不合法，只能使用应用目录内的相对路径"), "路径不合法"
	}
	full := filepath.Join(opuiAppDir(), filepath.FromSlash(p))
	_, statErr := os.Stat(full)
	created := os.IsNotExist(statErr)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return aiToolFail("创建目录失败: " + err.Error()), "写入失败 " + p
	}
	if err := os.WriteFile(full, []byte(a.Content), 0o644); err != nil {
		return aiToolFail("文件写入失败: " + err.Error()), "写入失败 " + p
	}
	action := "已写入"
	if created {
		action = "已创建"
	}
	aiNotifyFileChanged("write", p, map[string]any{"created": created})
	return aiToolResult(map[string]any{"status": "ok", "path": p, "created": created, "bytes": len(a.Content)}),
		fmt.Sprintf("%s %s（%d 字节）", action, p, len(a.Content))
}

// aiToolDeleteFile 删除文件或目录。
func aiToolDeleteFile(argsJSON string) (string, string) {
	var a struct {
		Path string `json:"path"`
	}
	if err := aiToolDecode(argsJSON, &a); err != nil {
		return aiToolFail("参数解析失败: " + err.Error()), "参数错误"
	}
	p := strings.TrimSpace(a.Path)
	if !checkFilePath(p) {
		return aiToolFail("路径不合法，只能使用应用目录内的相对路径"), "路径不合法"
	}
	if err := os.RemoveAll(filepath.Join(opuiAppDir(), filepath.FromSlash(p))); err != nil {
		return aiToolFail("删除失败: " + err.Error()), "删除失败 " + p
	}
	aiNotifyFileChanged("delete", p, nil)
	return aiToolResult(map[string]any{"status": "ok", "path": p}), "已删除 " + p
}

// aiToolRenameFile 重命名文件或目录。
func aiToolRenameFile(argsJSON string) (string, string) {
	var a struct {
		Path    string `json:"path"`
		NewName string `json:"new_name"`
	}
	if err := aiToolDecode(argsJSON, &a); err != nil {
		return aiToolFail("参数解析失败: " + err.Error()), "参数错误"
	}
	p := strings.TrimSpace(a.Path)
	if !checkFilePath(p) {
		return aiToolFail("路径不合法，只能使用应用目录内的相对路径"), "路径不合法"
	}
	newName := strings.TrimSpace(a.NewName)
	if newName == "" || newName == "." || newName == ".." || strings.ContainsAny(newName, `/\`) {
		return aiToolFail("新名称不合法"), "重命名失败 " + p
	}
	oldFull := filepath.Join(opuiAppDir(), filepath.FromSlash(p))
	if _, err := os.Stat(oldFull); err != nil {
		return aiToolFail("原文件不存在"), "重命名失败 " + p
	}
	newFull := filepath.Join(filepath.Dir(oldFull), newName)
	if _, err := os.Stat(newFull); err == nil {
		return aiToolFail("已存在同名项"), "重命名失败 " + p
	}
	if err := os.Rename(oldFull, newFull); err != nil {
		return aiToolFail("重命名失败: " + err.Error()), "重命名失败 " + p
	}
	newPath := filepath.ToSlash(filepath.Join(filepath.Dir(filepath.FromSlash(p)), newName))
	aiNotifyFileChanged("rename", p, map[string]any{"new_path": newPath})
	return aiToolResult(map[string]any{"status": "ok", "path": newPath}), p + " 已重命名为 " + newName
}

// aiToolMoveFile 移动一个或多个文件/目录到目标目录。
func aiToolMoveFile(argsJSON string) (string, string) {
	var a struct {
		Paths  []string `json:"paths"`
		Target string   `json:"target"`
	}
	if err := aiToolDecode(argsJSON, &a); err != nil {
		return aiToolFail("参数解析失败: " + err.Error()), "参数错误"
	}
	if len(a.Paths) == 0 {
		return aiToolFail("未指定要移动的路径"), "移动失败"
	}
	for _, p := range a.Paths {
		if !checkFilePath(strings.TrimSpace(p)) {
			return aiToolFail("源路径不合法: " + p), "路径不合法"
		}
	}
	target := strings.TrimSpace(a.Target)
	if target != "" && !checkFilePath(target) {
		return aiToolFail("目标路径不合法"), "路径不合法"
	}
	appDir := opuiAppDir()
	targetDir := filepath.Join(appDir, filepath.FromSlash(target))
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return aiToolFail("创建目标目录失败: " + err.Error()), "移动失败"
	}
	for _, p := range a.Paths {
		src := filepath.Join(appDir, filepath.FromSlash(strings.TrimSpace(p)))
		base := filepath.Base(src)
		dst := filepath.Join(targetDir, base)
		if filepath.Clean(src) == filepath.Clean(dst) {
			continue
		}
		if _, err := os.Stat(dst); err == nil {
			return aiToolFail("目标已存在同名项: " + base), "移动失败"
		}
		if err := os.Rename(src, dst); err != nil {
			return aiToolFail("移动失败: " + err.Error()), "移动失败"
		}
		aiNotifyFileChanged("move", strings.TrimSpace(p), map[string]any{
			"new_path": filepath.ToSlash(filepath.Join(target, base)),
		})
	}
	return aiToolResult(map[string]any{"status": "ok", "paths": a.Paths, "target": target}),
		fmt.Sprintf("已移动 %d 项到 %s", len(a.Paths), target)
}

// aiToolReadDic 读取词库代码并返回编译诊断。
func aiToolReadDic(argsJSON string) (string, string) {
	var a struct {
		Path string `json:"path"`
	}
	if err := aiToolDecode(argsJSON, &a); err != nil {
		return aiToolFail("参数解析失败: " + err.Error()), "参数错误"
	}
	p := strings.TrimSpace(a.Path)
	if !checkDicPath(p) {
		return aiToolFail("词库路径不合法，需为应用目录内的相对路径且以 .n 结尾"), "路径不合法"
	}
	content, err := utils.NewFileQueue(p).ReadFromFile()
	if err != nil {
		if os.IsNotExist(err) && filepath.ToSlash(filepath.Clean(p)) == defaultDebugDic() {
			utils.NewFileQueue(p).WriteToFile("")
			content = ""
		} else if os.IsNotExist(err) {
			return aiToolFail("词库文件不存在: " + p), "词库不存在 " + p
		} else {
			return aiToolFail("词库读取失败: " + err.Error()), "读取失败 " + p
		}
	}
	warnings := aiDicWarnings(p)
	text := content
	if len([]rune(text)) > aiToolReadMaxRunes {
		text = aiClipRunes(text, aiToolReadMaxRunes)
	}
	return aiToolResult(map[string]any{"path": p, "content": text, "warnings": warnings}),
		fmt.Sprintf("读取词库 %s（%d 字符，%d 条编译诊断）", p, len([]rune(content)), len(warnings))
}

// aiDicLintContent 保存词库前的静态规范检查。
// 只拦截「编译器不报错、却会让代码静默失效」的写法，正常的词条分隔空行不在此列：
//  1. 行首拿 # 当注释：# 只属于 #引入= / #: / #{ 等指令，注释必须写 // 或 /* */；
//  2. 块结构（如果>/循环>/匹配>/遍历>/文本>/函数> 等）内部出现空行：空行会把块从中截断。
// 返回逐条问题（含行号），供回灌给模型自我修正后再次保存。
// 若块开启/闭合数量不匹配（编译本身就会报错），跳过块内空行检查，避免层级误判刷屏。
func aiDicLintContent(content string) []string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	problems := make([]string, 0)

	// 第一遍：确认块层级整体平衡
	depth := 0
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if aiDicBlockClose(line) {
			if depth > 0 {
				depth--
			}
			continue
		}
		if aiDicBlockOpen(line) {
			depth++
		}
	}
	balanced := depth == 0

	// 第二遍：逐行检查
	depth = 0
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		lineNo := i + 1
		if line == "" {
			if balanced && depth > 0 {
				problems = append(problems, fmt.Sprintf(
					"第 %d 行：块结构内部出现空行，空行会把块从中截断导致内部代码不执行，请删除该空行", lineNo))
			}
			continue
		}
		if strings.HasPrefix(line, "#") &&
			!strings.HasPrefix(line, "#引入=") &&
			!strings.HasPrefix(line, "#:") &&
			!strings.HasPrefix(line, "#{") {
			problems = append(problems, fmt.Sprintf(
				"第 %d 行：把 # 当成了注释（%s）；本语言没有 # 注释，分段标题请改用 //", lineNo, aiClipRunes(line, 40)))
		}
		if aiDicBlockClose(line) {
			if depth > 0 {
				depth--
			}
			continue
		}
		if aiDicBlockOpen(line) {
			depth++
		}
	}
	return problems
}

// aiDicBlockOpen 判断该行是否为块结构的开启行。
// 判断框、匹配框内的分支行（>否则如果:、>否则、如果是:、如果不是）属于同层，不计入层级。
func aiDicBlockOpen(line string) bool {
	if strings.Contains(line, ":函数>") {
		return true
	}
	for _, p := range []string{"如果>", "循环>", "判断循环>", "匹配>", "遍历>", "文本>", "JSON>", "函数>", "#:执行函数>", "#:>>>"} {
		if strings.HasPrefix(line, p) {
			return true
		}
	}
	return false
}

// aiDicBlockClose 判断该行是否为块结构的闭合行（结尾不带 >）。
func aiDicBlockClose(line string) bool {
	for _, p := range []string{"<如果", "<循环", "<匹配", "<遍历", "<文本", "<JSON", "<函数"} {
		if strings.HasPrefix(line, p) {
			return true
		}
	}
	return false
}

// aiToolSaveDic 保存词库并返回编译诊断。保存前先做语法规范检查，不合格则拒绝写入。
func aiToolSaveDic(argsJSON string) (string, string) {
	var a struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := aiToolDecode(argsJSON, &a); err != nil {
		return aiToolFail("参数解析失败: " + err.Error()), "参数错误"
	}
	p := strings.TrimSpace(a.Path)
	if !checkDicPath(p) {
		return aiToolFail("词库路径不合法，需为应用目录内的相对路径且以 .n 结尾"), "路径不合法"
	}
	// 保存前强校验：拦下编译查不出、却会让代码静默失效的写法，把问题回灌给模型让它改对再存
	if problems := aiDicLintContent(a.Content); len(problems) > 0 {
		return aiToolResult(map[string]any{
			"status": "rejected",
			"path":   p,
			"error": "词库内容不符合语法规范，已拒绝写入（磁盘文件未改动）。" +
				"请逐条修正下列问题后，用修正后的完整内容再次调用 save_dic：\n" + strings.Join(problems, "\n"),
		}), fmt.Sprintf("词库内容不规范，已拒绝保存（%d 处问题）", len(problems))
	}
	utils.NewFileQueue(p).WriteToFile(a.Content)
	warnings := aiDicWarnings(p)
	aiNotifyFileChanged("save", p, nil)
	return aiToolResult(map[string]any{"status": "ok", "path": p, "warnings": warnings}),
		fmt.Sprintf("已保存词库 %s（%d 条编译诊断）", p, len(warnings))
}

// aiToolCheckDic 仅编译检查词库。
func aiToolCheckDic(argsJSON string) (string, string) {
	var a struct {
		Path string `json:"path"`
	}
	if err := aiToolDecode(argsJSON, &a); err != nil {
		return aiToolFail("参数解析失败: " + err.Error()), "参数错误"
	}
	p := strings.TrimSpace(a.Path)
	if !checkDicPath(p) {
		return aiToolFail("词库路径不合法，需为应用目录内的相对路径且以 .n 结尾"), "路径不合法"
	}
	if _, err := os.Stat(filepath.Join(opuiAppDir(), filepath.FromSlash(p))); err != nil {
		return aiToolFail("词库文件不存在: " + p), "词库不存在 " + p
	}
	warnings := aiDicWarnings(p)
	errCount := 0
	for _, w := range warnings {
		if w.Level == "error" {
			errCount++
		}
	}
	return aiToolResult(map[string]any{"path": p, "warnings": warnings, "errorCount": errCount}),
		fmt.Sprintf("编译检查 %s：%d 个错误，%d 条诊断", p, errCount, len(warnings))
}

// aiToolRunDic 运行词库并返回输出。
// 运行结果（输出分段 / 变量 / 警告）经 WS 推送到前端「运行结果」面板，
// 与手动运行一致，避免 AI 调用运行时面板没有任何执行信息。
func aiToolRunDic(argsJSON, streamID string) (string, string) {
	var a struct {
		Path    string `json:"path"`
		Trigger string `json:"trigger"`
		Timeout int    `json:"timeout"`
	}
	if err := aiToolDecode(argsJSON, &a); err != nil {
		return aiToolFail("参数解析失败: " + err.Error()), "参数错误"
	}
	p := strings.TrimSpace(a.Path)
	if !checkDicPath(p) {
		return aiToolFail("词库路径不合法，需为应用目录内的相对路径且以 .n 结尾"), "路径不合法"
	}
	dic, err := dic_dto.RunDic(p)
	if err != nil {
		return aiToolFail("词库加载失败: " + err.Error()), "运行失败 " + p
	}
	defer dic.Close()

	// 与词库调试运行一致：运行期间启用删除操作人工确认
	ensureDicDeleteConfirm()
	dicDeleteConfirmActive.Add(1)
	defer dicDeleteConfirmActive.Add(-1)

	for _, warn := range dic.Data.Warnings {
		if warn.Level == "error" {
			// 编译错误同样推送到前端面板，让用户直接看到未运行的原因
			aiStreamNotify(streamID, "ai_stream_dic_run", map[string]any{
				"path":         p,
				"compileError": "编译存在错误，无法运行",
				"warnings":     dic.Data.Warnings,
			})
			return aiToolResult(map[string]any{
				"compileError": "编译存在错误，无法运行",
				"warnings":     dic.Data.Warnings,
			}), "词库编译存在错误，未运行"
		}
	}

	timeout := a.Timeout
	if timeout <= 0 {
		timeout = 15
	}
	if timeout > 60 {
		timeout = 60
	}
	trigger := strings.TrimSpace(a.Trigger)
	if trigger == "" {
		trigger = "Main"
	}
	output, timedOut := dic_api.Api.DicRunScript(dic, trigger, time.Duration(timeout)*time.Second)

	// 执行结果推送到前端「运行结果」面板（含输出分段、变量快照、错误行与警告）
	panel := dicRunResultPayload(dic, output, timedOut)
	panel["path"] = p
	panel["trigger"] = trigger
	aiStreamNotify(streamID, "ai_stream_dic_run", panel)

	resp := map[string]any{
		"output":   aiClipRunes(output, aiToolOutputMaxRunes),
		"timedOut": timedOut,
	}
	if len(dic.Data.Warnings) > 0 {
		resp["warnings"] = dic.Data.Warnings
	}
	brief := "已运行 " + p
	if timedOut {
		brief += "（超时）"
	}
	return aiToolResult(resp), brief
}

// aiToolCallSeq 为「文本形式工具调用」补全的调用编号，保证后续 tool 结果能按 id 正确回填。
var aiToolCallSeq atomic.Uint64

// aiParseTextToolCalls 兜底解析「被当成正文输出」的工具调用。
// 部分服务商/模型在流式响应下不走原生 tool_calls 字段，而是把调用写成文本混进正文：
//   <tool_call>工具名<arg_key>参数名</arg_key><arg_value>参数值</arg_value>...</tool_call>
//   <tool_call>{"name":"工具名","arguments":{...}}</tool_call>
// 若不解析，模型就会「口头声称已按某语法重写」，实际从未调用 save_dic，文件根本没变。
// 返回解析出的调用列表与剔除这些片段后的正文。
func aiParseTextToolCalls(content string) ([]aiToolCall, string) {
	if !strings.Contains(content, "<tool_call>") {
		return nil, content
	}
	var calls []aiToolCall
	var rest strings.Builder
	for {
		start := strings.Index(content, "<tool_call>")
		if start < 0 {
			rest.WriteString(content)
			break
		}
		rest.WriteString(content[:start])
		body := content[start+len("<tool_call>"):]
		end := strings.Index(body, "</tool_call>")
		if end < 0 {
			// 未闭合：整体保留，避免误删正常正文
			rest.WriteString(content[start:])
			break
		}
		inner := body[:end]
		content = body[end+len("</tool_call>"):]
		if call, ok := aiParseTextToolCall(inner); ok {
			calls = append(calls, call)
		}
	}
	return calls, strings.TrimSpace(rest.String())
}

// aiParseTextToolCall 解析单个 <tool_call> 片段内部内容，兼容 JSON 与 arg_key/arg_value 两种写法。
func aiParseTextToolCall(inner string) (aiToolCall, bool) {
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return aiToolCall{}, false
	}
	// 形式一：JSON（{"name":..,"arguments":{..}} 或 {"function":{"name":..,"arguments":..}}）
	if strings.HasPrefix(inner, "{") {
		var raw struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
			Function  *struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"function"`
		}
		if json.Unmarshal([]byte(inner), &raw) == nil {
			name, args := raw.Name, raw.Arguments
			if raw.Function != nil {
				name, args = raw.Function.Name, raw.Function.Arguments
			}
			if name != "" {
				return aiNewTextToolCall(name, string(args)), true
			}
		}
	}
	// 形式二：工具名 + 重复的 <arg_key>键</arg_key><arg_value>值</arg_value>
	name := inner
	if i := strings.Index(inner, "<arg_key>"); i >= 0 {
		name = inner[:i]
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return aiToolCall{}, false
	}
	args := map[string]string{}
	remain := inner
	for {
		ks := strings.Index(remain, "<arg_key>")
		if ks < 0 {
			break
		}
		afterK := remain[ks+len("<arg_key>"):]
		ke := strings.Index(afterK, "</arg_key>")
		if ke < 0 {
			break
		}
		key := strings.TrimSpace(afterK[:ke])
		afterV := afterK[ke+len("</arg_key>"):]
		if !strings.HasPrefix(afterV, "<arg_value>") {
			break
		}
		afterV = afterV[len("<arg_value>"):]
		ve := strings.Index(afterV, "</arg_value>")
		if ve < 0 {
			break
		}
		args[key] = afterV[:ve]
		remain = afterV[ve+len("</arg_value>"):]
	}
	buf, err := json.Marshal(args)
	if err != nil {
		return aiToolCall{}, false
	}
	return aiNewTextToolCall(name, string(buf)), true
}

// aiNewTextToolCall 组装一个由文本形式工具调用还原出的结构化调用（补全 id 与 type）。
func aiNewTextToolCall(name, argsJSON string) aiToolCall {
	var call aiToolCall
	call.ID = fmt.Sprintf("call_text_%d", aiToolCallSeq.Add(1))
	call.Type = "function"
	call.Function.Name = name
	call.Function.Arguments = argsJSON
	return call
}

// aiChatWithTools 执行「带工具能力」的多轮对话：模型请求工具调用时本地执行并把结果回灌，
// 直至模型给出最终答复；达到轮数上限时追加一次不带工具的收尾请求强制模型基于已有信息作答，
// 不再以错误中止整轮回复。
// 工具执行进度与思维链经 WS 复用 ai_stream_delta（kind=reasoning）实时呈现，
// 并一并写入最终返回的思维链——前端在 ai_stream_end 时会用返回值覆盖思考区，
// 若只推送增量，工具调用进度会在收尾时被抹掉。
// 正文增量同样在流式解析时经 ai_stream_delta（kind=content）逐片推送，前端边生成边渲染。
// 首轮即失败（常见于服务商不支持 function calling）时显式提示后降级为无工具流式对话，
// 避免「静默降级」让用户误以为 AI 具备改写能力却始终没有动作。
func aiChatWithTools(c *dto.AIConfig, model string, msgs []aiChatMessage, effort, streamID, permissionMode, sessionID string) (string, string, error) {
	tools := aiToolDefinitions()
	deadline := time.Now().Add(aiToolTotalBudget)
	work := append([]aiChatMessage(nil), msgs...)
	var reasoningAll strings.Builder
	// 本流的取消上下文：用户点击「终止」时连审批等待一起打断
	parentCtx := aiStreamParentCtx(streamID)

	// appendReasoning 累积思考内容并实时推送，保证收尾时思考区仍保留完整过程
	appendReasoning := func(text string) {
		if text == "" {
			return
		}
		reasoningAll.WriteString(text)
		aiStreamNotify(streamID, "ai_stream_delta", map[string]any{"kind": "reasoning", "text": text})
	}

	// 首轮优先下发 tool_choice=auto；若服务商不接受该字段导致失败，去掉后重试一次
	toolChoice := "auto"
	// 前 aiToolMaxRounds 轮允许调用工具；若到上限模型仍未收尾，再追加一轮不带工具的请求，
	// 强制其基于已获得的信息给出最终答复——不能直接报错中止整轮回复，否则用户只能重试。
	for round := 0; round <= aiToolMaxRounds; round++ {
		final := round == aiToolMaxRounds
		// 收尾轮或超过总时间预算后不再下发工具，强制模型基于已有信息给出最终答复
		roundTools := tools
		if final || (round > 0 && time.Now().After(deadline)) {
			roundTools = nil
		}
		content, reasoning, calls, err := aiChatOnceTools(c, model, work, effort, roundTools, toolChoice, streamID)
		// 用户主动终止：保留本轮已流出的思考与正文，交由上层收尾
		if errors.Is(err, errAIStreamCancelled) {
			if r := strings.TrimSpace(reasoning); r != "" {
				reasoningAll.WriteString(r + "\n")
			}
			return content, reasoningAll.String(), err
		}
		if err != nil && round == 0 && toolChoice != "" {
			toolChoice = ""
			content, reasoning, calls, err = aiChatOnceTools(c, model, work, effort, roundTools, "", streamID)
			if errors.Is(err, errAIStreamCancelled) {
				if r := strings.TrimSpace(reasoning); r != "" {
					reasoningAll.WriteString(r + "\n")
				}
				return content, reasoningAll.String(), err
			}
		}
		if err != nil {
			if round > 0 {
				return "", "", err
			}
			// 模型或接口不支持工具调用：显式提示后降级为无工具流式对话
			appendReasoning("\n⚠ 工具调用不可用，已降级为纯对话，本轮 AI 无法直接读写文件。\n原因：" + err.Error() + "\n")
			text, streamReasoning, streamErr := aiChatStreamReasoning(c, model, msgs, effort, streamID)
			if streamErr != nil {
				if errors.Is(streamErr, errAIStreamCancelled) {
					return text, reasoningAll.String() + streamReasoning, streamErr
				}
				return "", "", streamErr
			}
			return text, reasoningAll.String() + streamReasoning, nil
		}
		// 思维链已由 aiChatOnceTools 在流式解析时逐字推送，此处仅累积，避免重复推送
		if r := strings.TrimSpace(reasoning); r != "" {
			reasoningAll.WriteString(r + "\n")
		}
		// 兜底：模型有时把工具调用写成 <tool_call> 文本混进正文、不走原生 tool_calls，
		// 不解析就会出现「模型声称已按正确语法重写，实际从未调用 save_dic、文件没变」。
		// 收尾轮不再解析执行，避免又开启新一轮工具调用，此时正文原样返回。
		if !final {
			if parsed, rest := aiParseTextToolCalls(content); len(parsed) > 0 {
				if len(calls) == 0 {
					calls = parsed
				}
				content = rest
			}
		}
		if len(calls) == 0 || final {
			if strings.TrimSpace(content) == "" {
				if !final {
					return "", "", errors.New("AI 未返回内容")
				}
				// 收尾轮仍无正文：给出可读提示，而不是报错中止
				content = "（工具调用轮数已达上限，本次未能生成完整答复。可以回复「继续」让我接着处理。）"
			}
			// 正文已在 aiChatOnceTools 流式解析时逐片推送，此处不再重复推整段
			return content, reasoningAll.String(), nil
		}
		// 保留本轮工具请求，作为后续上下文；content 需原样回传以保持消息结构完整
		work = append(work, aiChatMessage{Role: "assistant", Content: content, ToolCalls: calls})
		for _, call := range calls {
			appendReasoning("\n▸ 调用工具 " + call.Function.Name + "\n")
			// 权限闸门：按任务级权限档位决定是否需用户审批；被拒绝时把结果回灌给模型，让其调整后再收尾
			if aiToolNeedsApproval(permissionMode, call.Function.Name) && !requestAIToolApproval(parentCtx, sessionID, call.Function.Name, call.Function.Arguments) {
				appendReasoning("  用户未批准该操作，已跳过\n")
				work = append(work, aiChatMessage{
					Role:       "tool",
					ToolCallID: call.ID,
					Name:       call.Function.Name,
					Content:    aiToolFail("用户未批准该工具调用，已跳过执行。请勿重复尝试，向用户说明情况或给出替代建议。"),
				})
				continue
			}
			result, brief := aiToolExecute(call.Function.Name, call.Function.Arguments, streamID)
			appendReasoning("  " + brief + "\n")
			work = append(work, aiChatMessage{Role: "tool", ToolCallID: call.ID, Name: call.Function.Name, Content: result})
		}
	}
	// 不可达：收尾轮（round == aiToolMaxRounds）必定在上方返回。
	// 保底交回已累积的思考，避免因意外路径丢掉整轮内容。
	return "", reasoningAll.String(), nil
}

// ============== AI 工具调用审批（盾牌权限闸门） ==============
//
// 任务级权限档位（盾牌菜单）：
//   manual 手动审批：所有工具调用都需用户在对话流内联卡片中确认
//   auto   自动审批：只读工具与词库写入（save_dic）自动放行，其余写入/删除类工具需确认
//   full   完全访问：全部工具自动放行
// 审批请求经 WS 推送 ai_tool_approval，前端在对话流内联卡片中答复后回传
// ai_tool_approval_result 唤醒此处阻塞等待的对话协程；与词库删除确认一致，超时按拒绝处理。

// aiApprovalTimeout 单次工具审批的最长等待时间，超时按拒绝处理，避免对话协程长期挂起。
const aiApprovalTimeout = 10 * time.Minute

// aiToolReadOnly 只读工具集合：不产生磁盘副作用，auto 档位下自动放行。
var aiToolReadOnly = map[string]bool{
	"list_files":   true,
	"search_files": true,
	"read_file":    true,
	"read_dic":     true,
	"check_dic":    true,
}

// aiToolAutoAllow auto（自动审批）档位下额外免审批的写入工具。
// save_dic 是词库调试的核心动作，用户已在对话里明确要求改词库；若继续弹卡片，
// 一旦用户没及时答复（切走 / 超时），模型就会退化成「让用户手动复制粘贴」，文件根本没写入。
// 其余写文件、删除、移动、重命名类工具在 auto 档下仍需确认。
var aiToolAutoAllow = map[string]bool{
	"save_dic": true,
}

// aiToolNeedsApproval 判断某次工具调用在当前权限档位下是否需要人工审批。
func aiToolNeedsApproval(permissionMode, toolName string) bool {
	switch aiNormalizePermissionMode(permissionMode) {
	case "full":
		return false
	case "auto":
		return !aiToolReadOnly[toolName] && !aiToolAutoAllow[toolName]
	default:
		return true
	}
}

// aiApprovalItem 一次正在等待用户批示的工具调用。
type aiApprovalItem struct {
	ID        string
	SessionID string
	Tool      string
	Args      string
	CreatedAt int64
	Ch        chan bool
}

var (
	aiApprovalSeq     atomic.Uint64
	aiApprovalMu      sync.Mutex
	aiApprovalPending = map[string]*aiApprovalItem{}
)

// aiApprovalPendingList 返回指定任务下仍在等待审批的工具调用，供前端刷新后恢复内联审批卡片。
func aiApprovalPendingList(sessionID string) []map[string]any {
	aiApprovalMu.Lock()
	defer aiApprovalMu.Unlock()
	list := make([]map[string]any, 0, len(aiApprovalPending))
	for _, it := range aiApprovalPending {
		if sessionID != "" && it.SessionID != sessionID {
			continue
		}
		list = append(list, map[string]any{
			"id":         it.ID,
			"session_id": it.SessionID,
			"tool":       it.Tool,
			"args":       it.Args,
			"time":       it.CreatedAt,
		})
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i]["time"].(int64) < list[j]["time"].(int64)
	})
	return list
}

// resolveAIToolApproval 唤醒等待中的审批请求（WS 回传入口）。
func resolveAIToolApproval(id string, allow bool) {
	aiApprovalMu.Lock()
	it := aiApprovalPending[id]
	aiApprovalMu.Unlock()
	if it == nil {
		return
	}
	select {
	case it.Ch <- allow:
	default:
	}
}

// requestAIToolApproval 推送审批请求并阻塞等待用户答复；超时未答复按拒绝处理。
// ctx 取消（用户点击「终止」生成）时立即返回 false，避免对话协程继续挂起。
func requestAIToolApproval(ctx context.Context, sessionID, toolName, argsJSON string) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	id := fmt.Sprintf("%d", aiApprovalSeq.Add(1))
	item := &aiApprovalItem{
		ID:        id,
		SessionID: sessionID,
		Tool:      toolName,
		Args:      argsJSON,
		CreatedAt: time.Now().Unix(),
		Ch:        make(chan bool, 1),
	}
	aiApprovalMu.Lock()
	aiApprovalPending[id] = item
	aiApprovalMu.Unlock()
	defer func() {
		aiApprovalMu.Lock()
		delete(aiApprovalPending, id)
		aiApprovalMu.Unlock()
	}()

	data, _ := json.Marshal(map[string]any{
		"type": "ai_tool_approval",
		"data": map[string]any{
			"id":         item.ID,
			"session_id": item.SessionID,
			"tool":       item.Tool,
			"args":       item.Args,
			"time":       item.CreatedAt,
		},
	})
	broadcastOpuiNotify(data)

	select {
	case ok := <-item.Ch:
		return ok
	case <-ctx.Done():
		return false
	case <-time.After(aiApprovalTimeout):
		return false
	}
}
