package bc

import (
	"strconv"
	"strings"
	"testing"

	"github.com/cjxpj/nebula/dic/ast"
)

// mockRT 一个最小运行时：叶子语句回显文本并支持 %var% 插值，条件支持 true/false 与 ==比较。
type mockRT struct {
	vars     map[string]string
	feFrames []mockForEachFrame
	out      strings.Builder
}

// mockForEachFrame 模拟遍历帧（按 VM 帧深度隔离）。
type mockForEachFrame struct {
	v1    string
	v2    string
	items []mockForEachItem
}

// mockForEachItem 模拟遍历项（键为数组下标字符串，值为项字符串）。
type mockForEachItem struct {
	key   string
	value string
}

func newMockRT() *mockRT { return &mockRT{vars: map[string]string{}} }

func (m *mockRT) Line(line int, text string) string {
	// 简单 %var% 插值
	var b strings.Builder
	for i := 0; i < len(text); {
		if text[i] == '%' {
			if j := strings.IndexByte(text[i+1:], '%'); j >= 0 {
				b.WriteString(m.vars[text[i+1:i+1+j]])
				i += j + 2
				continue
			}
		}
		b.WriteByte(text[i])
		i++
	}
	return b.String() + "\n"
}

func (m *mockRT) Cond(expr string) bool {
	expr = strings.TrimSpace(expr)
	switch expr {
	case "true":
		return true
	case "false":
		return false
	}
	// 依次匹配比较运算符，>= / <= 需排在 > / < 之前避免误匹配
	for _, op := range []string{">=", "<=", "==", ">", "<"} {
		idx := strings.Index(expr, op)
		if idx == -1 {
			continue
		}
		lhs := strings.TrimSpace(expr[:idx])
		rhs := strings.TrimSpace(expr[idx+len(op):])
		// 数值比较（循环计数场景）
		if lv, lerr := strconv.Atoi(m.vars[lhs]); lerr == nil {
			if rv, rerr := strconv.Atoi(rhs); rerr == nil {
				switch op {
				case ">=":
					return lv >= rv
				case "<=":
					return lv <= rv
				case "==":
					return lv == rv
				case ">":
					return lv > rv
				case "<":
					return lv < rv
				}
			}
		}
		// 回退字符串比较
		if op == "==" {
			return m.vars[lhs] == rhs
		}
		return false
	}
	return false
}

func (m *mockRT) SetVarInt(n string, v int) { m.vars[n] = strconv.Itoa(v) }
func (m *mockRT) GetVar(n string) string    { return m.vars[n] }

// Resolve 模拟表达式求值：%var% 插值，其余原样返回。
func (m *mockRT) Resolve(expr string) string {
	var b strings.Builder
	for i := 0; i < len(expr); {
		if expr[i] == '%' {
			if j := strings.IndexByte(expr[i+1:], '%'); j >= 0 {
				b.WriteString(m.vars[expr[i+1:i+1+j]])
				i += j + 2
				continue
			}
		}
		b.WriteByte(expr[i])
		i++
	}
	return b.String()
}

func (m *mockRT) LoopCount(expr string) int {
	if n, err := strconv.Atoi(m.vars[expr]); err == nil {
		return n
	}
	return 1
}

func (m *mockRT) LoopRange(expr string) (int, int) {
	startExpr, endExpr, ok := strings.Cut(expr, "~")
	if !ok {
		return 1, 1
	}
	atoi := func(s string) int {
		if n, err := strconv.Atoi(m.vars[s]); err == nil {
			return n
		}
		return 1
	}
	return atoi(startExpr), atoi(endExpr)
}

// SetLine 模拟设置当前行号（写入 行数 变量，供 %行数% 读取）。
func (m *mockRT) SetLine(line int) { m.vars["行数"] = strconv.Itoa(line) }

