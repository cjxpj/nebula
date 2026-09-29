package run

import "testing"

func TestIsInlineControlLine(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"如果是:你好", true},
		{"如果是:1", true},
		{"如果:%x%==1", true},
		{"否则如果:%x%==2", true},
		{"if:%x%==1", true},
		{"elif:%x%==2", true},
		{"如果不是", false}, // 匹配框默认分支，无冒号，不是赋值，也无需豁免
		{"如果是", false},   // 冒号后无内容
		{"名字=张三", false},
		{"变量:值", false},
	}
	for _, c := range cases {
		if got := isInlineControlLine(c.line); got != c.want {
			t.Errorf("isInlineControlLine(%q) = %v, want %v", c.line, got, c.want)
		}
	}
}
