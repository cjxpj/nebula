package dic_server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	dic_funcs "github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"
	"github.com/cjxpj/nebula/utils"
)

var (
	dicDeleteSeq           atomic.Uint64
	dicDeletePending       sync.Map // id(string) -> chan bool
	dicDeleteConfirmOnce   sync.Once
	dicDeleteConfirmActive atomic.Int32 // 正在进行的调试运行数，0 表示非交互式执行（如机器人正式运行）
)

// ensureDicDeleteConfirm 注册词库删除确认回调（仅注册一次）。
// 回调内部按「是否处于调试运行」决定是否拦截：非调试运行直接放行，保持原有非交互行为。
func ensureDicDeleteConfirm() {
	dicDeleteConfirmOnce.Do(func() {
		dic_funcs.SetFileDeleteConfirm(func(action, target string) bool {
			if dicDeleteConfirmActive.Load() == 0 {
				return true
			}
			return requestDicDeleteConfirm(action, target)
		})
	})
}

// requestDicDeleteConfirm 向前端推送删除确认请求并阻塞等待答复，超时按拒绝处理。
func requestDicDeleteConfirm(action, target string) bool {
	id := fmt.Sprintf("%d", dicDeleteSeq.Add(1))
	ch := make(chan bool, 1)
	dicDeletePending.Store(id, ch)
	defer dicDeletePending.Delete(id)

	data, _ := json.Marshal(map[string]any{
		"type": "dic_delete_confirm",
		"data": map[string]any{"id": id, "action": action, "target": target},
	})
	broadcastOpuiNotify(data)

	select {
	case ok := <-ch:
		return ok
	case <-time.After(60 * time.Second):
		return false
	}
}

// maxServerLogLines 面板回放日志时最多在内存中收集的最近行数（防止过大卡顿）
func loadDicDebugDefaults() map[string]any {
	def := map[string]any{}
	cfg, err := dto.LoadConfigFile()
	if err != nil {
		return def
	}
	sec := cfg.Section("词库调试")
	if v := sec.Key("默认词库").String(); v != "" {
		def["path"] = v
	}
	if v := sec.Key("打开的标签").String(); v != "" {
		var tabs []string
		if err := json.Unmarshal([]byte(v), &tabs); err == nil {
			def["tabs"] = tabs
		}
	}
	if v := sec.Key("触发文本").String(); v != "" {
		def["trigger"] = v
	}
	if b, err := sec.Key("保存运行").Bool(); err == nil {
		def["saveRun"] = b
	}
	if b, err := sec.Key("实时保存").Bool(); err == nil {
		def["autoSave"] = b
	}
	// 保存自动格式化：未配置过时默认启用
	def["autoFormat"] = sec.Key("保存自动格式化").MustBool(true)
	if n := sec.Key("超时").MustInt(0); n > 0 {
		def["timeout"] = n
	}
	if n := sec.Key("历史记录数量").MustInt(0); n > 0 {
		def["historyMax"] = n
	}
	if v := sec.Key("全局变量").String(); v != "" {
		// 值可含任意换行，因此整体按 JSON 数组存储；解析失败时兼容旧格式（每行一个 key=value）
		var g []string
		if err := json.Unmarshal([]byte(v), &g); err == nil {
			// JSON 数组解析成功；注意 "null" 字面量会解析成 nil 切片，视为空配置
			if g != nil {
				def["g"] = g
			}
		} else {
			// 旧格式：每行一个 key=value
			g = nil
			for line := range strings.SplitSeq(v, "\n") {
				if s := strings.TrimSpace(line); s != "" {
					g = append(g, s)
				}
			}
			if g != nil {
				def["g"] = g
			}
		}
	}
	return def
}

// dicAutoFormatEnabled 读取运行配置的「保存自动格式化」开关，未配置时默认启用。
func dicAutoFormatEnabled() bool {
	cfg, err := dto.LoadConfigFile()
	if err != nil {
		return true
	}
	return cfg.Section("词库调试").Key("保存自动格式化").MustBool(true)
}

