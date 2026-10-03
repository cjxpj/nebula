package extloader

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// 独立进程扩展（IPC 后端）
//
// 扩展以独立可执行程序（任意语言）形式存在，主程序按扩展目录下的 plugin.json 启动它，
// 并通过本地 TCP(127.0.0.1) 与其交换「换行分隔的 JSON」消息（每行一条）。
//
// 端口由主程序自动分配后经环境变量 NEBULA_PLUGIN_PORT 注入，扩展须监听该端口。
// 消息格式：
//
//	请求：{"id":1,"type":"list"}
//	      {"id":2,"type":"call","fn":"扩展加法","args":["1","2"]}
//	      {"id":0,"type":"close"}
//	响应：{"id":1,"type":"list","funcs":[{"name":"扩展加法","l":"2"}]}
//	      {"id":2,"type":"call","result":"3"}
//	      {"id":2,"type":"call","error":"出错信息"}
//
// 主程序对单个扩展串行发请求、收到对应 id 的响应才发下一条，扩展无需处理并发。

// ipcManifestName 独立进程扩展的清单文件名。
const ipcManifestName = "plugin.json"

// ipcDialTimeout 启动扩展后等待其监听端口的超时。解释型/编译型（如 python、go run）
// 首次启动可能较慢，故给较宽的窗口；进程若提前退出会立刻返回，不会白等。
const ipcDialTimeout = 10 * time.Second

// ipcHandshakeTimeout 握手（取函数列表）的超时。
const ipcHandshakeTimeout = 5 * time.Second

// pluginManifest 独立进程扩展的清单（private/plugins/<目录>/plugin.json）。
type pluginManifest struct {
	Name    string            `json:"name"`    // 可选，仅用于日志
	Command string            `json:"command"` // 必填：可执行文件或解释器（相对路径优先相对插件目录）
	Args    []string          `json:"args"`    // 可选：启动参数
	Env     map[string]string `json:"env"`     // 可选：附加环境变量
	Port    int               `json:"port"`    // 可选：固定监听端口，0 表示由主程序自动分配
	Timeout int               `json:"timeout"` // 可选：单次调用超时（毫秒），0 表示不限制
}

// ipcRequest 主程序发往扩展的请求。
type ipcRequest struct {
	ID   int      `json:"id"`
	Type string   `json:"type"` // list / call / close
	Fn   string   `json:"fn,omitempty"`
	Args []string `json:"args,omitempty"`
}

// ipcFunc 扩展声明的字典函数。
type ipcFunc struct {
	Name string `json:"name"`
	L    string `json:"l"`
}

// ipcResponse 扩展返回给主程序的响应。
type ipcResponse struct {
	ID     int       `json:"id"`
	Type   string    `json:"type"`
	Funcs  []ipcFunc `json:"funcs,omitempty"`
	Result string    `json:"result,omitempty"`
	Error  string    `json:"error,omitempty"`
}

// ipcLib 独立进程扩展句柄，实现 nativeLib。
type ipcLib struct {
	dir      string
	label    string
	manifest pluginManifest

	cmd    *exec.Cmd
	waitCh chan error // cmd.Wait 的返回，close 时用于等待进程退出

	conn net.Conn
	r    *bufio.Reader

	funcs   []ipcFunc
	timeout time.Duration
	nextID  int

	mu     sync.Mutex
	closed bool
}

// openIPC 读取扩展目录下的 plugin.json，返回尚未启动的独立进程扩展句柄；
// 进程启动、连接与握手在 init 中完成。
func openIPC(dir string) (nativeLib, error) {
	m, err := loadManifest(dir)
	if err != nil {
		return nil, err
	}
	// 端口只在此处分配一次并写回 manifest，保证 start 注入、dial 连接使用同一端口。
	if m.Port <= 0 {
		port, err := allocPort()
		if err != nil {
			return nil, fmt.Errorf("分配扩展端口失败：%w", err)
		}
		m.Port = port
	}
	label := m.Name
	if label == "" {
		label = filepath.Base(dir)
	}
	return &ipcLib{
		dir:      dir,
		label:    label,
		manifest: m,
		timeout:  time.Duration(m.Timeout) * time.Millisecond,
	}, nil
}

// loadManifest 读取并解析扩展目录下的 plugin.json。
func loadManifest(dir string) (pluginManifest, error) {
	path := filepath.Join(dir, ipcManifestName)
	data, err := os.ReadFile(path)
	if err != nil {
		return pluginManifest{}, fmt.Errorf("读取 %s 失败：%w", ipcManifestName, err)
	}
	var m pluginManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return pluginManifest{}, fmt.Errorf("解析 %s 失败：%w", ipcManifestName, err)
	}
	if m.Command == "" {
		return pluginManifest{}, fmt.Errorf("%s 缺少 command 字段", ipcManifestName)
	}
	return m, nil
}

