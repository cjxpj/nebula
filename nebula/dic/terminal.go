//go:build !js && !dll

package dic

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/build"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
)

// defaultTrigger 为未显式指定触发词时的默认触发词，与 Web 端 AI 工具 run_dic 保持一致。
const defaultTrigger = "Main"

// cliStdout 指向进程真实标准输出，供 CLI 结果写入。
//
// server 包在 init 中会把 os.Stdout 替换为日志管道，再由后台 goroutine 异步回显到真实终端；
// CLI 执行完会立即退出，管道内的输出可能尚未回显就被丢弃。故此处直接绑定真实标准输出句柄，
// 保证脚本与外部 AI 能稳定读到 -run/-check 的结果。
var cliStdout = os.Stdout

func init() {
	if f := os.NewFile(uintptr(syscall.Stdout), "/dev/stdout"); f != nil {
		cliStdout = f
	}
}

// CLIStdout 返回进程真实标准输出，供命令行分支（帮助、版本等）在 server 包接管 os.Stdout 后仍能稳定打印。
func CLIStdout() *os.File { return cliStdout }

// RunResult 为 -run 的结构化结果，同时用于人类可读输出与 JSON 输出。
type RunResult struct {
	Path         string             `json:"path"`                   // 词库文件路径
	Trigger      string             `json:"trigger"`                // 实际使用的触发词
	Output       string             `json:"output"`                 // 执行输出
	TimedOut     bool               `json:"timedOut"`               // 是否超时中断
	FellBack     bool               `json:"fellBack"`               // 触发词是否未命中任何词条
	CompileError string             `json:"compileError,omitempty"` // 编译存在错误时的原因，非空表示拒绝执行
	ErrorCount   int                `json:"errorCount"`             // 编译诊断中 error 级数量
	Warnings     []dto.BuildWarning `json:"warnings,omitempty"`     // 编译诊断（含 error 与 warning）
}

// CheckResult 为 -check 的结构化结果。
type CheckResult struct {
	Path       string             `json:"path"`       // 词库文件路径
	Passed     bool               `json:"passed"`     // 是否无 error 级诊断
	ErrorCount int                `json:"errorCount"` // error 级诊断数量
	WarnCount  int                `json:"warnCount"`  // warning 级诊断数量
	Warnings   []dto.BuildWarning `json:"warnings"`   // 全部诊断
}

// FormatResult 为 -format 的结构化结果。
type FormatResult struct {
	Path      string `json:"path"`      // 词库文件路径
	Changed   bool   `json:"changed"`   // 格式化后内容是否与原文不同
	Written   bool   `json:"written"`   // 是否已写回原文件（-w）
	Formatted string `json:"formatted"` // 格式化后的完整词库源码
}

// RunFile 加载并执行指定词库文件，返回结构化结果。
// trigger 为空时默认 Main；timeoutSec <= 0 表示不限制执行时间。
// 编译存在 error 级诊断时拒绝执行，结果中 CompileError 与 Warnings 会携带原因。
func RunFile(path, trigger string, timeoutSec int) (*RunResult, error) {
	trigger = strings.TrimSpace(trigger)
	if trigger == "" {
		trigger = defaultTrigger
	}

	infoDic, err := dic_dto.NewDicFile(path)
	if err != nil {
		return nil, err
	}
	defer infoDic.Close()

	res := &RunResult{Path: path, Trigger: trigger}
	res.Warnings, res.ErrorCount = splitWarnings(infoDic.Data.Warnings)
	if res.ErrorCount > 0 {
		res.CompileError = "编译存在错误，无法运行"
		return res, nil
	}

	GV := dto.NewVal()
	GV.Set("版本", appfiles.Version)
	infoDic.SetGlobal_v(GV)

	timeout := time.Duration(0)
	if timeoutSec > 0 {
		timeout = time.Duration(timeoutSec) * time.Second
	}
	res.Output, res.TimedOut, res.FellBack = dic_api.Api.DicRunScript(infoDic, trigger, timeout)
	return res, nil
}

