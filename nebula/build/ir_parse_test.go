package build

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// normalizeNonBlankLines 归一化源码用于「非空行序列」比较：
// CRLF / 单个 CR 统一为 LF，每行去首尾空白，丢弃全部空行。
// 之所以忽略空行：IRToNebula 会在词条之间恒定补一个空行，
// 原文件中「注释紧贴触发词」的形态往返后会多出一个空行，但语义等价。
func normalizeNonBlankLines(src string) []string {
	normalized := strings.ReplaceAll(src, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	var out []string
	for _, l := range strings.Split(normalized, "\n") {
		if t := strings.TrimSpace(l); t != "" {
			out = append(out, normalizeTriggerTag(t))
		}
	}
	return out
}

// normalizeTriggerTag 把行首 [类别] 前缀里的类别缩写归一为全称（[f]→[函数]、[l]→[内部]）。
// 触发词解析按运行时口径识别缩写，重发时统一写成全称，故比较前先归一，语义等价。
func normalizeTriggerTag(line string) string {
	if !strings.HasPrefix(line, "[") {
		return line
	}
	end := strings.Index(line, "]")
	if end <= 1 {
		return line
	}
	tag := line[1:end]
	rule := ""
	if before, after, ok := strings.Cut(tag, "|"); ok {
		tag, rule = before, after
		if rule != "" {
			rule = "|" + rule
		}
	}
	category, class, hasClass := tag, "", false
	if before, after, ok := strings.Cut(tag, ":"); ok {
		category, class, hasClass = before, after, true
	}
	normalized := normalizeTriggerCategory(category)
	if normalized == category {
		return line
	}
	if hasClass {
		tag = normalized + ":" + class
	} else {
		tag = normalized
	}
	return "[" + tag + rule + "]" + line[end+1:]
}

// walkNebulaFiles 遍历内置词库目录，返回全部 .n 文件路径。
func walkNebulaFiles(t *testing.T) []string {
	t.Helper()
	root := filepath.Join("..", "appfiles", "static", "dic")
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".n") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("遍历词库目录失败：%v", err)
	}
	if len(files) == 0 {
		t.Fatal("未找到任何 .n 词库文件")
	}
	return files
}

// TestNebulaToIRRoundTripLines 验证真实词库文件往返后「非空行序列」完全一致，
// 即 NebulaToIR → IRToNebula 不丢行、不改内容、不错顺序。
func TestNebulaToIRRoundTripLines(t *testing.T) {
	for _, path := range walkNebulaFiles(t) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读取 %s 失败：%v", path, err)
		}
		src := string(data)
		got := normalizeNonBlankLines(IRToNebula(NebulaToIR(src)))
		want := normalizeNonBlankLines(src)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("往返非空行序列不一致：%s\n--- got ---\n%s\n--- want ---\n%s",
				path, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
}

// TestNebulaToIRIdempotent 验证 IR 幂等：
// NebulaToIR(IRToNebula(NebulaToIR(src))) 与 NebulaToIR(src) 结构完全一致。
func TestNebulaToIRIdempotent(t *testing.T) {
	for _, path := range walkNebulaFiles(t) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读取 %s 失败：%v", path, err)
		}
		src := string(data)
		first := NebulaToIR(src)
		second := NebulaToIR(IRToNebula(first))
		if !reflect.DeepEqual(first, second) {
			t.Errorf("IR 不幂等：%s\n--- first ---\n%+v\n--- second ---\n%+v", path, first, second)
		}
	}
}

