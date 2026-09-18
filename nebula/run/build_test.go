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
	p := dicCachePath(importFilePath(dicPath))
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
	defer os.Remove(dicCachePath(importFilePath(path)))
	text1 := "Main\n缓存测试内容1"
	text2 := "Main\n缓存测试内容2"

	// 清理旧缓存，保证从干净状态开始
	_ = os.Remove(dicCachePath(importFilePath(path)))

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