// allocPort 让操作系统分配一个空闲的本地 TCP 端口。
func allocPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// resolveCommand 解析可执行文件路径：绝对路径原样使用；否则优先相对插件目录查找，
// 找不到再回退到 PATH（便于直接写 python、node 等解释器）。
func resolveCommand(dir, command string) (string, error) {
	if filepath.IsAbs(command) {
		return command, nil
	}
	cand := filepath.Join(dir, command)
	if fileExists(cand) {
		return cand, nil
	}
	if p, err := exec.LookPath(command); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("找不到可执行文件 %s", command)
}

// init 启动扩展进程、连接其端口并完成函数列表握手。
func (l *ipcLib) init() error {
	if err := l.start(); err != nil {
		return err
	}
	conn, err := l.dial()
	if err != nil {
		return err
	}
	l.conn = conn
	l.r = bufio.NewReader(conn)
	funcs, err := l.list()
	if err != nil {
		return err
	}
	l.funcs = funcs
	return nil
}

// start 依据清单启动扩展进程，并通过 NEBULA_PLUGIN_PORT 注入监听端口。
func (l *ipcLib) start() error {
	exe, err := resolveCommand(l.dir, l.manifest.Command)
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, l.manifest.Args...)
	cmd.Dir = l.dir
	env := append(os.Environ(), "NEBULA_PLUGIN_PORT="+strconv.Itoa(l.manifest.Port))
	for k, v := range l.manifest.Env {
		env = append(env, k+"="+v)
	}
	cmd.Env = env
	// 扩展进程的 stdout/stderr 与主程序共用日志管道，便于在日志中查看其输出
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动扩展进程失败：%w", err)
	}
	l.cmd = cmd
	l.waitCh = make(chan error, 1)
	go func() { l.waitCh <- cmd.Wait() }()
	return nil
}

// dial 在超时内重试连接扩展端口；进程提前退出时立即返回错误。
func (l *ipcLib) dial() (net.Conn, error) {
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(l.manifest.Port))
	deadline := time.Now().Add(ipcDialTimeout)
	for {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			return conn, nil
		}
		select {
		case werr := <-l.waitCh:
			// 进程已退出：把返回值放回通道，供 close 再读一次
			l.waitCh <- werr
			return nil, fmt.Errorf("扩展进程已退出：%w", werr)
		default:
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("连接扩展端口超时：%w", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// list 握手：向扩展索取其注册的函数列表。
func (l *ipcLib) list() ([]ipcFunc, error) {
	saved := l.timeout
	if saved <= 0 {
		l.timeout = ipcHandshakeTimeout
	}
	resp, err := l.roundTrip(ipcRequest{Type: "list"})
	l.timeout = saved
	if err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}
	return resp.Funcs, nil
}

func (l *ipcLib) count() int {
	return len(l.funcs)
}

func (l *ipcLib) name(index int) string {
	if index < 0 || index >= len(l.funcs) {
		return ""
	}
	return l.funcs[index].Name
}

func (l *ipcLib) l(index int) string {
	if index < 0 || index >= len(l.funcs) {
		return ""
	}
	return l.funcs[index].L
}

// call 调用扩展中的函数。
func (l *ipcLib) call(name string, args []string) (string, error) {
	resp, err := l.roundTrip(ipcRequest{Type: "call", Fn: name, Args: args})
	if err != nil {
		return "", err
	}
	if resp.Error != "" {
		return "", errors.New(resp.Error)
	}
	return resp.Result, nil
}

// close 通知扩展退出并回收进程：先发 close 消息，再关闭连接；
// 若扩展未在宽限期内自行退出，则强制结束进程。
func (l *ipcLib) close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	conn := l.conn
	l.mu.Unlock()

	if conn != nil {
		_ = conn.SetDeadline(time.Now().Add(500 * time.Millisecond))
		if data, err := json.Marshal(ipcRequest{Type: "close"}); err == nil {
			_, _ = conn.Write(append(data, '\n'))
		}
		_ = conn.Close()
	}

	if l.cmd != nil && l.cmd.Process != nil && l.waitCh != nil {
		select {
		case <-l.waitCh:
		case <-time.After(2 * time.Second):
			_ = l.cmd.Process.Kill()
			<-l.waitCh
		}
	}
	return nil
}

// roundTrip 发送一条请求并读取对应 id 的响应。发送与读取均受 timeout 约束（0 表示不限时）。
func (l *ipcLib) roundTrip(req ipcRequest) (*ipcResponse, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil, errors.New("扩展进程已关闭")
	}
	l.nextID++
	req.ID = l.nextID

	if l.timeout > 0 {
		_ = l.conn.SetDeadline(time.Now().Add(l.timeout))
	} else {
		_ = l.conn.SetDeadline(time.Time{})
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := l.conn.Write(append(data, '\n')); err != nil {
		return nil, fmt.Errorf("发送请求失败：%w", err)
	}
	line, err := l.r.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("读取扩展响应失败：%w", err)
	}
	var resp ipcResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("扩展响应不是合法 JSON：%w", err)
	}
	if resp.ID != req.ID {
		return nil, fmt.Errorf("扩展响应 id 不匹配（期望 %d，收到 %d）", req.ID, resp.ID)
	}
	return &resp, nil
}