// JumpAbsOffset 模拟 $跳行 行号表达式求值：先 %var% 插值，再字面量整数、变量表查找，最后简单 [x+1] 算术。
func (m *mockRT) JumpAbsOffset(expr string) (int, bool) {
	expr = strings.TrimSpace(m.Resolve(expr))
	if n, err := strconv.Atoi(expr); err == nil {
		return n, true
	}
	if v, ok := m.vars[expr]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n, true
		}
	}
	// 简单 [行数+1] 算术（仅测试用，覆盖 [%行数%+1] 插值后的形态 [N+1]）。
	if strings.HasPrefix(expr, "[") && strings.HasSuffix(expr, "]") {
		inner := strings.TrimSpace(expr[1 : len(expr)-1])
		if lhs, rhs, ok := strings.Cut(inner, "+"); ok {
			lv, err1 := strconv.Atoi(strings.TrimSpace(lhs))
			rv, err2 := strconv.Atoi(strings.TrimSpace(rhs))
			if err1 == nil && err2 == nil {
				return lv + rv, true
			}
		}
	}
	return 0, false
}

func (m *mockRT) Stop() bool { return false }
func (m *mockRT) Halt()      {}

func (m *mockRT) Append(s string) { m.out.WriteString(s) }
func (m *mockRT) Output() string  { return m.out.String() }

// LoopVarChanged 模拟：循环变量不被改写，正常步进。
func (m *mockRT) LoopVarChanged(name string, cur int) (int, bool, bool) {
	return cur, false, false
}

// TextBlock 模拟文本框：内容行原样拼接，纯文本/文本差异忽略（仅验证字节码控制流）。
func (m *mockRT) TextBlock(text string, lines []string, pure bool) string {
	return "TEXT:" + strings.Join(lines, "|")
}

// JsonBlock 模拟 JSON> 框（仅验证字节码控制流，不做真实 JSON 语义）。
func (m *mockRT) JsonBlock(text string, lines []string) string {
	return "JSON:" + strings.Join(lines, "|")
}

// NewJsonBlock 模拟 JSON>{/[ 框（仅验证字节码控制流，不做真实 JSON 语义）。
func (m *mockRT) NewJsonBlock(text string, lines []string) string {
	return "NEWJSON:" + strings.Join(lines, "|")
}

// FuncBlock 模拟 变量:函数> 框（仅验证字节码控制流，不做真实函数框语义）。
func (m *mockRT) FuncBlock(text string, lines []string, lineNums []int) string {
	return "FUNC:" + strings.Join(lines, "|")
}

// ExecFuncBlock 模拟 变量:执行函数> 框（仅验证字节码控制流，不做真实函数框语义）。
func (m *mockRT) ExecFuncBlock(text string, lines []string, lineNums []int) string {
	return "EXECFUNC:" + strings.Join(lines, "|")
}

// ForEachInit 模拟 遍历> 框入口：解析 `遍历>k,v=[...]`，物化数组项，按帧深度隔离。
func (m *mockRT) ForEachInit(depth int, text string) int {
	rest := text[len("遍历>"):]
	valueName := rest
	valueExpr := ""
	if idx := strings.IndexByte(rest, '='); idx >= 0 {
		valueName = rest[:idx]
		valueExpr = rest[idx+1:]
	}
	v1, v2 := "_", "_"
	if idx := strings.IndexByte(valueName, ','); idx >= 0 {
		v1, v2 = valueName[:idx], valueName[idx+1:]
	} else {
		v1 = valueName
	}
	items := parseMockArray(valueExpr)
	if len(m.feFrames) <= depth {
		m.feFrames = append(m.feFrames, make([]mockForEachFrame, depth+1-len(m.feFrames))...)
	}
	m.feFrames[depth] = mockForEachFrame{v1: v1, v2: v2, items: items}
	return len(items)
}

// ForEachNext 模拟遍历项设置：把第 idx 项的键/值写入变量表。
func (m *mockRT) ForEachNext(depth int, idx int) bool {
	if depth < 0 || depth >= len(m.feFrames) {
		return false
	}
	f := &m.feFrames[depth]
	if idx < 0 || idx >= len(f.items) {
		return false
	}
	m.vars[f.v1] = f.items[idx].key
	m.vars[f.v2] = f.items[idx].value
	return true
}

