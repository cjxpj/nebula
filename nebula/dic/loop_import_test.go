package dic

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
)

// TestFuncParamRule 验证 [函数|参数规则] 语法：按函数名精确匹配 + 参数数量校验，不使用正则。
func TestFuncParamRule(t *testing.T) {
	chdirToAppWin()

	const funcDef = "\n[函数|1|2]echo\n%参数1%\n\n[函数|1..]tail\n%参数2%\n\nMain\n"

	cases := []struct {
		name    string
		body    string
		want    string
		wantErr bool // 期望报参数数量错误
	}{
		{"1个参数", "$echo 甲$", "甲", false},
		{"2个参数", "$echo 甲 乙$", "甲", false},
		{"无限参数读取第2个", "$tail a b c$", "b", false},
		{"参数不足报错", "$echo$", "", true},
		{"参数过多报错", "$echo 甲 乙 丙$", "", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			D := dic_dto.NewDic("t.n", funcDef+c.body)
			got := dic_api.Api.DicRun(D, "Main")
			if c.wantErr {
				if !strings.Contains(got, "参数数量错误") {
					t.Errorf("%s 期望报参数数量错误，实际 %q", c.name, got)
				}
				return
			}
			if got != c.want {
				t.Errorf("%s 期望 %q，实际 %q", c.name, c.want, got)
			}
		})
	}
}
// TestFuncDefaultZeroParam 验证 [函数]名称（未声明规则）默认 0 个参数，不再走正则匹配。
func TestFuncDefaultZeroParam(t *testing.T) {
	chdirToAppWin()

	const funcDef = "\n[函数]test\nok\n\nMain\n"

	// 0 参数命中
	D := dic_dto.NewDic("t.n", funcDef+"$test$")
	if got := dic_api.Api.DicRun(D, "Main"); got != "ok" {
		t.Errorf("0 参数应命中，期望 ok，实际 %q", got)
	}

	// 带参数报错（不再正则匹配/静默返回原文）
	D = dic_dto.NewDic("t.n", funcDef+"$test 参数$")
	if got := dic_api.Api.DicRun(D, "Main"); !strings.Contains(got, "参数数量错误") {
		t.Errorf("带参数应报参数数量错误，实际 %q", got)
	}
}

// TestFuncPassOut 验证 [函数|规则]名称->变量列表 的变量传出在新语法下可用。
func TestFuncPassOut(t *testing.T) {
	chdirToAppWin()

	const funcDef = "\n[函数|1]测试->a,b\n%参数1%\na:1\nb:2\n\nMain\n"
	D := dic_dto.NewDic("t.n", funcDef+"$测试 成功$\n%a%\n%b%")
	got := dic_api.Api.DicRun(D, "Main")
	if got != "成功12" {
		t.Errorf("变量传出错误，期望 成功12，实际 %q", got)
	}
}

// TestFuncParamRuleQuoted 验证带空格的引用参数按 1 个参数计数（不会因空格被误判为多个）。
func TestFuncParamRuleQuoted(t *testing.T) {
	chdirToAppWin()

	const funcDef = "\n[函数|1]fixed\nok\n\nMain\n"
	D := dic_dto.NewDic("t.n", funcDef+`$fixed "甲 乙"$`)
	if got := dic_api.Api.DicRun(D, "Main"); got != "ok" {
		t.Errorf("引用参数计数错误，期望 ok，实际 %q", got)
	}
}

