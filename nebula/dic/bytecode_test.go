package dic

import (
	"testing"

	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
)

// runNew 用字节码 VM 执行一条词库正文，返回输出。
func runNew(body []string, setup func(p *dto.Val)) string {
	out, _ := runNewState(body, setup)
	return out
}

// runNewState 用字节码 VM 执行一条词库正文，返回输出与全局终止标志。
func runNewState(body []string, setup func(p *dto.Val)) (string, bool) {
	r := dic_dto.NewRunDicEntry()
	r.Trigger = false
	if setup != nil {
		setup(r.Val.P)
	}
	out := (&dicImpl{}).dicRunLineBytecode(r, body)
	return out, r.Sys_v.Stop.Load()
}

// golden 记录每个等价性场景的期望输出（删除旧解释器后作为固定基准）。
var golden = map[string]string{
	"纯文本":            "helloworld",
	"变量插值":           "前值后",
	"判断为真":           "是",
	"判断为假走否则":        "否",
	"循环":             "123",
	"嵌套循环":           "1,11,22,12,2",
	"文本框":            "helloworld",
	"文本框变量插值":        "前值后",
	"函数框内联执行":        "hi",
	"函数框内联带触发":       "你好",
	"函数框存储调用":        "你好",
	"函数框触发词匹配":       "命中$%foo% 再见$",
	"嵌套函数框":          "外",
	"遍历":             "0:a1:b",
	"判断否则如果":         "二",
	"判断否则如果走否则":      "三",
	"循环内终止循环":        "12",
	"循环内跳过":          "1234",
	"判断内循环":          "123",
	"嵌套判断框":          "外层内层",
	"循环变量改写":         "1",
	"无限循环终止":         "123",
	"循环动态次数":         "123",
	"循环动态次数终止":       "12",
	"循环负数次数":         "尾",
	"循环动态负数次数":       "尾",
	"范围循环":           "2345",
	"范围循环负数起始":       "-1012",
	"范围循环空范围":        "尾",
	"范围循环动态":         "234",
	"判断循环":            "11",
	"循环嵌套遍历终止循环":     "0结束",
	"判断嵌套遍历":         "01",
	"JSON对象赋值":       `{"x":1,"y":"值"}`,
	"JSON路径读取":       "%@a->x->y%",
	"算术表达式":          "6",
	"遍历对象":           "a=1",
	"遍历终止":           "01尾",
	"遍历单变量":          "01",
	"遍历空源":           "尾",
	"遍历对象多键保序":       "b=2a=1c=3",
	"遍历数组非字符串值":      `0:11:{"x":2}2:[3]`,
	"嵌套遍历内层终止":       "外0:内0外1:内0",
	"文本框赋值":          "第二行",
	"文本框换行分隔":        "第一行\n第二行\n第三行",
	"文本框多行赋值":        "甲\n乙",
	"纯文本框":           "%x%$复读 你好$",
	"文本框不执行函数":       "$复读 你好$值",
	"循环内文本框":         "行1行2",
	"JSON键值框":        "k=值<JSON%a%",
	"JSON键值框追加":      `{"k":"值"}`,
	"JSON键值框赋值追加":    `{"k":"1"}`,
	"JSON键值框反序列化":    `{"k":1}`,
	"JSON键值框嵌套路径":    `{"x":{"y":"值"}}`,
	"JSON键值框数组追加":    `["1","2"]`,
	"新建JSON对象":       `{"a":1}`,
	"新建JSON数组":       `["a","b"]`,
	"连续执行":           "oo",
	"连续执行框":          "oooo",
	"连续执行框回退":        "oo",
	"连续执行框空行":        "oo",
	"赋值文本框插值":        "前你好后",
	"赋值文本框原样":        "前%v%后",
	"赋值文本框多行":        "甲\n乙",
	"赋值文本框转义":        "a'''b",
	"JS框":            "2",
	"循环变量经 map 回退读取": "abc1abc2abc3",
	"累加器槽直写后字面量自增":   "259",
	"多行JSON赋值":       `{"x":1}`,
	"多行JSON数组赋值":     `[1,2]`,
	"多行JSON嵌套赋值":     `{"x":{"y":1}}`,
	"多行JSON变量插值":     `{"x":"%v%"}`,
	"行内判断真":          "是后",
	"行内判断假":          "后",
	"行内判断否则":         "否",
	"行内判断否则真":        "是",
	"行内判断elif否则":     "二",
	"行内判断elif否则无命中":  "三",
	"行内判断elif尾后首分支":  "一后",
	"行内判断elif尾后末分支":  "二后",
	"行内判断elif尾后无命中":  "后",
	"行内判断否则尾后真":   "是后",
	"行内判断否则尾后假":   "否后",
	"行内判断elif否则尾后首": "一后",
	"行内判断elif否则尾后中": "二后",
	"行内判断elif否则尾后末": "三后",
	"行内判断英文":         "三",
	"行内判断返回尾":        "是",
	"行内判断返回尾假":       "后",
	"行内判断在循环":        "1命中234",
	"行内判断elif在循环":    "其他1二三其他4",
	"行内判断在判断框":       "内命中外",
	"行内判断elif在判断框":   "一后",
	"跳行命中":           "后",
	"跳行未命中":          "跳过后",
	"跳行负数偏移循环":       "12345ok",
	"循环中断":           "1尾",
	"匹配命中":           "二",
	"匹配走否则":          "其他",
	"匹配跳过":           "一尾",
	"匹配嵌套":           "内二",
}

