package dic_server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/cjxpj/nebula/appfiles"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// aiDicToolsPrompt 启用工具能力时追加的系统提示词。
const aiDicToolsPrompt = `
【工具能力】
你可以直接调用工具读写本应用目录下的文件（含 .n 词库），不再只是给建议：
1. 默认操作对象是「当前任务关联的词库文件」（下方会给出该路径，即用户此刻在编辑器中打开的文件）：需要阅读或修改词库时优先直接 read_dic / save_dic 这个文件，不要先用 list_files / search_files 满目录查找，也不要读取无关文件；仅当用户明确指向其他文件时才切换；
2. 修改文件前先读取最新内容；.n 词库请用专用工具（read_dic / save_dic / check_dic / run_dic）。save_dic 保存后会自动重新编译并返回诊断：errors 非空（error 级诊断）说明词库跑不起来，必须逐条修复并用完整内容再次调用 save_dic，直到 error 级清零再收尾，不要只在答复里罗列问题；warnings 属于可运行的警告，属本次改动引入的也应一并修掉，并在答复里汇报本轮保存后的诊断结果（有几个错误 / 几条警告、是否已清零）；
3. 用户要求「修改 / 新增词库内容」时必须真正调用写入工具完成修改，不要只输出代码让用户手动粘贴，也不要先反问「是否需要我帮你修改」——直接执行，完成后汇报结果；
4. 新增 / 插入词条时保证词条边界：触发词必须独占一行，且其上方留一个空行与头部或上一条词条分隔（空行是词条的硬边界，直接贴在上一条正文或头部后面会被并入上一条，导致该触发词完全失效）；头部与第一个词条之间同样要空行分隔。触发词的取舍：功能 / 娱乐类词库（用户以后会发消息反复唤起，如五子棋、签到）不要写 Main，按功能自行命名一个简短贴合的触发词；但「一次性脚本」——你写完马上就要用 run_dic 运行并回报结果（如「画个九宫格」、临时生成 / 计算）——触发词一律写 Main（run_dic 默认用 Main 触发，写 Main 才能直接跑通，别自创触发词导致默认触发跑不出来）；只有用户明确要求其他触发词时才另写；
5. 工具调用必须走函数调用通道（tool_calls），禁止把调用写成 <tool_call>工具名<arg_key>参数名</arg_key><arg_value>参数值</arg_value></tool_call> 这类文本混进正文（那样不会真正执行），也不要只回复「已保存 / 已重写」却不调用工具；若本轮提示「工具调用不可用」，说明当前模型或接口不支持 function calling，此时只能给出代码与建议，并明确告知用户「未能直接写入文件」，不得谎称已修改；
6. 遇到不熟悉的内置函数、对象方法或参数用法（如画布怎么导出图片），先用 read_dic_doc 读取内置语法文档 dic.md 求证，或直接向用户说明「不确定」并请其确认；严禁往 .n 词库文件里批量写入 u1/u2/xxx1/xxx2 之类「试探词条」去猜 API 名——这类写法既不生效，还会污染用户正在编辑的词库；
7. 修改完成后说明改了哪个文件，并附上修改后的完整内容（Markdown 代码围栏）；前端编辑器会自动同步你写入的内容，无需提醒用户重新打开，仅当编辑器有未保存修改时才说明本次改动尚未同步；
8. 路径一律用相对应用目录的相对路径，词库文件以 .n 结尾，不得访问应用目录之外的路径；工具返回的原始 JSON 无需复述，只汇报结论与必要改动。`

// ============== AI 工具调用（词库调试 AI 的文件/词库能力） ==============
//
// 词库调试的 AI 对话不再只是「聊天 + 给建议」，而是通过 OpenAI function calling 直接读写
// 应用目录下的文件（含 .n 词库），形成「读取 → 修改 → 校验 → 汇报」的闭环。
// 复用 OPUI 文件管理既有的路径校验（checkFilePath / checkDicPath）与根目录（opuiAppDir），
// 所有工具只能在应用目录内操作，禁止绝对路径与 .. 越权。

const (
	// aiToolMaxRounds 单轮对话内工具调用的硬上限，只作防死循环的安全网。
	// 正常收尾由 aiToolTotalBudget 时间预算驱动（见 aiChatWithTools 的 final 判定）：
	// 「读取 → 修改 → 编译 → 运行验证」这类任务本就需要多次往返，
	// 若轮数卡得过紧，会在任务中途就把工具撤掉，逼得模型只能把调用写成文本。
	aiToolMaxRounds = 30
	// aiToolTotalBudget 单轮对话内工具调用的总时间预算，超出后不再允许调用工具，强制模型基于已有信息收尾
	aiToolTotalBudget = 180 * time.Second
	// aiToolMaxNudges 模型只回「我这就去做 X」式计划前言、却没真正发起工具调用时，
	// 最多补几轮「请直接执行」的提醒。设上限避免与模型互相空转；用尽后按普通答复收尾。
	aiToolMaxNudges = 2
	// aiToolMaxContinues 上游因输出长度上限截断（finish_reason=length）时的最大自动续写轮数。
	// 截断会让用户看到「话说到一半突然没了」，把已生成部分回灌并请模型续写即可补全；
	// 设上限避免上游反复截断、模型反复续写导致空转。
	aiToolMaxContinues = 3
	// aiToolMaxEmptyRetries 上游偶发「只产出思考、正文为空」时的最大自动重试次数。
	// 此前这种情况直接以「AI 未返回内容」报错、整轮生成中断，用户只能手动重发；
	// 改为撤下工具后自动重试，逼迫模型用文字作答。
	aiToolMaxEmptyRetries = 2
	// aiToolEmptyRetryDelay 空回复重试前的等待时长（按重试次数递增），给上游一点恢复时间
	aiToolEmptyRetryDelay = 2 * time.Second
	// aiToolReadMaxRunes 工具单次返回的文本最大字符数（超出截断，避免撑爆模型上下文）
	aiToolReadMaxRunes = 60000
	// aiToolOutputMaxRunes 词库运行输出回灌给模型时的最大字符数
	aiToolOutputMaxRunes = 20000
)