// parseMockArray 解析简单 JSON 数组 `[a,b,c]`（项可带双引号），返回按序下标为键的遍历项。
func parseMockArray(s string) []mockForEachItem {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '[' || s[len(s)-1] != ']' {
		return nil
	}
	inner := strings.TrimSpace(s[1 : len(s)-1])
	if inner == "" {
		return nil
	}
	parts := strings.Split(inner, ",")
	items := make([]mockForEachItem, 0, len(parts))
	for i, p := range parts {
		v := strings.Trim(strings.TrimSpace(p), `"`)
		items = append(items, mockForEachItem{key: strconv.Itoa(i), value: v})
	}
	return items
}

// VarNewJsonBlock 模拟 变量:{/[ 框（仅验证字节码控制流，不做真实 JSON 语义）。
func (m *mockRT) VarNewJsonBlock(text string, lines []string) string {
	return "VARNEWJSON:" + strings.Join(lines, "|")
}

// ValChainBlock 模拟 变量:>>> 框（仅验证字节码控制流，不做真实连续执行语义）。
func (m *mockRT) ValChainBlock(text string, lines []string) string {
	return "VALCHAIN:" + strings.Join(lines, "|")
}

// ValTextBlock 模拟 变量:"""/”' 框（仅验证字节码控制流，不做真实赋值文本语义）。
func (m *mockRT) ValTextBlock(text string, lines []string, raw bool) string {
	return "VALTEXT:" + strings.Join(lines, "|")
}

// NodeJsBlock 模拟 --js 框（仅验证字节码控制流，不做真实 JS 语义）。
func (m *mockRT) NodeJsBlock(lines []string, line int) string {
	return "NODEJS:" + strings.Join(lines, "|")
}

// Assign 模拟无副作用赋值：将右值写入变量表并返回空。
func (m *mockRT) Assign(line int, text string, vType int8, prefix, suffix string) string {
	// 模拟算术赋值：n-:v 自减、n+:v 自增（循环计数场景），其余按字面量直接赋值
	switch vType {
	case 1: // n-:v
		if ov, err := strconv.Atoi(m.vars[prefix]); err == nil {
			if sv, err2 := strconv.Atoi(suffix); err2 == nil {
				m.vars[prefix] = strconv.Itoa(ov - sv)
				return ""
			}
		}
	case 2: // n+:v
		if ov, err := strconv.Atoi(m.vars[prefix]); err == nil {
			if sv, err2 := strconv.Atoi(suffix); err2 == nil {
				m.vars[prefix] = strconv.Itoa(ov + sv)
				return ""
			}
		}
	}
	m.vars[prefix] = suffix
	return ""
}

// runBody 便捷函数：解析 → 编译 → 执行。
func runBody(t *testing.T, body []string) string {
	t.Helper()
	nodes := ast.ParseBody(body, nil)
	return Run(Compile(nodes), newMockRT())
}

func TestLeafLines(t *testing.T) {
	out := runBody(t, []string{"a", "b", "c"})
	if out != "a\nb\nc\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "a\nb\nc\n")
	}
}

func TestIfTrue(t *testing.T) {
	out := runBody(t, []string{"如果>true", "A", ">否则", "B", "<如果"})
	if out != "A\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "A\n")
	}
}

func TestIfFalse(t *testing.T) {
	out := runBody(t, []string{"如果>false", "A", ">否则", "B", "<如果"})
	if out != "B\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "B\n")
	}
}

func TestIfElseIfElse(t *testing.T) {
	rt := newMockRT()
	rt.vars["x"] = "2"
	nodes := ast.ParseBody([]string{"如果>x==1", "A", ">否则如果:x==2", "B", ">否则", "C", "<如果"}, nil)
	out := Run(Compile(nodes), rt)
	if out != "B\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "B\n")
	}
}

func TestSwitchMatch(t *testing.T) {
	rt := newMockRT()
	rt.vars["x"] = "2"
	nodes := ast.ParseBody([]string{"匹配>%x%", "如果是:1", "一", "如果是:2", "二", "如果是:3", "三", "如果不是", "其他", "<匹配"}, nil)
	out := Run(Compile(nodes), rt)
	if out != "二\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "二\n")
	}
}

