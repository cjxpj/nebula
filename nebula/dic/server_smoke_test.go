package dic

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// compileTestDic 把源码文本编译为「创建服务器」第二参数所需的编译产物（gob 字节）。
func compileTestDic(t *testing.T, src string) []byte {
	t.Helper()
	b := compileServerText(src)
	if b == nil {
		t.Fatalf("编译词库失败: %q", src)
	}
	return b
}

// callDicFn 直接调用字典函数/对象方法（输入第 1 位起为函数参数）
func callDicFn(f dto.DicFunc, args ...any) (any, error) {
	inputs := utils.NewDicInputs()
	list := []any{""}
	list = append(list, args...)
	inputs.Set(list)
	return f.Fn(dto.NewDicInputsWithOutput(nil, nil, &inputs, &dto.SingleValue{}))
}

func smokeGet(t *testing.T, url string) (string, error) {
	t.Helper()
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b), nil
}

func TestServerNewSmoke(t *testing.T) {
	// 等待 init 中异步注册完成
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, ok := funcs.GetFunc("创建服务器"); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("创建服务器函数未完成注册")
		}
		time.Sleep(10 * time.Millisecond)
	}

	f, _ := funcs.GetFunc("创建服务器")

	// 创建后不应立即启动：注册表为空、端口不可访问
	res, err := callDicFn(f, "19991", compileTestDic(t, "/你好页\n你好内容\n"))
	if err != nil {
		t.Fatalf("创建服务器失败: %v", err)
	}
	cls, ok := res.(*dto.DicClass)
	if !ok {
		t.Fatalf("返回类型错误: %T", res)
	}
	if snap := dto.FuncServers.Snapshot(); len(snap) != 0 {
		t.Fatalf("创建后不应立即注册: %+v", snap)
	}
	if _, err := smokeGet(t, "http://127.0.0.1:19991/你好页"); err == nil {
		t.Fatal("创建后端口不应可访问")
	}

	// 启动
	startFn, ok := cls.Fn["启动"]
	if !ok {
		t.Fatal("缺少启动方法")
	}
	if _, err := callDicFn(startFn); err != nil {
		t.Fatalf("启动失败: %v", err)
	}

	// 注册表：启动后应有一台（端口自动补全为 :19991），且不自动成为核心服务器
	snap := dto.FuncServers.Snapshot()
	if len(snap) != 1 || snap[0].Addr != ":19991" || !snap[0].Cors || snap[0].Core {
		t.Fatalf("注册表快照错误: %+v", snap)
	}

	// 请求 1：以 /你好页 为触发词
	body, err := smokeGet(t, "http://127.0.0.1:19991/你好页")
	if err != nil || body != "你好内容" {
		t.Fatalf("请求1失败: %q err=%v", body, err)
	}

	// 设置词库
	upd, ok := cls.Fn["设置词库"]
	if !ok {
		t.Fatal("缺少设置词库方法")
	}
	if _, err := callDicFn(upd, "/第二页\n第二句内容\n"); err != nil {
		t.Fatalf("设置词库失败: %v", err)
	}

	body, err = smokeGet(t, "http://127.0.0.1:19991/第二页")
	if err != nil || body != "第二句内容" {
		t.Fatalf("请求2失败: %q err=%v", body, err)
	}

	// 注册表快照应同步更新词库数据
	if snap := dto.FuncServers.Snapshot(); len(snap) != 1 || snap[0].DicData != "/第二页\n第二句内容\n" {
		t.Fatalf("设置词库后注册表未同步: %+v", snap)
	}

	// 模拟前端编辑（save_func_server）：仅关闭跨域（词库源码不再可通过前端编辑）
	entry := dto.FuncServers.Get(":19991")
	if entry == nil || entry.Update == nil {
		t.Fatal("注册表缺少服务器条目")
	}
	cors := false
	if err := entry.Update(dto.FuncServerUpdate{Cors: &cors}); err != nil {
		t.Fatalf("Update失败: %v", err)
	}

	// 词库保持「设置词库」后的内容，仅跨域关闭
	body, err = smokeGet(t, "http://127.0.0.1:19991/第二页")
	if err != nil || body != "第二句内容" {
		t.Fatalf("关闭跨域后词库应保持: %q err=%v", body, err)
	}
	if snap := dto.FuncServers.Snapshot(); len(snap) != 1 || snap[0].Cors {
		t.Fatalf("跨域更新未同步: %+v", snap)
	}

	// 关闭服务器
	closeFn, ok := cls.Fn["关闭"]
	if !ok {
		t.Fatal("缺少关闭方法")
	}
	if _, err := callDicFn(closeFn); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}

	if snap := dto.FuncServers.Snapshot(); len(snap) != 0 {
		t.Fatalf("关闭后注册表未清空: %+v", snap)
	}

	if _, err := smokeGet(t, "http://127.0.0.1:19991/你好"); err == nil {
		t.Fatal("关闭后端口仍可访问")
	}
}

