package run

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// chdirToAppWin 切换到 nebula/app/win（NebulaData 所在），基于源码文件位置计算绝对路径，多次调用幂等
func chdirToAppWin() {
	_, file, _, _ := runtime.Caller(0)
	winDir := filepath.Join(filepath.Dir(file), "..", "app", "win")
	if err := os.Chdir(winDir); err != nil {
		panic(err)
	}
}

func Test_log(t *testing.T) {
	t.Run("build_dic", func(t *testing.T) {
		chdirToAppWin()
		text := `开头
		#引入=f
		
		Main
		$test ok$

		[F]test a b=ok2 #{
		%a%
		
		>
		
		$b%
		}#

		[L]内部
		ok` + "\r\n" + `3

		`
		r := BuildDic("test.n", text)
		debugLog.Infof("===========词库===========")
		debugLog.Infof("%v", utils.AnyToString(r))
		debugLog.Infof("===========结尾===========")
	})
}

// TestParseImportLine 引入行的两种写法解析：#引入= 与 $引入 函数形式等价。
func TestParseImportLine(t *testing.T) {
	cases := []struct {
		line    string
		varName string
		target  string
		ok      bool
	}{
		{"#引入=@QQBot", "", "@QQBot", true},
		{"$引入 @QQBot$", "", "@QQBot", true},
		{"#引入=dic/*", "", "dic/*", true},
		{"$引入 dic/*$", "", "dic/*", true},
		{"变量:#引入=dic/test.n", "", "", false},
		{"变量:$引入 dic/test.n$", "变量", "dic/test.n", true},
		{"$引入$", "", "", false},
		{"$引入  $", "", "", false},
		{"$执行词库 private/test.n Main$", "", "", false},
	}
	for _, c := range cases {
		varName, target, ok := parseImportLine(c.line)
		if varName != c.varName || target != c.target || ok != c.ok {
			t.Fatalf("%q：got (%q,%q,%v) want (%q,%q,%v)", c.line, varName, target, ok, c.varName, c.target, c.ok)
		}
	}
}

// waitForCacheFile 等待异步写盘完成（缓存文件已原子替换到位）。
func waitForCacheFile(t *testing.T, dicPath string) {
	t.Helper()
	p := dicCachePath(dicCacheDirFor(dicPath), importFilePath(dicPath))
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(p); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("等待缓存写入超时：%v", p)
}

// TestDicCompileCache 验证磁盘编译缓存的命中与失效。
func TestDicCompileCache(t *testing.T) {
	chdirToAppWin()

	// 词库编译缓存默认关闭，测试需显式开启并在结束后还原
	dto.ServerConfig.DicCache = true
	defer func() { dto.ServerConfig.DicCache = false }()

	const path = "cache_test_unique.n"
	// 测试收尾清理缓存文件，避免每次跑测试都在真实数据目录残留 .dic_cache 垃圾文件
	defer os.Remove(dicCachePath(dicCacheDirFor(path), importFilePath(path)))
	text1 := "Main\n缓存测试内容1"
	text2 := "Main\n缓存测试内容2"

	// 清理旧缓存，保证从干净状态开始
	_ = os.Remove(dicCachePath(dicCacheDirFor(path), importFilePath(path)))

	r1 := BuildDic(path, text1)
	if r1 == nil || len(r1.Dic) == 0 {
		t.Fatalf("首次编译失败")
	}

	// 等待异步写盘完成，再验证缓存命中
	waitForCacheFile(t, path)

	// 相同内容再次编译应命中缓存，词条与函数表结果一致
	r2 := BuildDic(path, text1)
	if !reflect.DeepEqual(r1.Dic, r2.Dic) {
		t.Fatalf("缓存命中后词条结果不一致")
	}
	if !reflect.DeepEqual(r1.DicFuncs, r2.DicFuncs) {
		t.Fatalf("缓存命中后函数表结果不一致")
	}

	// 内容变化后缓存应失效，词条更新
	r3 := BuildDic(path, text2)
	if len(r3.Dic) == 0 || reflect.DeepEqual(r1.Dic, r3.Dic) {
		t.Fatalf("内容变化后缓存未失效")
	}
	if r3.Dic[0].Text[0] != "缓存测试内容2" {
		t.Fatalf("内容变化后词条未更新，got=%v", r3.Dic[0].Text)
	}
}