// CheckFile 预编译检测指定词库文件（不写编译缓存），返回结构化诊断结果。
func CheckFile(path string) (*CheckResult, error) {
	infoDic, err := dic_dto.NewDicFileNoCache(path)
	if err != nil {
		return nil, err
	}
	defer infoDic.Close()

	res := &CheckResult{Path: path}
	res.Warnings, res.ErrorCount = splitWarnings(infoDic.Data.Warnings)
	res.WarnCount = len(res.Warnings) - res.ErrorCount
	res.Passed = res.ErrorCount == 0
	return res, nil
}

// splitWarnings 按级别拆分编译诊断，返回全部诊断（保证非 nil）与 error 级数量。
func splitWarnings(all []dto.BuildWarning) ([]dto.BuildWarning, int) {
	list := make([]dto.BuildWarning, 0, len(all))
	errCount := 0
	for _, w := range all {
		if w.Level == "error" {
			errCount++
		}
		list = append(list, w)
	}
	return list, errCount
}

// ExtractJSONFlag 从参数（通常为 os.Args[1:]）中提取 -json/--json 开关。
// 返回去掉开关后的参数与是否启用 JSON 输出；开关可出现在任意位置。
func ExtractJSONFlag(args []string) (rest []string, jsonOut bool) {
	rest = make([]string, 0, len(args))
	for _, a := range args {
		if a == "-json" || a == "--json" {
			jsonOut = true
			continue
		}
		rest = append(rest, a)
	}
	return rest, jsonOut
}

// RunCLI 执行 -run / -check / -format 命令并把结果写入标准输出/标准错误，返回进程退出码。
// cmd 为 run、check 或 format；args 为命令后的参数；jsonOut 为 true 时输出 JSON，便于脚本或外部 AI 消费。
// 退出码：0 成功；1 执行/检测/格式化失败（编译错误、触发词未命中、超时、存在 error 级诊断、读写失败）；2 参数不合法。
func RunCLI(cmd string, args []string, jsonOut bool) int {
	switch cmd {
	case "run":
		return cliRun(args, jsonOut)
	case "check":
		return cliCheck(args, jsonOut)
	case "format":
		return cliFormat(args, jsonOut)
	default:
		fmt.Fprintln(os.Stderr, "未知命令:", cmd)
		return 2
	}
}

// cliRun 处理 -run：-run <文件> [触发词] [超时秒]
func cliRun(args []string, jsonOut bool) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "用法：-run <词库文件路径> [触发词] [超时秒]")
		return 2
	}

	timeoutSec := 0
	if len(args) > 2 {
		n, err := strconv.Atoi(strings.TrimSpace(args[2]))
		if err != nil || n < 0 {
			fmt.Fprintf(os.Stderr, "超时秒数不合法：%s\n", args[2])
			return 2
		}
		timeoutSec = n
	}

	res, err := RunFile(args[0], argAt(args, 1), timeoutSec)
	if err != nil {
		if jsonOut {
			writeJSON(map[string]string{"path": args[0], "error": err.Error()})
		} else {
			fmt.Fprintln(os.Stderr, "执行失败:", err)
		}
		return 1
	}

	if jsonOut {
		writeJSON(res)
	} else {
		printRunResult(res)
	}

	if res.CompileError != "" || res.TimedOut || res.FellBack {
		return 1
	}
	return 0
}

// cliCheck 处理 -check：-check <文件>
func cliCheck(args []string, jsonOut bool) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "用法：-check <词库文件路径>")
		return 2
	}

	res, err := CheckFile(args[0])
	if err != nil {
		if jsonOut {
			writeJSON(map[string]string{"path": args[0], "error": err.Error()})
		} else {
			fmt.Fprintln(os.Stderr, "检测失败:", err)
		}
		return 1
	}

	if jsonOut {
		writeJSON(res)
	} else if res.ErrorCount == 0 && res.WarnCount == 0 {
		fmt.Fprintln(cliStdout, "检测通过：无警告，无报错")
	} else {
		printWarnings(cliStdout, res.Warnings)
		fmt.Fprintf(cliStdout, "检测结果：%d 个错误，%d 个警告\n", res.ErrorCount, res.WarnCount)
	}

	if res.Passed {
		return 0
	}
	return 1
}