// assertEquivalent 以字节码 VM 输出比对 golden 基准。
func assertEquivalent(t *testing.T, name string, body []string, setup func(p *dto.Val)) {
	t.Helper()
	want, ok := golden[name]
	if !ok {
		t.Fatalf("%s 缺少 golden 基准", name)
	}
	got := runNew(body, setup)
	if got != want {
		t.Fatalf("%s 输出不一致:\n  期望=%q\n  实际=%q", name, want, got)
	}
}

func TestEquivPlainText(t *testing.T) {
	assertEquivalent(t, "纯文本", []string{"hello", "world"}, nil)
}

func TestEquivVarInterp(t *testing.T) {
	assertEquivalent(t, "变量插值", []string{"前%x%后"}, func(p *dto.Val) {
		p.Set("x", "值")
	})
}

// TestAssignUnescape 验证赋予值直接支持 \r 换行、\\r 转义字面 \r（不再需要 "..." 包裹）。
func TestAssignUnescape(t *testing.T) {
	cases := []struct {
		name string
		body []string
		want string
	}{
		{"直接换行", []string{`a:a\rb`, "%a%"}, "a\nb"},
		{"转义反斜杠r", []string{`a:a\\rb`, "%a%"}, "a\\rb"},
		{"无转义原样", []string{`a:abc`, "%a%"}, "abc"},
		{"纯文本赋值不支持转义", []string{`a::x\ry`, "%a%"}, "x\\ry"},
		{"引号按字面处理", []string{`a:"a\rb"`, "%a%"}, "\"a\nb\""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := runNew(c.body, nil); got != c.want {
				t.Fatalf("期望=%q 实际=%q", c.want, got)
			}
		})
	}
}

// TestColonEscapeOutput 验证 \: 转义为字面冒号且不触发赋予值。
func TestColonEscapeOutput(t *testing.T) {
	cases := []struct {
		name string
		body []string
		want string
	}{
		{"转义冒号输出", []string{`名\:值`}, "名:值"},
		{"转义冒号不赋值", []string{`a\:b`, "%a%"}, "a:b%a%"},
		{"普通冒号仍赋值", []string{`a:b`, "%a%"}, "b"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := runNew(c.body, nil); got != c.want {
				t.Fatalf("期望=%q 实际=%q", c.want, got)
			}
		})
	}
}

func TestEquivIfTrue(t *testing.T) {
	assertEquivalent(t, "判断为真", []string{"如果>%x%==1", "是", "<如果"}, func(p *dto.Val) {
		p.Set("x", "1")
	})
}

func TestEquivIfElse(t *testing.T) {
	assertEquivalent(t, "判断为假走否则", []string{"如果>%x%==1", "是", ">否则", "否", "<如果"}, func(p *dto.Val) {
		p.Set("x", "2")
	})
}

func TestEquivForLoop(t *testing.T) {
	assertEquivalent(t, "循环", []string{"循环>i=3", "%i%", "<循环"}, nil)
}

func TestEquivWhileLoop(t *testing.T) {
	assertEquivalent(t, "判断循环", []string{
		"i:0",
		"判断循环>%i%<=10",
		"i+:1",
		"<循环",
		"%i%",
	}, nil)
}

func TestEquivInterrupt(t *testing.T) {
	// >中断 等价 >终止循环：跳出当前循环
	assertEquivalent(t, "循环中断", []string{
		"循环>i=5",
		"%i%",
		">中断",
		"<循环",
		"尾",
	}, nil)
}

func TestEquivNested(t *testing.T) {
	assertEquivalent(t, "嵌套循环", []string{
		"循环>i=2",
		"循环>j=2",
		"%i%,%j%",
		"<循环",
		"<循环",
	}, nil)
}

func TestEquivSwitchMatch(t *testing.T) {
	assertEquivalent(t, "匹配命中", []string{
		"匹配>%x%",
		"如果是:1", "一",
		"如果是:2", "二",
		"如果是:3", "三",
		"如果不是", "其他",
		"<匹配",
	}, func(p *dto.Val) { p.Set("x", "2") })
}

func TestEquivSwitchDefault(t *testing.T) {
	assertEquivalent(t, "匹配走否则", []string{
		"匹配>%x%",
		"如果是:1", "一",
		"如果是:2", "二",
		"如果不是", "其他",
		"<匹配",
	}, func(p *dto.Val) { p.Set("x", "9") })
}

func TestEquivSwitchSkip(t *testing.T) {
	assertEquivalent(t, "匹配跳过", []string{
		"匹配>%x%",
		"如果是:1", "一", ">跳过", "二",
		"如果不是", "其他",
		"<匹配",
		"尾",
	}, func(p *dto.Val) { p.Set("x", "1") })
}

