package run

import (
	"os"
	"strings"
	"testing"
)

func TestWebDicCheckBlocksAndTemplateKeys(t *testing.T) {
	text := `<html><body>
<p>{{.缺失键}}</p>
<script type="nebula">
甲:1
乙:2
</script>
<p>{{.甲}}</p>
</body></html>`

	warnings := WebDicCheck(text)

	lineOf := func(sub string) int {
		for _, w := range warnings {
			if strings.Contains(w.Text, sub) {
				return w.Line
			}
		}
		t.Fatalf("未找到告警：%s，实际：%v", sub, warningsText(warnings))
		return 0
	}

	if got := lineOf("模板键不存在：缺失键"); got != 2 {
		t.Fatalf("模板键告警行号 = %d，期望 2", got)
	}
	if got := lineOf("变量未使用：乙"); got != 5 {
		t.Fatalf("变量未使用告警行号 = %d，期望 5", got)
	}
	if containsText(warnings, "模板键不存在：甲") {
		t.Fatalf("甲 是脚本块赋值的变量，应是模板键，不应告警：%v", warningsText(warnings))
	}
	if containsText(warnings, "变量未使用：甲") {
		t.Fatalf("甲 被 {{.甲}} 引用，不应报未使用：%v", warningsText(warnings))
	}
}

// TestWebDicCheckInlineBlock <?n ... ?> 内联块与脚本块一样算执行块：
// 块里的语句不报「脚本块外的 Nebula 语法」，但内联块赋值的变量不进模板数据。
func TestWebDicCheckInlineBlock(t *testing.T) {
	text := `<html><body>
<p><?n
甲:1
%甲%
?></p>
<p>{{.甲}}</p>
</body></html>`

	warnings := WebDicCheck(text)
	if !containsText(warnings, "模板键不存在：甲") {
		t.Fatalf("内联块赋值的变量不进模板数据，{{.甲}} 应报模板键不存在，实际：%v", warningsText(warnings))
	}

	count, keys := WebDicRenderInfo(text)
	if count != 1 {
		t.Fatalf("执行块数 = %d，期望 1（内联块应计入）", count)
	}
	if len(keys) != 0 {
		t.Fatalf("内联块不提供模板键，期望空，实际：%v", keys)
	}
}

// TestWebDicCheckInlineBlockOutsideStillWarns 有内联块时，执行块之外的 Nebula 语句仍要报：
// 只是漏写了 <?n ... ?> 标记，语句不会执行。
func TestWebDicCheckInlineBlockOutsideStillWarns(t *testing.T) {
	text := `<html><body>
<p><?n
甲:1
%甲%
?></p>
gridContent:文本>
<p>{{.甲}}</p>
</body></html>`

	for _, w := range WebDicCheck(text) {
		if w.Line == 6 && strings.Contains(w.Text, "脚本块外的 Nebula 语法") {
			return
		}
	}
	t.Fatalf("第 6 行（执行块之外）应报「脚本块外的 Nebula 语法」，实际：%v", warningsText(WebDicCheck(text)))
}

// TestWebDicCheckUnclosedInlineBlock 漏写 ?> 的内联执行块必须报出来：运行时整块不执行，
// 并且从这里起的剩余内容都按普通文字原样输出。其后的语句不能再被误报成「脚本块外的 Nebula 语法」——
// 真正的原因是漏写 ?>，不是「语句没写进执行块」。
func TestWebDicCheckUnclosedInlineBlock(t *testing.T) {
	text := `<html><body>
<p><?n
甲:1
%甲%
</body></html>`

	warnings := WebDicCheck(text)
	if len(warnings) != 1 {
		t.Fatalf("期望只报一条「缺少结束标记」，实际：%v", warningsText(warnings))
	}
	if w := warnings[0]; w.Line != 2 || !strings.Contains(w.Text, "缺少结束标记") {
		t.Fatalf("告警 = 第 %d 行 %q，期望第 2 行报「缺少结束标记」", w.Line, w.Text)
	}
}

func TestWebDicCheckScriptIndentAndGlobalVars(t *testing.T) {
	text := `<html><body>
<script type="nebula">
    设置头部 Content-Type text/plain
    访问数据:$GET 访问数据$
    $GET %访问数据%$
</script>
</body></html>`

	if warnings := WebDicCheck(text); len(warnings) != 0 {
		t.Fatalf("不应有告警，实际：%v", warningsText(warnings))
	}
}

