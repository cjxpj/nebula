// 独立进程扩展示例（Go）。
//
// 主程序按同目录下的 plugin.json 启动本程序，并通过环境变量 NEBULA_PLUGIN_PORT
// 注入监听端口。本程序监听该端口，用「换行分隔的 JSON」协议与主程序通信（每行一条消息）：
//
//	请求：{"id":1,"type":"list"}
//	响应：{"id":1,"type":"list","funcs":[{"name":"Go示例加法","l":"2"}]}
//	请求：{"id":2,"type":"call","fn":"Go示例加法","args":["1","2"]}
//	响应：{"id":2,"type":"call","result":"3"}
//	请求：{"id":3,"type":"call","fn":"Go示例报错","args":[]}
//	响应：{"id":3,"type":"call","error":"这是一个测试错误"}
//	请求：{"id":0,"type":"close"}
//
// 主程序对单个扩展串行发请求（收到上一条响应才发下一条），本程序无需处理并发。
//
// 用法：把本目录整体复制到 <数据目录>/private/plugins/ 下，重启主程序即可。
// 生产环境建议先 `go build -o ext-go.exe`，再把 plugin.json 的 command 改为 ext-go.exe。
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
)

// ipcRequest 主程序发来的请求。
type ipcRequest struct {
	ID   int      `json:"id"`
	Type string   `json:"type"` // list / call / close
	Fn   string   `json:"fn,omitempty"`
	Args []string `json:"args,omitempty"`
}

// ipcFunc 本扩展注册的字典函数。
type ipcFunc struct {
	Name string `json:"name"`
	L    string `json:"l"`
}

// ipcResponse 返回给主程序的响应。
type ipcResponse struct {
	ID     int       `json:"id"`
	Type   string    `json:"type"`
	Funcs  []ipcFunc `json:"funcs,omitempty"`
	Result string    `json:"result,omitempty"`
	Error  string    `json:"error,omitempty"`
}

// funcs 本扩展对外提供的函数列表。
var funcs = []ipcFunc{
	{Name: "Go示例测试", L: "0"},
	{Name: "Go示例回显", L: "1"},
	{Name: "Go示例加法", L: "2"},
	{Name: "Go示例报错", L: "0"},
}

func main() {
	port := os.Getenv("NEBULA_PLUGIN_PORT")
	if port == "" {
		log.Fatal("缺少环境变量 NEBULA_PLUGIN_PORT")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		log.Fatalf("监听端口失败：%v", err)
	}
	defer ln.Close()

	conn, err := ln.Accept()
	if err != nil {
		log.Fatalf("接受连接失败：%v", err)
	}
	defer conn.Close()

	r := bufio.NewReader(conn)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return // 主程序关闭了连接
		}
		var req ipcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			return
		}
		if req.Type == "close" {
			return
		}
		resp := handle(req)
		data, err := json.Marshal(resp)
		if err != nil {
			return
		}
		if _, err := conn.Write(append(data, '\n')); err != nil {
			return
		}
	}
}

// handle 处理一条请求，返回响应。
func handle(req ipcRequest) ipcResponse {
	resp := ipcResponse{ID: req.ID, Type: req.Type}
	switch req.Type {
	case "list":
		resp.Funcs = funcs
	case "call":
		result, err := call(req.Fn, req.Args)
		if err != nil {
			resp.Error = err.Error()
		} else {
			resp.Result = result
		}
	}
	return resp
}

// call 分发并执行具体的扩展函数。
func call(name string, args []string) (string, error) {
	switch name {
	case "Go示例测试":
		return "独立进程扩展加载成功（Go）", nil
	case "Go示例回显":
		return arg(args, 0), nil
	case "Go示例加法":
		return formatNum(toFloat(arg(args, 0)) + toFloat(arg(args, 1))), nil
	case "Go示例报错":
		return "", fmt.Errorf("这是一个测试错误")
	}
	return "", fmt.Errorf("未知函数 %s", name)
}

// arg 取第 i 个参数，越界返回空串。
func arg(args []string, i int) string {
	if i >= 0 && i < len(args) {
		return args[i]
	}
	return ""
}

// toFloat 宽松地把字符串解析为浮点数，失败按 0 处理。
func toFloat(s string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return f
}

// formatNum 以最简形式输出浮点数（与 C 的 %g 一致）。
func formatNum(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}