func TestEquivSwitchNested(t *testing.T) {
	assertEquivalent(t, "匹配嵌套", []string{
		"匹配>%x%",
		"如果是:1", "匹配>%y%", "如果是:2", "内二", "<匹配",
		"如果不是", "其他",
		"<匹配",
	}, func(p *dto.Val) { p.Set("x", "1"); p.Set("y", "2") })
}

func TestEquivTextBlock(t *testing.T) {
	assertEquivalent(t, "文本框", []string{"文本>", "hello", "world", "<文本"}, nil)
}

func TestEquivTextBlockInterp(t *testing.T) {
	// 文本框内容 %变量% 插值（委托给解释器，语义一致）
	assertEquivalent(t, "文本框变量插值", []string{"文本>", "前%x%后", "<文本"}, func(p *dto.Val) {
		p.Set("x", "值")
	})
}

func TestEquivFuncInline(t *testing.T) {
	// 裸 函数> 内联执行（新作用域执行内容并输出）
	assertEquivalent(t, "函数框内联执行", []string{"函数>", "hi", "<函数"}, nil)
}

func TestEquivFuncInlineTrigger(t *testing.T) {
	// 函数>触发词 内联执行（值名留空），新作用域「触发」设为触发词
	assertEquivalent(t, "函数框内联带触发", []string{"函数>=你好", "%触发%", "<函数"}, nil)
}

func TestEquivFuncBoxStore(t *testing.T) {
	// 函数>变量名 存储函数框，经 $%变量名% 参数$ 调用
	assertEquivalent(t, "函数框存储调用", []string{
		"函数>foo",
		"你好",
		"<函数",
		"$%foo% 世界$",
	}, nil)
}

func TestEquivFuncBoxTrigger(t *testing.T) {
	// 函数>变量名=触发 存储带触发词的函数框：匹配则执行，不匹配则原样回显
	assertEquivalent(t, "函数框触发词匹配", []string{
		"函数>foo=你好",
		"命中",
		"<函数",
		"$%foo% 你好$",
		"$%foo% 再见$",
	}, nil)
}

func TestEquivNestedFuncBox(t *testing.T) {
	// 嵌套函数框：外层内容含内层函数框原始行，整体存储后调用执行
	assertEquivalent(t, "嵌套函数框", []string{
		"函数>outer",
		"外",
		"函数>inner",
		"内",
		"<函数",
		"<函数",
		"$%outer% x$",
	}, nil)
}

func TestEquivForEach(t *testing.T) {
	// 遍历 JSON 数组，键为下标、值为元素
	assertEquivalent(t, "遍历", []string{"遍历>k,v=%数据%", "%k%:%v%", "<遍历"}, func(p *dto.Val) {
		p.Set("数据", `["a","b"]`)
	})
}

func TestEquivIfElseIf(t *testing.T) {
	// 判断框 >否则如果 命中中间分支
	assertEquivalent(t, "判断否则如果", []string{
		"如果>%x%==1", "一",
		">否则如果:%x%==2", "二",
		">否则", "三",
		"<如果",
	}, func(p *dto.Val) { p.Set("x", "2") })
}

func TestEquivIfElseIfFallthrough(t *testing.T) {
	assertEquivalent(t, "判断否则如果走否则", []string{
		"如果>%x%==1", "一",
		">否则如果:%x%==2", "二",
		">否则", "三",
		"<如果",
	}, func(p *dto.Val) { p.Set("x", "3") })
}

func TestEquivForBreak(t *testing.T) {
	assertEquivalent(t, "循环内终止循环", []string{
		"循环>i=5",
		"如果>%i%==3",
		">终止循环",
		"<如果",
		"%i%",
		"<循环",
	}, nil)
}

func TestEquivForContinue(t *testing.T) {
	assertEquivalent(t, "循环内跳过", []string{
		"循环>i=4",
		"如果>%i%==2",
		">跳过",
		"<如果",
		"%i%",
		"<循环",
	}, nil)
}

func TestEquivForInIf(t *testing.T) {
	assertEquivalent(t, "判断内循环", []string{
		"如果>%x%==1",
		"循环>i=3",
		"%i%",
		"<循环",
		"<如果",
	}, func(p *dto.Val) { p.Set("x", "1") })
}

func TestEquivIfNested(t *testing.T) {
	assertEquivalent(t, "嵌套判断框", []string{
		"如果>%x%==1",
		"外层",
		"如果>%x%==1",
		"内层",
		"<如果",
		">否则",
		"否",
		"<如果",
	}, func(p *dto.Val) { p.Set("x", "1") })
}

func TestEquivHalt(t *testing.T) {
	// >终止 与 >终止 文案：固定基准校验输出与全局终止标志。
	for name, tc := range map[string]struct {
		body    []string
		wantOut string
	}{
		"终止":   {[]string{"A", ">终止", "B"}, "A"},
		"终止文案": {[]string{"A", ">终止 已停", "B"}, "A已停"},
	} {
		got, stopped := runNewState(tc.body, nil)
		if got != tc.wantOut || !stopped {
			t.Fatalf("%s 不一致:\n  期望输出=%q 终止=true\n  实际输出=%q 终止=%v", name, tc.wantOut, got, stopped)
		}
	}
}

