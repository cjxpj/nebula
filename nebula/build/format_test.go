package build

import "testing"

// TestFirstHeadLikeLineBlockOpen 全文无空行时，首个有效行是框开启行（循环>、遍历> 等）
// 也应视为头部初始化脚本：头部同样是词块容器，可承载框。
func TestFirstHeadLikeLineBlockOpen(t *testing.T) {
	cases := map[string][]string{
		"循环框":   {"循环>i=10", "    a", "    <循环"},
		"遍历框":   {"遍历>i 1 3", "    %i%", "    <遍历"},
		"赋值行":   {"a:1", "b:2"},
		"函数调用": {"$执行词库 private/test.n Main$"},
		"纯正文":   {"循环测试", "    内容"},
	}
	for name, lines := range cases {
		want := name != "纯正文"
		if got := FirstHeadLikeLine(lines); got != want {
			t.Fatalf("%s：FirstHeadLikeLine 期望 %v，实际 %v", name, want, got)
		}
	}
}

// TestFormatDicHeadLikeBlock 无触发词的头部循环框应整篇按头部排版：
// 框内缩进一级，关闭标记与开启行同层；正文部分不因末尾换行差异而变化
//（末尾换行本身按 FormatDic 约定保留原文）。
func TestFormatDicHeadLikeBlock(t *testing.T) {
	const body = "循环>i=10\n    a\n<循环"
	for _, src := range []string{body, body + "\n"} {
		if got := FormatDic(src); got != src {
			t.Fatalf("格式化结果不符:\n--- got ---\n%q\n--- src ---\n%q", got, src)
		}
	}
}

// TestFormatDicTextBlockVarPrefix 变量名:文本> / 变量名:纯文本> 是叶子框：
// 内容行缩进一级，<文本 与开启行同层，框外后续行回到原层级。
func TestFormatDicTextBlockVarPrefix(t *testing.T) {
	src := "a:文本>\n第一行\n第二行\n<文本\na:纯文本>|\n%x%\n<文本\n%b%\n"
	want := "a:文本>\n    第一行\n    第二行\n<文本\na:纯文本>|\n    %x%\n<文本\n%b%\n"
	if got := FormatDic(src); got != want {
		t.Fatalf("格式化结果不符:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
