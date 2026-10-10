package dic_server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cjxpj/nebula/debugLog"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

const maxServerLogLines = 20000

// serverLogPageSize 面板回放日志时每页默认返回的行数
const serverLogPageSize = 300

// serverStarted 标记服务器是否已启动（由 Start 置位）。
// 仅当服务器真正运行后才把终端输出落盘：本包的 init 会在被导入时即接管 stdout，
// 若不设此闸门，go test 等「只导入本包、不启动服务器」的场景会把测试输出当作
// 服务日志写入相对工作目录的 database/log，污染源码树。
var serverStarted atomic.Bool

// serverLogDir 返回服务端日志目录（应用储存目录下的日志目录，绝对路径）。
// 具体子路径由 utils.SetLogDir 决定，默认 database/log。
func serverLogDir() string {
	return utils.GetLogDir()
}

// serverLogFq 返回当天对应的日志文件句柄（database/log/YYYYMMDD.txt，按天区分，不再建子目录）
func serverLogFq() *utils.FileQueue {
	now := time.Now()
	return utils.NewFileQueue(filepath.Join(serverLogDir(), now.Format("20060102")+".txt"))
}

// init 重定向标准输出，监听终端全部信息：写入日志文件并实时推送到 OPUI 面板
func init() {
	originalStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return
	}
	os.Stdout = w

	go func() {
		reader := bufio.NewReader(r)
		for {
			raw, readErr := reader.ReadString('\n')
			if len(raw) > 0 {
				// 回显到真实终端：还原 debugLog 转义的控制字符，让多行内容正常换行显示
				_, _ = originalStdout.WriteString(debugLog.UnescapeControlChars(raw))
				processServerLogLine(raw)
			}
			if readErr != nil {
				return
			}
		}
	}()
}

// startupLogDir 返回启动日志的自定义落盘目录。
// 由启动词库通过 $线程变量 _日志目录_ <目录>$ 设置，留空表示关闭启动日志落盘（默认）。
func startupLogDir() string {
	return strings.TrimSpace(dto.GV.GetStr("_日志目录_"))
}

// startupLogFq 返回启动日志当天文件句柄（启动日志目录/YYYYMMDD.txt），命名与常规日志一致。
func startupLogFq() *utils.FileQueue {
	now := time.Now()
	return utils.NewFileQueue(filepath.Join(startupLogDir(), now.Format("20060102")+".txt"))
}

// processServerLogLine 处理一行终端输出：写入日志文件，仅当有客户端正在查看实时终端时才推送
func processServerLogLine(raw string) {
	line := strings.TrimRight(raw, "\r\n")
	if line == "" {
		return
	}
	level := parseLogLevel(line)

	// 启动阶段默认不落盘，避免自动创建 database/log；若启动词库通过线程变量
	// _日志目录_ 指定了日志目录，则写入该目录下的当天日志文件，留空默认关闭。
	if utils.InStartupMode() {
		if startupLogDir() != "" {
			startupLogFq().AppendToFile(line + "\n")
		}
	} else if serverStarted.Load() {
		serverLogFq().AppendToFile(line + "\n")
	}

	// 无客户端查看实时终端时跳过序列化与推送，避免无谓的网络开销
	if !hasServerLogSubscriber() {
		return
	}
	data, _ := json.Marshal(map[string]string{
		"type":  "server_log",
		"level": level,
		"line":  line,
	})
	broadcastServerLog(data)
}

// listServerLogFiles 列出 database/log 下所有 .txt 日志文件，按路径升序（即时间升序）
func listServerLogFiles() []string {
	var files []string
	filepath.Walk(serverLogDir(), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), ".txt") {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files
}

