package extloader

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// serveFakePlugin 按换行 JSON 协议处理一个连接上的请求，模拟独立进程扩展。
// 收到 close 或连接断开即返回。
func serveFakePlugin(conn net.Conn, funcs []ipcFunc, call func(req ipcRequest) ipcResponse) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return
		}
		var req ipcRequest
		if json.Unmarshal(line, &req) != nil {
			return
		}
		if req.Type == "close" {
			return
		}
		resp := ipcResponse{ID: req.ID, Type: req.Type}
		switch req.Type {
		case "list":
			resp.Funcs = funcs
		case "call":
			if call != nil {
				resp = call(req)
				resp.ID = req.ID
				resp.Type = req.Type
			}
		}
		data, err := json.Marshal(resp)
		if err != nil {
			return
		}
		if _, err := conn.Write(append(data, '\n')); err != nil {
			return
		}
	}
}

// TestIPCRoundTrip 用内存管道验证 list 握手与 call 调用。
func TestIPCRoundTrip(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	go serveFakePlugin(server, []ipcFunc{{Name: "扩展加法", L: "2"}}, func(req ipcRequest) ipcResponse {
		return ipcResponse{Result: strings.Join(req.Args, ",")}
	})

	l := &ipcLib{conn: client, r: bufio.NewReader(client), timeout: time.Second}

	funcs, err := l.list()
	if err != nil {
		t.Fatalf("list 失败：%v", err)
	}
	if len(funcs) != 1 || funcs[0].Name != "扩展加法" || funcs[0].L != "2" {
		t.Fatalf("函数列表不符：%+v", funcs)
	}

	got, err := l.call("扩展加法", []string{"1", "2"})
	if err != nil {
		t.Fatalf("call 失败：%v", err)
	}
	if got != "1,2" {
		t.Fatalf("call 结果不符：%q", got)
	}
}

// TestIPCErrorResponse 验证扩展返回 error 字段时被透传为错误。
func TestIPCErrorResponse(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	go serveFakePlugin(server, nil, func(req ipcRequest) ipcResponse {
		return ipcResponse{Error: "自定义错误"}
	})

	l := &ipcLib{conn: client, r: bufio.NewReader(client), timeout: time.Second}
	if _, err := l.call("f", nil); err == nil || err.Error() != "自定义错误" {
		t.Fatalf("期望透传错误，实际：%v", err)
	}
}

// TestIPCIDMismatch 验证响应 id 不匹配时报错。
func TestIPCIDMismatch(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	go func() {
		r := bufio.NewReader(server)
		if _, err := r.ReadBytes('\n'); err != nil {
			return
		}
		data, _ := json.Marshal(ipcResponse{ID: 999, Type: "call", Result: "x"})
		_, _ = server.Write(append(data, '\n'))
	}()

	l := &ipcLib{conn: client, r: bufio.NewReader(client), timeout: time.Second}
	if _, err := l.call("f", nil); err == nil || !strings.Contains(err.Error(), "id 不匹配") {
		t.Fatalf("期望 id 不匹配错误，实际：%v", err)
	}
}

// TestLoadManifest 验证 plugin.json 的解析与校验。
func TestLoadManifest(t *testing.T) {
	dir := t.TempDir()

	// 缺 command
	if err := os.WriteFile(filepath.Join(dir, ipcManifestName), []byte(`{"name":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadManifest(dir); err == nil {
		t.Fatal("缺少 command 时应报错")
	}

	// 合法清单
	valid := `{"name":"演示","command":"python","args":["main.py"],"env":{"K":"V"},"timeout":1000}`
	if err := os.WriteFile(filepath.Join(dir, ipcManifestName), []byte(valid), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := loadManifest(dir)
	if err != nil {
		t.Fatalf("解析合法清单失败：%v", err)
	}
	if m.Command != "python" || len(m.Args) != 1 || m.Env["K"] != "V" || m.Timeout != 1000 {
		t.Fatalf("清单字段不符：%+v", m)
	}

	// 非法 JSON
	if err := os.WriteFile(filepath.Join(dir, ipcManifestName), []byte(`{`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadManifest(dir); err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}

// TestOpenIPCPortAlloc 验证 openIPC 会分配端口并写回 manifest，且同一实例端口稳定。
func TestOpenIPCPortAlloc(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ipcManifestName), []byte(`{"command":"python"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := openIPC(dir)
	if err != nil {
		t.Fatalf("openIPC 失败：%v", err)
	}
	l := n.(*ipcLib)
	if l.manifest.Port <= 0 {
		t.Fatalf("应分配有效端口，实际 %d", l.manifest.Port)
	}
	if l.label != filepath.Base(dir) {
		t.Fatalf("label 应回退到目录名，实际 %q", l.label)
	}

	// 固定端口时应原样保留
	dir2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir2, ipcManifestName), []byte(`{"command":"python","port":43210}`), 0o644); err != nil {
		t.Fatal(err)
	}
	n2, err := openIPC(dir2)
	if err != nil {
		t.Fatalf("openIPC 失败：%v", err)
	}
	if got := n2.(*ipcLib).manifest.Port; got != 43210 {
		t.Fatalf("固定端口应保留，实际 %d", got)
	}
}

// TestResolveCommand 验证可执行文件解析：优先插件目录内相对文件。
func TestResolveCommand(t *testing.T) {
	dir := t.TempDir()
	name := "run.bat"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("@echo off\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := resolveCommand(dir, name)
	if err != nil {
		t.Fatalf("解析相对文件失败：%v", err)
	}
	if got != filepath.Join(dir, name) {
		t.Fatalf("应命中插件目录内文件，实际 %q", got)
	}
	if _, err := resolveCommand(dir, "不存在的可执行文件-xyz"); err == nil {
		t.Fatal("找不到时应报错")
	}
}

// TestIPCIntegration 端到端验证：以测试二进制自身充当扩展进程，
// 完整走一遍 start → dial → list 握手 → call → close。
func TestIPCIntegration(t *testing.T) {
	// 子进程分支：作为扩展运行协议服务端。
	if os.Getenv("NEBULA_TEST_PLUGIN") == "1" {
		runSelfAsPlugin()
		return
	}

	exe, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	manifest := `{"name":"自测扩展","command":` + strconv.Quote(exe) + `,` +
		`"args":["-test.run=TestIPCIntegration"],"env":{"NEBULA_TEST_PLUGIN":"1"}}`
	if err := os.WriteFile(filepath.Join(dir, ipcManifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	lib, err := Load(dir)
	if err != nil {
		t.Fatalf("Load 失败：%v", err)
	}
	defer lib.Close()

	if len(lib.Funcs) != 1 || lib.Funcs[0].Name != "扩展加法" {
		t.Fatalf("函数列表不符：%+v", lib.Funcs)
	}
	got, err := lib.Call("扩展加法", []any{1, 2})
	if err != nil {
		t.Fatalf("Call 失败：%v", err)
	}
	if got != "ok:扩展加法" {
		t.Fatalf("Call 结果不符：%v", got)
	}
}

// runSelfAsPlugin 让测试二进制以扩展身份运行：监听注入端口并按协议应答。
func runSelfAsPlugin() {
	port := os.Getenv("NEBULA_PLUGIN_PORT")
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		os.Exit(3)
	}
	conn, err := ln.Accept()
	if err != nil {
		os.Exit(4)
	}
	serveFakePlugin(conn, []ipcFunc{{Name: "扩展加法", L: "2"}}, func(req ipcRequest) ipcResponse {
		return ipcResponse{Result: "ok:" + req.Fn}
	})
}
