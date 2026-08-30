package dic

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	dicBuild "github.com/cjxpj/nebula/build"
	"github.com/cjxpj/nebula/count"
	"github.com/cjxpj/nebula/dic/ast"
	"github.com/cjxpj/nebula/dic/bc"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/buffer"
	"github.com/dop251/goja_nodejs/console"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/process"
	"github.com/dop251/goja_nodejs/require"
	"github.com/dop251/goja_nodejs/url"
)

// dicRuntime 把字节码 VM 的 Runtime 接口接到现有 dic 运行时：
//   - Line 原生执行单行叶子语句（含 $函数$/%变量%/赋值等全部语义）；
//   - Cond 复用 Pd 条件求值；
//   - SetVar/GetVar/Stop 直接映射到共享变量表与系统状态。
type dicRuntime struct {
	m     *dicImpl
	r     *dic_dto.DicEntry
	funcV *dic_dto.DicFunc
	// sub 复用的叶子执行子条目：避免循环体每轮为单行重新分配 DicEntry 与 SingleValue。
	sub *dic_dto.DicEntry
	// slots 循环变量名 → 槽号缓存：循环变量名固定，槽号只解析一次，后续轮次免重复驻留。
	slots map[string]int32
	// forEach 遍历框迭代状态：按 VM 帧深度（depth）隔离，支持嵌套遍历/循环的穿越。
	// 帧深度同时包含循环帧与遍历帧，非遍历帧对应槽位为空（v1/v2 未赋值）。
	feFrames []forEachFrame
}

// forEachFrame 一个遍历帧的迭代状态：键值变量名与物化后的迭代项。
type forEachFrame struct {
	v1    string
	v2    string
	items []forEachItem
}

// forEachItem 遍历框的一项迭代：键（对象为字符串键，数组为 int 下标）与值（字符串）。
type forEachItem struct {
	key   any
	value string
}

// leafSub 返回可复用的叶子子条目（懒初始化），Output 每次执行前由 runLeaf 清空。
func (a *dicRuntime) leafSub() *dic_dto.DicEntry {
	if a.sub == nil {
		a.sub = &dic_dto.DicEntry{
			Output:         &dto.SingleValue{},
			Val:            a.r.Val,
			Sys_v:          a.r.Sys_v,
			Trigger:        false,
			Dic:            a.r.Dic,
		}
	}
	return a.sub
}

// slotFor 返回循环变量名对应的槽号（缓存解析结果）。-1 表示非普通变量名，回退 map 路径。
func (a *dicRuntime) slotFor(name string) int32 {
	if a.slots != nil {
		if s, ok := a.slots[name]; ok {
			return s
		}
	}
	s := dto.SlotForName(name)
	if a.slots == nil {
		a.slots = make(map[string]int32)
	}
	a.slots[name] = s
	return s
}

func (a *dicRuntime) Line(line int, text string) string {
	return a.runLeaf(line, text)
}