// TestTrimWebScriptIndent 与运行时解析前的处理保持一致：逐行去掉行首空白，
// //@关闭缩进 到 //@启用缩进 之间的行首空白有语义，保持原样。
func TestTrimWebScriptIndent(t *testing.T) {
	lines := []string{
		"    甲:1",
		"\t    如果>$甲$>0",
		"    //@关闭缩进",
		"        保留 行首空白",
		"    //@启用缩进",
		"    文本>正数",
		"",
	}
	want := []string{
		"甲:1",
		"如果>$甲$>0",
		"//@关闭缩进",
		"        保留 行首空白",
		"//@启用缩进",
		"文本>正数",
		"",
	}
	got := TrimWebScriptIndent(lines)
	if len(got) != len(want) {
		t.Fatalf("行数 = %d，期望 %d：%q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("第 %d 行 = %q，期望 %q", i+1, got[i], want[i])
		}
	}
}

// TestTrimWebScriptIndentCRLF CRLF 文件按 \n 切分后行尾残留 \r，
// 必须被去掉，否则 <文本\r、//@关闭缩进\r 这类精确匹配会失败。
func TestTrimWebScriptIndentCRLF(t *testing.T) {
	got := TrimWebScriptIndent([]string{"    甲:1\r", "    <文本\r", "    //@关闭缩进\r"})
	want := []string{"甲:1", "<文本", "//@关闭缩进"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("第 %d 行 = %q，期望 %q", i+1, got[i], want[i])
		}
	}
}

// TestWebDicCheckOutsideSyntax 覆盖「文件看着像词库、页面却什么都没渲染」的典型写法：
// Nebula 语句漏写 <script type="nebula"> 标签、直接写在 HTML 里时不会执行，会原样输出到页面。
func TestWebDicCheckOutsideSyntax(t *testing.T) {
	text := `<html><body>
<div>九宫格</div>
gridContent:文本>
甲
<文本
$某函数 1$
%某变量%
</body></html>`

	warnings := WebDicCheck(text)
	got := map[int]string{}
	for _, w := range warnings {
		if strings.Contains(w.Text, "脚本块外的 Nebula 语法") {
			got[w.Line] = w.Text
		}
	}
	// 3 框开启行、5 框结束标记、6 $函数$ 调用、7 %变量% 取值
	for _, ln := range []int{3, 5, 6, 7} {
		if _, ok := got[ln]; !ok {
			t.Fatalf("第 %d 行应报「脚本块外的 Nebula 语法」，实际：%v", ln, warningsText(warnings))
		}
	}
}

// TestWebDicCheckOutsideSyntaxNoFalsePositive CSS / 普通 JS 里满是 `属性: 值;`、$、% 这类形状，
// 不能被判成 Nebula 语句，否则任何正常页面都会被刷屏。
func TestWebDicCheckOutsideSyntaxNoFalsePositive(t *testing.T) {
	text := `<html><head>
<style>
.box { width: 300px; color: #fff; }
</style>
<script>
var s = $("div");
var t = 50%;
function f() { return 1 > 0; }
</script>
</head><body>
<script type="nebula" id="甲">
甲:1
%甲%
</script>
<p>{{.甲}}</p>
</body></html>`

	if warnings := WebDicCheck(text); len(warnings) != 0 {
		t.Fatalf("不应有告警，实际：%v", warningsText(warnings))
	}
}

// TestWebDicRenderInfo 渲染摘要用于让模型判断脚本块是否真的存在、页面能取到哪些键。
// 脚本块里赋值的变量并入模板数据（{{.变量名}}），内联块赋值不进模板键。
func TestWebDicRenderInfo(t *testing.T) {
	text := `<html><body>
<script type="nebula">
甲:1
乙:2
</script>
</body></html>`

	count, keys := WebDicRenderInfo(text)
	if count != 1 {
		t.Fatalf("脚本块数 = %d，期望 1", count)
	}
	got := map[string]bool{}
	for _, k := range keys {
		got[k] = true
	}
	if len(keys) != 2 {
		t.Fatalf("模板键 = %v，期望 [甲 乙]", keys)
	}
	for _, k := range []string{"甲", "乙"} {
		if !got[k] {
			t.Fatalf("缺少模板键 %s，实际：%v", k, keys)
		}
	}

	if count, keys := WebDicRenderInfo("<html><body>静态页</body></html>"); count != 0 || len(keys) != 0 {
		t.Fatalf("无脚本块时应返回 0 / 空，实际：%d / %v", count, keys)
	}
}

func TestWebDicCheckRealFile(t *testing.T) {
	text, err := os.ReadFile("../appfiles/static/dic/public/404.wn")
	if err != nil {
		t.Skipf("读取示例网页词库失败：%v", err)
	}
	if warnings := WebDicCheck(string(text)); len(warnings) != 0 {
		t.Fatalf("404.wn 不应有告警，实际：%v", warningsText(warnings))
	}
}
