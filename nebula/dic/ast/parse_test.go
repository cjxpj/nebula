package ast

import "testing"

func TestParseEntry_NestedBlocks(t *testing.T) {
	body := []string{
		"函数>foo",
		"输出一行",
		"如果>条件",
		"$函数 1$",
		"<如果",
		"<函数",
	}
	lineNums := []int{10, 11, 12, 13, 14, 15}

	e := ParseEntry("触发", 1, body, lineNums)

	if e.Trigger != "触发" || e.TriggerLine != 1 {
		t.Fatalf("触发词/行号错误: %+v", e)
	}
	if len(e.Body) != 1 {
		t.Fatalf("顶层节点数 = %d, 期望 1", len(e.Body))
	}

	fn, ok := e.Body[0].(*Block)
	if !ok || fn.OpenKind != BlockFunc {
		t.Fatalf("顶层首节点应为函数框, 得到 %#v", e.Body[0])
	}
	if fn.OpenLine != 10 || fn.CloseLine != 15 {
		t.Fatalf("函数框行号错误: open=%d close=%d", fn.OpenLine, fn.CloseLine)
	}
	if len(fn.Children) != 2 {
		t.Fatalf("函数框子节点数 = %d, 期望 2", len(fn.Children))
	}

	if s, ok := fn.Children[0].(*Stmt); !ok || s.Text != "输出一行" {
		t.Fatalf("第1个子节点应为语句, 得到 %#v", fn.Children[0])
	}

	if_, ok := fn.Children[1].(*Block)
	if !ok || if_.OpenKind != BlockIf {
		t.Fatalf("第2个子节点应为判断框, 得到 %#v", fn.Children[1])
	}
	if if_.OpenLine != 12 || if_.CloseLine != 14 {
		t.Fatalf("判断框行号错误: open=%d close=%d", if_.OpenLine, if_.CloseLine)
	}
	if len(if_.Children) != 1 {
		t.Fatalf("判断框子节点数 = %d, 期望 1", len(if_.Children))
	}
}

func TestParseBody_LeafFrames(t *testing.T) {
	body := []string{
		"文本>",
		"这是文本",
		"<文本",
		"JSON>{",
		"\"a\": 1",
		"}",
		"JSON>",
		"{\"b\":2}",
		"<JSON",
	}
	roots := ParseBody(body, []int{1, 2, 3, 4, 5, 6, 7, 8, 9})

	if len(roots) != 3 {
		t.Fatalf("顶层节点数 = %d, 期望 3", len(roots))
	}

	text, ok := roots[0].(*Block)
	if !ok || text.OpenKind != BlockText || text.CloseLine == 0 {
		t.Fatalf("第1个应为已闭合文本框, 得到 %#v", roots[0])
	}
	if len(text.Children) != 1 {
		t.Fatalf("文本框子节点数 = %d, 期望 1", len(text.Children))
	}

	nj, ok := roots[1].(*Block)
	if !ok || nj.OpenKind != BlockNewJson || nj.CloseLine == 0 {
		t.Fatalf("第2个应为已闭合新建JSON框, 得到 %#v", roots[1])
	}

	j, ok := roots[2].(*Block)
	if !ok || j.OpenKind != BlockJson || j.CloseLine == 0 {
		t.Fatalf("第3个应为已闭合JSON框, 得到 %#v", roots[2])
	}
}

func TestParseBody_UnclosedBlock(t *testing.T) {
	roots := ParseBody([]string{"循环>", "内容"}, []int{1, 2})
	if len(roots) != 1 {
		t.Fatalf("顶层节点数 = %d, 期望 1", len(roots))
	}
	b := roots[0].(*Block)
	if b.OpenKind != BlockFor || b.CloseLine != 0 {
		t.Fatalf("未闭合循环框应 CloseLine=0, 得到 %#v", b)
	}
	if len(b.Children) != 1 {
		t.Fatalf("未闭合循环框子节点数 = %d, 期望 1", len(b.Children))
	}
}