// runLeaf 原生执行一条叶子语句（字节码路径专用），返回该叶子的完整输出（含 $函数$ 产生的输出）。
// 语义与旧解释器对单行（StateNormal、非块开启行）的执行一致，但不再经过编译/状态机框架。
// 此处只需处理单行叶子：简单赋值/算术、普通赋值（含单行 >>> 链式）、网络请求、类成员、异步 #: 与纯文本输出。
// 规范的 >跳行(条件)>>偏移 已下沉为 OpJumpRel，畸形 >跳行( 已由编译器静默消费（OpNop），均不会作为叶子到达此处。
// line 为该语句的真实源文件行号（1-based，0 表示未知），用于报错定位。
func (a *dicRuntime) runLeaf(line int, text string) string {
	r := a.leafSub()
	funcV := a.funcV
	r.Output.Clear()
	r.Val.P.Class = r.Dic.ClassValues()

	funcV.CurLine = line

	if r.Sys_v.Stop.Load() {
		return r.Output.Get()
	}

	vt, vp, vs := dicBuild.ValTextTest(text)

	// 快速路径：普通状态下的简单赋值/算术（vType 1~5、7、8）。
	if vt != 0 && vt != 6 {
		prefixSlot := int32(-1)
		if vp != "" {
			prefixSlot = a.slotFor(vp)
		}
		suffixSlot := int32(-1)
		if vt == 1 || vt == 2 || vt == 7 || vt == 8 {
			if len(vs) > 2 && vs[0] == '%' && vs[len(vs)-1] == '%' {
				if name := vs[1 : len(vs)-1]; !strings.Contains(name, "%") {
					suffixSlot = a.slotFor(name)
				}
			}
		}
		if runSimpleAssign(r, funcV, vt, vp, vs, prefixSlot, suffixSlot, true) {
			return r.Output.Get()
		}
	}

	textLen := len(text)

	// 异步执行 #:
	if textLen > 2 && text[:2] == "#:" {
		val := funcV.Val.NewDicVal(funcV.Val.P.Clone())
		dic := funcV.Dic
		go func() {
			independentFuncV := &dic_dto.DicFunc{
				Val: val,
				Sys: &dto.LocalDicValue{},
				Dic: dic,
			}
			Runs(independentFuncV, utils.AnyToString(count.RunCountText(val, text[2:])))
		}()
		return r.Output.Get()
	}

	// 网络请求文本
	if strings.HasPrefix(text, "https://") || strings.HasPrefix(text, "http://") {
		res := utils.AnyToString(Runs(funcV, text))
		if r.Sys_v.Stop.Load() {
			return r.Output.Get()
		}
		r.Output.Add(res)
		return r.Output.Get()
	}

	// 类成员变量赋值：.成员名:值
	if textLen > 2 && text[0] == '.' {
		if idx := strings.IndexByte(text, ':'); idx > 1 {
			if classData, ok := r.Val.P.Get("Class").(*dto.DicClass); ok && classData != nil {
				classData.LocalValue.Set(text[1:idx], RunsAny(funcV, text[idx+1:]))
				return r.Output.Get()
			}
		}
	}

	// vType 6 普通赋值
	if vt == 6 {
		if vp == "" {
			res := utils.AnyToString(Runs(funcV, text))
			if r.Sys_v.Stop.Load() {
				return r.Output.Get()
			}
			r.Output.Add(res)
			return r.Output.Get()
		}
		if vs == "" {
			r.Val.P.Set(vp, "")
			return r.Output.Get()
		}
		// 注：vSuffix == ">>>"（变量:>>> 块开启行）/ "{" / "[" / `"""` / `'''` 已被 AST 解析为块，
		// 不会作为单行叶子到达此处；单行链式 >>> 在下方原生下沉，语义与解释器 case 6 一致。
		// >>> 连续执行：值为非 JSON 且含 >>> 时按 SplitValChain 分段，逐段写回 vp（复用当前值）。
		if strings.Contains(vs, ">>>") && utils.IsJSONResult(vs) == nil {
			for _, chainPart := range SplitValChain(vs) {
				if chainPart == "" {
					continue
				}
				runValSet(r, funcV, vp, chainPart)
			}
			return r.Output.Get()
		}
		if setJsonHead := strings.Split(vp, "->"); len(setJsonHead) > 1 {
			vp = setJsonHead[0]
			setJsonHead = setJsonHead[1:]
			if str, ok := r.Val.P.Get(vp).(string); ok {
				if j := utils.IsJSONResult(str); j != nil {
					if j, ok := j.(map[string]any); ok {
						vSetData := utils.AnyToString(Runs(funcV, utils.AnyToString(count.RunCountText(r.Val, vs))))
						j := funcs.JsonSetValue(j, setJsonHead, vSetData, false)
						if j, err := json.Marshal(j); err == nil {
							r.Val.P.Set(vp, string(j))
							return r.Output.Get()
						}
						r.Val.P.Set(vp, vSetData)
					}
					if j, ok := j.([]any); ok {
						vSetData := utils.AnyToString(Runs(funcV, utils.AnyToString(count.RunCountText(r.Val, vs))))
						j := funcs.JsonSetValue(j, setJsonHead, vSetData, false)
						if j, err := json.Marshal(j); err == nil {
							r.Val.P.Set(vp, string(j))
							return r.Output.Get()
						}
					}
				}
			}
			return r.Output.Get()
		}
		// [计算]
		if strings.HasPrefix(vs, "[") && strings.HasSuffix(vs, "]") {
			r.Val.P.Set(vp, count.RunCountText(r.Val, Runs(funcV, vs)))
			return r.Output.Get()
		}
		runValSet(r, funcV, vp, vs)
		return r.Output.Get()
	}

	// 默认纯文本输出（含 \: 转义冒号、\r 换行、\\r 字面 \r 处理）
	text = unescapeText(text)
	res := utils.AnyToString(Runs(funcV, text))
	if r.Sys_v.Stop.Load() {
		return r.Output.Get()
	}
	r.Output.Add(res)
	return r.Output.Get()
}