// defaultDebugDic 返回词库调试默认词库路径（未配置时回退 private/debug.n）。
// 默认调试词库首次打开时若不存在会自动创建，避免报「词库文件不存在」。
func defaultDebugDic() string {
	if cfg, err := dto.LoadConfigFile(); err == nil {
		if v := strings.TrimSpace(cfg.Section("词库调试").Key("默认词库").String()); v != "" {
			return filepath.ToSlash(filepath.Clean(v))
		}
	}
	return "private/debug.n"
}

// checkDicPath 校验词库调试路径：仅允许应用目录内相对路径的 .n 文件，
// 拒绝绝对路径、包含 .. 的越权路径以及非词库文件
func checkDicPath(path string) bool {
	if path == "" || filepath.IsAbs(path) {
		return false
	}
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "\\") {
		return false
	}
	if strings.Contains(path, "..") {
		return false
	}
	if !strings.HasSuffix(strings.ToLower(path), ".n") {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

// checkWebDicPath 校验网页词库路径：仅允许应用目录内相对路径的 .wn 文件
func checkWebDicPath(path string) bool {
	if path == "" || filepath.IsAbs(path) {
		return false
	}
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "\\") {
		return false
	}
	if strings.Contains(path, "..") {
		return false
	}
	if !strings.HasSuffix(strings.ToLower(path), ".wn") {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

// checkDicOrWebPath 校验「词库文件」路径：.n 或 .wn，供读取 / 保存这类两类词库通用的操作用。
func checkDicOrWebPath(path string) bool {
	return checkDicPath(path) || checkWebDicPath(path)
}

// respHeader 本地运行词库时收集到的响应头（保持写入顺序，便于前端原样展示）。

// 词库调试 / 编译 / 运行 / 线程变量 API。
func init() {
	registerOpuiApi(opuiHandleDicDebugAPI,
		"dic_debug_run", "get_compile_env", "dic_build", "dic_build_status",
		"dic_run", "get_thread_vars", "set_thread_var", "del_thread_var",
		"dic_thread_vars_clear",
	)
}

// opuiHandleDicDebugAPI 处理本模块注册的 OPUI API 类型。
func opuiHandleDicDebugAPI(w http.ResponseWriter, r *http.Request, h *HttpOpUiData) {
	switch h.Type {
	case "dic_debug_run":
		var j struct {
			Path    string            `json:"path"`
			Trigger string            `json:"trigger"`
			G       map[string]string `json:"g"`
			// 超时（秒），0 表示不限时；超时后强行打断词库执行
			Timeout int `json:"timeout"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if j.Path == "" {
			http.Error(w, `{"status":"error","error":"词库路径不能为空"}`, http.StatusBadRequest)
			return
		}
		if !checkDicPath(j.Path) && !checkWebDicPath(j.Path) {
			http.Error(w, `{"status":"error","error":"词库路径不合法"}`, http.StatusBadRequest)
			return
		}
		// 网页词库（.wn）：先执行页面里全部 <?n ... ?> 内联块、再执行 <script type="nebula"> 脚本块后做模板渲染，
		// 没有触发词与编译诊断，走独立的执行分支，返回渲染后的 HTML 供前端展示，并附带静态检查诊断
		if checkWebDicPath(j.Path) {
			// 与 .n 运行一致：运行期间启用删除操作人工确认，脚本删除文件时弹窗等待用户放行
			ensureDicDeleteConfirm()
			dicDeleteConfirmActive.Add(1)
			defer dicDeleteConfirmActive.Add(-1)
			runRes, err := runWebDicLocal(j.Path, j.G)
			if err != nil {
				http.Error(w, `{"status":"error","error":"词库加载失败: `+err.Error()+`"}`, http.StatusBadRequest)
				return
			}
			output := runRes.Output
			warnings := []dto.BuildWarning{}
			blockCount := 0
			templateKeys := []string{}
			if content, err := utils.NewFileQueue(j.Path).ReadFromFile(); err == nil {
				warnings = run.WebDicCheck(content)
				blockCount, templateKeys = run.WebDicRenderInfo(content)
			}
			jsonResp, _ := json.Marshal(map[string]any{
				"path":             j.Path,
				"output":           output,
				"warnings":         warnings,
				"timedOut":         false,
				"segments":         parseOutputSegments(output),
				"vars":             map[string]any{"P": map[string]any{}, "G": map[string]any{}, "GV": map[string]any{}},
				"webDic":           true,
				"respStatus":       runRes.Status,
				"respHeaders":      runRes.Headers,
				"scriptBlockCount": blockCount,
				"templateKeys":     templateKeys,
				"renderNote":       webDicRenderNote(blockCount, templateKeys),
			})
			w.Write(jsonResp)
			return
		}
		dic, err := dic_dto.RunDic(j.Path)
		if err != nil {
			http.Error(w, `{"status":"error","error":"词库加载失败: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		defer dic.Close()

		// 调试运行期间启用删除操作人工确认：脚本删除文件 / 文件夹时弹窗等待用户放行
		ensureDicDeleteConfirm()
		dicDeleteConfirmActive.Add(1)
		defer dicDeleteConfirmActive.Add(-1)

		// 编译存在 error 级诊断（框配对错误、触发词正则错误等）时拒绝运行：
		// 不执行词库，直接把错误与编译诊断回传前端高亮。
		for _, warn := range dic.Data.Warnings {
			if warn.Level == "error" {
				jsonResp, _ := json.Marshal(map[string]any{
					"output":       "",
					"timedOut":     false,
					"segments":     []any{},
					"vars":         map[string]any{"P": map[string]any{}, "G": map[string]any{}, "GV": map[string]any{}},
					"warnings":     dic.Data.Warnings,
					"compileError": "编译存在错误，无法运行",
				})
				w.Write(jsonResp)
				return
			}
		}

		// 注入全局变量
		for k, v := range j.G {
			dic.Val.G.Set(k, v)
		}

		// 与 HTTP 链路的 .n 分支一致地注入 设置头部 / GET / POST，并补齐响应全局变量默认值：
		// 本地调试没有真实响应对象，词库设置的响应状态与响应头改为随运行结果返回，
		// 前端据此展示响应信息并按 Content-Type 决定输出区域的展示方式
		respHeaders := attachLocalHTTPFuncs(dic)

		// 触发词命中则正常执行；未命中（如调试脚本没有写触发词）则整体按线性脚本执行
		output, timedOut, fellBack := dic_api.Api.DicRunScript(dic, j.Trigger, time.Duration(j.Timeout)*time.Second)

		// 输出/分段/错误行/变量/警告统一组装（与 AI 工具运行共用同一实现）；
		// 兜底分支额外附一条黄色警告，提示触发词未命中、触发词行被当正文输出
		resp := dicRunResultPayload(dic, output, timedOut, fellBack, j.Trigger)
		resp["respStatus"], resp["respHeaders"] = collectLocalResp(dic.Val.G, *respHeaders)
		jsonResp, _ := json.Marshal(resp)
		w.Write(jsonResp)
		return

	case "get_compile_env":
		// 检测词库打包能力（前端据此显隐「编译词库」入口与目标平台）：
		//   - windows：当前主机是否为 Windows
		//   - go：是否存在可用 Go 工具链（「扩展部署」go.exe，缺失时回退 PATH 中的 go）
		//   - linux：可打包 Linux ELF = 找到 Nebula 源码 module 根（含 go.mod）+ 可用 Go 工具链
		//   - apk：可打包 APK = Windows 主机 + Android 工程 + JDK + Android SDK
		//     （NEBULA_SRC 统一指向仓库根，含 go.work / android/ / nebula/；module 根由 nebulaModuleRoot 定位）
		goAvailable := false
		if _, err := resolveGoExe(); err == nil {
			goAvailable = true
		}
		linuxSrcOK := false
		if _, err := nebulaModuleRoot(); err == nil {
			linuxSrcOK = true
		}
		linuxReady := linuxSrcOK && goAvailable

		// APK 打包只在 Windows 打包机上进行（gradle + JDK + Android SDK）
		apkReady := false
		if runtime.GOOS == "windows" {
			if _, err := androidProjectRoot(); err == nil {
				if _, _, err := apkToolchain(); err == nil {
					apkReady = true
				}
			}
		}
		resp := map[string]any{
			"status":  "ok",
			"host":    runtime.GOOS, // 后端主机系统（windows / linux），前端据此判断本机「运行」按钮可用性
			"windows": runtime.GOOS == "windows",
			"go":      goAvailable,
			"linux":   linuxReady,
			"apk":     apkReady,
		}
		jsonResp, _ := json.Marshal(resp)
		w.Write(jsonResp)
		return

	case "dic_build":
		// 词库打包：三个平台共用同一份词库文件（Path），target 决定产出与流程：
		//   exe   - Windows 单文件运行器（嵌入 nebula.dll + 词库 gob，需可用 Go 工具链）
		//   linux - Linux 单文件 ELF（源码现编直连引擎：NEBULA_SRC + 可用 Go，可 Windows 交叉编译）
		//   apk   - Android 安装包：把该词库预置进 assets（App 首启作为启动词库执行）后 gradle 打包
		var j struct {
			Path      string `json:"path"`       // 词库（.n）路径：exe/linux/apk 统一要打包的词库
			Target    string `json:"target"`     // exe（默认）/ linux / apk
			StartPath string `json:"start_path"` // 兼容字段：旧版前端 apk 专用启动词库路径，Path 为空时回退
			GOOS      string `json:"goos"`       // exe/linux 目标系统，默认当前系统
			GOARCH    string `json:"goarch"`     // 目标架构，默认当前架构
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if j.Target == "" {
			j.Target = "exe"
		}
		var dicPath, goos, goarch string
		switch j.Target {
		case "exe", "linux":
			if j.Path == "" {
				http.Error(w, `{"status":"error","error":"词库路径不能为空"}`, http.StatusBadRequest)
				return
			}
			if !checkDicPath(j.Path) {
				http.Error(w, `{"status":"error","error":"词库路径不合法"}`, http.StatusBadRequest)
				return
			}
			dicPath = j.Path
			goos = j.GOOS
			if j.Target == "linux" {
				goos = "linux"
			}
			if goos == "" {
				goos = runtime.GOOS
			}
			if goos != "windows" && goos != "linux" {
				http.Error(w, `{"status":"error","error":"仅支持打包 windows / linux 目标平台"}`, http.StatusBadRequest)
				return
			}
			// windows 产物需要 nebula.dll 与「扩展部署」Go 工具链，仅 Windows 主机可打；
			// linux 产物为纯 Go 交叉编译，任意主机均可打（见 buildLinuxDicBundle）。
			if goos == "windows" && runtime.GOOS != "windows" {
				http.Error(w, `{"status":"error","error":"Windows exe 需在 Windows 主机上打包"}`, http.StatusBadRequest)
				return
			}
			goarch = j.GOARCH
			if goarch == "" {
				goarch = runtime.GOARCH
			}
		case "apk":
			if runtime.GOOS != "windows" {
				http.Error(w, `{"status":"error","error":"APK 打包需在 Windows 主机（Android 工程 + JDK + Android SDK + gradle）上执行"}`, http.StatusBadRequest)
				return
			}
			// exe/linux/apk 共用同一份词库文件；start_path 仅为旧版前端的兼容字段。
			dicPath = j.Path
			if dicPath == "" {
				dicPath = j.StartPath
			}
			if dicPath == "" {
				http.Error(w, `{"status":"error","error":"词库路径不能为空"}`, http.StatusBadRequest)
				return
			}
			if !checkDicPath(dicPath) {
				http.Error(w, `{"status":"error","error":"词库路径不合法"}`, http.StatusBadRequest)
				return
			}
		default:
			http.Error(w, `{"status":"error","error":"未知 target，仅支持 exe / linux / apk"}`, http.StatusBadRequest)
			return
		}
		// 打包耗时较长（首次 exe/linux 需编译引擎依赖、apk 需跑完整 gradle），异步执行，前端按 taskId 轮询。
		taskID := strconv.FormatInt(time.Now().UnixNano(), 10)
		task := &buildTask{}
		buildTasks.Store(taskID, task)
		go func() {
			var out, logText string
			var err error
			if j.Target == "apk" {
				out, logText, err = buildApkBundle(dicPath)
			} else {
				out, logText, err = buildDicBundle(dicPath, goos, goarch)
			}
			if err != nil {
				task.finish("", logText, err.Error(), "", "")
				return
			}
			task.finish(out, logText, "", "", "")
		}()
		resp := map[string]any{"status": "ok", "taskId": taskID}
		jsonResp, _ := json.Marshal(resp)
		w.Write(jsonResp)
		return

	case "dic_build_status":
		// 查询词库打包任务进度（dic_build 异步任务）
		var j struct {
			TaskID string `json:"task_id"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		v, ok := buildTasks.Load(j.TaskID)
		if !ok {
			http.Error(w, `{"status":"error","error":"任务不存在或已过期"}`, http.StatusNotFound)
			return
		}
		task := v.(*buildTask)
		done, output, logText, errText, runOutput, runErr := task.snapshot()
		resp := map[string]any{
			"done":      done,
			"output":    output,
			"log":       logText,
			"error":     errText,
			"runOutput": runOutput,
			"runErr":    runErr,
		}
		jsonResp, _ := json.Marshal(resp)
		w.Write(jsonResp)
		return

	case "dic_run":
		// 运行已编译的「本机原生产物」词库可执行文件，触发词固定为 Main，返回标准输出/错误。
		//   windows：nebula-dic.exe（嵌入 DLL 加载器）
		//   linux：nebula-dic（源码现编直连引擎 ELF）
		// 仅能运行当前主机平台编译的产物；例如 Windows 上打出的 linux ELF 需拷到 Linux 主机运行。
		if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
			http.Error(w, `{"status":"error","error":"运行仅支持 Windows / Linux 平台"}`, http.StatusBadRequest)
			return
		}
		exeName := "nebula-dic"
		if runtime.GOOS == "windows" {
			exeName = "nebula-dic.exe"
		}
		exePath, _ := filepath.Abs(filepath.Join(utils.GetAppDir(), "private", "build", "dist", exeName))
		if _, err := os.Stat(exePath); err != nil {
			http.Error(w, `{"status":"error","error":"未找到编译产物，请先点击「编译」"}`, http.StatusNotFound)
			return
		}
		stdout, stderr := runDicBundle(exePath, "Main")
		// 加载器为纯 Go，结果直接写入 stdout（stderr 仅承载错误信息）。
		resp := map[string]any{"status": "ok", "runOutput": stdout, "runErr": stderr}
		jsonResp, _ := json.Marshal(resp)
		w.Write(jsonResp)
		return

	case "get_thread_vars":
		// 返回线程变量（分页），只在请求区间内做类型识别/展开，避免变量过多时全量解析拖慢响应
		var j struct {
			Limit int `json:"limit"`
			Skip  int `json:"skip"`
		}
		_ = json.Unmarshal(h.Data, &j)
		if j.Limit <= 0 {
			j.Limit = 50
		}
		if j.Skip < 0 {
			j.Skip = 0
		}
		all := dto.GV.GetAll()
		keys := make([]string, 0, len(all))
		for k := range all {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		total := len(keys)
		start := min(j.Skip, total)
		end := min(j.Skip+j.Limit, total)
		items := make([]map[string]any, 0, end-start)
		for _, k := range keys[start:end] {
			item := varDebugItem(all[k])
			item["key"] = k
			items = append(items, item)
		}
		r, _ := json.Marshal(map[string]any{
			"list":    items,
			"total":   total,
			"hasMore": end < total,
		})
		w.Write(r)
		return

	case "set_thread_var":
		var j struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if j.Key == "" {
			http.Error(w, `{"status":"error","error":"线程变量键不能为空"}`, http.StatusBadRequest)
			return
		}
		dto.SetThreadVar(j.Key, j.Value)
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "del_thread_var":
		var j struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if j.Key == "" {
			http.Error(w, `{"status":"error","error":"线程变量键不能为空"}`, http.StatusBadRequest)
			return
		}
		dto.DeleteThreadVar(j.Key)
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "dic_thread_vars_clear":
		dto.ClearThreadVars()
		jsonResp, _ := json.Marshal(map[string]any{"ok": true})
		w.Write(jsonResp)
		return

	}
}