func TestSwitchDefault(t *testing.T) {
	rt := newMockRT()
	rt.vars["x"] = "9"
	nodes := ast.ParseBody([]string{"匹配>%x%", "如果是:1", "一", "如果是:2", "二", "如果不是", "其他", "<匹配"}, nil)
	out := Run(Compile(nodes), rt)
	if out != "其他\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "其他\n")
	}
}

func TestSwitchNoFallthrough(t *testing.T) {
	// 命中第一个 case 后跳出，不再匹配后续同值 case（不穿透）
	rt := newMockRT()
	rt.vars["x"] = "1"
	nodes := ast.ParseBody([]string{"匹配>%x%", "如果是:1", "一", "如果是:1", "再一", "如果不是", "其他", "<匹配"}, nil)
	out := Run(Compile(nodes), rt)
	if out != "一\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "一\n")
	}
}

func TestSwitchSkip(t *testing.T) {
	// >跳过 跳出匹配框，后续语句（二/否则）不执行，框后继续
	rt := newMockRT()
	rt.vars["x"] = "1"
	nodes := ast.ParseBody([]string{"匹配>%x%", "如果是:1", "一", ">跳过", "二", "如果不是", "其他", "<匹配", "尾"}, nil)
	out := Run(Compile(nodes), rt)
	if out != "一\n尾\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "一\n尾\n")
	}
}

func TestSwitchNested(t *testing.T) {
	rt := newMockRT()
	rt.vars["x"] = "1"
	rt.vars["y"] = "2"
	nodes := ast.ParseBody([]string{"匹配>%x%", "如果是:1", "匹配>%y%", "如果是:2", "内二", "<匹配", "如果不是", "其他", "<匹配"}, nil)
	out := Run(Compile(nodes), rt)
	if out != "内二\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "内二\n")
	}
}

func TestForLoop(t *testing.T) {
	out := runBody(t, []string{"循环>i=3", "%i%", "<循环"})
	if out != "1\n2\n3\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "1\n2\n3\n")
	}
}

func TestForLoopDyn(t *testing.T) {
	rt := newMockRT()
	rt.vars["%n%"] = "3"
	nodes := ast.ParseBody([]string{"循环>i=%n%", "%i%", "<循环"}, nil)
	out := Run(Compile(nodes), rt)
	if out != "1\n2\n3\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "1\n2\n3\n")
	}
}

func TestForLoopBreak(t *testing.T) {
	out := runBody(t, []string{"循环>i=100", "%i%", ">终止循环", "<循环"})
	if out != "1\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "1\n")
	}
}

func TestForLoopInterrupt(t *testing.T) {
	// >中断 等价于 >终止循环：跳出当前循环
	out := runBody(t, []string{"循环>i=100", "%i%", ">中断", "<循环"})
	if out != "1\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "1\n")
	}
}

func TestForLoopContinue(t *testing.T) {
	// >跳过 跳过本轮剩余语句（X 不输出），进入下一轮
	out := runBody(t, []string{"循环>i=3", "%i%", ">跳过", "X", "<循环"})
	if out != "1\n2\n3\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "1\n2\n3\n")
	}
}

func TestWhileLoopBreak(t *testing.T) {
	// 判断循环>true 恒真，靠 >终止循环 跳出
	out := runBody(t, []string{"判断循环>true", "A", ">终止循环", "B", "<循环", "尾"})
	if out != "A\n尾\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "A\n尾\n")
	}
}

func TestWhileLoopFalse(t *testing.T) {
	// 条件首次为假：不进入循环体
	out := runBody(t, []string{"判断循环>false", "A", "<循环", "尾"})
	if out != "尾\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "尾\n")
	}
}

func TestWhileLoopContinue(t *testing.T) {
	// >跳过 跳过本轮剩余语句（X 不输出），重新判断条件后因条件变化正常退出
	out := runBody(t, []string{"i::0", "判断循环>i==0", "A", "i::1", ">跳过", "X", "<循环", "尾"})
	if out != "A\n尾\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "A\n尾\n")
	}
}