// Assign 执行一条无副作用的赋值/算术叶子（编译期已识别为结构化信息），
// 直接调用 runSimpleAssign。赋值语句无输出，返回空。
// text 为原始行文本，仅当 runSimpleAssign 意外未处理时防御性回退到 Line。
func (a *dicRuntime) Assign(line int, text string, vType int8, prefix, suffix string) string {
	suffixSlot := int32(-1)
	if vType == 1 || vType == 2 || vType == 7 || vType == 8 {
		// 仅当 suffix 为纯 %var% 时提取变量名求槽号；纯字面量操作数走 runSimpleAssign 的通用路径。
		if len(suffix) > 2 && suffix[0] == '%' && suffix[len(suffix)-1] == '%' {
			suffixSlot = a.slotFor(suffix[1 : len(suffix)-1])
		}
	}
	if runSimpleAssign(a.r, a.funcV, vType, prefix, suffix, a.slotFor(prefix), suffixSlot, true) {
		return ""
	}
	return a.Line(line, text)
}

func (a *dicRuntime) Cond(expr string) bool {
	return Pd(a.funcV, expr)
}

// Resolve 求值表达式为字符串（%变量%/$函数$/[算术] 等），供 匹配> 框一次性求值主体与 case 值。
// 与 LoopCount/loopBound 的表达式求值路径一致。
func (a *dicRuntime) Resolve(expr string) string {
	return utils.AnyToString(Runs(a.funcV, utils.AnyToString(count.RunCountText(a.r.Val, expr))))
}

// JumpRelOffset 求值 跳行 偏移表达式，语义与解释器 >跳行 一致：
// 偏移经 r.Val.Text（%变量% 插值）后按整数解析，解析失败返回 ok=false（未命中，不跳转）。
func (a *dicRuntime) JumpRelOffset(expr string) (int, bool) {
	runText := utils.AnyIsString(a.r.Val.Text(expr))
	n, err := strconv.Atoi(runText)
	if err != nil {
		return 0, false
	}
	return n, true
}

func (a *dicRuntime) SetVarInt(name string, value int) {
	// 与解释器一致：循环变量以整数写入，保证后续算术/比较语义一致。
	// 普通变量名直写无锁槽（num 映射由块结束 FlushSlotsToMap 统一同步），非普通回退 map。
	if slot := a.slotFor(name); slot >= 0 {
		a.r.Val.P.SlotSetInt64(slot, int64(value))
	} else {
		a.r.Val.P.SetInt64(name, int64(value))
	}
}

func (a *dicRuntime) GetVar(name string) string {
	return utils.AnyToString(a.r.Val.P.Get(name))
}

// LoopCount 求值动态循环次数表达式，语义与解释器 opFor 一致：
// 表达式经 %变量% 插值/$函数$ 执行后按整数解析，解析失败回退为 1。
// 负数次数等价 0 次（解释器 for i:=1; i<=count 不执行），避免与字节码无限循环哨兵冲突。
func (a *dicRuntime) LoopCount(expr string) int {
	runText := utils.AnyToString(Runs(a.funcV, utils.AnyToString(count.RunCountText(a.r.Val, expr))))
	if n, err := strconv.Atoi(runText); err == nil {
		if n < 0 {
			return 0
		}
		return n
	}
	return 1
}