// TestNebulaToIRTriggerAbbr 验证 [f]/[F]/[l]/[L] 缩写被识别为类别前缀，
// 不会与触发词名混在一起（缩写词条重发为全称，语义与运行时归一规则一致）。
func TestNebulaToIRTriggerAbbr(t *testing.T) {
	cases := []struct {
		src      string
		wantKind string
		wantCls  string
		wantName string
		wantRule string
	}{
		{"[f]_初始化\n    $设置工作目录 NebulaData$\n", "func", "", "_初始化", ""},
		{"[F]打招呼\n    你好\n", "func", "", "打招呼", ""},
		{"[l]戳一戳\n    你好\n", "inner", "", "戳一戳", ""},
		{"[L:机器人]戳一戳\n    你好\n", "inner_class", "机器人", "戳一戳", ""},
		{"[f:机器人]打招呼\n    你好\n", "func_class", "机器人", "打招呼", ""},
		{"[f|1]打招呼\n    你好\n", "func", "", "打招呼", "1"},                                  // 函数类别带参数规则
		{"[f|2]等级_兑经验\n    你好\n", "func", "", "等级_兑经验", "2"},                         // 带参数规则不被误判为「当 收到」
		{"[函数:机器人|2]唤醒\n    你好\n", "func_class", "机器人", "唤醒", "2"},                // 类方法带参数规则
		{"[内部|1]打招呼\n    你好\n", "normal", "", "[内部|1]打招呼", ""},                      // 参数规则仅函数类别生效，内部原样保留
		{"[群事件]群成员进群\n    你好\n", "normal", "", "[群事件]群成员进群", ""},
	}
	for _, c := range cases {
		ir := NebulaToIR(c.src)
		if len(ir.Entries) != 1 {
			t.Fatalf("词条数错误：src=%q，entries=%d", c.src, len(ir.Entries))
		}
		e := ir.Entries[0]
		if e.Kind != c.wantKind || e.ClassName != c.wantCls || e.Name != c.wantName || e.Rule != c.wantRule {
			t.Errorf("触发词解析错误：src=%q\n got kind=%q class=%q name=%q rule=%q\nwant kind=%q class=%q name=%q rule=%q",
				c.src, e.Kind, e.ClassName, e.Name, e.Rule, c.wantKind, c.wantCls, c.wantName, c.wantRule)
		}
	}
}

// TestNebulaToIRIfElif 验证「否则如果」多分支的解析与重发：
// 多个 >否则如果: 依次建模为 Elifs，>否则 为最后分支；文本往返逐字一致。
func TestNebulaToIRIfElif(t *testing.T) {
	src := `
判断测试
    如果>%变量%==1
        是1
    >否则如果:%变量%==2
        是2
    >否则如果:%变量%==3
        是3
    >否则
        其他
    <如果
`
	ir := NebulaToIR(src)
	if len(ir.Entries) != 1 || len(ir.Entries[0].Body) != 1 {
		t.Fatalf("词条/语句数错误：%+v", ir.Entries)
	}
	s := ir.Entries[0].Body[0]
	if s.T != "if" {
		t.Fatalf("期望 if 语句，实际 t=%q", s.T)
	}
	if s.Cond != "%变量%==1" {
		t.Errorf("主条件错误：%q", s.Cond)
	}
	if len(s.Elifs) != 2 {
		t.Fatalf("Elifs 数量错误：%d", len(s.Elifs))
	}
	if s.Elifs[0].Cond != "%变量%==2" || s.Elifs[1].Cond != "%变量%==3" {
		t.Errorf("Elifs 条件错误：%+v", s.Elifs)
	}
	if len(s.Else) != 1 {
		t.Errorf("Else 分支缺失：%+v", s.Else)
	}
	// 无 >否则 的形态也应可往返；「否则如果」分支体允许为空（前端刚加出空分支时不降级）
	single := "\n判断测试\n    如果>%变量%==1\n        是1\n    >否则如果:%变量%==2\n        是2\n    <如果\n"
	if got := IRToNebula(NebulaToIR(single)); got != single {
		t.Errorf("单支否则如果往返不一致：\n--- got ---\n%s\n--- want ---\n%s", got, single)
	}
	empty := "\n判断测试\n    如果>%变量%==1\n        是1\n    >否则如果:%变量%==2\n    <如果\n"
	if got := IRToNebula(NebulaToIR(empty)); got != empty {
		t.Errorf("空分支体否则如果往返不一致：\n--- got ---\n%s\n--- want ---\n%s", got, empty)
	}
	if got := IRToNebula(ir); got != src {
		t.Errorf("多支否则如果往返不一致：\n--- got ---\n%s\n--- want ---\n%s", got, src)
	}
}

// TestNebulaToIRCanonical 验证规范文本（IRToNebula 的输出）能被反向解析并逐字重发。
func TestNebulaToIRCanonical(t *testing.T) {
	canonical := `$引入 工具函数.n$

回声
    收到：%参数0%

数到十
    循环>i=10
        第 %i% 次
    <循环

赋值测试
    结果:$替换 字符串 字符 X$

如果测试
    如果>%变量%==1
        是1
    >否则
        不是1
    <如果

匹配测试
    匹配>%参数0%
    如果是:你好
        你好啊
    如果不是
        不知道
    <匹配

遍历测试
    遍历>键,值=%数据%
        %键%=%值%
    <遍历

文本框测试
    内容:文本>%换行%
        第一行
        第二行
    <文本
    输出：%内容%

JSON测试
    JSON>数据={}
        名字=张三
        数量:=1
    <JSON
    JSON结果：%数据%
`
	if got := IRToNebula(NebulaToIR(canonical)); got != canonical {
		t.Errorf("规范文本往返不一致：\n--- got ---\n%s\n--- want ---\n%s", got, canonical)
	}
}

