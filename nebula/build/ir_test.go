package build

import (
	"encoding/json"
	"testing"
)

// TestIRToNebula 验证 IR → .n 序列化输出，覆盖所有语句类型与格式规则。
func TestIRToNebula(t *testing.T) {
	ir := IR{
		Header: []Stmt{{T: "import", Path: "工具函数.n"}},
		Entries: []Entry{
			{Kind: "normal", Name: "回声", Body: []Stmt{{T: "output", V: "收到：%参数0%"}}},
			{Kind: "normal", Name: "数到十", Body: []Stmt{{T: "loopCount", Var: "i", Count: "10", Body: []Stmt{{T: "output", V: "第 %i% 次"}}}}},
			{Kind: "normal", Name: "赋值测试", Body: []Stmt{{T: "assign", Name: "结果", Op: "set", V: "$替换 字符串 字符 X$"}}},
			{Kind: "normal", Name: "如果测试", Body: []Stmt{{T: "if", Cond: "%变量%==1", Do: []Stmt{{T: "output", V: "是1"}}, Else: []Stmt{{T: "output", V: "不是1"}}}}},
			{Kind: "normal", Name: "匹配测试", Body: []Stmt{{T: "match", Expr: "%参数0%", Cases: []Case{{V: "你好", Body: []Stmt{{T: "output", V: "你好啊"}}}}, Def: []Stmt{{T: "output", V: "不知道"}}}}},
			{Kind: "normal", Name: "遍历测试", Body: []Stmt{{T: "foreach", Mode: "kv", Key: "键", Val: "值", Target: "%数据%", Body: []Stmt{{T: "output", V: "%键%=%值%"}}}}},
			{Kind: "normal", Name: "文本框测试", Body: []Stmt{{T: "textblock", Name: "内容", Sep: "%换行%", Body: []Stmt{{T: "output", V: "第一行"}, {T: "output", V: "第二行"}}}, {T: "output", V: "输出：%内容%"}}},
			{Kind: "normal", Name: "JSON测试", Body: []Stmt{{T: "json", Name: "数据", Kind: "obj", Body: []Stmt{{T: "kv", Key: "名字", Mode: "=", V: "张三"}, {T: "kv", Key: "数量", Mode: ":=", V: "1"}}}, {T: "output", V: "JSON结果：%数据%"}}},
		},
	}

	got := IRToNebula(ir)
	want := `$引入 工具函数.n$

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
	if got != want {
		t.Errorf("IRToNebula 输出不符：\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestIRToNebulaNoHeader 验证无头部时开头补空行（避免第一个词条被吞进头部）。
func TestIRToNebulaNoHeader(t *testing.T) {
	ir := IR{Entries: []Entry{{Kind: "normal", Name: "回声", Body: []Stmt{{T: "output", V: "收到：%参数0%"}}}}}
	got := IRToNebula(ir)
	want := "\n回声\n    收到：%参数0%\n"
	if got != want {
		t.Errorf("无头部输出不符：got %q, want %q", got, want)
	}
}

// TestIRUnmarshal 验证前端积木编程 workspaceToIr 产出的 JSON 能被 build.IR 正确解析，
// 并序列化为 .n（前后端契约对齐：字段名与类型一致）。
func TestIRUnmarshal(t *testing.T) {
	raw := `{
		"header": [{"t":"import","path":"工具函数.n"}],
		"entries": [
			{"kind":"normal","name":"回声","body":[{"t":"output","v":"收到：%参数0%"}]},
			{"kind":"normal","name":"循环","body":[{"t":"loopCount","var":"i","count":"10","body":[{"t":"output","v":"第 %i% 次"}]}]},
			{"kind":"func","name":"打招呼","body":[{"t":"assign","name":"名","op":"set","v":"张三"},{"t":"output","v":"你好 %名%"}]}
		]
	}`
	var ir IR
	if err := json.Unmarshal([]byte(raw), &ir); err != nil {
		t.Fatalf("JSON 反序列化失败: %v", err)
	}
	got := IRToNebula(ir)
	want := `$引入 工具函数.n$

回声
    收到：%参数0%

循环
    循环>i=10
        第 %i% 次
    <循环

[函数]打招呼
    名:张三
    你好 %名%
`
	if got != want {
		t.Errorf("反序列化后序列化输出不符：\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