// LoopRange 求值范围循环的 "起始表达式~结束表达式"（各经 %变量% 插值/$函数$ 执行后按整数解析，
// 解析失败回退 1），返回起始值与结束值。与 LoopCount 不同，边界值保留负数语义。
func (a *dicRuntime) LoopRange(expr string) (int, int) {
	startExpr, endExpr, ok := strings.Cut(expr, "~")
	if !ok {
		return 1, 1
	}
	return a.loopBound(startExpr), a.loopBound(endExpr)
}

// loopBound 求值范围循环的单个边界表达式（保留负数，解析失败回退 1）。
func (a *dicRuntime) loopBound(expr string) int {
	runText := utils.AnyToString(Runs(a.funcV, utils.AnyToString(count.RunCountText(a.r.Val, expr))))
	if n, err := strconv.Atoi(runText); err == nil {
		return n
	}
	return 1
}

func (a *dicRuntime) Stop() bool {
	return a.r.Sys_v.Stop.Load()
}

func (a *dicRuntime) Halt() {
	a.r.Sys_v.Stop.Store(true)
}

// Append 将文本追加到共享输出缓冲（即词条输出 r.Output）。VM 不再自行持有输出，
// 函数报错等场景通过 handleFuncError 清空 r.Output 时会一并清空已累积的输出。
func (a *dicRuntime) Append(s string) {
	a.r.Output.Add(s)
}

// Output 返回共享输出缓冲当前内容。
func (a *dicRuntime) Output() string {
	return a.r.Output.Get()
}

// LoopVarChanged 槽优先的循环变量步进检查：普通变量名无锁读槽，未命中整数或为字符串时
// 复用解释器 loopVarChangedFromValue；非普通变量名回退 map 版 loopVarChanged。
func (a *dicRuntime) LoopVarChanged(name string, cur int) (int, bool, bool) {
	if slot := a.slotFor(name); slot >= 0 {
		if n, ok := a.r.Val.P.SlotGetInt64(slot); ok {
			if int(n) != cur {
				return int(n), true, false
			}
			return cur, false, false
		}
		if val, ok := a.r.Val.P.SlotGet(slot); ok {
			return loopVarChangedFromValue(val, cur)
		}
	}
	return loopVarChanged(a.r.Val.P, name, cur)
}

// TextBlock 原生执行 文本>/纯文本> 框，语义与解释器 StateText 状态机一致：
//   - 开启行后缀（`=` 右侧或整段后缀）经 %变量% 插值得到换行分隔符 LineFeed；
//   - `=` 左侧为赋值目标变量名（无 `=` 则直接输出）；
//   - 内容行按 pure 决定原样输出或 %变量% 插值，行间用 LineFeed 连接，末尾不加分隔符。
func (a *dicRuntime) TextBlock(text string, lines []string, pure bool) string {
	r := a.r
	prefixLen := len("文本>")
	if pure {
		prefixLen = len("纯文本>")
	}
	getInput := text[prefixLen:]

	valueName := ""
	var lineFeed string
	if startIdx := strings.IndexByte(getInput, '='); startIdx != -1 {
		valueName = getInput[:startIdx]
		lineFeed = utils.AnyIsString(r.Val.Text(getInput[startIdx+1:]))
	} else {
		lineFeed = utils.AnyIsString(r.Val.Text(getInput))
	}

	var b strings.Builder
	for i, line := range lines {
		if pure {
			b.WriteString(line)
		} else {
			b.WriteString(utils.AnyIsString(r.Val.Text(line)))
		}
		if i < len(lines)-1 {
			b.WriteString(lineFeed)
		}
	}

	content := b.String()
	if valueName != "" {
		r.Val.P.Set(valueName, content)
		return ""
	}
	return content
}

