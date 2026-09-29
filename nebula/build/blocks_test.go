package build

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

// blocksRoundTrip 走一遍 IR → 积木工作区 JSON → IR。
// 工作区 JSON 取 workspaces.save 的产物（整份工作区状态，形如 {"blocks": {...}}），
// 与前端 gen_dic_from_blocks 提交给 BlocksToIR 的数据形态一致。
func blocksRoundTrip(t *testing.T, ir IR) IR {
	t.Helper()
	payload, err := json.Marshal(IRToBlocks(ir))
	if err != nil {
		t.Fatalf("序列化工作区失败：%v", err)
	}
	got, err := BlocksToIR(payload)
	if err != nil {
		t.Fatalf("解析工作区失败：%v\n%s", err, payload)
	}
	return got
}

// TestBlocksRealFilesRoundTrip 真实 .n 经「IR → 积木 JSON → IR」后，
// 重新生成的 .n「非空行序列」与原文件一致，即积木层不丢行、不改内容、不错顺序。
func TestBlocksRealFilesRoundTrip(t *testing.T) {
	for _, path := range walkNebulaFiles(t) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读取 %s 失败：%v", path, err)
		}
		src := string(data)
		got := normalizeNonBlankLines(IRToNebula(blocksRoundTrip(t, NebulaToIR(src))))
		want := normalizeNonBlankLines(src)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("积木往返非空行序列不一致：%s\n--- got ---\n%s\n--- want ---\n%s",
				path, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
}

// TestBlocksIdempotent 验证积木层幂等：IR → 积木 → IR 连续两次结果结构完全一致。
func TestBlocksIdempotent(t *testing.T) {
	for _, path := range walkNebulaFiles(t) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读取 %s 失败：%v", path, err)
		}
		first := blocksRoundTrip(t, NebulaToIR(string(data)))
		second := blocksRoundTrip(t, first)
		if !reflect.DeepEqual(first, second) {
			t.Errorf("积木层不幂等：%s\n--- first ---\n%+v\n--- second ---\n%+v", path, first, second)
		}
	}
}