func TestEquivHaltInLoop(t *testing.T) {
	// >终止 在循环内应全局终止（不只是跳出循环）
	body := []string{"循环>i=5", "%i%", "如果>%i%==3", ">终止", "<如果", "<循环", "尾部"}
	got, stopped := runNewState(body, nil)
	if got != "123" || !stopped {
		t.Fatalf("循环内终止 不一致:\n  期望输出=%q 终止=true\n  实际输出=%q 终止=%v", "123", got, stopped)
	}
}

func TestEquivForModifyVar(t *testing.T) {
	// 循环体内改写循环变量：解释器会据此调整循环序号（i 跳到 3 后再 +1 超过次数即结束）
	assertEquivalent(t, "循环变量改写", []string{
		"循环>i=3",
		"%i%",
		"i:3",
		"<循环",
	}, nil)
}

func TestEquivForInfiniteBreak(t *testing.T) {
	// 无限循环（无次数）配合 >终止循环 跳出
	assertEquivalent(t, "无限循环终止", []string{
		"循环>i",
		"%i%",
		"如果>%i%==3",
		">终止循环",
		"<如果",
		"<循环",
	}, nil)
}

func TestEquivForDynamicCount(t *testing.T) {
	// 循环次数为变量（运行时求值）：整段委托旧解释器
	assertEquivalent(t, "循环动态次数", []string{
		"循环>i=%n%",
		"%i%",
		"<循环",
	}, func(p *dto.Val) { p.Set("n", "3") })
}

func TestEquivForDynamicCountBreak(t *testing.T) {
	// 动态次数循环内 >终止循环 跳出
	assertEquivalent(t, "循环动态次数终止", []string{
		"循环>i=%n%",
		"%i%",
		"如果>%i%==2",
		">终止循环",
		"<如果",
		"<循环",
	}, func(p *dto.Val) { p.Set("n", "5") })
}

func TestEquivForNegativeCount(t *testing.T) {
	// 负数次数等价 0 次（不执行循环体）
	assertEquivalent(t, "循环负数次数", []string{
		"循环>i=-3",
		"%i%",
		"<循环",
		"尾",
	}, nil)
}

func TestEquivForDynamicNegativeCount(t *testing.T) {
	// 动态求值为负数次数：不执行循环体
	assertEquivalent(t, "循环动态负数次数", []string{
		"循环>i=%n%",
		"%i%",
		"<循环",
		"尾",
	}, func(p *dto.Val) { p.Set("n", "-2") })
}

func TestEquivForRange(t *testing.T) {
	// 范围循环：循环>变量=起始~结束，循环变量从起始值递增到结束值（含两端）。
	assertEquivalent(t, "范围循环", []string{
		"循环>i=2~5",
		"%i%",
		"<循环",
	}, nil)
}

func TestEquivForRangeNegativeStart(t *testing.T) {
	// 负数起始值：循环变量从负数起步递增。
	assertEquivalent(t, "范围循环负数起始", []string{
		"循环>i=-1~2",
		"%i%",
		"<循环",
	}, nil)
}

func TestEquivForRangeEmpty(t *testing.T) {
	// 空范围（起始值 > 结束值）：不执行循环体。
	assertEquivalent(t, "范围循环空范围", []string{
		"循环>i=5~2",
		"%i%",
		"<循环",
		"尾",
	}, nil)
}

func TestEquivForRangeDynamic(t *testing.T) {
	// 动态范围：起始/结束值由变量求值。
	assertEquivalent(t, "范围循环动态", []string{
		"循环>i=%a%~%b%",
		"%i%",
		"<循环",
	}, func(p *dto.Val) {
		p.Set("a", "2")
		p.Set("b", "4")
	})
}

func TestEquivLoopWithForEach(t *testing.T) {
	// 循环内嵌遍历：整段委托旧解释器，确保 >终止循环 能跨遍历向上传播
	assertEquivalent(t, "循环嵌套遍历终止循环", []string{
		"循环>x=2",
		"遍历>j,jj=[\"a\",\"b\"]",
		"%j%",
		">终止循环",
		"<遍历",
		"X",
		"<循环",
		"结束",
	}, nil)
}

func TestEquivIfWithForEach(t *testing.T) {
	// 判断内嵌遍历：整段委托旧解释器
	assertEquivalent(t, "判断嵌套遍历", []string{
		"如果>%x%==1",
		"遍历>i,ii=[\"a\",\"b\"]",
		"%i%",
		"<遍历",
		"<如果",
	}, func(p *dto.Val) { p.Set("x", "1") })
}

func TestEquivJsonAssign(t *testing.T) {
	// 单行 JSON 对象赋值（叶子语句由 Runtime.Line 委托解释器执行）
	assertEquivalent(t, "JSON对象赋值", []string{
		`a:{"x":1,"y":"值"}`,
		"%a%",
	}, nil)
}

func TestEquivJsonPath(t *testing.T) {
	// @json 路径读取
	assertEquivalent(t, "JSON路径读取", []string{
		`a:{"x":{"y":"结果"}}`,
		"%@a->x->y%",
	}, nil)
}