// JsonBlock 原生执行 JSON> 框，语义与解释器 StateSetJson 状态机一致：
//   - 开启行后缀（`=` 右侧或整段后缀）经 %变量% 插值后作为初始 JSON 反序列化；
//   - `=` 左侧为赋值目标变量名（无 `=` 则直接输出）；
//   - 内容行按 `key=value`（字符串）或 `key:=value`（反序列化）设置键值，
//     支持 `->` 嵌套路径与 `[]` 数组追加，键值均做 %变量% 插值；
//   - 结束返回 JSON 序列化结果。
//
// 初始 JSON 反序列化失败时（退化场景），整段委托旧解释器保持语义一致。
func (a *dicRuntime) JsonBlock(text string, lines []string) string {
	r := a.r
	getInput := text[len("JSON>"):]

	valueName := ""
	jsonText := ""
	if idx := strings.IndexByte(getInput, '='); idx != -1 {
		valueName = getInput[:idx]
		jsonText = utils.AnyIsString(r.Val.Text(getInput[idx+1:]))
	} else {
		jsonText = utils.AnyIsString(r.Val.Text(getInput))
	}

	var data any
	if err := json.Unmarshal([]byte(jsonText), &data); err != nil {
		// 退化：初始 JSON 反序列化失败时，解释器 opJson 不会进入 StateSetJson（开启行无副作用），
		// 内容行与 <JSON 关闭行均按普通语句执行。此处跳过开启行，仅把内容行 + <JSON 作为普通正文走字节码。
		body := make([]string, 0, len(lines)+1)
		body = append(body, lines...)
		body = append(body, "<JSON")
		return a.m.dicRunLineBytecode(a.leafSub(), body)
	}

	okLen := false
	ln := 0
	for _, line := range lines {
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		str := true
		key := line[:idx]
		if idx > 0 && line[idx-1] == ':' {
			key = line[:idx-1]
			str = false
		}
		keys := strings.Split(key, "->")
		if keys[0] == "[]" {
			if !okLen {
				if m, ok := data.(map[string]any); ok {
					ln = len(m)
				}
				if s, ok := data.([]any); ok {
					ln = len(s)
				}
				okLen = true
			} else {
				ln++
			}
			keys[0] = strconv.Itoa(ln)
		}
		value := utils.AnyIsString(r.Val.Text(line[idx+1:]))
		for k, setv := range keys {
			keys[k] = utils.AnyIsString(r.Val.Text(setv))
		}
		data = funcs.JsonSetValue(data, keys, value, str)
	}

	resS, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	jsonString := string(resS)
	if valueName != "" {
		r.Val.P.Set(valueName, jsonString)
		return ""
	}
	return jsonString
}

// NewJsonBlock 原生执行 JSON>{ / JSON>[ 框，语义与解释器 StateSetNewJson 状态机一致：
// 内容行原样累积为 JSON 文本，按括号平衡判断闭合，闭合后经 NewJson 做 %变量% 递归替换并输出。
func (a *dicRuntime) NewJsonBlock(text string, lines []string) string {
	r := a.r
	prefix := "{"
	if text == "JSON>[" {
		prefix = "["
	}

	var b strings.Builder
	b.WriteString(prefix)
	depth := 1
	for _, line := range lines {
		b.WriteString(line)
		if strings.HasSuffix(line, "{") || strings.HasSuffix(line, "[") {
			depth++
		}
		if line == "}" || line == "]" || line == "}," || line == "]," {
			depth--
			if depth == 0 && (line == "}" || line == "]") {
				return NewJson(r, r.Val.P, b.String())
			}
		}
	}
	// 未闭合（编译检查已保证闭合，此处防御性兜底）：按累积文本输出。
	return NewJson(r, r.Val.P, b.String())
}

// VarNewJsonBlock 原生执行 变量:{ / 变量:[ 框，语义与解释器 StateSetNewJson 状态机一致：
// 开启行 `变量:{` 或 `变量:[` 解析出变量名与括号前缀，内容行原样累积为 JSON 文本，
// 按括号平衡判断闭合，闭合后经 NewJson 做 %变量% 递归替换并赋值到变量（无输出）。
func (a *dicRuntime) VarNewJsonBlock(text string, lines []string) string {
	r := a.r
	_, varName, suffix := dicBuild.ValTextTest(text)
	prefix := "{"
	if suffix == "[" {
		prefix = "["
	}

	var b strings.Builder
	b.WriteString(prefix)
	depth := 1
	for _, line := range lines {
		b.WriteString(line)
		if strings.HasSuffix(line, "{") || strings.HasSuffix(line, "[") {
			depth++
		}
		if line == "}" || line == "]" || line == "}," || line == "]," {
			depth--
			if depth == 0 && (line == "}" || line == "]") {
				r.Val.P.Set(varName, NewJson(r, r.Val.P, b.String()))
				return ""
			}
		}
	}
	// 未闭合（编译检查已保证闭合，此处防御性兜底）：按累积文本赋值。
	r.Val.P.Set(varName, NewJson(r, r.Val.P, b.String()))
	return ""
}

