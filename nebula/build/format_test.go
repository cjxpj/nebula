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

// TestFormatDicIfBranches 判断框的分支行（>否则如果: / >否则）与其开启行同层，
// 分支正文落在分支行的下一层；嵌套在循环框里时按所属层级对齐。
func TestFormatDicIfBranches(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{
			"判断框分支",
			"测试\n如果>%a%==1\n一\n>否则如果:%a%==2\n二\n>否则\n三\n<如果\n",
			"测试\n如果>%a%==1\n    一\n>否则如果:%a%==2\n    二\n>否则\n    三\n<如果\n",
		},
		{
			"循环内判断框分支",
			"测试\n循环>i=2\n如果>%i%==1\n一\n>否则\n二\n<如果\n<循环\n",
			"测试\n循环>i=2\n    如果>%i%==1\n        一\n    >否则\n        二\n    <如果\n<循环\n",
		},
	}
	for _, c := range cases {
		got := FormatDic(c.src)
		if got != c.want {
			t.Fatalf("%s 格式化结果不符:\n--- got ---\n%s--- want ---\n%s", c.name, got, c.want)
		}
		if twice := FormatDic(got); twice != got {
			t.Fatalf("%s 二次格式化结果不一致:\n--- once ---\n%s--- twice ---\n%s", c.name, got, twice)
		}
	}
}

// TestFormatDicMatchBranches 匹配框的分支行（如果是:值 / 如果不是，前导 > 可有可无）
// 与其开启行同层，分支正文落在下一层。
func TestFormatDicMatchBranches(t *testing.T) {
	src := "测试\n匹配>%a%==1\n如果是:1\n一\n>如果不是\n二\n<匹配\n"
	want := "测试\n匹配>%a%==1\n如果是:1\n    一\n>如果不是\n    二\n<匹配\n"
	if got := FormatDic(src); got != want {
		t.Fatalf("格式化结果不符:\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

// TestFormatDicTriggerBodyFlush 触发词之下的正文不再整体缩进一级：
// 正文顶格，框内按层级逐级缩进。
func TestFormatDicTriggerBodyFlush(t *testing.T) {
	src := "测试\n甲:1\n%甲%\n\n测试2\n循环>i=2\n%i%\n<循环\n"
	want := "测试\n甲:1\n%甲%\n\n测试2\n循环>i=2\n    %i%\n<循环\n"
	if got := FormatDic(src); got != want {
		t.Fatalf("格式化结果不符:\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}