// TestListAndRemoveDicCache 验证缓存列表能反推出主词库路径，且单条清理按文件名生效、
// 非法文件名（路径穿越）被拒绝。
func TestListAndRemoveDicCache(t *testing.T) {
	chdirToAppWin()

	dto.ServerConfig.DicCache = true
	defer func() { dto.ServerConfig.DicCache = false }()

	const path = "cache_list_test_unique.n"
	name := dicHash(importFilePath(path)) + ".gob"
	defer os.Remove(dicCachePath(dicCacheDirFor(path), importFilePath(path)))

	BuildDic(path, "Main\n缓存列表测试")
	waitForCacheFile(t, path)

	var found *DicCacheItem
	for _, item := range ListDicCache() {
		if item.Name == name {
			found = &item
			break
		}
	}
	if found == nil {
		t.Fatalf("缓存列表中未找到 %v", name)
	}
	if found.DicPath != importFilePath(path) {
		t.Fatalf("主词库路径反推不符，got=%q want=%q", found.DicPath, importFilePath(path))
	}
	if found.Size <= 0 || found.Deps == 0 {
		t.Fatalf("缓存条目信息不完整：%+v", *found)
	}

	// 路径穿越或非缓存文件名必须被拒绝
	for _, bad := range []string{"../secret.gob", "a/b.gob", "noext", ""} {
		if err := RemoveDicCache(bad); err == nil {
			t.Fatalf("非法文件名 %q 应被拒绝", bad)
		}
	}

	if err := RemoveDicCache(name); err != nil {
		t.Fatalf("清理缓存失败：%v", err)
	}
	if _, err := os.Stat(dicCachePath(dicCacheDirFor(path), importFilePath(path))); !os.IsNotExist(err) {
		t.Fatalf("缓存文件应已被删除")
	}
	// 重复清理视为成功（文件已不存在）
	if err := RemoveDicCache(name); err != nil {
		t.Fatalf("重复清理应成功，实际：%v", err)
	}
}

// TestBuildDicHeadBlockNoTrigger 无触发词、无空行、无末尾换行的头部循环框：
// 整篇应按头部初始化脚本解析，不应把首行当触发词、把 <循环 当多余关闭标记告警。
func TestBuildDicHeadBlockNoTrigger(t *testing.T) {
	chdirToAppWin()

	const text = "循环>i=10\n    a\n    <循环"
	for _, src := range []string{text, text + "\n"} {
		r := BuildDic("head_block_test_unique.n", src)
		if len(r.Warnings) != 0 {
			t.Fatalf("不应有告警，实际：%v", warningsText(r.Warnings))
		}
		mw := r.LifecycleFunc(dto.MiddlewareTrigger)
		if mw == nil || len(r.Dic) != 0 {
			t.Fatalf("头部应并入 _中间件 函数且不产生正文词条，实际 Dic=%v", r.Dic)
		}
		want := []string{"循环>i=10", "a", "<循环"}
		if !reflect.DeepEqual(mw.Text, want) {
			t.Fatalf("头部解析不符，got=%v want=%v", mw.Text, want)
		}
	}
}

// TestBuildDicHeadFuncCallNoTrigger 无触发词、无空行、无末尾换行的头部函数调用行：
// 整篇应按头部初始化脚本解析（$执行词库$ 等函数调用只在头部生效），
// 不应把首行当成触发词导致运行没反应。覆盖全部「执行词库」相关函数及常见函数调用形态。
func TestBuildDicHeadFuncCallNoTrigger(t *testing.T) {
	chdirToAppWin()

	cases := []string{
		"$执行词库 private/test.n Main$",
		"$执行词库文件 private/test.n Main$",
		"$执行PHP网页词库 <p>你好</p>$",
		"$执行PHP网页词库文件 private/test.wn$",
		"$执行网页词库 <p>你好</p>$",
		"$执行网页词库文件 private/test.wn$",
		"$重定向触发词 新触发词$",
		"$设置工作目录 NebulaData$",
	}
	for _, text := range cases {
		for _, src := range []string{text, text + "\n"} {
			r := BuildDic("head_func_call_test_unique.n", src)
			mw := r.LifecycleFunc(dto.MiddlewareTrigger)
			if mw == nil || len(r.Dic) != 0 {
				t.Fatalf("%q：头部应并入 _中间件 函数且不产生正文词条，实际 Dic=%v", text, r.Dic)
			}
			want := []string{text}
			if !reflect.DeepEqual(mw.Text, want) {
				t.Fatalf("%q：头部解析不符，got=%v want=%v", text, mw.Text, want)
			}
		}
	}
}

// TestBuildDicHeadOverriddenByMiddleware 头部 与 [f]_中间件 二选一：已定义 [f]_中间件 时函数覆盖头部，
// 头部运行时语句被忽略（不并入 _中间件），并产生一条「覆盖」警告。
func TestBuildDicHeadOverriddenByMiddleware(t *testing.T) {
	chdirToAppWin()

	const text = "循环>i=1\n    a\n\n[f]_中间件\n    b\n\nMain\n    ok"
	r := BuildDic("head_middleware_override_test.n", text)
	if !containsText(r.Warnings, "覆盖") {
		t.Fatalf("头部与 _中间件 同时使用应产生「覆盖」警告，实际：%v", warningsText(r.Warnings))
	}
	mw := r.LifecycleFunc(dto.MiddlewareTrigger)
	if mw == nil {
		t.Fatalf("应存在 _中间件 函数")
	}
	// 头部运行时语句被覆盖忽略，_中间件 只含函数自身内容
	want := []string{"b"}
	if !reflect.DeepEqual(mw.Text, want) {
		t.Fatalf("头部应被覆盖，_中间件 只含函数内容，got=%v want=%v", mw.Text, want)
	}
}