// ValChainBlock 原生执行 变量:>>> 框，语义与解释器 StateValChain 状态机一致：
// 开启行解析出变量名，内容行逐行执行 runValSet 写回变量（空行跳过），
// 支持 ?: 回退与 @json 路径，<<< 关闭行不参与执行。无输出。
func (a *dicRuntime) ValChainBlock(text string, lines []string) string {
	r := a.r
	_, varName, _ := dicBuild.ValTextTest(text)
	for _, line := range lines {
		if line == "" {
			continue
		}
		runValSet(r, a.funcV, varName, line)
	}
	return ""
}

// ValTextBlock 原生执行 变量:""" / 变量:”' 框，语义与解释器 StateValText / StateValTextr 一致：
//   - raw=true（”'）：内容行原样，仅替换 \”' → ”'；
//   - raw=false（"""）：内容行先做 %变量% 插值，再替换 \""" → """；
//
// 内容行以 \n 连接后赋值给变量（无输出）。
func (a *dicRuntime) ValTextBlock(text string, lines []string, raw bool) string {
	r := a.r
	_, varName, _ := dicBuild.ValTextTest(text)
	content := make([]string, 0, len(lines))
	if raw {
		for _, line := range lines {
			content = append(content, strings.ReplaceAll(line, `\'''`, `'''`))
		}
	} else {
		for _, line := range lines {
			content = append(content, strings.ReplaceAll(utils.AnyIsString(r.Val.Text(line)), `\"""`, `"""`))
		}
	}
	r.Val.P.Set(varName, strings.Join(content, "\n"))
	return ""
}

// NodeJsBlock 原生执行 --js ... --end 框，语义与解释器 StateNodeJs 状态机一致：
// 内容行以 \n 连接为 JS 脚本，在 goja 运行时内执行；返回最后一个表达式的字符串结果
// （undefined 返回空），报错时返回「JS错误(line:N)」文本。
func (a *dicRuntime) NodeJsBlock(lines []string, line int) string {
	r := a.r

	registry := new(require.Registry)
	loop := eventloop.NewEventLoop()
	vm := goja.New()
	registry.Enable(vm)
	console.Enable(vm)
	url.Enable(vm)
	buffer.Enable(vm)
	process.Enable(vm)

	// dic(文本) 在 JS 内执行一段词库正文，返回其输出文本。
	vm.Set("dic", func(call goja.FunctionCall) goja.Value {
		dicLine := call.Argument(0).String()
		dicLineArr := strings.Split(dicLine, "\n")
		RunDics := dic_dto.NewRunDicEntry().
			SetGlobal_v(r.Val.G).
			Set_v(r.Val.P)
		res := a.m.dicRunLineBytecode(RunDics, dicLineArr)
		return vm.ToValue(res)
	})

	// dic_value(k, v) 在 JS 内写回变量。
	vm.Set("dic_value", func(call goja.FunctionCall) goja.Value {
		k := call.Argument(0).String()
		v := call.Argument(1).String()
		r.Val.P.Set(k, v)
		return goja.Undefined()
	})

	vm.Set("setTimeout", func(call goja.FunctionCall) goja.Value {
		fn := call.Argument(0)
		delay := call.Argument(1).ToInteger()
		if fnFn, ok := goja.AssertFunction(fn); ok {
			loop.SetTimeout(func(vm *goja.Runtime) {
				fnFn(goja.Undefined())
			}, time.Duration(delay)*time.Millisecond)
		}
		return goja.Undefined()
	})

	vm.Set("setInterval", func(call goja.FunctionCall) goja.Value {
		fn := call.Argument(0)
		delay := call.Argument(1).ToInteger()
		if fnFn, ok := goja.AssertFunction(fn); ok {
			loop.SetInterval(func(vm *goja.Runtime) {
				fnFn(goja.Undefined())
			}, time.Duration(delay)*time.Millisecond)
		}
		return goja.Undefined()
	})

	loop.Start()
	defer loop.Stop()

	for k, v := range r.Val.GetAll() {
		vm.Set(k, v)
	}

	scriptText := strings.Join(lines, "\n")
	res, err := vm.RunString(scriptText)
	if err != nil {
		return fmt.Sprintf("JS错误(line:%d)：%v", line, err)
	}
	if res == goja.Undefined() {
		return ""
	}
	return res.String()
}