func TestEquivArithBracket(t *testing.T) {
	// [表达式] 算术
	assertEquivalent(t, "算术表达式", []string{
		"a:3",
		"b:[%a%*2]",
		"%b%",
	}, nil)
}

func TestEquivForEachObject(t *testing.T) {
	// 遍历 JSON 对象（键值对）
	assertEquivalent(t, "遍历对象", []string{
		`遍历>k,v={"a":1}`,
		"%k%=%v%",
		"<遍历",
	}, nil)
}

func TestEquivForEachBreak(t *testing.T) {
	// 遍历内 >终止遍历 跳出
	assertEquivalent(t, "遍历终止", []string{
		`遍历>i,ii=["a","b","c"]`,
		"%i%",
		"如果>%i%==1",
		">终止遍历",
		"<如果",
		"<遍历",
		"尾",
	}, nil)
}

func TestEquivForEachSingleVar(t *testing.T) {
	// 单变量遍历：v1 为下标，v2 为 "_"
	assertEquivalent(t, "遍历单变量", []string{
		`遍历>i=["a","b"]`,
		"%i%",
		"<遍历",
	}, nil)
}

func TestEquivForEachEmpty(t *testing.T) {
	// 无遍历源（无 = 表达式）：不迭代
	assertEquivalent(t, "遍历空源", []string{
		"遍历>k",
		"不会输出",
		"<遍历",
		"尾",
	}, nil)
}

func TestEquivForEachObjectMulti(t *testing.T) {
	// 遍历多键对象：jsonparser 保序（b、a、c 依次输出）
	assertEquivalent(t, "遍历对象多键保序", []string{
		`遍历>k,v={"b":2,"a":1,"c":3}`,
		"%k%=%v%",
		"<遍历",
	}, nil)
}

func TestEquivForEachArrayNonString(t *testing.T) {
	// 遍历数组非字符串元素：非字符串经 json.Marshal 转为字符串
	assertEquivalent(t, "遍历数组非字符串值", []string{
		`遍历>i,v=[1,{"x":2},[3]]`,
		"%i%:%v%",
		"<遍历",
	}, nil)
}

func TestEquivForEachNestedBreak(t *testing.T) {
	// 遍历体含嵌套遍历，内层 >终止遍历 仅跳出内层，外层继续
	assertEquivalent(t, "嵌套遍历内层终止", []string{
		`遍历>i,v=[1,2]`,
		"外%i%:",
		`遍历>j,w=["a","b"]`,
		"内%j%",
		">终止遍历",
		"<遍历",
		"<遍历",
	}, nil)
}

func TestEquivTextBlockAssign(t *testing.T) {
	// 文本框赋值到变量
	assertEquivalent(t, "文本框赋值", []string{
		"文本>x=第一行",
		"第二行",
		"<文本",
		"%x%",
	}, nil)
}

func TestEquivTextBlockLineFeed(t *testing.T) {
	// 开启行后缀 %变量% 插值作为内容行间分隔符
	assertEquivalent(t, "文本框换行分隔", []string{
		"文本>%换行%",
		"第一行",
		"第二行",
		"第三行",
		"<文本",
	}, func(p *dto.Val) { p.Set("换行", "\n") })
}

func TestEquivTextBlockAssignMulti(t *testing.T) {
	// 文本框赋值且含多行 + 分隔符
	assertEquivalent(t, "文本框多行赋值", []string{
		"文本>x=%换行%",
		"甲",
		"乙",
		"<文本",
		"%x%",
	}, func(p *dto.Val) { p.Set("换行", "\n") })
}

func TestEquivPureTextBlock(t *testing.T) {
	// 纯文本> 内容原样输出，不做 %变量% 插值
	assertEquivalent(t, "纯文本框", []string{
		"纯文本>",
		"%x%",
		"$复读 你好$",
		"<文本",
	}, func(p *dto.Val) { p.Set("x", "值") })
}

func TestEquivTextBlockNoFunc(t *testing.T) {
	// 文本> 内容只插值 %变量%，不执行 $函数$
	assertEquivalent(t, "文本框不执行函数", []string{
		"文本>",
		"$复读 你好$",
		"%x%",
		"<文本",
	}, func(p *dto.Val) { p.Set("x", "值") })
}

func TestEquivTextBlockInLoop(t *testing.T) {
	// 循环内嵌文本框：文本框已原生下沉，循环应照常展开
	assertEquivalent(t, "循环内文本框", []string{
		"循环>i=2",
		"文本>",
		"行%i%",
		"<文本",
		"<循环",
	}, nil)
}

func TestEquivJsonBlock(t *testing.T) {
	// JSON> 键值框赋值
	assertEquivalent(t, "JSON键值框", []string{
		"JSON>a",
		"k=值",
		"<JSON",
		"%a%",
	}, nil)
}

func TestEquivJsonBlockAdd(t *testing.T) {
	// JSON> 初始 {} + key=value 追加（字符串值）
	assertEquivalent(t, "JSON键值框追加", []string{
		"JSON>{}",
		"k=值",
		"<JSON",
	}, nil)
}