// TestServerCreateEmptyDic 验证「创建服务器」第二个参数可留空：词库后续通过设置词库补齐。
func TestServerCreateEmptyDic(t *testing.T) {
	f, _ := funcs.GetFunc("创建服务器")

	res, err := callDicFn(f, "19992")
	if err != nil {
		t.Fatalf("创建服务器（词库留空）失败: %v", err)
	}
	cls, ok := res.(*dto.DicClass)
	if !ok {
		t.Fatalf("返回类型错误: %T", res)
	}

	// 留空创建后启动，随后设置词库
	startFn := cls.Fn["启动"]
	if _, err := callDicFn(startFn); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	defer func() { _, _ = callDicFn(cls.Fn["关闭"], nil) }()

	// 词库留空时请求返回空
	body, err := smokeGet(t, "http://127.0.0.1:19992/任意页")
	if err != nil || body != "" {
		t.Fatalf("词库留空请求应返回空: %q err=%v", body, err)
	}

	// 设置词库后正常命中
	setFn := cls.Fn["设置词库"]
	if _, err := callDicFn(setFn, "/补页\n补充内容\n"); err != nil {
		t.Fatalf("设置词库失败: %v", err)
	}
	body, err = smokeGet(t, "http://127.0.0.1:19992/补页")
	if err != nil || body != "补充内容" {
		t.Fatalf("设置词库后请求失败: %q err=%v", body, err)
	}
}

// TestServerCoreManual 验证核心服务器不会自动指定：首个启动的服务器 Core 为 false，
// 必须显式调用「设置核心服务器」后才成为核心，切换时其余服务器自动取消核心。
func TestServerCoreManual(t *testing.T) {
	f, _ := funcs.GetFunc("创建服务器")

	// 启动两台服务器，均不应自动成为核心
	resA, err := callDicFn(f, "19993", compileTestDic(t, "/甲\n甲内容\n"))
	if err != nil {
		t.Fatalf("创建服务器A失败: %v", err)
	}
	clsA, ok := resA.(*dto.DicClass)
	if !ok {
		t.Fatalf("返回类型错误: %T", resA)
	}
	if _, err := callDicFn(clsA.Fn["启动"]); err != nil {
		t.Fatalf("启动A失败: %v", err)
	}
	t.Cleanup(func() { _, _ = callDicFn(clsA.Fn["关闭"]) })

	resB, err := callDicFn(f, "19994", compileTestDic(t, "/乙\n乙内容\n"))
	if err != nil {
		t.Fatalf("创建服务器B失败: %v", err)
	}
	clsB, ok := resB.(*dto.DicClass)
	if !ok {
		t.Fatalf("返回类型错误: %T", resB)
	}
	if _, err := callDicFn(clsB.Fn["启动"]); err != nil {
		t.Fatalf("启动B失败: %v", err)
	}
	t.Cleanup(func() { _, _ = callDicFn(clsB.Fn["关闭"]) })

	for _, s := range dto.FuncServers.Snapshot() {
		if s.Core {
			t.Fatalf("启动后不应自动成为核心服务器: %+v", dto.FuncServers.Snapshot())
		}
	}
	if addr := dto.FuncServers.CoreAddr(); addr != "" {
		t.Fatalf("未设置核心时应返回空串，实际: %q", addr)
	}

	// 显式把 A 设为核心
	if _, err := callDicFn(clsA.Fn["设置核心服务器"]); err != nil {
		t.Fatalf("设置A为核心失败: %v", err)
	}
	if addr := dto.FuncServers.CoreAddr(); addr != ":19993" {
		t.Fatalf("设置A为核心后 CoreAddr 应为 :19993，实际: %q", addr)
	}
	for _, s := range dto.FuncServers.Snapshot() {
		if want := s.Addr == ":19993"; s.Core != want {
			t.Fatalf("核心标记错误: %+v", dto.FuncServers.Snapshot())
		}
	}

	// 切换到 B，A 自动取消核心
	if _, err := callDicFn(clsB.Fn["设置核心服务器"]); err != nil {
		t.Fatalf("设置B为核心失败: %v", err)
	}
	if addr := dto.FuncServers.CoreAddr(); addr != ":19994" {
		t.Fatalf("切换核心到B后 CoreAddr 应为 :19994，实际: %q", addr)
	}
	for _, s := range dto.FuncServers.Snapshot() {
		if want := s.Addr == ":19994"; s.Core != want {
			t.Fatalf("切换后核心标记错误: %+v", dto.FuncServers.Snapshot())
		}
	}
}
