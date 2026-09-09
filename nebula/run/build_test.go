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