// TestBlocksToIRHandwritten 用手写的 Blockly 工作区 JSON 验证前端的序列化形态能被正确解析：
// 顶层块按坐标排序、触发词三种积木、类方法、初始化、内置注释（icons.comment）、
// nbc_if / nbc_match 的动态分支（extraState + CASEVAL 顶层字段）逐条还原为 .n。
func TestBlocksToIRHandwritten(t *testing.T) {
	const workspace = `{
	  "blocks": {
	    "languageVersion": 0,
	    "blocks": [
	    {
	      "type": "nbc_middleware", "x": 40, "y": 40,
	      "next": {"block": {"type": "nbc_import", "fields": {"PATH": "工具函数.n"},
	        "next": {"block": {"type": "nbc_import_as", "fields": {"NAME": "甲", "PATH": "我的包"}}}}}
	    },
	    {
	      "type": "nbc_trigger_func", "x": 40, "y": 80,
	      "fields": {"NAME": "打招呼"},
	      "icons": {"comment": {"text": "// 函数说明"}},
	      "next": {"block": {"type": "nbc_raw", "fields": {"RAW": "你好"}}}
	    },
	    {
	      "type": "nbc_trigger_inner", "x": 40, "y": 120,
	      "fields": {"NAME": "戳一戳"},
	      "next": {"block": {
	        "type": "nbc_if",
	        "extraState": {"elifCount": 1, "hasElse": true},
	        "inputs": {
	          "COND": {"shadow": {"type": "nebulaText", "fields": {"TEXT": "%变量% == 1"}}},
	          "DO": {"block": {"type": "nbc_raw", "fields": {"RAW": "是1"}}},
	          "ELIFCOND0": {"shadow": {"type": "nebulaText", "fields": {"TEXT": "%变量% == 2"}}},
	          "ELIFDO0": {"block": {"type": "nbc_raw", "fields": {"RAW": "是2"}}},
	          "ELSE": {"block": {"type": "nbc_raw", "fields": {"RAW": "其他"}}}
	        }
	      }}
	    },
	    {
	      "type": "nbc_trigger_class_func", "x": 40, "y": 160,
	      "fields": {"CLASS": "机器人", "NAME": "唤醒"},
	      "next": {"block": {
	        "type": "nbc_match",
	        "extraState": {"caseCount": 2, "hasElse": true},
	        "fields": {"CASEVAL0": "你好", "CASEVAL1": "再见"},
	        "inputs": {
	          "EXPR": {"shadow": {"type": "nebulaText", "fields": {"TEXT": "%参数0%"}}},
	          "CASEBODY0": {"block": {"type": "nbc_raw", "fields": {"RAW": "答你好"}}},
	          "CASEBODY1": {"block": {"type": "nbc_raw", "fields": {"RAW": "答再见"}}},
	          "ELSE": {"block": {"type": "nbc_raw", "fields": {"RAW": "不知道"}}}
	        }
	      }}
	    },
	    {
	      "type": "nbc_init", "x": 40, "y": 200,
	      "next": {"block": {"type": "nbc_set_var", "fields": {"NAME": "计数", "OP": "set"},
	        "inputs": {"V": {"shadow": {"type": "nebulaText", "fields": {"TEXT": "0"}}}}}}
	    }
	  ]
	  }
	}`

	ir, err := BlocksToIR([]byte(workspace))
	if err != nil {
		t.Fatalf("解析工作区失败：%v", err)
	}

	// 头部：中间件积木的正文（含赋予值形式引入）
	if len(ir.Header) != 2 || ir.Header[0].T != "import" || ir.Header[0].Path != "工具函数.n" ||
		ir.Header[1].T != "import" || ir.Header[1].Name != "甲" || ir.Header[1].Path != "我的包" {
		t.Fatalf("头部解析错误：%+v", ir.Header)
	}

	// 词条：内置注释词条 + 五种触发词形态
	wantKinds := []string{"normal", "func", "inner", "func_class", "func"}
	if len(ir.Entries) != len(wantKinds) {
		t.Fatalf("词条数错误：got %d，entries=%+v", len(ir.Entries), ir.Entries)
	}
	for i, k := range wantKinds {
		if ir.Entries[i].Kind != k {
			t.Errorf("第 %d 个词条 kind 错误：got %q want %q", i, ir.Entries[i].Kind, k)
		}
	}
	if name := ir.Entries[0].Name; name != "// 函数说明" {
		t.Errorf("内置注释词条 name 错误：%q", name)
	}
	if e := ir.Entries[3]; e.ClassName != "机器人" || e.Name != "唤醒" {
		t.Errorf("类方法词条解析错误：%+v", e)
	}
	if e := ir.Entries[4]; e.Name != initFuncName {
		t.Errorf("初始化词条 name 错误：%q", e.Name)
	}

	// nbc_if 的动态分支：extraState 数量 + ELIFCOND/ELIFDO/ELSE
	ifStmt := ir.Entries[2].Body[0]
	if ifStmt.T != "if" || ifStmt.Cond != "%变量% == 1" || len(ifStmt.Elifs) != 1 ||
		ifStmt.Elifs[0].Cond != "%变量% == 2" || len(ifStmt.Else) != 1 {
		t.Errorf("「判断」解析错误：%+v", ifStmt)
	}
	// nbc_match 的动态分支：extraState.caseCount + 顶层 CASEVAL* 字段
	matchStmt := ir.Entries[3].Body[0]
	if matchStmt.T != "match" || matchStmt.Expr != "%参数0%" || len(matchStmt.Cases) != 2 ||
		matchStmt.Cases[0].V != "你好" || matchStmt.Cases[1].V != "再见" || len(matchStmt.Def) != 1 {
		t.Errorf("「匹配」解析错误：%+v", matchStmt)
	}

	// 序列化后的 .n：非空行序列与预期逐字一致
	const want = `$引入 工具函数.n$
甲:$引入 我的包$
// 函数说明
[函数]打招呼
你好
[内部]戳一戳
如果>%变量% == 1
是1
>否则如果:%变量% == 2
是2
>否则
其他
<如果
[函数:机器人]唤醒
匹配>%参数0%
如果是:你好
答你好
如果是:再见
答再见
如果不是
不知道
<匹配
[函数]_初始化
计数:0`
	got := normalizeNonBlankLines(IRToNebula(ir))
	if !reflect.DeepEqual(got, normalizeNonBlankLines(want)) {
		t.Errorf(".n 生成结果不一致：\n--- got ---\n%s\n--- want ---\n%s", strings.Join(got, "\n"), want)
	}
}

