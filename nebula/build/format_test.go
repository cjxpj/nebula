package build

import "testing"

// TestFormatDicTextBlockVarPrefix 变量名:文本> / 变量名:纯文本> 是叶子框：
// 内容行缩进一级，<文本 与开启行同层，框外后续行回到原层级。
func TestFormatDicTextBlockVarPrefix(t *testing.T) {
	src := "a:文本>\n第一行\n第二行\n<文本\na:纯文本>|\n%x%\n<文本\n%b%\n"
	want := "a:文本>\n    第一行\n    第二行\n<文本\na:纯文本>|\n    %x%\n<文本\n%b%\n"
	if got := FormatDic(src); got != want {
		t.Fatalf("格式化结果不符:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