func TestEquivJsonBlockAssignAdd(t *testing.T) {
	// JSON>a={} 赋值 + key=value 追加后经变量读出
	assertEquivalent(t, "JSON键值框赋值追加", []string{
		"JSON>a={}",
		"k=1",
		"<JSON",
		"%a%",
	}, nil)
}

func TestEquivJsonBlockAssignUnmarshal(t *testing.T) {
	// JSON>a={} + key:=value（右值反序列化，数字保持为数字）
	assertEquivalent(t, "JSON键值框反序列化", []string{
		"JSON>a={}",
		"k:=1",
		"<JSON",
		"%a%",
	}, nil)
}

func TestEquivJsonBlockNestedPath(t *testing.T) {
	// key->sub 嵌套路径
	assertEquivalent(t, "JSON键值框嵌套路径", []string{
		"JSON>a={}",
		"x->y=值",
		"<JSON",
		"%a%",
	}, nil)
}

func TestEquivJsonBlockArrayAppend(t *testing.T) {
	// [] 数组追加
	assertEquivalent(t, "JSON键值框数组追加", []string{
		"JSON>a=[]",
		"[]=1",
		"[]=2",
		"<JSON",
		"%a%",
	}, nil)
}

func TestEquivNewJsonObject(t *testing.T) {
	// JSON>{ 新建 JSON 对象
	assertEquivalent(t, "新建JSON对象", []string{
		"JSON>{",
		`"a":1`,
		"}",
	}, nil)
}

func TestEquivNewJsonArray(t *testing.T) {
	// JSON>[ 新建 JSON 数组
	assertEquivalent(t, "新建JSON数组", []string{
		"JSON>[",
		`"a",`,
		`"b"`,
		"]",
	}, nil)
}

func TestEquivInlineTextChain(t *testing.T) {
	// 单行 >>> 连续执行（原生下沉）
	assertEquivalent(t, "连续执行", []string{
		"a:o>>>$复读 %a%$",
		"%a%",
	}, nil)
}

func TestEquivValChainBlock(t *testing.T) {
	// 变量:>>> ... <<< 连续执行框：逐行执行写回变量（原生下沉为 OpValChainBlock）
	assertEquivalent(t, "连续执行框", []string{
		"f:>>>",
		"o",
		"$复读 %f%$",
		"$复读 %f%$",
		"<<<",
		"%f%",
	}, nil)
}

func TestEquivValChainBlockQuestion(t *testing.T) {
	// 连续执行框内 ?: 回退
	assertEquivalent(t, "连续执行框回退", []string{
		"f:>>>",
		"?:o",
		"$复读 %f%$",
		"<<<",
		"%f%",
	}, nil)
}

func TestEquivValChainBlockEmptyLine(t *testing.T) {
	// 连续执行框内空行跳过
	assertEquivalent(t, "连续执行框空行", []string{
		"f:>>>",
		"o",
		"",
		"$复读 %f%$",
		"<<<",
		"%f%",
	}, nil)
}

func TestEquivValTextBlock(t *testing.T) {
	// 变量:""" 赋值文本框（内容 %变量% 插值）
	assertEquivalent(t, "赋值文本框插值", []string{
		"v:你好",
		`t:"""`,
		"前%v%后",
		`"""`,
		"%t%",
	}, nil)
}

func TestEquivValTextBlockRaw(t *testing.T) {
	// 变量:''' 赋值文本框（内容原样，不做插值）
	assertEquivalent(t, "赋值文本框原样", []string{
		"v:你好",
		`t:'''`,
		"前%v%后",
		`'''`,
		"%t%",
	}, nil)
}

func TestEquivValTextBlockMultiline(t *testing.T) {
	// 赋值文本框多行以 \n 连接
	assertEquivalent(t, "赋值文本框多行", []string{
		`t:"""`,
		"甲",
		"乙",
		`"""`,
		"%t%",
	}, nil)
}

func TestEquivValTextBlockEscape(t *testing.T) {
	// 原样文本框内 \''' 转义为 '''
	assertEquivalent(t, "赋值文本框转义", []string{
		`t:'''`,
		`a\'''b`,
		`'''`,
		"%t%",
	}, nil)
}

func TestEquivNodeJsBlock(t *testing.T) {
	// --js ... --end JS 框：运行 JS 并返回最后表达式结果
	assertEquivalent(t, "JS框", []string{
		"--js",
		"1+1",
		"--end",
	}, nil)
}

func TestEquivLoopAccumulatorReadback(t *testing.T) {
	// 循环体内：循环变量（槽直写）作为非纯整数右操作数时，回退路径会经 map 读取。
	// 旧解释器 n 依次拼接 abc1 / abc12 / abc123，字节码若槽/map 不同步会读到空值。
	assertEquivalent(t, "循环变量经 map 回退读取", []string{
		"循环>i=3",
		"n:abc",
		"n+:%i%",
		"%n%",
		"<循环",
	}, nil)
}

func TestEquivLoopAccumulatorLiteral(t *testing.T) {
	// 累加器先经整数快路径写槽，再经字面量算术回退路径读 map（n 只存在槽中）。
	assertEquivalent(t, "累加器槽直写后字面量自增", []string{
		"循环>i=3",
		"n+:%i%",
		"n+:1",
		"%n%",
		"<循环",
	}, nil)
}