func TestWhileLoopVarCount(t *testing.T) {
	// 变量计数终止：i 从 0 自增，条件 i<10 变假后正常退出，不触发死循环保护
	out := runBody(t, []string{"i::0", "判断循环>i<10", "i+:1", "<循环", "%i%"})
	if out != "10\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "10\n")
	}
}

func TestNestedLoop(t *testing.T) {
	out := runBody(t, []string{
		"循环>i=2",
		"循环>j=2",
		"%i%,%j%",
		"<循环",
		"<循环",
	})
	if out != "1,1\n1,2\n2,1\n2,2\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "1,1\n1,2\n2,1\n2,2\n")
	}
}

func TestLoopWithForEach(t *testing.T) {
	// 循环内嵌遍历：已原生下沉为跳转指令（不再整段委托旧解释器）。
	out := runBody(t, []string{"循环>i=2", "遍历>j,jj=[1,2]", "%i%,%j%,%jj%", "<遍历", "<循环"})
	if out != "1,0,1\n1,1,2\n2,0,1\n2,1,2\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "1,0,1\n1,1,2\n2,0,1\n2,1,2\n")
	}
}

func TestForEach(t *testing.T) {
	out := runBody(t, []string{"遍历>i,ii=[1,2]", "%i%,%ii%", "<遍历"})
	if out != "0,1\n1,2\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "0,1\n1,2\n")
	}
}

func TestForEachBreak(t *testing.T) {
	// >终止遍历 结束最内层遍历
	out := runBody(t, []string{"遍历>i,ii=[1,2,3]", "%ii%", ">终止遍历", "<遍历"})
	if out != "1\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "1\n")
	}
}

func TestForEachContinue(t *testing.T) {
	// >跳过 结束本轮遍历剩余语句
	out := runBody(t, []string{"遍历>i,ii=[1,2]", "%ii%", ">跳过", "X", "<遍历"})
	if out != "1\n2\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "1\n2\n")
	}
}

func TestForEachBreakForCross(t *testing.T) {
	// 遍历体内 >终止循环 跨过遍历边界跳出最内层循环
	out := runBody(t, []string{
		"循环>i=2",
		"遍历>j,jj=[1,2]",
		"%i%,%jj%",
		">终止循环",
		"<遍历",
		"<循环",
	})
	if out != "1,1\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "1,1\n")
	}
}

func TestFuncBlock(t *testing.T) {
	out := runBody(t, []string{"foo:函数>", "a", "b", "<函数"})
	if out != "FUNC:a|b" {
		t.Fatalf("输出 = %q, 期望 %q", out, "FUNC:a|b")
	}
}

func TestExecFuncBlock(t *testing.T) {
	out := runBody(t, []string{"foo:执行函数>", "a", "b", "<函数"})
	if out != "EXECFUNC:a|b" {
		t.Fatalf("输出 = %q, 期望 %q", out, "EXECFUNC:a|b")
	}
}

func TestExecFuncBlockAsyncHash(t *testing.T) {
	out := runBody(t, []string{"#:执行函数>", "a", "b", "<函数"})
	if out != "EXECFUNC:a|b" {
		t.Fatalf("输出 = %q, 期望 %q", out, "EXECFUNC:a|b")
	}
}

func TestTextBlock(t *testing.T) {
	out := runBody(t, []string{"文本>", "a", "b", "<文本"})
	if out != "TEXT:a|b" {
		t.Fatalf("输出 = %q, 期望 %q", out, "TEXT:a|b")
	}
}

func TestJsonBlock(t *testing.T) {
	out := runBody(t, []string{"JSON>{}", "a=1", "<JSON"})
	if out != "JSON:a=1" {
		t.Fatalf("输出 = %q, 期望 %q", out, "JSON:a=1")
	}
}