// TestNebulaToIRBlockComment 验证跨行注释（/* ... */）不会拆散词条：
// 注释出现在正文中间时，其后的代码行仍属于同一词条；出现在触发词之前时，
// 整段作为前置注释词条，注释内部的行不会被误当成触发词。
func TestNebulaToIRBlockComment(t *testing.T) {
	body := "测试词条\n    输出 \"a\"\n    /* 说明\n       多行 */\n    输出 \"b\"\n"
	ir := NebulaToIR(body)
	if len(ir.Entries) != 1 {
		t.Fatalf("正文内多行注释拆散了词条：entries=%d（%+v）", len(ir.Entries), ir.Entries)
	}
	if n := len(ir.Entries[0].Body); n != 4 {
		t.Fatalf("正文语句数不符：got=%d body=%+v", n, ir.Entries[0].Body)
	}
	if got, want := normalizeNonBlankLines(IRToNebula(ir)), normalizeNonBlankLines(body); !reflect.DeepEqual(got, want) {
		t.Errorf("正文多行注释往返不一致：\n--- got ---\n%s\n--- want ---\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got, want := normalizeNonBlankLines(IRToNebula(blocksRoundTrip(t, ir))), normalizeNonBlankLines(body); !reflect.DeepEqual(got, want) {
		t.Errorf("正文多行注释经积木层往返不一致：\n--- got ---\n%s\n--- want ---\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	// 注释内部含空行：空行不应切分词条，也不应在重发时丢失
	blank := "测试词条\n    输出 \"a\"\n    /* 说明\n\n       多行 */\n    输出 \"b\"\n"
	ir = NebulaToIR(blank)
	if len(ir.Entries) != 1 {
		t.Fatalf("注释内空行拆散了词条：entries=%d（%+v）", len(ir.Entries), ir.Entries)
	}
	if got := IRToNebula(ir); !strings.Contains(got, "/* 说明\n\n") {
		t.Errorf("多行注释内的空行未保留：\n%s", got)
	}
	if !reflect.DeepEqual(NebulaToIR(IRToNebula(ir)), ir) {
		t.Errorf("含空行的多行注释不幂等：%+v", ir)
	}

	lead := "/* 说明\n   多行 */\n测试词条\n    输出 \"a\"\n"
	ir = NebulaToIR(lead)
	if len(ir.Entries) != 2 {
		t.Fatalf("前置多行注释词条数不符：entries=%d（%+v）", len(ir.Entries), ir.Entries)
	}
	if !isCommentEntry(ir.Entries[0]) || ir.Entries[0].Name != "/* 说明" {
		t.Fatalf("前置多行注释未按注释词条解析：%+v", ir.Entries[0])
	}
	if ir.Entries[1].Name != "测试词条" {
		t.Fatalf("前置多行注释后触发词解析错误：%+v", ir.Entries[1])
	}
	if got, want := normalizeNonBlankLines(IRToNebula(ir)), normalizeNonBlankLines(lead); !reflect.DeepEqual(got, want) {
		t.Errorf("前置多行注释往返不一致：\n--- got ---\n%s\n--- want ---\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got, want := normalizeNonBlankLines(IRToNebula(blocksRoundTrip(t, ir))), normalizeNonBlankLines(lead); !reflect.DeepEqual(got, want) {
		t.Errorf("前置多行注释经积木层往返不一致：\n--- got ---\n%s\n--- want ---\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestNebulaToIRImportAssign 验证赋予值形式引入（变量:$引入 路径$）：
// 解析为 import 语句且携带实例变量名（而非退化成赋值），往返逐字一致。
func TestNebulaToIRImportAssign(t *testing.T) {
	src := "甲:$引入 我的包$\n\n回声\n    收到：%参数0%\n"
	ir := NebulaToIR(src)
	if len(ir.Header) != 1 {
		t.Fatalf("头部语句数不符：%+v", ir.Header)
	}
	s := ir.Header[0]
	if s.T != "import" || s.Name != "甲" || s.Path != "我的包" {
		t.Fatalf("赋予值引入解析结果不符：%+v", s)
	}
	if got := IRToNebula(ir); got != src {
		t.Errorf("赋予值引入往返不一致：\n--- got ---\n%s\n--- want ---\n%s", got, src)
	}
}