// readFileTailLines 流式读取文件末尾最多 need 行（按时间升序返回）。
// 使用固定容量环形缓冲，内存占用始终有界，避免大日志文件一次性加载导致内存暴涨。
func readFileTailLines(path string, need int) []string {
	if need <= 0 {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	reader := bufio.NewReaderSize(f, 64*1024)
	ring := make([]string, need)
	head := 0   // 下一个写入位置
	filled := 0 // 环形缓冲中已保存的行数
	for {
		raw, readErr := reader.ReadString('\n')
		if len(raw) > 0 {
			line := strings.TrimRight(raw, "\r\n")
			if line != "" {
				ring[head] = line
				head = (head + 1) % need
				if filled < need {
					filled++
				}
			}
		}
		if readErr != nil {
			break
		}
	}

	// 从最旧到最新依次取出，保持时间升序
	start := head - filled
	if start < 0 {
		start += need
	}
	lines := make([]string, 0, filled)
	for i := 0; i < filled; i++ {
		lines = append(lines, ring[(start+i)%need])
	}
	return lines
}

// collectRecentLogLines 收集最近的日志行（时间升序），最多 maxLines 行
func collectRecentLogLines(maxLines int) []string {
	files := listServerLogFiles()
	var lines []string
	for i := len(files) - 1; i >= 0 && len(lines) < maxLines; i-- {
		need := maxLines - len(lines)
		fileLines := readFileTailLines(files[i], need)
		// 前置到结果前，保持整体时间升序
		lines = append(fileLines, lines...)
	}
	return lines
}

// readServerLogs 分页读取最近日志：跳过最新的 skip 行，返回其后 limit 行；hasMore 表示是否还有更早的日志
func readServerLogs(limit, skip int) ([]map[string]string, bool) {
	if limit <= 0 {
		limit = serverLogPageSize
	}
	if skip < 0 {
		skip = 0
	}
	lines := collectRecentLogLines(maxServerLogLines)
	total := len(lines)
	end := total - skip
	if end <= 0 {
		return []map[string]string{}, false
	}
	start := max(end-limit, 0)
	logs := make([]map[string]string, 0, end-start)
	for _, l := range lines[start:end] {
		logs = append(logs, map[string]string{"level": parseLogLevel(l), "line": l})
	}
	return logs, start > 0
}

// ClearServerLogs 清空当前（当天）日志文件，不影响历史日期的日志文件
func ClearServerLogs() {
	serverLogFq().DeleteFile()
}

// serverLogKeepDays 启动时保留的日志天数，超过该天数的旧日志文件会被清理
const serverLogKeepDays = 7

// ClearOldServerLogs 清理 database/log 下超过保留天数的旧日志文件（按 YYYYMMDD.txt 文件名判断日期）
func ClearOldServerLogs() {
	cutoff := time.Now().AddDate(0, 0, -serverLogKeepDays)
	for _, path := range listServerLogFiles() {
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if t, err := time.Parse("20060102", name); err == nil && t.Before(cutoff) {
			_ = os.Remove(path)
		}
	}
}

// runTerminalDic 运行专门监听终端触发的词库（默认 private/system/terminal.n，可用 _终端词库路径_ 覆盖）。
// 前端实时终端输入作为触发词运行该词库，输出写入进程 stdout（已被 init 重定向到日志管道），实时回显到终端。
func runTerminalDic(input string) {
	terminalPath := dto.GV.GetStr("_终端词库路径_")
	if terminalPath == "" {
		terminalPath = "private/system/terminal.n"
	}
	dic, err := dic_dto.RunDic(terminalPath)
	if err != nil || dic == nil {
		fmt.Printf("[终端] 终端触发词库不可用: %v\n", err)
		return
	}
	defer dic.Close()

	if res := dic_api.Api.DicRun(dic, input); res != "" {
		fmt.Printf("%v\n", res)
	}
}

// parseLogLevel 从日志行首的 [Level] 提取级别，仅识别已知级别，其余默认 Info
func parseLogLevel(line string) string {
	if len(line) == 0 || line[0] != '[' {
		return "Info"
	}
	end := strings.IndexByte(line, ']')
	if end <= 1 {
		return "Info"
	}
	switch line[1:end] {
	case "Debug", "Info", "Warning", "Error":
		return line[1:end]
	default:
		return "Info"
	}
}

// GetOpuiOnlineClients 返回当前 OPUI 在线用户列表
func GetOpuiOnlineClients() []map[string]any {
	opuiNotifyClientsMu.Lock()
	defer opuiNotifyClientsMu.Unlock()
	list := make([]map[string]any, 0, len(opuiNotifyClients))
	for _, info := range opuiNotifyClients {
		list = append(list, map[string]any{
			"name":   info.IP,
			"type":   "opui",
			"online": true,
			"detail": "已连接 " + formatDuration(time.Since(info.Connected)),
		})
	}
	return list
}

// wsResponseWriter 实现 http.ResponseWriter，用于 WebSocket 消息处理时捕获输出
type wsResponseWriter struct {
	header http.Header
	buf    bytes.Buffer
	code   int
}

func (w *wsResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *wsResponseWriter) Write(b []byte) (int, error) {
	return w.buf.Write(b)
}

func (w *wsResponseWriter) WriteHeader(code int) {
	w.code = code
}

// ---------- IP 黑名单 ----------