func TestNewJsonBlock(t *testing.T) {
	out := runBody(t, []string{"JSON>{", `"a":1`, "}"})
	if out != "NEWJSON:\"a\":1|}" {
		t.Fatalf("输出 = %q, 期望 %q", out, "NEWJSON:\"a\":1|}")
	}
}

func TestVarNewJsonBlock(t *testing.T) {
	out := runBody(t, []string{"a:{", `"x":1`, "}"})
	if out != `VARNEWJSON:"x":1|}` {
		t.Fatalf("输出 = %q, 期望 %q", out, `VARNEWJSON:"x":1|}`)
	}
}

func TestValChainBlock(t *testing.T) {
	out := runBody(t, []string{"a:>>>", "x", "y", "<<<"})
	if out != "VALCHAIN:x|y" {
		t.Fatalf("输出 = %q, 期望 %q", out, "VALCHAIN:x|y")
	}
}

func TestValTextBlock(t *testing.T) {
	out := runBody(t, []string{"a:\"\"\"", "x", "y", "\"\"\""})
	if out != "VALTEXT:x|y" {
		t.Fatalf("输出 = %q, 期望 %q", out, "VALTEXT:x|y")
	}
}

func TestValTextBlockRaw(t *testing.T) {
	out := runBody(t, []string{"a:'''", "x", "y", "'''"})
	if out != "VALTEXT:x|y" {
		t.Fatalf("输出 = %q, 期望 %q", out, "VALTEXT:x|y")
	}
}

func TestNodeJsBlock(t *testing.T) {
	out := runBody(t, []string{"--js", "1+1", "--end"})
	if out != "NODEJS:1+1" {
		t.Fatalf("输出 = %q, 期望 %q", out, "NODEJS:1+1")
	}
}

func TestHalt(t *testing.T) {
	out := runBody(t, []string{"A", ">终止", "B"})
	if out != "A\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "A\n")
	}
}

func TestJumpAbsLiteral(t *testing.T) {
	// $跳行 3$ 绝对跳转到第 3 行（跳过第 2 行）
	out := runBody(t, []string{"$跳行 3$", "跳过", "后"})
	if out != "后\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "后\n")
	}
}

func TestJumpAbsArith(t *testing.T) {
	// $跳行 [1+2]$ 行号算术求值后跳转到第 3 行
	out := runBody(t, []string{"$跳行 [1+2]$", "跳过", "后"})
	if out != "后\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "后\n")
	}
}

func TestJumpAbsLineVar(t *testing.T) {
	// $跳行 [%行数%+2]$ 从当前行(1)跳转到第 3 行
	out := runBody(t, []string{"$跳行 [%行数%+2]$", "跳过", "后"})
	if out != "后\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "后\n")
	}
}

func TestJumpAbsMiss(t *testing.T) {
	// 目标行号不存在（9 超出范围）：不跳转，顺序执行
	out := runBody(t, []string{"$跳行 9$", "A", "B"})
	if out != "A\nB\n" {
		t.Fatalf("输出 = %q, 期望 %q", out, "A\nB\n")
	}
}

func TestAssignableLeaf(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"n::hello", true},  // vType 5 纯文本赋值
		{"x:%:%y%", true},   // vType 4 执行变量
		{"x:%:hello", true}, // vType 4 无 % 字面量
		{"n+:%x%", true},    // 自增 纯变量
		{"n+:5", true},      // 自增 字面量
		{"n*:3", true},      // 乘法 字面量
		{"n/:2", true},      // 除法 字面量
		{"n-:1", true},      // 自减 字面量
		{"n+:[1+2]", true},  // 自增 纯字面量计算（不含 %/$）
		{"n+:$函数$", false},  // 自增 函数 有副作用
		{"n+:%x%5", false},  // 含 % 非纯变量
		{"n+:", false},      // 空操作数
		{"你好", false},       // 普通输出
		{"n:5", false},      // vType 6 普通赋值
	}
	for _, c := range cases {
		_, _, _, ok := assignableLeaf(c.text)
		if ok != c.want {
			t.Errorf("assignableLeaf(%q) = %v, 期望 %v", c.text, ok, c.want)
		}
	}
}
