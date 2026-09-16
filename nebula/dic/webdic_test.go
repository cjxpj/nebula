package dic

import (
	"regexp"
	"strings"
	"testing"

	"github.com/cjxpj/nebula/build"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
)

var webDicPRe = regexp.MustCompile(`(?s)<p>(.*?)</p>`)

// runWebDicRes 执行网页词库并取回 <p>{{.d}}</p> 的渲染结果。
func runWebDicRes(t *testing.T, text string) string {
	t.Helper()
	out := (&dicImpl{}).WebDicRun(dic_dto.NewWebDic("执行", text))
	m := webDicPRe.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("未找到 <p> 渲染结果：%q", out)
	}
	return m[1]
}

// runWebDicAllPRes 取回全部 <p> 的内部文本（用 | 连接）。
// 用于比较格式化前后的渲染结果：格式化会调整 HTML 缩进，但段落内容必须一致。
func runWebDicAllPRes(t *testing.T, text string) string {
	t.Helper()
	out := (&dicImpl{}).WebDicRun(dic_dto.NewWebDic("执行", text))
	ms := webDicPRe.FindAllStringSubmatch(out, -1)
	if len(ms) == 0 {
		t.Fatalf("未找到 <p> 渲染结果：%q", out)
	}
	parts := make([]string, 0, len(ms))
	for _, m := range ms {
		parts = append(parts, m[1])
	}
	return strings.Join(parts, "|")
}

// TestWebDicRunScriptIndent 脚本块正文缩进（格式化后的网页词库）不影响解析与执行：
// 顶格与缩进两种写法必须得到相同结果，且缩进不会进入脚本输出。
func TestWebDicRunScriptIndent(t *testing.T) {
	body := "甲:1\n$甲$\n$加法 1 2$"
	flat := "<html><body>\n<script type=\"nebula\" id=\"d\">\n" + body + "\n</script>\n<p>{{.d}}</p>\n</body></html>"
	indented := "<html><body>\n    <script type=\"nebula\" id=\"d\">\n        " +
		strings.ReplaceAll(body, "\n", "\n        ") +
		"\n    </script>\n    <p>{{.d}}</p>\n</body></html>"

	flatRes := runWebDicRes(t, flat)
	indentedRes := runWebDicRes(t, indented)
	if flatRes == "" {
		t.Fatalf("脚本无输出，用例失去判别力：%q", flatRes)
	}
	if flatRes != indentedRes {
		t.Fatalf("缩进改变了脚本结果：\n顶格 %q\n缩进 %q", flatRes, indentedRes)
	}
	for _, line := range strings.Split(indentedRes, "\n") {
		if line != strings.TrimLeft(line, " \t") {
			t.Fatalf("缩进进入了脚本输出：%q", indentedRes)
		}
	}
}

// TestWebDicRunInlineBlock 内联执行块 <?n ... ?> 就地执行并输出：
// 块的结果直接插在块所在的位置，块内赋值的变量并入模板数据、与其它执行块共享。
func TestWebDicRunInlineBlock(t *testing.T) {
	wn := "<html><body>\n" +
		"<p><?n\n甲:7\n%甲%\n?></p>\n" +
		"<p><?n\n乙:%甲%0\n%乙%\n?></p>\n" +
		"<script type=\"nebula\" id=\"丙\">\n丙:%甲%#%乙%\n%丙%\n</script>\n" +
		"<p>{{.甲}}|{{.丙}}</p>\n" +
		"</body></html>"

	out := (&dicImpl{}).WebDicRun(dic_dto.NewWebDic("执行", wn))

	// 第一个内联块就地输出（不经过模板数据）
	if !strings.Contains(out, "<p>7</p>") {
		t.Fatalf("内联块未就地输出：%q", out)
	}
	// 第二个内联块读到第一个块的变量
	if !strings.Contains(out, "<p>70</p>") {
		t.Fatalf("内联块之间的变量未共享：%q", out)
	}
	// <script type="nebula"> 继续接力，且内联块赋值的变量能通过 {{.键}} 取到
	if !strings.Contains(out, "<p>7|7#70</p>") {
		t.Fatalf("脚本块未读到内联块的变量：%q", out)
	}
}

// TestWebDicRunInlineBlockIndent 内联块正文缩进（格式化后的网页词库）不影响解析与执行：
// 顶格与缩进两种写法得到相同的块输出，且缩进不会进入输出。
func TestWebDicRunInlineBlockIndent(t *testing.T) {
	body := "甲:7\n%甲%"
	flat := "<html><body>\n<p><?n\n" + body + "\n?></p>\n</body></html>"
	indented := "<html><body>\n    <p><?n\n        " +
		strings.ReplaceAll(body, "\n", "\n        ") +
		"\n    ?></p>\n</body></html>"

	flatRes := runWebDicRes(t, flat)
	indentedRes := runWebDicRes(t, indented)
	if flatRes != "7" {
		t.Fatalf("内联块输出不是期望的 7（可能混入缩进）：%q", flatRes)
	}
	if flatRes != indentedRes {
		t.Fatalf("缩进改变了内联块结果：\n顶格 %q\n缩进 %q", flatRes, indentedRes)
	}
}

// TestWebDicRunInlineBlockAfterFormat 内联块经格式化（正文按 .n 块结构排版并缩进）后执行结果不变。
func TestWebDicRunInlineBlockAfterFormat(t *testing.T) {
	wn := "<html>\n<body>\n<p><?n\n甲:1\n乙:%甲%2\n%乙%\n?></p>\n<p>{{.乙}}</p>\n</body>\n</html>\n"
	formatted := build.FormatWebDic(wn)
	if formatted == wn {
		t.Fatalf("格式化结果未发生变化：%q", wn)
	}

	want := runWebDicAllPRes(t, wn)
	got := runWebDicAllPRes(t, formatted)
	if want != "12|12" {
		t.Fatalf("内联块无输出，用例失去判别力：%q", want)
	}
	if got != want {
		t.Fatalf("格式化后执行结果变化：\n格式化前 %q\n格式化后 %q", want, got)
	}
}

// TestWebDicRunAfterFormat 整份网页词库格式化后，执行结果与格式化前一致。
func TestWebDicRunAfterFormat(t *testing.T) {
	wn := "<html>\n<body>\n<script type=\"nebula\" id=\"d\">\n甲:1\n$甲$\n</script>\n<p>{{.d}}</p>\n</body>\n</html>\n"
	formatted := build.FormatWebDic(wn)
	if formatted == wn {
		t.Fatalf("格式化结果未发生变化：%q", wn)
	}
	if got, want := runWebDicRes(t, formatted), runWebDicRes(t, wn); got != want {
		t.Fatalf("格式化后执行结果变化：\n格式化前 %q\n格式化后 %q", want, got)
	}
}