// FuncBlock 原生执行 函数> 框，语义与解释器 StateFunc 状态机一致：
//   - 开启行 `函数>变量名=触发` 或 `函数>变量名` 解析出变量名与触发词（触发词原样保留，不做插值）；
//   - 变量名非空：把内容行与行号存入 FuncBox（不执行、不输出）；
//   - 变量名为空：以当前变量表快照新建局部作用域（触发=触发词、触发词=""）直接执行内容并返回输出。
//
// 内容行含嵌套函数框时仍作为原始行整体存储/执行，与解释器累积 content 完全一致。
func (a *dicRuntime) FuncBlock(text string, lines []string, lineNums []int) string {
	r := a.r
	rest := text[len("函数>"):]
	valueName := rest
	trigger := ""
	if idx := strings.IndexByte(rest, '='); idx != -1 {
		valueName = rest[:idx]
		trigger = rest[idx+1:]
	}

	// 复制，避免 FuncBox 持久引用 ParseBody 的 body 底层切片，被后续复用改写。
	content := append([]string(nil), lines...)
	nums := append([]int(nil), lineNums...)

	if valueName == "" {
		// 直接执行：以当前变量表快照新建局部 Val，触发词/触发词变量初始化后运行内容。
		funcv := dto.NewVal().
			Reset(r.Val.P.GetAll()).
			Set("触发", trigger).
			Set("触发词", "")
		RunDic := dic_dto.NewRunDicEntry().
			SetGlobal_v(r.Val.G).
			Set_v(funcv).
			SetDic_v(r.Dic)
		RunDic.LineNums = nums
		return a.m.dicRunLineBytecode(RunDic, content)
	}

	// 存储函数框，供后续 $%变量名% 参数$ 调用。
	r.Val.P.Set(valueName, &dto.FuncBox{
		Trigger:  trigger,
		Content:  content,
		LineNums: nums,
	})
	return ""
}