func TestEquivMultiLineJsonAssign(t *testing.T) {
	// 多行 JSON 对象赋值（变量:{ ... }）原生下沉为 OpVarNewJsonBlock。
	// 单键避免 Go map 键序随机导致字符串比较不稳定。
	assertEquivalent(t, "多行JSON赋值", []string{
		"a:{",
		`"x":1`,
		"}",
		"%a%",
	}, nil)
}

func TestEquivMultiLineJsonArrayAssign(t *testing.T) {
	// 多行 JSON 数组赋值（变量:[ ... ]）原生下沉
	assertEquivalent(t, "多行JSON数组赋值", []string{
		"a:[",
		"1,",
		"2",
		"]",
		"%a%",
	}, nil)
}

func TestEquivMultiLineJsonNestedAssign(t *testing.T) {
	// 多行 JSON 嵌套对象赋值（含内层 { } 平衡）
	assertEquivalent(t, "多行JSON嵌套赋值", []string{
		"a:{",
		`"x":{`,
		`"y":1`,
		"}",
		"}",
		"%a%",
	}, nil)
}

func TestEquivMultiLineJsonVarInterp(t *testing.T) {
	// 多行 JSON 内容含 %变量% 插值
	assertEquivalent(t, "多行JSON变量插值", []string{
		"v:你好",
		"a:{",
		`"x":"%v%"`,
		"}",
		"%a%",
	}, nil)
}

func TestEquivInlineIfTrue(t *testing.T) {
	// 行内 如果: 命中 + 如果尾 终止，后续语句继续执行
	assertEquivalent(t, "行内判断真", []string{
		"如果:%x%==1",
		"是",
		"如果尾",
		"后",
	}, func(p *dto.Val) { p.Set("x", "1") })
}

func TestEquivInlineIfFalse(t *testing.T) {
	// 行内 如果: 未命中：跳过正文，如果尾 后继续
	assertEquivalent(t, "行内判断假", []string{
		"如果:%x%==1",
		"是",
		"如果尾",
		"后",
	}, func(p *dto.Val) { p.Set("x", "2") })
}

func TestEquivInlineIfElse(t *testing.T) {
	// 行内 如果: ... 否则（无 如果尾）：else 正文延续到块末尾
	assertEquivalent(t, "行内判断否则", []string{
		"如果:%x%==1",
		"是",
		"否则",
		"否",
	}, func(p *dto.Val) { p.Set("x", "2") })
}

func TestEquivInlineIfElseTrue(t *testing.T) {
	assertEquivalent(t, "行内判断否则真", []string{
		"如果:%x%==1",
		"是",
		"否则",
		"否",
	}, func(p *dto.Val) { p.Set("x", "1") })
}

func TestEquivInlineIfElifElse(t *testing.T) {
	// 否则如果 命中中间分支
	assertEquivalent(t, "行内判断elif否则", []string{
		"如果:%x%==1",
		"一",
		"否则如果:%x%==2",
		"二",
		"否则",
		"三",
	}, func(p *dto.Val) { p.Set("x", "2") })
}

func TestEquivInlineIfElifElseNone(t *testing.T) {
	assertEquivalent(t, "行内判断elif否则无命中", []string{
		"如果:%x%==1",
		"一",
		"否则如果:%x%==2",
		"二",
		"否则",
		"三",
	}, func(p *dto.Val) { p.Set("x", "3") })
}

func TestEquivInlineIfElifEndifFirst(t *testing.T) {
	// elif 链以 如果尾 收尾且其后有语句：命中首个分支后跳到 如果尾 之后继续
	assertEquivalent(t, "行内判断elif尾后首分支", []string{
		"如果:%x%==1",
		"一",
		"否则如果:%x%==2",
		"二",
		"如果尾",
		"后",
	}, func(p *dto.Val) { p.Set("x", "1") })
}

func TestEquivInlineIfElifEndifLast(t *testing.T) {
	assertEquivalent(t, "行内判断elif尾后末分支", []string{
		"如果:%x%==1",
		"一",
		"否则如果:%x%==2",
		"二",
		"如果尾",
		"后",
	}, func(p *dto.Val) { p.Set("x", "2") })
}

func TestEquivInlineIfElifEndifNone(t *testing.T) {
	assertEquivalent(t, "行内判断elif尾后无命中", []string{
		"如果:%x%==1",
		"一",
		"否则如果:%x%==2",
		"二",
		"如果尾",
		"后",
	}, func(p *dto.Val) { p.Set("x", "3") })
}

func TestEquivInlineIfElseEndifTrue(t *testing.T) {
	// 否则 + 如果尾 自动结尾：命中真分支后跳到 如果尾 之后继续
	assertEquivalent(t, "行内判断否则尾后真", []string{
		"如果:%x%==1",
		"是",
		"否则",
		"否",
		"如果尾",
		"后",
	}, func(p *dto.Val) { p.Set("x", "1") })
}