// TestUnusedFuncAcrossImport 验证「函数未使用」检查的当前文件口径：
// 经 #引入= 合并进来的库函数不算当前文件的函数，不在编译主文件时告警；
// 但引入链上任一文件里的调用都算已使用，主文件函数被库文件调用时不应告警。
func TestUnusedFuncAcrossImport(t *testing.T) {
	chdirToAppWin()
	// 词库文件路径按进程工作目录解析，切到 NebulaData 才能让 #引入= 读到 private/ 下的测试库。
	prevDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("获取工作目录失败：%v", err)
	}
	if err := os.Chdir(filepath.Join(prevDir, "NebulaData")); err != nil {
		t.Fatalf("切换工作目录失败：%v", err)
	}
	defer os.Chdir(prevDir)

	const (
		libName  = "unused_func_lib_test_unique.n"
		mainName = "unused_func_main_test_unique.n"
	)
	libPath := filepath.Join("private", libName)
	// 库函数 定义后未被调用（应归属库文件、不在主文件告警）；
	// 被主文件调用 只被库文件正文调用（说明引入链上的调用算已使用）。
	libText := "[函数]库函数\n    ok\n\n[函数]被主文件调用\n    $主文件函数$\n"
	if err := os.WriteFile(libPath, []byte(libText), 0o644); err != nil {
		t.Fatalf("写入测试词库失败：%v", err)
	}
	defer os.Remove(libPath)

	mainText := "#引入=" + libName + "\n\n[函数]主文件函数\n    ok\n\n[函数]主文件未用函数\n    ok\n\nMain\n    ok"
	r := BuildDic(mainName, mainText)
	if containsText(r.Warnings, "函数未使用：库函数") {
		t.Fatalf("被引入的库函数不应在编译主文件时告警，实际：%v", warningsText(r.Warnings))
	}
	if containsText(r.Warnings, "函数未使用：主文件函数") {
		t.Fatalf("被引入文件调用过的函数应算已使用，实际：%v", warningsText(r.Warnings))
	}
	if !containsText(r.Warnings, "函数未使用：主文件未用函数") {
		t.Fatalf("当前文件里定义且未被调用的函数应告警，实际：%v", warningsText(r.Warnings))
	}

	// 主文件调用库函数后同样不产生未使用告警。
	r2 := BuildDic(mainName, "#引入="+libName+"\n\nMain\n    $库函数$")
	if containsText(r2.Warnings, "函数未使用") {
		t.Fatalf("库函数已被调用且不属于当前文件，不应有未使用告警，实际：%v", warningsText(r2.Warnings))
	}
}

// TestContinuePlacementAcrossLayouts 验证 $继续执行$ 位置检查在真实编译链路上的效果：
// 正文触发词下正常，落在 [函数] 或头部（被并入 _中间件）时编译即报错。
func TestContinuePlacementAcrossLayouts(t *testing.T) {
	chdirToAppWin()

	// 正文触发词（Main）里调用：合法，不报错。（前置空行避免 Main 被当成头部）
	body := BuildDic("continue_body_test_unique.n", "\nMain\n$继续执行$\n\nMain\nok")
	if containsText(body.Warnings, "继续执行：仅允许") {
		t.Fatalf("正文触发词下调用 $继续执行$ 不应报错，实际：%v", warningsText(body.Warnings))
	}

	// [函数] 里调用：应报错。
	fn := BuildDic("continue_func_test_unique.n", "\n[函数]测试函数\n    $继续执行$\n\nMain\n    $测试函数$")
	if !containsText(fn.Warnings, "继续执行：仅允许在正文触发词下使用") {
		t.Fatalf("[函数] 中调用 $继续执行$ 应报错，实际：%v", warningsText(fn.Warnings))
	}

	// 头部（首行到第一个空行之间）内容会被并入 _中间件，同样不属于正文触发词：应报错。
	// 这也是「[f]a + $继续执行$ 被整体当成头部」的真实场景。
	head := BuildDic("continue_head_test_unique.n", "[f]a\n$继续执行$\n")
	if !containsText(head.Warnings, "继续执行：仅允许在正文触发词下使用") {
		t.Fatalf("头部/中间件里的 $继续执行$ 应报错，实际：%v", warningsText(head.Warnings))
	}
}