// TestBlocksNoElseCollapsed 验证「否则 / 如果不是」区被收起（hasElse=false）时不输出分支行。
func TestBlocksNoElseCollapsed(t *testing.T) {
	const workspace = `{
	  "blocks": {
	    "languageVersion": 0,
	    "blocks": [
	    {
	      "type": "nbc_trigger", "x": 40, "y": 40, "fields": {"NAME": "测试"},
	      "next": {"block": {
	        "type": "nbc_if",
	        "extraState": {"elifCount": 0, "hasElse": false},
	        "inputs": {
	          "COND": {"shadow": {"type": "nebulaText", "fields": {"TEXT": "%变量% == 1"}}},
	          "DO": {"block": {"type": "nbc_raw", "fields": {"RAW": "是1"}}}
	        }
	      }}
	    }
	  ]
	  }
	}`
	ir, err := BlocksToIR([]byte(workspace))
	if err != nil {
		t.Fatalf("解析工作区失败：%v", err)
	}
	const want = "测试\n如果>%变量% == 1\n是1\n<如果"
	got := normalizeNonBlankLines(IRToNebula(ir))
	if !reflect.DeepEqual(got, normalizeNonBlankLines(want)) {
		t.Errorf("收起的「否则」区被错误输出：\n--- got ---\n%s\n--- want ---\n%s", strings.Join(got, "\n"), want)
	}
}

// TestBlocksToIRBothWayConsistency 验证 IRToBlocks 产出的 extraState 与前端约定一致：
// nbc_if 用 elifCount、nbc_match 用 caseCount，且 CASEVAL 落在顶层 fields。
func TestBlocksToIRBothWayConsistency(t *testing.T) {
	ir := IR{
		Header: []Stmt{},
		Entries: []Entry{{
			Kind: "normal",
			Name: "测试",
			Body: []Stmt{{
				T:     "match",
				Expr:  "%参数0%",
				Cases: []Case{{V: "甲", Body: []Stmt{{T: "raw", Text: "A"}}}, {V: "乙", Body: []Stmt{}}},
			}},
		}},
	}
	ws := IRToBlocks(ir)
	payload, err := json.Marshal(ws)
	if err != nil {
		t.Fatalf("序列化工作区失败：%v", err)
	}
	// 断言序列化结构：caseCount 与顶层 CASEVAL*
	if !strings.Contains(string(payload), `"caseCount":2`) {
		t.Errorf("nbc_match 缺少 extraState.caseCount：%s", payload)
	}
	if !strings.Contains(string(payload), `"CASEVAL0"`) || !strings.Contains(string(payload), `"CASEVAL1"`) {
		t.Errorf("nbc_match 的 CASEVAL 未落在顶层 fields：%s", payload)
	}
	// 再反向解析回来应完全一致
	back, err := BlocksToIR(payload)
	if err != nil {
		t.Fatalf("解析工作区失败：%v", err)
	}
	if !reflect.DeepEqual(ir.Entries, back.Entries) {
		t.Errorf("匹配分支往返不一致：\n--- got ---\n%+v\n--- want ---\n%+v", back.Entries, ir.Entries)
	}
}

// TestBlocksJSONRows 验证 JSON 构建积木的行式结构：IR 生成 nbc_json 时每行对应一个 VAL{i}
// 值输入、键/模式落顶层 fields、extraState.rowCount 记录行数；反向解析逐行还原，空体亦一致。
func TestBlocksJSONRows(t *testing.T) {
	ir := IR{
		Header: []Stmt{},
		Entries: []Entry{{
			Kind: "normal",
			Name: "测试",
			Body: []Stmt{{
				T:    "json",
				Name: "数据",
				Kind: "obj",
				Body: []Stmt{
					{T: "kv", Key: "名字", Mode: "=", V: "%参数0%"},
					{T: "kv", Key: "年龄", Mode: ":=", V: "18"},
				},
			}},
		}},
	}
	ws := IRToBlocks(ir)
	payload, err := json.Marshal(ws)
	if err != nil {
		t.Fatalf("序列化工作区失败：%v", err)
	}
	// 断言行式结构：rowCount + 顶层 KEY*/MODE* + VAL* 值输入
	for _, want := range []string{`"rowCount":2`, `"KEY0"`, `"KEY1"`, `"MODE1"`, `"VAL1"`} {
		if !strings.Contains(string(payload), want) {
			t.Errorf("nbc_json 缺少 %s：%s", want, payload)
		}
	}
	back, err := BlocksToIR(payload)
	if err != nil {
		t.Fatalf("解析工作区失败：%v", err)
	}
	if !reflect.DeepEqual(ir.Entries, back.Entries) {
		t.Errorf("JSON 行往返不一致：\n--- got ---\n%+v\n--- want ---\n%+v", back.Entries, ir.Entries)
	}

	// 空体：JSON 无行（JSON>变量={}）时不产生任何键值行
	empty := IR{
		Header:  []Stmt{},
		Entries: []Entry{{Kind: "normal", Name: "测试", Body: []Stmt{{T: "json", Name: "空", Kind: "arr", Body: []Stmt{}}}}},
	}
	if emptyBack := blocksRoundTrip(t, empty); !reflect.DeepEqual(empty.Entries, emptyBack.Entries) {
		t.Errorf("空 JSON 往返不一致：\n--- got ---\n%+v\n--- want ---\n%+v", emptyBack.Entries, empty.Entries)
	}
}