// aiNotifyFileChanged 通知前端：AI 已改动磁盘上的文件（写入 / 保存 / 删除 / 重命名 / 移动）。
// 前端据此刷新编辑器内容、同步标签路径与文件树，避免编辑器停留在旧内容、随后又被自动保存覆盖。
func aiNotifyFileChanged(action, path string, extra map[string]any) {
	data := map[string]any{"action": action, "path": path}
	maps.Copy(data, extra)
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
	// 无参工具（如 read_dic_doc）也必须给出空对象属性表：
	// 序列化成 "properties":null 会被服务商按非法 JSON Schema 拒绝整份工具定义，
	// 进而导致首轮请求失败、降级为无工具对话（模型只能把调用写成正文，实际不执行）。
	if props == nil {
		props = map[string]any{}
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
		aiTool("read_dic_doc", "读取内置的 Nebula 词库语法文档（dic.md）全文，用于查阅词库语法、内置函数与对象实例（如画布）的准确用法与参数。遇到不确定的语法 / API 时优先调用它，不要靠猜、也不要往词库写试探词条。", nil),
		aiTool("read_skill", "读取指定技能的完整内容（步骤与约定）。每轮系统提示的「可用技能」清单只给出技能名称与描述；当任务与某个技能的描述相符时，先调用本工具读取该技能全文，再严格按其中的步骤执行。", map[string]any{"name": str("技能名称（与「可用技能」清单中的名称一致）")}, "name"),
		aiTool("save_dic", "保存 .n 词库文件（覆盖写入），保存后自动重新编译并返回编译报错（errors）与警告（warnings）。保存前会做语法规范校验（如把 # 当注释、块结构内空行），不合格会拒绝写入并返回问题列表，需修正后重新保存；返回的 errors 非空时必须继续修复后再次保存，直到无 error 级诊断。",
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

// aiToolExecute 执行一次工具调用，返回回灌给模型的文本结果、展示在思考区的简要说明，
// 以及本次调用捕获到的图片（多模态上传用；目前仅 run_dic 且视觉能力开启时非空）。
// streamID 为本轮 AI 流式请求 id，运行词库时用于把执行结果推送到前端「运行结果」面板。
// vision 为全局「AI 视觉能力」开关，run_dic 据此决定图片是上传给模型还是仅提示无法查看。
func aiToolExecute(name, argsJSON, streamID string, vision bool) (result string, brief string, images []string) {
	switch name {
	case "list_files":
		return aiToolPair(aiToolListFiles(argsJSON))
	case "search_files":
		return aiToolPair(aiToolSearchFiles(argsJSON))
	case "read_file":
		return aiToolPair(aiToolReadFile(argsJSON))
	case "write_file":
		return aiToolPair(aiToolWriteFile(argsJSON))
	case "delete_file":
		return aiToolPair(aiToolDeleteFile(argsJSON))
	case "rename_file":
		return aiToolPair(aiToolRenameFile(argsJSON))
	case "move_file":
		return aiToolPair(aiToolMoveFile(argsJSON))
	case "read_dic":
		return aiToolPair(aiToolReadDic(argsJSON))
	case "read_dic_doc":
		return aiToolPair(aiToolReadDicDoc())
	case "read_skill":
		return aiToolPair(aiToolReadSkill(argsJSON))
	case "save_dic":
		return aiToolPair(aiToolSaveDic(argsJSON))
	case "check_dic":
		return aiToolPair(aiToolCheckDic(argsJSON))
	case "run_dic":
		return aiToolRunDic(argsJSON, streamID, vision)
	default:
		return aiToolFail("未知工具: " + name), "未知工具 " + name, nil
	}
}

// aiToolPair 把「结果, 摘要」二元返回值补上空的图片列表，供 aiToolExecute 统一返回三元组。
func aiToolPair(result, brief string) (string, string, []string) {
	return result, brief, nil
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

// aiToolReadDicDoc 读取内置的 Nebula 词库语法文档（dic.md）。
// 该文档是语法、内置函数与对象实例方法的权威出处，供模型在不熟悉的 API 上求证，
// 替代「往词库写试探词条猜方法名」的破坏性做法。
func aiToolReadDicDoc() (string, string) {
	data, err := appfiles.GetFile("dic.md")
	if err != nil {
		return aiToolFail("内置语法文档不可用: " + err.Error()), "语法文档不可用"
	}
	content := string(data)
	truncated := false
	if len([]rune(content)) > aiToolReadMaxRunes {
		content = aiClipRunes(content, aiToolReadMaxRunes)
		truncated = true
	}
	return aiToolResult(map[string]any{"doc": "dic.md", "content": content, "truncated": truncated}),
		fmt.Sprintf("阅读语法文档 dic.md（%d 字符）", len([]rune(content)))
}

// aiToolReadSkill 按名称读取某个技能的完整内容（系统提示只注入技能名称与描述，正文按需读取）。
func aiToolReadSkill(argsJSON string) (string, string) {
	var a struct {
		Name string `json:"name"`
	}
	if err := aiToolDecode(argsJSON, &a); err != nil {
		return aiToolFail("参数解析失败: " + err.Error()), "参数错误"
	}
	name := strings.TrimSpace(a.Name)
	if name == "" {
		return aiToolFail("请提供技能名称"), "缺少技能名称"
	}
	skill, ok := aiSkillFindByName(name)
	if !ok {
		return aiToolFail("技能不存在：" + name + "（请使用「可用技能」清单中的名称）"), "技能不存在"
	}
	content := skill.Content
	truncated := false
	if len([]rune(content)) > aiToolReadMaxRunes {
		content = aiClipRunes(content, aiToolReadMaxRunes)
		truncated = true
	}
	return aiToolResult(map[string]any{"name": skill.Name, "content": content, "truncated": truncated}),
		fmt.Sprintf("读取技能「%s」（%d 字符）", skill.Name, len([]rune(content)))
}

// aiDicLintContent 保存词库前的静态规范检查。
// 只拦截「编译器不报错、却会让代码静默失效」的写法，正常的词条分隔空行不在此列：
//  1. 行首拿 # 当注释：# 只属于 #引入= / #: / #{ 等指令，注释必须写 // 或 /* */；
//  2. 块结构（如果>/循环>/匹配>/遍历>/文本>/函数> 等）内部出现空行：空行会把块从中截断。
//  3. 触发词（[函数] / [内部] 等）上方缺空行：会被并入头部或上一条词条，触发词失效。
//
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
		// 触发词上方没有空行：该行会被并入头部或上一条词条的正文，触发词实际失效。
		// 只认 [函数 / [内部 这类必然属于词条的写法，避免误判普通正文行。
		if (strings.HasPrefix(line, "[函数") || strings.HasPrefix(line, "[内部")) &&
			i > 0 && strings.TrimSpace(lines[i-1]) != "" {
			problems = append(problems, fmt.Sprintf(
				"第 %d 行：触发词（%s）上方缺少空行，已与头部或上一条词条粘连，会被并入上一条导致该触发词完全失效；请在它上方补一个空行", lineNo, aiClipRunes(line, 40)))
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

// aiToolSaveDic 保存词库并返回编译诊断。保存前先做语法规范检查，不合格则拒绝写入；
// 写入后自动重新编译，把诊断（error/warning）回灌给模型并推送给前端，
// 用户无需再手动运行一次才能看到警告/报错。
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

	// 保存后自动重新编译：诊断一并回灌给模型（据此继续修复）并推送给前端（据此高亮行号并提示用户）
	warnings := aiDicWarnings(p)
	errors := make([]dto.BuildWarning, 0, len(warnings))
	for _, w := range warnings {
		if w.Level == "error" {
			errors = append(errors, w)
		}
	}
	aiNotifyFileChanged("save", p, map[string]any{"warnings": warnings})

	resp := map[string]any{"status": "ok", "path": p, "warnings": warnings, "errorCount": len(errors)}
	brief := fmt.Sprintf("已保存词库 %s（无编译诊断）", p)
	switch {
	case len(errors) > 0:
		// error 级诊断意味着词库实际跑不起来：明确要求模型继续修复，而不是就此收尾
		resp["status"] = "saved_with_errors"
		resp["errors"] = errors
		resp["nextStep"] = fmt.Sprintf(
			"本次内容已写入磁盘，但编译存在 %d 个 error 级诊断（见 errors），词库无法正常运行。"+
				"必须逐条修复后用完整内容再次调用 save_dic 覆盖保存，直到 error 级诊断清零再收尾。"+
				"不要只在答复里说明问题而不修复。", len(errors))
		brief = fmt.Sprintf("已保存词库 %s（%d 个编译错误，%d 条警告）", p, len(errors), len(warnings)-len(errors))
	case len(warnings) > 0:
		resp["nextStep"] = fmt.Sprintf(
			"本次内容已写入磁盘，编译有 %d 条 warning 级诊断（见 warnings），词库可以运行；"+
				"若属于本次改动引入的问题，请修复后再次保存，并在答复里说明。", len(warnings))
		brief = fmt.Sprintf("已保存词库 %s（%d 条编译警告）", p, len(warnings))
	}
	return aiToolResult(resp), brief
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
// vision 为全局「AI 视觉能力」开关：开启时把输出中的图片作为多模态内容上传给模型；
// 未开启时图片无法上传，改为回灌提示文本并在面板给出黄色警告，告知模型与用户「看不见图片」。
func aiToolRunDic(argsJSON, streamID string, vision bool) (string, string, []string) {
	var a struct {
		Path    string `json:"path"`
		Trigger string `json:"trigger"`
		Timeout int    `json:"timeout"`
	}
	if err := aiToolDecode(argsJSON, &a); err != nil {
		return aiToolFail("参数解析失败: " + err.Error()), "参数错误", nil
	}
	p := strings.TrimSpace(a.Path)
	if !checkDicPath(p) {
		return aiToolFail("词库路径不合法，需为应用目录内的相对路径且以 .n 结尾"), "路径不合法", nil
	}
	dic, err := dic_dto.RunDic(p)
	if err != nil {
		return aiToolFail("词库加载失败: " + err.Error()), "运行失败 " + p, nil
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
			}), "词库编译存在错误，未运行", nil
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
	output, timedOut, fellBack := dic_api.Api.DicRunScript(dic, trigger, time.Duration(timeout)*time.Second)

	// 捕获本次运行输出中的图片（±img= 标记 / <img> / ![]() / 直接输出的图片二进制）
	images := outputImages(output)

	// 执行结果推送到前端「运行结果」面板（含输出分段、变量快照、错误行与警告）；
	// 兜底分支额外附一条黄色警告，提示触发词未命中、触发词行被当正文输出
	panel := dicRunResultPayload(dic, output, timedOut, fellBack, trigger)
	panel["path"] = p
	panel["trigger"] = trigger
	if len(images) > 0 && !vision {
		// 视觉能力未开启：图片不会上传给模型，在面板给出黄色警告，让用户知道模型看不见
		ws, _ := panel["warnings"].([]dto.BuildWarning)
		panel["warnings"] = append(append([]dto.BuildWarning{}, ws...), dto.BuildWarning{
			Line:  1,
			Text:  aiVisionDisabledHint(len(images)),
			Level: "warning",
		})
	}
	aiStreamNotify(streamID, "ai_stream_dic_run", panel)

	// 回灌给模型的文本里补一句提示：触发词没命中、运行已退化成线性脚本，
	// 避免模型据此误判词库行为（模型只读文本，看不到前端的黄色警告）
	modelOutput := output
	if fellBack {
		modelOutput = dicTriggerMissHint(trigger) + "\n" + output
	}
	if len(images) > 0 && !vision {
		// 视觉能力未开启：明确告知模型图片无法查看，避免其凭空臆测图片内容
		modelOutput = aiVisionDisabledHint(len(images)) + "\n" + modelOutput
	}

	resp := map[string]any{
		"output":   aiClipRunes(modelOutput, aiToolOutputMaxRunes),
		"timedOut": timedOut,
	}
	if len(dic.Data.Warnings) > 0 {
		resp["warnings"] = dic.Data.Warnings
	}
	brief := "已运行 " + p
	if timedOut {
		brief += "（超时）"
	}
	if len(images) > 0 {
		if vision {
			brief += fmt.Sprintf("（含 %d 张图片，已上传）", len(images))
		} else {
			brief += fmt.Sprintf("（含 %d 张图片，视觉能力未开启）", len(images))
		}
	}
	upload := images
	if !vision {
		upload = nil
	}
	return aiToolResult(resp), brief, upload
}

// aiToolCallSeq 为「文本形式工具调用」补全的调用编号，保证后续 tool 结果能按 id 正确回填。
var aiToolCallSeq atomic.Uint64

// aiParseTextToolCalls 兜底解析「被当成正文输出」的工具调用。
// 部分服务商/模型在流式响应下不走原生 tool_calls 字段，而是把调用写成文本混进正文：
//
//	<tool_call>工具名<arg_key>参数名</arg_key><arg_value>参数值</arg_value>...</tool_call>
//	<tool_call>{"name":"工具名","arguments":{...}}</tool_call>
//
// 若不解析，模型就会「口头声称已按某语法重写」，实际从未调用 save_dic，文件根本没变。
// 返回解析出的调用列表与剔除这些片段后的正文。
func aiParseTextToolCalls(content string) ([]aiToolCall, string) {
	// 兼容 DeepSeek 等模型输出的 DSML 形式（见 aiCanonDSMLToolCalls），先规整为统一标记再解析
	content = aiCanonDSMLToolCalls(content)
	// 兼容 Anthropic / DSML 风格的 invoke+parameter 裸标签（见 aiCanonInvokeToolCalls）
	content = aiCanonInvokeToolCalls(content)
	// 兼容 <save_dic><parameter name="…">…</parameter></save_dic> 这类裸 XML（见 aiCanonXMLToolCalls）
	content = aiCanonXMLToolCalls(content)
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
		inner, after, ok := strings.Cut(content[start+len("<tool_call>"):], "</tool_call>")
		if !ok {
			// 未闭合：整体保留，避免误删正常正文
			rest.WriteString(content[start:])
			break
		}
		content = after
		if call, ok := aiParseTextToolCall(inner); ok {
			calls = append(calls, call)
		}
	}
	return calls, strings.TrimSpace(rest.String())
}

// ============== DSML 形式文本工具调用的规整 ==============
//
// 部分模型（DeepSeek 系列）在流式响应下不返回 OpenAI 原生 tool_calls，而是把调用写成
// 自带分隔符的 DSML 标记混进正文：外层是 calls 包裹标签，内层是 invoke，参数用 parameter 表示。
// 该写法既不进原生字段，也不符合上面那种标记，解析会落空，整段标记就原样留在答复正文里
// （用户看到的正是「文字里冒出一段调用，文件却毫无变化」）。
// 这里把它规整成上面那套统一标记（工具名 + arg_key/arg_value 参数），复用既有解析逻辑；
// 文本不含 DSML 标记时原样返回，避免误伤正常正文。
var (
	// 分隔符本身（两侧竖线数量不定），去掉后标签名才可辨认
	aiDSMLMarkRe = regexp.MustCompile(`[｜|]+\s*DSML\s*[｜|]+`)
	// 分隔符删除后标签名与左尖括号之间会残留空白，需收敛掉
	aiDSMLTagGapRe = regexp.MustCompile(`<(/)?\s+(calls|invoke|parameter)\b`)
	// 外层的 calls 包裹标签：内层 invoke 才是真正的调用单元
	aiDSMLWrapRe = regexp.MustCompile(`</?calls\s*>`)
	// 单个 invoke 块及其参数（DSML 去掉分隔符后与 Anthropic 风格同形，共用一套匹配）
	aiInvokeBlockRe = regexp.MustCompile(`(?s)<invoke\s+name="([^"]*)"\s*>(.*?)</invoke>`)
	aiInvokeParamRe = regexp.MustCompile(`(?s)<parameter\s+name="([^"]*)"[^>]*>(.*?)</parameter>`)
)

// aiCanonDSMLToolCalls 去掉 DSML 分隔符与外层 calls 包裹标签，露出标准的 invoke 结构；不含 DSML 时原样返回。
func aiCanonDSMLToolCalls(s string) string {
	if !strings.Contains(s, "DSML") {
		return s
	}
	s = aiDSMLMarkRe.ReplaceAllString(s, "")
	s = aiDSMLTagGapRe.ReplaceAllString(s, "<${1}${2}")
	s = aiDSMLWrapRe.ReplaceAllString(s, "")
	return s
}

// aiCanonInvokeToolCalls 把「invoke + parameter」形式的文本工具调用规整为统一标记；无匹配时原样返回。
// 该写法不光出现在 DSML（剥离分隔符后），Anthropic 风格的模型也会直接吐出这种裸标签，
// 而且往往不带 <tool_call> 包裹：只认 <tool_call> 时整段调用会被当正文输出、实际从未执行。
func aiCanonInvokeToolCalls(s string) string {
	// 只要求出现 invoke 开标签：无参工具（如 read_dic_doc）的 invoke 块里没有 parameter，
	// 若一并要求存在 <parameter，这类调用会被整体当成正文输出、永远不会执行。
	if !strings.Contains(s, "<invoke") {
		return s
	}
	return aiInvokeBlockRe.ReplaceAllStringFunc(s, func(block string) string {
		m := aiInvokeBlockRe.FindStringSubmatch(block)
		if m == nil {
			return block
		}
		return aiCanonInvokeBlock(m[1], m[2])
	})
}

// aiCanonInvokeBlock 把单个 invoke 的「工具名 + 参数区」拼装成统一标记（无参数时仅保留工具名）。
func aiCanonInvokeBlock(name, params string) string {
	var b strings.Builder
	b.WriteString("<tool_call>")
	b.WriteString(strings.TrimSpace(name))
	for _, pm := range aiInvokeParamRe.FindAllStringSubmatch(params, -1) {
		b.WriteString("<arg_key>")
		b.WriteString(strings.TrimSpace(pm[1]))
		b.WriteString("</arg_key><arg_value>")
		b.WriteString(strings.TrimSpace(pm[2]))
		b.WriteString("</arg_value>")
	}
	b.WriteString("</tool_call>")
	return b.String()
}

// ============== 裸 XML 形式文本工具调用的规整 ==============
//
// 部分模型既不返回原生 tool_calls，也套不上 DSML，而是直接把调用写成裸 XML：
// <save_dic><parameter name="path">a.n</parameter><parameter name="content">…</parameter></save_dic>
// 既没有 <tool_call> 包裹、也没有 DSML 标记，既有解析全部落空，整段会被当正文吐出、文件毫无变化。
// 这里把它规整成统一标记后复用既有解析逻辑；标签名限定为本应用已注册的工具，避免误伤正文里的普通 XML。
var aiXMLParamRe = regexp.MustCompile(`(?s)<parameter\s+name="([^"]*)"[^>]*>(.*?)</parameter>`)

// aiKnownToolNames 已注册工具的名单（惰性构建，供裸 XML 规整识别开标签）。
var (
	aiToolNameSetOnce sync.Once
	aiToolNameSet     map[string]bool
)

func aiKnownToolNames() map[string]bool {
	aiToolNameSetOnce.Do(func() {
		aiToolNameSet = map[string]bool{}
		for _, def := range aiToolDefinitions() {
			fn, ok := def["function"].(map[string]any)
			if !ok {
				continue
			}
			if n, ok := fn["name"].(string); ok && n != "" {
				aiToolNameSet[n] = true
			}
		}
	})
	return aiToolNameSet
}

// aiFindXMLToolOpen 在 s 中查找最早出现的「已注册工具的裸 XML 开标签」，返回起始下标、工具名与标签长度。
// 名称与 '>' 之间允许出现多余字符（模型常把标签写畸形，如 <read_dic">、<read_dic path="x">），
// 只要名称后紧跟的字符不是名称本身的延续字符即可，避免把 <read_dictionary> 误判为工具 read_dic。
func aiFindXMLToolOpen(s string, names map[string]bool) (int, string, int) {
	best, bestName, bestLen := -1, "", 0
	for name := range names {
		prefix := "<" + name
		from := 0
		for {
			i := strings.Index(s[from:], prefix)
			if i < 0 {
				break
			}
			i += from
			after := i + len(prefix)
			if after >= len(s) || aiXMLNameContinuation(s[after]) {
				from = after
				continue
			}
			gt := strings.IndexByte(s[after:], '>')
			if gt < 0 {
				break
			}
			if best < 0 || i < best {
				best, bestName, bestLen = i, name, after+gt+1-i
			}
			break
		}
	}
	return best, bestName, bestLen
}

// aiXMLNameContinuation 判断字符是否属于工具名的延续字符（字母 / 数字 / 下划线）。
// 用于把 <read_dic"…> 这类畸形标签认作工具 read_dic，同时排除 <read_dictionary> 这类更长的标签名。
func aiXMLNameContinuation(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// aiCanonXMLToolCalls 把裸 XML 形式的文本工具调用规整为统一标记；无匹配时原样返回。
func aiCanonXMLToolCalls(s string) string {
	if !strings.Contains(s, "<parameter") {
		return s
	}
	names := aiKnownToolNames()
	var out strings.Builder
	rest := s
	for {
		pos, name, tagLen := aiFindXMLToolOpen(rest, names)
		if pos < 0 {
			out.WriteString(rest)
			break
		}
		closeTag := "</" + name + ">"
		ci := strings.Index(rest[pos+tagLen:], closeTag)
		if ci < 0 {
			out.WriteString(rest)
			break
		}
		inner := rest[pos+tagLen : pos+tagLen+ci]
		params := aiXMLParamRe.FindAllStringSubmatch(inner, -1)
		if len(params) == 0 {
			// 不带参数、不是可识别的调用：原样保留，并跳过该开标签避免死循环
			out.WriteString(rest[:pos+tagLen])
			rest = rest[pos+tagLen:]
			continue
		}
		out.WriteString(rest[:pos])
		out.WriteString("<tool_call>")
		out.WriteString(name)
		for _, p := range params {
			out.WriteString("<arg_key>")
			out.WriteString(strings.TrimSpace(p[1]))
			out.WriteString("</arg_key><arg_value>")
			out.WriteString(p[2])
			out.WriteString("</arg_value>")
		}
		out.WriteString("</tool_call>")
		rest = rest[pos+tagLen+ci+len(closeTag):]
	}
	return out.String()
}

// aiToolCallTextRouter 流式识别正文里的文本形式工具调用（<tool_call>…</tool_call>），
// 让调用片段改走思考区、而不是出现在答复正文中（正文里出现裸 JSON 会严重干扰阅读）。
// 增量可能把一个标记拆成多片（如 "<tool" + "_call>"），用 pending 暂存疑似不完整的标记尾部，
// 等下一片拼接后再判定，避免把跨片的标记误当普通正文推送出去。
type aiToolCallTextRouter struct {
	inCall  bool   // 当前是否已进入 <tool_call> 片段内部
	pending string // 暂存的、可能与后续增量拼成完整标记的尾部
}

// aiRouteFeed 送入一段正文增量，返回应进入正文的文本与应进入思考区的文本。
func (r *aiToolCallTextRouter) aiRouteFeed(text string) (body, thought string) {
	r.pending += text
	var bodyBuf, thoughtBuf strings.Builder
	for r.pending != "" {
		if r.inCall {
			if i := strings.Index(r.pending, "</tool_call>"); i >= 0 {
				thoughtBuf.WriteString(r.pending[:i+len("</tool_call>")])
				r.pending = r.pending[i+len("</tool_call>"):]
				r.inCall = false
				continue
			}
			keep := aiMarkerPrefixSuffix(r.pending, "</tool_call>")
			thoughtBuf.WriteString(r.pending[:len(r.pending)-len(keep)])
			r.pending = keep
			break
		}
		if i := strings.Index(r.pending, "<tool_call>"); i >= 0 {
			bodyBuf.WriteString(r.pending[:i])
			r.pending = r.pending[i+len("<tool_call>"):]
			r.inCall = true
			continue
		}
		keep := aiMarkerPrefixSuffix(r.pending, "<tool_call>")
		bodyBuf.WriteString(r.pending[:len(r.pending)-len(keep)])
		r.pending = keep
		break
	}
	return bodyBuf.String(), thoughtBuf.String()
}

// aiRouteFlush 流结束时吐出仍滞留在 pending 中的文本：未闭合的调用片段归思考区，其余归正文。
func (r *aiToolCallTextRouter) aiRouteFlush() (body, thought string) {
	if r.pending == "" {
		return "", ""
	}
	if r.inCall {
		thought, r.pending = r.pending, ""
		return "", thought
	}
	body, r.pending = r.pending, ""
	return body, ""
}

// aiMarkerPrefixSuffix 返回 s 中最长的、可作为 marker 不完整前缀的后缀（用于跨增量拼接标记）。
func aiMarkerPrefixSuffix(s, marker string) string {
	limit := min(len(marker)-1, len(s))
	for n := limit; n > 0; n-- {
		if strings.HasPrefix(marker, s[len(s)-n:]) {
			return s[len(s)-n:]
		}
	}
	return ""
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
	name, _, _ := strings.Cut(inner, "<arg_key>")
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
		keyPart, tail, ok := strings.Cut(remain[ks+len("<arg_key>"):], "</arg_key>")
		if !ok {
			break
		}
		key := strings.TrimSpace(keyPart)
		if !strings.HasPrefix(tail, "<arg_value>") {
			break
		}
		valPart, next, ok := strings.Cut(tail[len("<arg_value>"):], "</arg_value>")
		if !ok {
			break
		}
		args[key] = valPart
		remain = next
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

// aiNormalizeToolCalls 规整本轮拿到的工具调用，保证 assistant(tool_calls) 与随后的 tool 消息
// 一定能一一配对——这是上游的硬性结构约束，配不上就会整轮拒绝请求。
// 部分服务商/网关在流式响应里不下发 tool_call 的 id（只给 index/name/arguments），若照原样回灌：
// assistant 消息会带着 id 为空的 tool_calls，而 tool 消息的 tool_call_id 又因空值被序列化时省略，
// 上游便返回「An assistant message with 'tool_calls' must be followed by tool messages responding to
// each 'tool_call_id'. (insufficient tool messages following tool_calls message)」。
// 这里为缺失 id 的调用补一个本地唯一 id、补全 type，并剔除没有工具名的残缺口（无法执行，
// 留着只会让 assistant 多出一条永远没有对应结果的调用）。
func aiNormalizeToolCalls(calls []aiToolCall) []aiToolCall {
	if len(calls) == 0 {
		return calls
	}
	out := make([]aiToolCall, 0, len(calls))
	for _, call := range calls {
		if strings.TrimSpace(call.Function.Name) == "" {
			continue
		}
		if call.Type == "" {
			call.Type = "function"
		}
		if strings.TrimSpace(call.ID) == "" {
			call.ID = fmt.Sprintf("call_gen_%d", aiToolCallSeq.Add(1))
		}
		out = append(out, call)
	}
	return out
}

// aiActionPreambleMarkers 计划/承诺式前言的典型措辞：模型用这些词表态「接下来要做什么」，
// 却在本轮没有发起任何工具调用。
var aiActionPreambleMarkers = []string{
	"我这就", "我马上", "我先", "让我先", "让我来", "接下来我", "然后我", "随后我",
	"我会", "我将", "我准备", "我打算", "我来看", "我来修", "我来改", "我来写",
	"先修复", "先修正", "先读取", "先看", "然后跑", "然后运行", "再运行",
}

// aiActionDoneMarkers 完成/汇报式措辞：出现这些说明模型在陈述已经做完的事，属正常收尾，不应再提醒。
var aiActionDoneMarkers = []string{
	"已完成", "已保存", "已修复", "已修正", "已修改", "已写入", "已更新", "已经", "完毕",
	"成功", "通过", "结果如下", "如下所示", "检查结果",
}

// aiLooksLikeActionPreamble 判断模型正文是否像「我先做 X，然后做 Y」式的计划前言：
// 这类正文不是最终答复，模型本应随之发起工具调用，却因上游截断或自身空转在此停住。
// 识别出来后可补一轮「请直接执行」的提醒，避免出现「说要去做、随后没反应」。
// 判断偏保守：仅对简短且不含完成/汇报措辞的正文生效，避免打断正常的文字答复。
func aiLooksLikeActionPreamble(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	// 计划前言通常很简短；长正文多为正式答复，误判会造成多余往返
	if utf8.RuneCountInString(t) > 400 {
		return false
	}
	for _, m := range aiActionDoneMarkers {
		if strings.Contains(t, m) {
			return false
		}
	}
	for _, m := range aiActionPreambleMarkers {
		if strings.Contains(t, m) {
			return true
		}
	}
	return false
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
	// 提醒计数：模型只回「我这就去做 X」式前言、未发起工具调用时，最多补 aiToolMaxNudges 轮促其真正执行
	nudgeCount := 0
	// 续写计数：上游因输出长度上限截断（finish_reason=length）时最多自动续写 aiToolMaxContinues 轮
	continueCount := 0
	// 空回复重试计数：上游只产出思考、正文为空时最多自动重试 aiToolMaxEmptyRetries 次
	emptyRetryCount := 0
	// plainOnly 置位后不再下发工具（空回复重试后的轮次），避免模型又陷入「要不要调工具」的长思考
	plainOnly := false
	// contentAll 累积因截断而分段产出的正文：续写会把答复拆成多轮，
	// 最终以它拼接为准返回，前端在 ai_stream_end 用返回值覆盖正文，避免只剩最后一段。
	var contentAll strings.Builder
	// mergeContent 把累积的续写片段与本轮正文拼成完整答复；本轮无正文时只返回累积片段。
	// 所有「返回最终答复」的分支都必须经它收口，否则续写的中间段会被后续轮次覆盖丢失。
	mergeContent := func(cur string) string {
		if contentAll.Len() == 0 {
			return cur
		}
		if strings.TrimSpace(cur) == "" {
			return contentAll.String()
		}
		contentAll.WriteString(cur)
		return contentAll.String()
	}
	// 收尾轮由时间预算驱动：预算用尽（或触及 aiToolMaxRounds 安全上限）后不再下发工具，
	// 强制模型基于已获得的信息给出最终答复——不能直接报错中止整轮回复，否则用户只能重试。
	// 轮数上限只是防死循环的安全网，正常任务应在预算内自然收尾，避免中途撤掉工具。
	for round := 0; ; round++ {
		final := round >= aiToolMaxRounds || (round > 0 && time.Now().After(deadline))
		// 收尾轮不下发工具，强制模型基于已有信息给出最终答复；空回复重试后的轮次同样不带工具
		roundTools := tools
		if final || plainOnly {
			roundTools = nil
		}
		content, reasoning, calls, finishReason, err := aiChatOnceTools(c, model, work, effort, roundTools, toolChoice, streamID)
		// 用户主动终止：保留本轮已流出的思考与正文，交由上层收尾
		if errors.Is(err, errAIStreamCancelled) {
			if r := strings.TrimSpace(reasoning); r != "" {
				reasoningAll.WriteString(r)
				reasoningAll.WriteByte('\n')
			}
			return mergeContent(content), reasoningAll.String(), err
		}
		if err != nil && round == 0 && toolChoice != "" {
			toolChoice = ""
			content, reasoning, calls, finishReason, err = aiChatOnceTools(c, model, work, effort, roundTools, "", streamID)
			if errors.Is(err, errAIStreamCancelled) {
				if r := strings.TrimSpace(reasoning); r != "" {
					reasoningAll.WriteString(r)
					reasoningAll.WriteByte('\n')
				}
				return mergeContent(content), reasoningAll.String(), err
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
			reasoningAll.WriteString(r)
			reasoningAll.WriteByte('\n')
		}
		// 兜底：模型有时把工具调用写成 <tool_call> 文本混进正文、不走原生 tool_calls，
		// 不解析就会出现「模型声称已按正确语法重写，实际从未调用 save_dic、文件没变」。
		// 收尾轮同样要解析：此时已不下发工具定义，模型仍可能把调用写成 DSML/invoke 文本，
		// 不剥离这些标记就会把 `<…DSML… invoke…>` 原样塞进答复，用户看到一段没被执行的调用。
		if parsed, rest := aiParseTextToolCalls(content); len(parsed) > 0 {
			if len(calls) == 0 {
				calls = parsed
			}
			content = rest
		}
		// 规整为可配对的调用列表：补齐缺失的 id、剔除残缺口，避免下游上游因
		// assistant(tool_calls) 与 tool 消息配不上对而拒绝整轮请求（见 aiNormalizeToolCalls）
		calls = aiNormalizeToolCalls(calls)
		if len(calls) == 0 {
			trimmed := strings.TrimSpace(content)
			// 根治输出截断：上游因长度上限中断（finish_reason=length）时，本轮正文是「半截话」，
			// 直接返回会让用户看到答复说到一半突然消失。把已生成部分作为 assistant 消息回灌，
			// 提示模型紧接上文续写，直到自然收尾或达到 aiToolMaxContinues 上限。
			// 本轮无正文（如输出预算被思维链吃满）时无法回灌 assistant 消息续写，交由下方提醒兜底
			if finishReason == "length" && trimmed != "" && continueCount < aiToolMaxContinues {
				continueCount++
				appendReasoning("\n⚠ 输出达到上游长度上限被截断，已自动续写\n")
				contentAll.WriteString(content)
				work = append(work, aiChatMessage{Role: "assistant", Content: content})
				work = append(work, aiChatMessage{
					Role: "user",
					Content: "你上一条回复因长度上限被截断了。请紧接上文继续输出剩余内容：" +
						"不要重复已经写过的部分，不要重新开头或重新总结；" +
						"若剩余内容是文件改动，请直接通过工具调用（tool_calls）执行，而不是继续用文字描述。",
				})
				continue
			}
			// 「说了要做、却没真做」：本轮有正文但没发起任何工具调用，且正文像
			// 「我先…然后…」式的计划前言（正文为空也算）。
			// 直接 return 会让用户看到一句承诺后彻底没反应，这里补一轮提醒促使模型真正执行。
			stuck := trimmed == "" || aiLooksLikeActionPreamble(trimmed)
			if !final && nudgeCount < aiToolMaxNudges && stuck {
				nudgeCount++
				appendReasoning("\n⚠ 本轮只返回了文字描述、未发起工具调用，已提醒模型直接执行\n")
				if trimmed != "" {
					// 保留该轮前言为 assistant 消息，使模型能接着往下做
					work = append(work, aiChatMessage{Role: "assistant", Content: content})
				}
				work = append(work, aiChatMessage{
					Role: "user",
					Content: "如果你上面描述的是你准备执行的操作，请立刻通过工具调用（tool_calls）真正执行，" +
						"不要只用文字描述计划，也不要仅声称「已完成」；需要改动文件时先读取最新内容再写入，保存后运行验证。" +
						"如果你上面已经给出了完整结论、确实无事可做，请直接说明这一点作为最终答复。",
				})
				continue
			}
			// 本轮与累积片段均为空才算无正文，避免把续写累积的内容误判为空
			if trimmed == "" && contentAll.Len() == 0 {
				// 上游偶发「只产出思考、正文为空」（思考过程吃满输出预算时尤其常见）：
				// 此前直接以「AI 未返回内容」报错、整轮生成中断，用户只能手动重发。
				// 这里改为自动重试——撤下工具并要求直接用文字作答，同时把重试状态推给前端倒计时展示。
				if !final && emptyRetryCount < aiToolMaxEmptyRetries {
					emptyRetryCount++
					wait := aiToolEmptyRetryDelay * time.Duration(emptyRetryCount)
					appendReasoning(fmt.Sprintf("\n⚠ 上游本轮只返回了思考、未返回正文，%v 后自动重试（第 %d/%d 次）\n",
						wait, emptyRetryCount, aiToolMaxEmptyRetries))
					aiStreamNotify(streamID, "ai_stream_delta", map[string]any{
						"kind":    "retry",
						"reason":  "上游未返回内容",
						"attempt": emptyRetryCount,
						"seconds": int(wait / time.Second),
					})
					select {
					case <-time.After(wait):
					case <-parentCtx.Done():
						return mergeContent(content), reasoningAll.String(), errAIStreamCancelled
					}
					plainOnly = true
					work = append(work, aiChatMessage{
						Role: "user",
						Content: "你上一条回复只产出了思考过程，没有输出任何正文。请直接用文字给出答复：" +
							"本轮不要调用工具、不要重复思考过程，也不要只描述你打算做什么。",
					})
					continue
				}
				if final {
					// 收尾轮仍无正文：给出可读提示，而不是报错中止
					content = "（本次工具调用已达到轮数/时间上限，未能生成完整答复。可以回复「继续」让我接着处理。）"
				} else {
					// 重试用尽仍无正文：不再以错误中断整轮生成（历史版本会报「AI 未返回内容」），
					// 给出可读提示并保留已产生的思考，用户可直接点「重试」再次发起。
					content = "（上游多次未返回正文内容，可能是思考过程占满了输出预算。可以点「重试」或回复「继续」再试一次。）"
				}
			}
			content = mergeContent(content)
			// 续写用尽后仍被截断：明确告知用户，避免误以为答复已经完整
			if finishReason == "length" {
				content += "\n\n（提示：本次输出多次触达模型长度上限，内容可能仍不完整。可回复「继续」让我接着补全。）"
			}
			// 正文已在 aiChatOnceTools 流式解析时逐片推送，此处不再重复推整段
			return content, reasoningAll.String(), nil
		}
		// 保留本轮工具请求，作为后续上下文；content 需原样回传以保持消息结构完整
		work = append(work, aiChatMessage{Role: "assistant", Content: content, ToolCalls: calls})
		// 本轮工具捕获到的图片先收集，待全部 tool 消息回灌完毕后再统一补发：
		// assistant(tool_calls) 之后必须紧跟一一对应的 tool 消息，若在中间插入 role=user
		// 的图片消息，上游会把 tool 消息块截断，判为「insufficient tool messages following
		// tool_calls message」并拒绝整轮请求（多工具并行时必然触发）。
		var roundImages []string
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
			// 文件改动前的快照：解析本工具会改动的文件目标并在执行前抓取，供回撤与逐行标注
			fileTargets := aiFileToolTargets(call.Function.Name, call.Function.Arguments)
			aiFileToolCapture(fileTargets)
			result, brief, images := aiToolExecute(call.Function.Name, call.Function.Arguments, streamID, c.Vision)
			// 执行后按实际落盘状态登记「本任务改过的文件」并推送逐行标注
			if len(fileTargets) > 0 {
				aiFileToolRecord(sessionID, call.Function.Name, fileTargets)
			}
			appendReasoning("  " + brief + "\n")
			work = append(work, aiChatMessage{Role: "tool", ToolCallID: call.ID, Name: call.Function.Name, Content: result})
			roundImages = append(roundImages, images...)
		}
		// 收尾轮：最后一批调用已就地执行完，不再回灌模型（否则又开启新一轮调用），
		// 直接以已剥离标记的正文收尾；正文为空时给出可读提示。
		if final {
			if strings.TrimSpace(content) == "" && contentAll.Len() == 0 {
				content = "（本次工具调用已达到轮数/时间上限，未能生成完整答复。可以回复「继续」让我接着处理。）"
			}
			// 若此前发生过截断续写，累积片段必须一并返回，否则收尾答复只剩最后一段
			return mergeContent(content), reasoningAll.String(), nil
		}
		// 视觉能力开启且工具捕获到图片：以多模态 user 消息补发图片，供模型查看
		if len(roundImages) > 0 {
			work = append(work, aiVisionUserMessage(
				"以下图片来自上面 run_dic 的运行输出，请结合运行结果查看：", roundImages))
		}
	}
}

// ============== AI 工具调用审批（盾牌权限闸门） ==============
//
// 任务级权限档位（盾牌菜单）：
//   manual 手动审批：除查阅文档（read_dic_doc）外，其余工具调用都需用户在对话流内联卡片中确认
//   auto   自动审批（默认）：应用目录（当前项目）内的读取、写入与运行词库自动放行，删除/移动/重命名类工具需确认
//   full   完全访问：全部工具自动放行
// read_dic_doc 在任何档位下都免审批（见 aiToolAlwaysAllow）。
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
	"read_dic_doc": true,
	"read_skill":   true,
	"check_dic":    true,
}

// aiToolAutoAllow auto（自动审批，默认档位）下额外免审批的写入与执行工具。
// 这些工具都经 checkFilePath / checkDicPath 校验，路径被限制在应用目录（当前项目）内，
// 越权路径（绝对路径、../）会被直接拒绝，因此项目内的读写与运行无需再让用户逐次放行。
// save_dic、write_file 是词库调试的核心动作，用户已在对话里明确要求改文件；若继续弹卡片，
// 一旦用户没及时答复（切走 / 超时），模型就会退化成「让用户手动复制粘贴」，文件根本没写入。
// run_dic 只读地执行词库并返回输出，是「改完即验证」的收尾动作，同样不该中断。
// 词库运行期自身的删除请求另有 dic_delete_confirm 确认流程，不受此处影响。
// 删除、移动、重命名属破坏性操作，在 auto 档下仍保留确认卡片。
var aiToolAutoAllow = map[string]bool{
	"save_dic":   true,
	"write_file": true,
	"run_dic":    true,
}

// aiToolAlwaysAllow 任何权限档位（含手动审批）都免审批的只读工具。
// 查阅文档/技能不产生磁盘副作用，也不改动词库，弹审批卡片只会卡住模型求证语法的路；
// 模型求证不了就退化成猜 API、往词库里写试探词条，破坏性反而更大。
var aiToolAlwaysAllow = map[string]bool{
	"read_dic_doc": true,
	"read_skill":   true,
}

// aiToolNeedsApproval 判断某次工具调用在当前权限档位下是否需要人工审批。
func aiToolNeedsApproval(permissionMode, toolName string) bool {
	if aiToolAlwaysAllow[toolName] {
		return false
	}
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
