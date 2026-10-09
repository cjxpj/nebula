package funcs

import (
	"testing"

	"github.com/cjxpj/nebula/utils"
)

// TestAddScheduledTaskLenRule 锁定「添加定时任务」的参数上限为 6 个。
//
// 函数体（task.go）明确读取第 6 个参数「异步执行」；注册规则若漏写 |6，该分支永远不可达，
// 且文档里 `$添加定时任务 5000 Main private/task.n false false false$` 会在编译期被
// run/compile_check.go 判「参数数量错误(需要 1|2|3|4|5，实际 6)」。
func TestAddScheduledTaskLenRule(t *testing.T) {
	Setup()
	for _, f := range ListFuncs() {
		if f.Name != "添加定时任务" {
			continue
		}
		for n := 1; n <= 6; n++ {
			if !utils.MatchLenRule(n, f.Rule) {
				t.Fatalf("「添加定时任务」应支持 %d 个参数，但注册规则是 %q", n, f.Rule)
			}
		}
		if utils.MatchLenRule(7, f.Rule) {
			t.Fatalf("「添加定时任务」不应支持 7 个参数，注册规则是 %q", f.Rule)
		}
		return
	}
	t.Fatal("未找到注册函数「添加定时任务」")
}