// TestImportAssignPackage 验证 #引入= 的赋予值形式：变量:#引入=目标 导入文件全部函数组成包并返回实例。
func TestImportAssignPackage(t *testing.T) {
	chdirToAppWin()

	// 准备引入文件 NebulaData/private/测试引入包.n（定义一个类及方法）
	const pkgName = "测试引入包"
	dir := filepath.Join("NebulaData", "private")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(dir, pkgName+".n")
	// 被引入的 [函数] 文件需以空行开头，避免头部区域误判
	fileContent := "\n[函数:" + pkgName + "]new\n.变量:初始值\n\n[函数:" + pkgName + "]读取变量\n%自己.变量%\n"
	if err := os.WriteFile(filePath, []byte(fileContent), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(filePath)

	const dicText = "甲:#引入=" + pkgName + "\n\nMain\n$甲.读取变量$"
	D := dic_dto.NewDic("t.n", dicText)
	got := dic_api.Api.DicRun(D, "Main")
	if got != "初始值" {
		t.Errorf("导入包并调用方法错误，期望 初始值，实际 %q", got)
	}
}

// TestClassCallback 验证类内置回调：类内定义 [内部:类名]名称，类方法内部用 $回调 名称$ 触发。
func TestClassCallback(t *testing.T) {
	chdirToAppWin()

	const dicText = `
[函数:我的包]run
$回调 内部名$

[内部:我的包]内部名
回调成功

Main
甲:$new 我的包$
$甲.run$
`
	D := dic_dto.NewDic("t.n", dicText)
	if got := dic_api.Api.DicRun(D, "Main"); got != "回调成功" {
		t.Errorf("类内回调错误，期望 回调成功，实际 %q", got)
	}
}

// TestInstanceCallbackMethod 验证实例内置回调方法：$变量.回调 名称$ 直接触发实例内 [内部]名称。
func TestInstanceCallbackMethod(t *testing.T) {
	chdirToAppWin()

	const pkgName = "测试回调包"
	dir := filepath.Join("NebulaData", "private")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(dir, pkgName+".n")
	// 被引入文件以空行开头，避免头部误判；[f]test 为函数、[l]a 为内部回调
	fileContent := "\n[f]test\nok\n\n[l]a\nok2\n"
	if err := os.WriteFile(filePath, []byte(fileContent), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(filePath)

	const dicText = "甲:#引入=" + pkgName + "\n\nMain\n$甲.test$\n$甲.回调 a$"
	D := dic_dto.NewDic("t.n", dicText)
	if got := dic_api.Api.DicRun(D, "Main"); got != "okok2" {
		t.Errorf("实例回调错误，期望 okok2，实际 %q", got)
	}
}

// TestImportCycle 验证循环引入会被检测并收集为编译警告，不会无限递归。
func TestImportCycle(t *testing.T) {
	chdirToAppWin()

	dir := filepath.Join("NebulaData", "private")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	aPath := filepath.Join(dir, "循环A.n")
	bPath := filepath.Join(dir, "循环B.n")
	// A 引入 B；B 引入 A（形成循环），同时 B 定义函数 a
	if err := os.WriteFile(aPath, []byte("#引入=循环B\n\nMain\n$a$"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bPath, []byte("#引入=循环A\n\n[函数]a\nok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(aPath)
	defer os.Remove(bPath)

	D := dic_dto.NewDic("循环A.n", "#引入=循环B\n\nMain\n$a$")
	got := dic_api.Api.DicRun(D, "Main")
	if got != "ok" {
		t.Errorf("循环引入被中断后函数应正常引入，期望 ok，实际 %q", got)
	}
	// 循环引入应收集为编译警告（供前端展示），而非打印到后端日志，且携带行号
	if len(D.Data.Warnings) == 0 {
		t.Error("期望收集到循环引入警告，实际为空")
	} else if D.Data.Warnings[0].Line < 1 {
		t.Errorf("期望警告携带行号（1-based），实际 %d", D.Data.Warnings[0].Line)
	}
}

// TestFuncAfterLoopBlock 回归测试：循环/遍历/判断块执行完毕后，
// 局部函数（含 #引入 注入到「函数」类别的词条）仍应可被 $函数$ 调用。
func TestFuncAfterLoopBlock(t *testing.T) {
	chdirToAppWin()

	const funcDef = "\n[函数]test\n引入函数可用\n\nMain\n"

	cases := []struct {
		name string
		body string
	}{
		{"循环", "循环>i\n>终止循环\n<循环\n$test$"},
		{"遍历", "遍历>i=[1]\n>终止遍历\n<遍历\n$test$"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			D := dic_dto.NewDic("t.n", funcDef+c.body)
			got := dic_api.Api.DicRun(D, "Main")
			if got != "引入函数可用" {
				t.Errorf("%s 块后函数失效，期望 引入函数可用，实际 %q", c.name, got)
			}
		})
	}
}