// TestBlocksHtmlMethodBlocks 验证对象方法积木（创建HTML 返回对象的「设置* / 创建布局 / 获取」）：
// 语句用法生成 `$对象.方法 参数$` 行；值用法（创建布局返回句柄、获取返回 HTML 文本）同样序列化为该
// 表达式并落在赋予值右侧；参数槽数由 extraState.paramCount 决定（0 参方法不写参数）。
func TestBlocksHtmlMethodBlocks(t *testing.T) {
	const workspace = `{
	  "blocks": {
	    "languageVersion": 0,
	    "blocks": [
	    {
	      "type": "nbc_trigger", "x": 40, "y": 40,
	      "fields": {"NAME": "网页"},
	      "next": {"block": {
	        "type": "nbc_set_var", "fields": {"NAME": "H", "OP": "set"},
	        "inputs": {"V": {"shadow": {"type": "nebulaText", "fields": {"TEXT": "$创建HTML 我的站点$"}}}},
	        "next": {"block": {
	          "type": "nebulaMethod_设置背景",
	          "fields": {"VAR": "H"},
	          "extraState": {"paramCount": 1},
	          "inputs": {"ARG0": {"shadow": {"type": "nebulaText", "fields": {"TEXT": "深色"}}}},
	          "next": {"block": {
	            "type": "nbc_set_var", "fields": {"NAME": "布局", "OP": "set"},
	            "inputs": {"V": {"block": {
	              "type": "nebulaMethod_创建布局",
	              "fields": {"VAR": "H"},
	              "extraState": {"paramCount": 1},
	              "inputs": {"ARG0": {"shadow": {"type": "nebulaText", "fields": {"TEXT": "容器"}}}}
	            }}},
	            "next": {"block": {
	              "type": "nebulaMethod_设置文本",
	              "fields": {"VAR": "布局"},
	              "extraState": {"paramCount": 2},
	              "inputs": {
	                "ARG0": {"shadow": {"type": "nebulaText", "fields": {"TEXT": "你好"}}},
	                "ARG1": {"shadow": {"type": "nebulaText", "fields": {"TEXT": "弱化"}}}
	              },
	              "next": {"block": {
	                "type": "nebulaMethod_设置分割线",
	                "fields": {"VAR": "H"},
	                "extraState": {"paramCount": 0},
	                "next": {"block": {
	                  "type": "nbc_set_var", "fields": {"NAME": "网页", "OP": "set"},
	                  "inputs": {"V": {"block": {
	                    "type": "nebulaMethod_获取",
	                    "fields": {"VAR": "H"},
	                    "extraState": {"paramCount": 0}
	                  }}}
	                }}
	              }}
	            }}
	          }}
	        }}
	      }}
	    }
	  ]
	  }
	}`

	ir, err := BlocksToIR([]byte(workspace))
	if err != nil {
		t.Fatalf("解析工作区失败：%v", err)
	}
	got := normalizeNonBlankLines(IRToNebula(ir))
	want := []string{
		"网页",
		"H:$创建HTML 我的站点$",
		"$H.设置背景 深色$",
		"布局:$H.创建布局 容器$",
		"$布局.设置文本 你好 弱化$",
		"$H.设置分割线$",
		"网页:$H.获取$",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("对象方法积木生成的 .n 不一致：\n--- got ---\n%s\n--- want ---\n%s",
			strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	// 生成的 IR 应为「raw 行 + 赋予值」形态（无新增 IR 语句类型）
	body := ir.Entries[0].Body
	raws := []string{}
	for _, s := range body {
		if s.T == "raw" {
			raws = append(raws, s.Text)
		}
	}
	wantRaw := []string{"$H.设置背景 深色$", "$布局.设置文本 你好 弱化$", "$H.设置分割线$"}
	if !reflect.DeepEqual(raws, wantRaw) {
		t.Errorf("对象方法语句未按 raw 行承载：\n--- got ---\n%+v\n--- want ---\n%+v", raws, wantRaw)
	}
}