// cliFormat 处理 -format：-format <文件> [-w]
// 默认把格式化结果打印到标准输出；带 -w 时写回原文件（内容无变化则不写）。
func cliFormat(args []string, jsonOut bool) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "用法：-format <词库文件路径> [-w]")
		return 2
	}

	path := args[0]
	write := false
	for _, a := range args[1:] {
		switch a {
		case "-w", "--write":
			write = true
		default:
			fmt.Fprintf(os.Stderr, "未知参数：%s\n", a)
			return 2
		}
	}

	format := build.FormatDicFile
	if strings.HasSuffix(strings.ToLower(path), ".wn") {
		// 网页词库是 HTML，只缩进结构并保留脚本块正文
		format = build.FormatWebDicFile
	}
	formatted, changed, err := format(path)
	if err != nil {
		if jsonOut {
			writeJSON(map[string]string{"path": path, "error": err.Error()})
		} else {
			fmt.Fprintln(os.Stderr, "格式化失败:", err)
		}
		return 1
	}

	written := false
	if write && changed {
		if err := os.WriteFile(path, []byte(formatted), 0o644); err != nil {
			if jsonOut {
				writeJSON(map[string]string{"path": path, "error": err.Error()})
			} else {
				fmt.Fprintln(os.Stderr, "写回失败:", err)
			}
			return 1
		}
		written = true
	}

	if jsonOut {
		writeJSON(&FormatResult{Path: path, Changed: changed, Written: written, Formatted: formatted})
		return 0
	}

	if write {
		if written {
			fmt.Fprintln(cliStdout, "词库已格式化并写回："+path)
		} else {
			fmt.Fprintln(cliStdout, "当前词库已是最佳缩进")
		}
		return 0
	}

	fmt.Fprint(cliStdout, formatted)
	if !strings.HasSuffix(formatted, "\n") {
		fmt.Fprintln(cliStdout)
	}
	return 0
}

// printRunResult 以人类可读形式输出 -run 结果：正文走标准输出，诊断走标准错误。
func printRunResult(res *RunResult) {
	if res.CompileError != "" {
		fmt.Fprintln(os.Stderr, "执行失败："+res.CompileError)
		printWarnings(os.Stderr, res.Warnings)
		return
	}
	if res.Output != "" {
		fmt.Fprint(cliStdout, res.Output)
		if !strings.HasSuffix(res.Output, "\n") {
			fmt.Fprintln(cliStdout)
		}
	}
	if res.FellBack {
		fmt.Fprintln(os.Stderr, triggerMissHint(res.Trigger))
	}
	if res.TimedOut {
		fmt.Fprintln(os.Stderr, "执行超时，已中断")
	}
	if len(res.Warnings) > 0 {
		printWarnings(os.Stderr, res.Warnings)
	}
}

// printWarnings 逐条打印编译诊断，带来源文件与行号。
func printWarnings(w io.Writer, warns []dto.BuildWarning) {
	for _, item := range warns {
		level := "警告"
		if item.Level == "error" {
			level = "错误"
		}
		if item.File != "" {
			fmt.Fprintf(w, "[%s] %s 第 %d 行：%s\n", level, item.File, item.Line, item.Text)
		} else {
			fmt.Fprintf(w, "[%s] 第 %d 行：%s\n", level, item.Line, item.Text)
		}
	}
}

// triggerMissHint 返回触发词未命中任何词条时的提示文本。
func triggerMissHint(trigger string) string {
	if strings.TrimSpace(trigger) == "" {
		return "触发词为空，未命中任何词条"
	}
	return "触发词「" + trigger + "」未命中任何词条"
}

// argAt 安全取第 i 个参数并去除首尾空白，越界时返回空串。
func argAt(args []string, i int) string {
	if i < len(args) {
		return strings.TrimSpace(args[i])
	}
	return ""
}

// writeJSON 以缩进格式把结果写入标准输出，便于脚本与外部 AI 解析。
func writeJSON(v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "输出 JSON 失败:", err)
		return
	}
	fmt.Fprintln(cliStdout, string(b))
}