func TestEquivInlineIfElseEndifFalse(t *testing.T) {
	// 否则 + 如果尾 自动结尾：走否则分支，如果尾 之后继续
	assertEquivalent(t, "行内判断否则尾后假", []string{
		"如果:%x%==1",
		"是",
		"否则",
		"否",
		"如果尾",
		"后",
	}, func(p *dto.Val) { p.Set("x", "2") })
}

func TestEquivInlineIfElifElseEndifFirst(t *testing.T) {
	assertEquivalent(t, "行内判断elif否则尾后首", []string{
		"如果:%x%==1",
		"一",
		"否则如果:%x%==2",
		"二",
		"否则",
		"三",
		"如果尾",
		"后",
	}, func(p *dto.Val) { p.Set("x", "1") })
}

func TestEquivInlineIfElifElseEndifMid(t *testing.T) {
	assertEquivalent(t, "行内判断elif否则尾后中", []string{
		"如果:%x%==1",
		"一",
		"否则如果:%x%==2",
		"二",
		"否则",
		"三",
		"如果尾",
		"后",
	}, func(p *dto.Val) { p.Set("x", "2") })
}

func TestEquivInlineIfElifElseEndifLast(t *testing.T) {
	assertEquivalent(t, "行内判断elif否则尾后末", []string{
		"如果:%x%==1",
		"一",
		"否则如果:%x%==2",
		"二",
		"否则",
		"三",
		"如果尾",
		"后",
	}, func(p *dto.Val) { p.Set("x", "3") })
}

func TestEquivInlineIfEnglish(t *testing.T) {
	// 英文缩写 if:/elif:/else
	assertEquivalent(t, "行内判断英文", []string{
		"if:%x%==1",
		"是",
		"elif:%x%==2",
		"二",
		"else",
		"三",
	}, func(p *dto.Val) { p.Set("x", "3") })
}

func TestEquivInlineIfReturnEndif(t *testing.T) {
	// 返回 + 如果尾 等价终止标记（命中时后续继续）
	assertEquivalent(t, "行内判断返回尾", []string{
		"如果:%x%==1",
		"是",
		"返回",
		"如果尾",
		"后",
	}, func(p *dto.Val) { p.Set("x", "1") })
}

func TestEquivInlineIfReturnEndifFalse(t *testing.T) {
	assertEquivalent(t, "行内判断返回尾假", []string{
		"如果:%x%==1",
		"是",
		"返回",
		"如果尾",
		"后",
	}, func(p *dto.Val) { p.Set("x", "2") })
}

func TestEquivInlineIfInLoop(t *testing.T) {
	// 行内判断在循环体内：逐轮重新求值
	assertEquivalent(t, "行内判断在循环", []string{
		"循环>i=4",
		"如果:%i%==2",
		"命中",
		"如果尾",
		"%i%",
		"<循环",
	}, nil)
}

func TestEquivInlineIfElifInLoop(t *testing.T) {
	// 行内判断含 elif/else 在循环体内：命中分支后 break 等价 continue
	assertEquivalent(t, "行内判断elif在循环", []string{
		"循环>i=4",
		"如果:%i%==2",
		"二",
		"否则如果:%i%==3",
		"三",
		"否则",
		"其他",
		"%i%",
		"<循环",
	}, nil)
}

func TestEquivInlineIfInIfBlock(t *testing.T) {
	// 行内判断嵌套在判断框分支内
	assertEquivalent(t, "行内判断在判断框", []string{
		"如果>%x%==1",
		"如果:%x%==1",
		"内命中",
		"如果尾",
		"外",
		"<如果",
	}, func(p *dto.Val) { p.Set("x", "1") })
}

func TestEquivInlineIfElifInIfBlock(t *testing.T) {
	// 行内判断含 elif/else 嵌套在判断框分支内：命中后 break 到判断框末尾
	assertEquivalent(t, "行内判断elif在判断框", []string{
		"如果>%x%==1",
		"如果:%x%==1",
		"一",
		"否则如果:%x%==2",
		"二",
		"否则",
		"三",
		"<如果",
		"后",
	}, func(p *dto.Val) { p.Set("x", "1") })
}

func TestEquivJumpLineCond(t *testing.T) {
	// >跳行(条件)>>偏移 命中：相对跳行跳过下一行（当前整段回退旧解释器，仅锁定语义）
	assertEquivalent(t, "跳行命中", []string{
		">跳行(%x%==1)>>1",
		"跳过",
		"后",
	}, func(p *dto.Val) { p.Set("x", "1") })
}

func TestEquivJumpLineCondMiss(t *testing.T) {
	// >跳行(条件)>>偏移 未命中：顺序执行
	assertEquivalent(t, "跳行未命中", []string{
		">跳行(%x%==1)>>1",
		"跳过",
		"后",
	}, func(p *dto.Val) { p.Set("x", "2") })
}

func TestEquivJumpLineNegLoop(t *testing.T) {
	// >跳行 负数偏移回退（真实 test.n Main 用到的局部循环写法）：i 自增到 5 后顺序落到终止
	assertEquivalent(t, "跳行负数偏移循环", []string{
		"i+:1",
		"%i%",
		">跳行(%i%!=5)>>-2",
		">终止 ok",
		">终止 ok2",
	}, nil)
}