// ForEachInit 求值 遍历> 框的遍历源并物化迭代项，记录键值变量名。语义与解释器 StateForEach 一致：
//   - 开启行 `遍历>k,v=表达式` 或 `遍历>k` 解析出键值变量名与遍历源表达式；
//   - 表达式经 %变量% 插值/$函数$ 执行后，反序列化为 JSON 对象（[]byte 保序）或 JSON 数组（[]any）；
//   - 对象逐 key/value、数组逐 index/value 物化；返回迭代项数量（0 表示无迭代）。
//
// depth 为该遍历帧在 VM 帧栈中的下标，用于嵌套遍历/循环时隔离各层迭代状态。
// 遍历体由字节码 VM 原生编译为跳转指令，循环控制（>终止遍历/>终止循环/>跳过）不再依赖
// For/ForEach 状态机，故此处仅负责物化迭代源，不逐轮执行内容。
func (a *dicRuntime) ForEachInit(depth int, text string) int {
	r := a.r
	rest := text[len("遍历>"):]
	valueName := rest
	valueExpr := ""
	if idx := strings.IndexByte(rest, '='); idx != -1 {
		valueName = rest[:idx]
		valueExpr = rest[idx+1:]
	}

	// 拆分键值变量名：逗号分隔 v1,v2；无逗号时 v2 固定为 "_"。
	v1, v2 := "_", "_"
	if idx := strings.IndexByte(valueName, ','); idx != -1 {
		v1 = valueName[:idx]
		v2 = valueName[idx+1:]
	} else {
		v1 = valueName
	}

	// 扩到 depth+1 个槽位（depth 同时含循环帧，中间可能留空），记录本层键值变量名。
	if len(a.feFrames) <= depth {
		a.feFrames = append(a.feFrames, make([]forEachFrame, depth+1-len(a.feFrames))...)
	}
	f := &a.feFrames[depth]
	f.v1, f.v2 = v1, v2
	f.items = nil

	// 求值遍历源：对象 → 保序逐键物化，数组 → 逐项物化；两者都失败则视为空（无迭代）。
	if valueExpr != "" {
		runText := utils.AnyToString(Runs(a.funcV, utils.AnyToString(count.RunCountText(r.Val, valueExpr))))
		var testjs map[string]any
		if json.Unmarshal([]byte(runText), &testjs) == nil {
			jsonparser.ObjectEach([]byte(runText), func(keyByte []byte, valueByte []byte, dataType jsonparser.ValueType, offset int) error {
				f.items = append(f.items, forEachItem{key: string(keyByte), value: string(valueByte)})
				return nil
			})
		} else {
			var thisjson []any
			if json.Unmarshal([]byte(runText), &thisjson) == nil {
				for key, value := range thisjson {
					it := forEachItem{key: key}
					if strVal, ok := value.(string); ok {
						it.value = strVal
					} else if resS, err := json.Marshal(value); err == nil {
						it.value = string(resS)
					}
					f.items = append(f.items, it)
				}
			}
		}
	}
	return len(f.items)
}

// ForEachNext 设置第 idx 项（0-based）的键值变量，返回是否在范围内（越界返回 false）。
// depth 为该遍历帧在 VM 帧栈中的下标。
func (a *dicRuntime) ForEachNext(depth int, idx int) bool {
	if depth < 0 || depth >= len(a.feFrames) {
		return false
	}
	f := &a.feFrames[depth]
	if idx < 0 || idx >= len(f.items) {
		return false
	}
	it := f.items[idx]
	a.r.Val.P.Set(f.v1, it.key)
	a.r.Val.P.Set(f.v2, it.value)
	return true
}

// dicRunLineBytecode 以字节码 VM 执行词条/块体，语义等价于旧解释器。
func (m *dicImpl) dicRunLineBytecode(r *dic_dto.DicEntry, body []string) string {
	return m.dicRunLineBytecodeInstrs(r, bc.Compile(ast.ParseBody(body, r.LineNums)))
}

// dicRunLineBytecodeInstrs 以预编译字节码执行块体。
// 触发词初始化/Class 挂载/槽同步与 dicRunLineBytecode 完全一致。
func (m *dicImpl) dicRunLineBytecodeInstrs(r *dic_dto.DicEntry, instrs []bc.Instr) string {
	r.Output.Clear()
	r.Val.P.Class = r.Dic.ClassValues()

	if r.Trigger {
		trigger := "Main"
		GetDicTrigger := "Main"
		if triggers, ok := r.Val.P.Get("触发词").(string); ok {
			trigger = triggers
		} else {
			r.Val.P.Set("触发词", trigger)
		}
		if GetDicTriggers, ok := r.Val.P.Get("触发").(string); ok {
			GetDicTrigger = GetDicTriggers
		} else {
			r.Val.P.Set("触发", GetDicTrigger)
		}
		dto.RunTrigger(trigger, GetDicTrigger, r.Val.P)
	}

	funcV := &dic_dto.DicFunc{
		Val:            r.Val,
		Sys:            r.Sys_v,
		Dic:            r.Dic,
		Output:         r.Output,
	}

	adapter := &dicRuntime{m: m, r: r, funcV: funcV}
	bc.Run(instrs, adapter)
	// 无锁热路径（循环变量/累加器直写槽）结束：一次性把槽同步回 num/obj，保持映射权威。
	r.Val.P.FlushSlotsToMap()
	return r.Output.Get()
}
