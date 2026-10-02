package build

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// ============== Blockly 工作区 JSON ↔ 词库 JSON 中间表示（IR）==============
//
// 积木编程页把「积木 ↔ IR」的映射也交给后端：前端只负责渲染与保存/载入
// Blockly 工作区 JSON（Blockly.serialization.workspaces 的产物），
// 后端负责工作区 JSON ↔ IR ↔ .n 源码，格式与语义由后端统一保证。
//
// 与前端 nebulaBlocks.js 的 workspaceToIr / irToWorkspace 严格互逆，
// 字段名、注释挂载、顶层块排序、载入布局全部保持一致。

const (
	blockLanguageVersion = 0
	commentIconType      = "comment"       // Blockly 注释图标在 icons 中的序列化键
	initFuncName         = "_初始化"          // 初始化积木固定的函数名
	funcBlockPrefix      = "nebulaFn_"     // 内置函数积木类型前缀
	methodBlockPrefix    = "nebulaMethod_" // 对象方法积木类型前缀（如 创建HTML 返回对象的 设置* 方法）

	ifMaxElif    = 8  // nbc_if 「否则如果」分支上限
	matchMaxCase = 16 // nbc_match 「如果是」分支上限
	jsonMaxRow   = 16 // nbc_json「键=值」行上限
)

// triggerKinds 触发词积木类型 → 词条 IR kind（普通 / 函数 / 内部 / 函数类方法 / 内部类方法五种独立积木）。
var triggerKinds = map[string]string{
	"nbc_trigger":             "normal",
	"nbc_trigger_func":        "func",
	"nbc_trigger_inner":       "inner",
	"nbc_trigger_class_func":  "func_class",
	"nbc_trigger_class_inner": "inner_class",
}

// scanOffset Blockly getTopBlocks(true) 排序用的偏移量（sin(SCAN_ANGLE°)，LTR 时 SCAN_ANGLE = 3）。
var scanOffset = math.Sin(3 * math.Pi / 180)

// blockState Blockly 序列化后的一块积木。
// 字段名与 Blockly.serialization.blocks 的 State 一致。
type blockState struct {
	Type       string                     `json:"type"`
	X          *float64                   `json:"x,omitempty"`
	Y          *float64                   `json:"y,omitempty"`
	Fields     map[string]any             `json:"fields,omitempty"`
	Inputs     map[string]connectionState `json:"inputs,omitempty"`
	Next       *connectionState           `json:"next,omitempty"`
	Icons      map[string]any             `json:"icons,omitempty"`
	ExtraState map[string]any             `json:"extraState,omitempty"`
}

// connectionState 一个连接上的影子块 / 子块。
type connectionState struct {
	Shadow *blockState `json:"shadow,omitempty"`
	Block  *blockState `json:"block,omitempty"`
}

// blockContainer 工作区里的积木集合（工作区状态 state.blocks 的值）。
type blockContainer struct {
	LanguageVersion int           `json:"languageVersion"`
	Blocks          []*blockState `json:"blocks"`
}

// workspaceState 整份工作区状态（Blockly.serialization.workspaces.save 的产物 / load 的入参）。
type workspaceState struct {
	Blocks blockContainer `json:"blocks"`
}

// fp 取浮点指针（序列化的 x/y 为整数，用指针区分「未设置」与 0）。
func fp(v float64) *float64 { return &v }

// newBlock 造一块带字段的积木。
func newBlock(typ string, fields map[string]any) *blockState {
	return &blockState{Type: typ, Fields: fields}
}

// field 取字段值（字段均为文本 / 下拉，统一按字符串读取）。
func (b *blockState) field(name string) string {
	if b == nil || b.Fields == nil {
		return ""
	}
	s, _ := b.Fields[name].(string)
	return s
}

// inputBlock 取某个输入上挂的积木（目标块优先，其次影子块），等价 Block.getInputTargetBlock。
func (b *blockState) inputBlock(name string) *blockState {
	if b == nil || b.Inputs == nil {
		return nil
	}
	c, ok := b.Inputs[name]
	if !ok {
		return nil
	}
	if c.Block != nil {
		return c.Block
	}
	return c.Shadow
}

// nextBlock 取 nextConnection 上挂的积木，等价 Block.getNextBlock。
func (b *blockState) nextBlock() *blockState {
	if b == nil || b.Next == nil {
		return nil
	}
	if b.Next.Block != nil {
		return b.Next.Block
	}
	return b.Next.Shadow
}

// setInput 设置一个输入连接。
func (b *blockState) setInput(name string, c *connectionState) {
	if b.Inputs == nil {
		b.Inputs = map[string]connectionState{}
	}
	b.Inputs[name] = *c
}

// extraInt 取 extraState 中的整数，缺失返回 -1。
func (b *blockState) extraInt(name string) int {
	if b == nil || b.ExtraState == nil {
		return -1
	}
	v, ok := b.ExtraState[name]
	if !ok {
		return -1
	}
	f, ok := v.(float64)
	if !ok {
		return -1
	}
	return int(f)
}

// extraBool 取 extraState 中的布尔值，缺失返回 def。
func (b *blockState) extraBool(name string, def bool) bool {
	if b == nil || b.ExtraState == nil {
		return def
	}
	v, ok := b.ExtraState[name]
	if !ok {
		return def
	}
	bv, ok := v.(bool)
	if !ok {
		return def
	}
	return bv
}

// x/y 取坐标（缺失按 0）。
func (b *blockState) x() float64 {
	if b == nil || b.X == nil {
		return 0
	}
	return *b.X
}

func (b *blockState) y() float64 {
	if b == nil || b.Y == nil {
		return 0
	}
	return *b.Y
}

/* ================= 工作区 → IR ================= */

// BlocksToIR 把 Blockly 工作区 JSON 转成结构化 IR。
// 入参是 workspaces.save 的「工作区状态」：{"blocks": {"languageVersion": 0, "blocks": [...]}}。
func BlocksToIR(data []byte) (IR, error) {
	ir := IR{Header: []Stmt{}, Entries: []Entry{}}
	var state workspaceState
	if err := json.Unmarshal(data, &state); err != nil {
		return ir, err
	}
	for _, b := range sortTopBlocks(state.Blocks.Blocks) {
		switch {
		case b.Type == "nbc_middleware":
			// 中间件积木对应文件头部：块自身的注释排在最前，正文语句按顺序展开。
			// 运行类积木为单行帽子块，正文挂在 next 链上（旧的 BODY 输入已废弃）。
			ir.Header = append(ir.Header, commentStmtsOf(b)...)
			ir.Header = append(ir.Header, blocksChainToIR(b.nextBlock())...)
		case b.Type == "nbc_init":
			// 初始化积木固定生成 [函数]_初始化 词条；内容为空时整块跳过。
			body := blocksChainToIR(b.nextBlock())
			if len(body) > 0 {
				ir.Entries = append(ir.Entries, commentEntriesOf(b)...)
				ir.Entries = append(ir.Entries, Entry{Kind: "func", Name: initFuncName, Body: body})
			}
		case triggerKinds[b.Type] != "" || b.Type == "nbc_comment":
			// 触发词 / 注释积木的内置注释还原为「前置注释词条」。
			ir.Entries = append(ir.Entries, commentEntriesOf(b)...)
			if e := blocksBuildEntry(b); e != nil {
				ir.Entries = append(ir.Entries, *e)
			}
		default:
			// 顶层自由语句积木（不在任何触发词 / 中间件 / 初始化积木里）直接忽略：
			// 不再归入文件头部，避免「运行」外的积木被当成初始化代码执行。
		}
	}
	return ir, nil
}

// sortTopBlocks 复刻 Blockly getTopBlocks(true)（sortByOrigin）：
// 按 y + sin(3°)·x 升序稳定排序，x/y 缺失按 0。
func sortTopBlocks(list []*blockState) []*blockState {
	out := make([]*blockState, len(list))
	copy(out, list)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].y()+scanOffset*out[i].x() < out[j].y()+scanOffset*out[j].x()
	})
	return out
}

// blocksBuildEntry 把词条积木（nbc_comment / nbc_trigger* / nbc_trigger_class_*）转换为词条 IR。
func blocksBuildEntry(b *blockState) *Entry {
	if b == nil {
		return nil
	}
	switch {
	case b.Type == "nbc_comment":
		return &Entry{Kind: "normal", Name: textToComment(b.field("TEXT")), Body: blocksChainToIR(b.nextBlock())}
	case triggerKinds[b.Type] != "":
		kind := triggerKinds[b.Type]
		rule := ""
		// 参数规则仅函数类别（[函数]/[函数:类]）使用，其余类别忽略 RULE 字段。
		if kind == "func" || kind == "func_class" {
			rule = strings.TrimSpace(b.field("RULE"))
		}
		return &Entry{
			Kind:      kind,
			ClassName: strings.TrimSpace(b.field("CLASS")),
			Name:      strings.TrimSpace(b.field("NAME")),
			Rule:      rule,
			Body:      blocksChainToIR(b.nextBlock()),
		}
	}
	return nil
}

// blocksChainToIR 沿 nextConnection 把语句块链构建为 IR 数组；
// 每块先输出其内置注释对应的注释语句，再输出语句本身，保证注释位置不变。
func blocksChainToIR(first *blockState) []Stmt {
	out := []Stmt{}
	for b := first; b != nil; b = b.nextBlock() {
		out = append(out, commentStmtsOf(b)...)
		if s := blocksStmtToIR(b); s != nil {
			out = append(out, *s)
		}
	}
	return out
}

// blocksStmtToIR 把单个语句块转换为 IR；无法识别返回 nil。
func blocksStmtToIR(b *blockState) *Stmt {
	if b == nil {
		return nil
	}
	// 对象方法积木：语句级 `$对象.方法 参数$` 行，与 `$函数$` 一样以 raw 行承载
	if strings.HasPrefix(b.Type, methodBlockPrefix) {
		name := methodCallName(strings.TrimPrefix(b.Type, methodBlockPrefix))
		return &Stmt{T: "raw", Text: blocksMethodCallText(b, name)}
	}
	// 内置函数积木：作语句使用时同样生成 `$函数名 参数$` 行（返回值被丢弃）
	if strings.HasPrefix(b.Type, funcBlockPrefix) {
		return &Stmt{T: "raw", Text: blocksValueToText(b)}
	}
	switch b.Type {
	case "nbc_set_var":
		return &Stmt{T: "assign", Name: strings.TrimSpace(b.field("NAME")), Op: b.field("OP"), V: blocksValueInputToText(b, "V")}
	case "nbc_raw":
		// 新格式：正文由圆形积木插槽 V（默认文本影子块）承载，可插入函数 / 变量 / 表达式积木；
		// 旧格式兼容直接写字段 RAW。
		if b.inputBlock("V") != nil {
			return &Stmt{T: "raw", Text: blocksValueInputToText(b, "V")}
		}
		return &Stmt{T: "raw", Text: b.field("RAW")}
	case "nbc_comment":
		return &Stmt{T: "comment", Text: b.field("TEXT")}
	case "nbc_import":
		return &Stmt{T: "import", Path: strings.TrimSpace(b.field("PATH"))}
	case "nbc_import_as":
		// 赋予值形式：变量:$引入 路径$
		return &Stmt{T: "import", Name: strings.TrimSpace(b.field("NAME")), Path: strings.TrimSpace(b.field("PATH"))}
	case "nbc_global_import":
		return &Stmt{T: "importGlobal", Path: strings.TrimSpace(b.field("PATH"))}
	case "nbc_if":
		elifs := []IfBranch{}
		for i := 0; i < blocksIfElifCount(b); i++ {
			n := strconv.Itoa(i)
			elifs = append(elifs, IfBranch{
				Cond: blocksValueInputToText(b, "ELIFCOND"+n),
				Body: blocksChainToIR(b.inputBlock("ELIFDO" + n)),
			})
		}
		s := &Stmt{T: "if", Cond: blocksValueInputToText(b, "COND"), Do: blocksChainToIR(b.inputBlock("DO")), Elifs: elifs}
		// 「否则」区被收起时不输出，避免生成空的 >否则 行
		if b.extraBool("hasElse", true) {
			s.Else = blocksChainToIR(b.inputBlock("ELSE"))
		}
		return s
	case "nbc_match":
		cases := []Case{}
		for i := 0; i < blocksMatchCaseCount(b); i++ {
			n := strconv.Itoa(i)
			cases = append(cases, Case{
				V:    strings.TrimSpace(b.field("CASEVAL" + n)),
				Body: blocksChainToIR(b.inputBlock("CASEBODY" + n)),
			})
		}
		s := &Stmt{T: "match", Expr: blocksValueInputToText(b, "EXPR"), Cases: cases}
		// 「如果不是」区被收起时不输出（后端要求其体非空）
		if b.extraBool("hasElse", true) {
			s.Def = blocksChainToIR(b.inputBlock("ELSE"))
		}
		return s
	case "nbc_loop_count":
		return &Stmt{T: "loopCount", Var: loopVar(b), Count: blocksValueInputToText(b, "COUNT"), Body: blocksChainToIR(b.inputBlock("BODY"))}
	case "nbc_loop_range":
		return &Stmt{
			T:    "loopRange",
			Var:  loopVar(b),
			From: blocksValueInputToText(b, "FROM"),
			To:   blocksValueInputToText(b, "TO"),
			Body: blocksChainToIR(b.inputBlock("BODY")),
		}
	case "nbc_loop_while":
		return &Stmt{T: "loopWhile", Cond: blocksValueInputToText(b, "COND"), Body: blocksChainToIR(b.inputBlock("BODY"))}
	case "nbc_foreach":
		return &Stmt{
			T:      "foreach",
			Mode:   b.field("MODE"),
			Key:    strings.TrimSpace(b.field("KEY")),
			Val:    strings.TrimSpace(b.field("VAL")),
			Target: blocksValueInputToText(b, "TARGET"),
			Body:   blocksChainToIR(b.inputBlock("BODY")),
		}
	case "nbc_break":
		return &Stmt{T: "break"}
	case "nbc_continue":
		return &Stmt{T: "continue"}
	case "nbc_stop":
		return &Stmt{T: "stop"}
	case "nbc_stop_loop":
		return &Stmt{T: "stopLoop"}
	case "nbc_stop_foreach":
		return &Stmt{T: "stopForeach"}
	case "nbc_json":
		kind := "obj"
		if b.field("KIND") == "arr" {
			kind = "arr"
		}
		body := []Stmt{}
		for i := 0; i < blocksJSONRowCount(b); i++ {
			n := strconv.Itoa(i)
			mode := "="
			if b.field("MODE"+n) == ":=" {
				mode = ":="
			}
			body = append(body, Stmt{
				T:    "kv",
				Key:  strings.TrimSpace(b.field("KEY" + n)),
				Mode: mode,
				V:    blocksValueInputToText(b, "VAL"+n),
			})
		}
		return &Stmt{T: "json", Name: strings.TrimSpace(b.field("NAME")), Kind: kind, Body: body}
	case "nbc_textblock":
		return &Stmt{
			T:    "textblock",
			Name: strings.TrimSpace(b.field("NAME")),
			Sep:  b.field("SEP"),
			Body: blocksChainToIR(b.inputBlock("BODY")),
		}
	}
	return nil
}

// loopVar 循环变量（缺失 / 为空时与积木默认值一致，取 i）。
func loopVar(b *blockState) string {
	if v := strings.TrimSpace(b.field("VAR")); v != "" {
		return v
	}
	return "i"
}

// blocksValueInputToText 取某个 value input 的值文本（未连接积木返回空串，影子文本块正常参与）。
func blocksValueInputToText(b *blockState, name string) string {
	return blocksValueToText(b.inputBlock(name))
}

// blocksValueToText 把值块（output 块）序列化为字符串。
func blocksValueToText(b *blockState) string {
	if b == nil {
		return ""
	}
	switch b.Type {
	case "nebulaText":
		return b.field("TEXT")
	case "nbc_var_get":
		return "%" + strings.TrimSpace(b.field("NAME")) + "%"
	case "nbc_expr":
		return "[" + strings.TrimSpace(b.field("EXPR")) + "]"
	case "nbc_quoted_text":
		return quotedText(b.field("TEXT"))
	}
	// 对象方法积木作值使用（如 `页面:$H.创建布局 页面$`、`网页:$H.获取$`）
	if strings.HasPrefix(b.Type, methodBlockPrefix) {
		return blocksMethodCallText(b, methodCallName(strings.TrimPrefix(b.Type, methodBlockPrefix)))
	}
	if !strings.HasPrefix(b.Type, funcBlockPrefix) {
		return ""
	}
	// 函数块类型即「前缀 + 函数名」，直接去前缀取回函数名
	name := strings.TrimPrefix(b.Type, funcBlockPrefix)
	count := funcArgCount(b)
	if count > 64 {
		count = 64
	}
	args := make([]string, 0, count)
	for i := 0; i < count; i++ {
		// 空槽保留占位（生成空参数），保证参数位置与积木上一致
		args = append(args, strings.TrimSpace(blocksValueToText(b.inputBlock("ARG"+strconv.Itoa(i)))))
	}
	if len(args) == 0 {
		return "$" + name + "$"
	}
	return "$" + name + " " + strings.Join(args, " ") + "$"
}

// quotedText 「引号文本」积木 → .n 参数文本：用双引号包裹整段，
// 内部的 " 转义为 \"、$ 转义为 \$（与 run.splitWithEscape 的解码口径对应），
// 使含空格的文本能作为一个完整参数，如 `$创建HTML "Nebula 词库"$`。
func quotedText(text string) string {
	return `"` + strings.NewReplacer(`"`, `\"`, `$`, `\$`).Replace(text) + `"`
}

// methodCallName 从对象方法积木的类型后缀取回方法名。
// 新格式为 `<类前缀>.<方法名>`（如 `创建画布.旋转`），取分隔符后的方法名；
// 旧格式（HTML 方法，如 `设置背景`）无分隔符，原样返回，保持向后兼容。
func methodCallName(suffix string) string {
	if i := strings.Index(suffix, "."); i > 0 {
		return suffix[i+1:]
	}
	return suffix
}

// blocksMethodCallText 把对象方法积木序列化为 `$对象.方法 参数$` 行文本
// （与手写词库的 `$H.设置文本 你好$` 完全一致）。
func blocksMethodCallText(b *blockState, name string) string {
	obj := strings.TrimSpace(b.field("VAR"))
	if obj == "" {
		obj = "H"
	}
	count := funcArgCount(b)
	if count > 64 {
		count = 64
	}
	args := make([]string, 0, count)
	for i := 0; i < count; i++ {
		// 空槽保留占位（生成空参数），保证参数位置与积木上一致
		args = append(args, strings.TrimSpace(blocksValueToText(b.inputBlock("ARG"+strconv.Itoa(i)))))
	}
	call := obj + "." + name
	if len(args) == 0 {
		return "$" + call + "$"
	}
	return "$" + call + " " + strings.Join(args, " ") + "$"
}

// funcArgCount 函数块的参数槽数（extraState.paramCount 为准，缺失时探测 ARG* 输入）。
func funcArgCount(b *blockState) int {
	if n := b.extraInt("paramCount"); n >= 0 {
		return n
	}
	n := 0
	for n < 64 {
		if _, ok := b.Inputs["ARG"+strconv.Itoa(n)]; !ok {
			break
		}
		n++
	}
	return n
}

// blocksIfElifCount 「否则如果」分支数（extraState.elifCount 为准，缺失时探测 ELIFCOND* 输入）。
func blocksIfElifCount(b *blockState) int {
	if n := b.extraInt("elifCount"); n >= 0 {
		return n
	}
	n := 0
	for n <= ifMaxElif {
		if _, ok := b.Inputs["ELIFCOND"+strconv.Itoa(n)]; !ok {
			break
		}
		n++
	}
	return n
}

// blocksMatchCaseCount 「如果是」分支数（extraState.caseCount 为准，
// 缺失时探测顶层字段 CASEVAL*：分支行是 dummy 输入，不写入 inputs）。
func blocksMatchCaseCount(b *blockState) int {
	if n := b.extraInt("caseCount"); n >= 0 {
		return n
	}
	n := 0
	for n <= matchMaxCase {
		if _, ok := b.Fields["CASEVAL"+strconv.Itoa(n)]; !ok {
			break
		}
		n++
	}
	return n
}

// blocksJSONRowCount JSON 构建的「键=值」行数（extraState.rowCount 为准，缺失时探测 VAL* 输入）。
func blocksJSONRowCount(b *blockState) int {
	if n := b.extraInt("rowCount"); n >= 0 {
		return n
	}
	n := 0
	for n <= jsonMaxRow {
		if _, ok := b.Inputs["VAL"+strconv.Itoa(n)]; !ok {
			break
		}
		n++
	}
	return n
}

/* ================= 注释辅助（与后端 ir_parse 的注释口径一致） ================= */

// isCommentText 判断文本（可含前导缩进）是否为注释行。
func isCommentText(t string) bool {
	return isCommentLine(strings.TrimLeft(t, " \t"))
}

// textToComment 注释积木文本 → 注释行（非注释形态补回 "// " 前缀）。
func textToComment(text string) string {
	if isCommentText(text) {
		return text
	}
	return "// " + text
}

// commentStmtOf 注释行 → IR 语句（"// " 形式转 comment，其余如 /* */ 转 raw 原样保留）。
func commentStmtOf(line string) Stmt {
	if strings.HasPrefix(line, "// ") {
		return Stmt{T: "comment", Text: line[3:]}
	}
	return Stmt{T: "raw", Text: line}
}

// commentTextLines 把内置注释文本切成注释行数组（CRLF 归一、去空行、非注释行补 "// " 前缀）。
// /* ... */ 跨行注释内部的行原样保留，避免被补上 "// " 前缀而改变内容。
func commentTextLines(text string) []string {
	s := strings.ReplaceAll(text, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	out := []string{}
	inBlock := false
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimRightFunc(l, unicode.IsSpace)
		if strings.TrimSpace(l) == "" {
			continue
		}
		switch {
		case inBlock:
			out = append(out, l)
			if strings.Contains(l, "*/") {
				inBlock = false
			}
		case isBlockCommentOpen(l):
			inBlock = true
			out = append(out, l)
		default:
			out = append(out, textToComment(l))
		}
	}
	return out
}

// blockCommentLines 读取积木的内置注释（Blockly 注释图标）并转为注释行数组。
func blockCommentLines(b *blockState) []string {
	if b == nil || b.Icons == nil {
		return nil
	}
	c, ok := b.Icons[commentIconType].(map[string]any)
	if !ok {
		return nil
	}
	text, _ := c["text"].(string)
	return commentTextLines(text)
}

// commentStmtsOf 积木的内置注释 → IR 注释语句数组（供正文 / 匹配分支按位置还原）。
func commentStmtsOf(b *blockState) []Stmt {
	lines := blockCommentLines(b)
	out := make([]Stmt, 0, len(lines))
	for _, l := range lines {
		out = append(out, commentStmtOf(l))
	}
	return out
}

// commentEntriesOf 积木的内置注释 → 前置注释词条
// （镜像后端 commentEntry 形态：首行作 name，其余行作 body 注释语句，保证往返幂等）。
func commentEntriesOf(b *blockState) []Entry {
	lines := blockCommentLines(b)
	if len(lines) == 0 {
		return nil
	}
	body := make([]Stmt, 0, len(lines)-1)
	for _, l := range lines[1:] {
		body = append(body, commentStmtOf(l))
	}
	return []Entry{{Kind: "normal", Name: lines[0], Body: body}}
}

// isCommentEntry 判断词条是否为「触发词上方的注释」——后端把它建成 kind=normal、name 为注释行的词条。
func isCommentEntry(e Entry) bool {
	return e.Kind == "normal" && isCommentText(e.Name)
}

// entryCommentLines 注释词条 → 注释行数组（name 为首行，body 中的注释 / 原样语句顺序展开）。
func entryCommentLines(e Entry) []string {
	lines := []string{}
	if e.Name != "" {
		lines = append(lines, e.Name)
	}
	for _, s := range e.Body {
		switch s.T {
		case "comment":
			lines = append(lines, textToComment(s.Text))
		case "raw":
			lines = append(lines, s.Text)
		}
	}
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

/* ================= IR → 工作区 ================= */

// IRToBlocks 把 IR 转成 Blockly 工作区载入状态（可直接喂给 workspaces.load）。
// 注释不生成独立积木：词条上方的注释挂到该词条积木的内置注释（comment 图标）上；
// 末尾无处挂载的注释才降级为注释积木链，保证内容不丢。
func IRToBlocks(ir IR) workspaceState {
	blocks := []*blockState{}
	var pending []string
	y := 40.0

	if hasNonComment(ir.Header) {
		b := &blockState{Type: "nbc_middleware", X: fp(40), Y: fp(y)}
		if body := irChain(ir.Header); body != nil {
			b.Next = &connectionState{Block: body}
		}
		blocks = append(blocks, b)
		y += 40
	} else {
		for _, s := range ir.Header {
			if s.T == "comment" {
				pending = append(pending, textToComment(s.Text))
			}
		}
	}

	for _, e := range ir.Entries {
		// 注释词条（后端把词条上方的注释解析成独立词条）：不建积木，累积给下一个词条。
		if isCommentEntry(e) {
			pending = append(pending, entryCommentLines(e)...)
			continue
		}
		b := irEntryToBlock(e, irChain(e.Body))
		if b == nil {
			continue
		}
		attachComment(b, pending)
		pending = nil
		b.X = fp(40)
		b.Y = fp(y)
		y += 40
		blocks = append(blocks, b)
	}

	if orphan := commentBlocks(pending); orphan != nil {
		orphan.X = fp(40)
		orphan.Y = fp(y)
		blocks = append(blocks, orphan)
	}

	return workspaceState{Blocks: blockContainer{LanguageVersion: blockLanguageVersion, Blocks: blocks}}
}

// hasNonComment 头部是否含非注释语句（决定头部是否建中间件积木）。
func hasNonComment(stmts []Stmt) bool {
	for _, s := range stmts {
		if s.T != "comment" {
			return true
		}
	}
	return false
}

// irEntryToBlock 把一条词条 IR 转成触发词积木 JSON。
func irEntryToBlock(e Entry, body *blockState) *blockState {
	if e.Kind == "" {
		return nil
	}
	if e.Kind == "func" && e.Name == initFuncName {
		b := &blockState{Type: "nbc_init"}
		if body != nil {
			b.Next = &connectionState{Block: body}
		}
		return b
	}
	if e.Kind == "func_class" || e.Kind == "inner_class" {
		typ := "nbc_trigger_class_inner"
		fields := map[string]any{"CLASS": e.ClassName, "NAME": e.Name}
		if e.Kind == "func_class" {
			typ = "nbc_trigger_class_func"
			// 参数规则仅函数类别（含类方法）写入积木。
			fields["RULE"] = e.Rule
		}
		b := newBlock(typ, fields)
		if body != nil {
			b.Next = &connectionState{Block: body}
		}
		return b
	}
	typ := "nbc_trigger"
	switch e.Kind {
	case "func":
		typ = "nbc_trigger_func"
	case "inner":
		typ = "nbc_trigger_inner"
	}
	fields := map[string]any{"NAME": e.Name}
	if e.Kind == "func" {
		fields["RULE"] = e.Rule
	}
	b := newBlock(typ, fields)
	if body != nil {
		b.Next = &connectionState{Block: body}
	}
	return b
}

// nbcRawBlock 造一个「词块」积木：正文放在圆形积木插槽 V 上，
// 默认挂文本影子块（可直接输入文本），也可换成函数 / 变量 / 表达式等圆形积木。
func nbcRawBlock(text string) *blockState {
	b := &blockState{Type: "nbc_raw"}
	b.setInput("V", irValueShadow(text))
	return b
}

// irStmtToBlock 把一条语句 IR 转成 Blockly 块 JSON；未知类型返回 nil。
func irStmtToBlock(s Stmt) *blockState {
	switch s.T {
	// output（普通文本行）与 raw（原样行）在 .n 里都是一行文本，统一用「词块」承载。
	case "output":
		return nbcRawBlock(s.V)
	case "raw":
		return nbcRawBlock(s.Text)
	case "assign":
		op := s.Op
		if op != "set" && op != "add" && op != "sub" && op != "raw" {
			op = "set"
		}
		b := newBlock("nbc_set_var", map[string]any{"NAME": s.Name, "OP": op})
		b.setInput("V", irValueShadow(s.V))
		return b
	case "import":
		// 赋予值形式：带实例变量名时映射到「引入赋值」积木
		if s.Name != "" {
			return newBlock("nbc_import_as", map[string]any{"NAME": s.Name, "PATH": s.Path})
		}
		return newBlock("nbc_import", map[string]any{"PATH": s.Path})
	case "importGlobal":
		return newBlock("nbc_global_import", map[string]any{"PATH": s.Path})
	case "if":
		b := &blockState{Type: "nbc_if"}
		b.setInput("COND", irValueShadow(s.Cond))
		if c, ok := irStmtInput(s.Do); ok {
			b.setInput("DO", c)
		}
		for i, e := range s.Elifs {
			n := strconv.Itoa(i)
			b.setInput("ELIFCOND"+n, irValueShadow(e.Cond))
			if c, ok := irStmtInput(e.Body); ok {
				b.setInput("ELIFDO"+n, c)
			}
		}
		if c, ok := irStmtInput(s.Else); ok {
			b.setInput("ELSE", c)
		}
		// 分支数量与「否则」区显隐状态；空「否则」导入后收起
		b.ExtraState = map[string]any{"elifCount": len(s.Elifs), "hasElse": len(s.Else) > 0}
		return b
	case "match":
		b := &blockState{Type: "nbc_match"}
		b.setInput("EXPR", irValueShadow(s.Expr))
		fields := map[string]any{}
		for i, c := range s.Cases {
			n := strconv.Itoa(i)
			fields["CASEVAL"+n] = c.V
			if cc, ok := irStmtInput(c.Body); ok {
				b.setInput("CASEBODY"+n, cc)
			}
		}
		b.Fields = fields
		if c, ok := irStmtInput(s.Def); ok {
			b.setInput("ELSE", c)
		}
		// 分支数量与「如果不是」区显隐状态；空「如果不是」导入后收起
		b.ExtraState = map[string]any{"caseCount": len(s.Cases), "hasElse": len(s.Def) > 0}
		return b
	case "loopCount":
		b := newBlock("nbc_loop_count", map[string]any{"VAR": s.Var})
		b.setInput("COUNT", irValueShadow(s.Count))
		if c, ok := irStmtInput(s.Body); ok {
			b.setInput("BODY", c)
		}
		return b
	case "loopRange":
		b := newBlock("nbc_loop_range", map[string]any{"VAR": s.Var})
		b.setInput("FROM", irValueShadow(s.From))
		b.setInput("TO", irValueShadow(s.To))
		if c, ok := irStmtInput(s.Body); ok {
			b.setInput("BODY", c)
		}
		return b
	case "loopWhile":
		b := &blockState{Type: "nbc_loop_while"}
		b.setInput("COND", irValueShadow(s.Cond))
		if c, ok := irStmtInput(s.Body); ok {
			b.setInput("BODY", c)
		}
		return b
	case "foreach":
		mode := "v"
		if s.Mode == "kv" {
			mode = "kv"
		}
		b := newBlock("nbc_foreach", map[string]any{"MODE": mode, "KEY": s.Key, "VAL": s.Val})
		b.setInput("TARGET", irValueShadow(s.Target))
		if c, ok := irStmtInput(s.Body); ok {
			b.setInput("BODY", c)
		}
		return b
	case "break":
		return &blockState{Type: "nbc_break"}
	case "continue":
		return &blockState{Type: "nbc_continue"}
	case "stop":
		return &blockState{Type: "nbc_stop"}
	case "stopLoop":
		return &blockState{Type: "nbc_stop_loop"}
	case "stopForeach":
		return &blockState{Type: "nbc_stop_foreach"}
	case "json":
		kind := "obj"
		if s.Kind == "arr" {
			kind = "arr"
		}
		// 每行「键=值」对应一个 VAL{i} 值输入，键/模式写为块级字段
		fields := map[string]any{"NAME": s.Name, "KIND": kind}
		b := &blockState{Type: "nbc_json", Fields: fields}
		for i, kv := range s.Body {
			n := strconv.Itoa(i)
			fields["KEY"+n] = kv.Key
			mode := "="
			if kv.Mode == ":=" {
				mode = ":="
			}
			fields["MODE"+n] = mode
			b.setInput("VAL"+n, irValueShadow(kv.V))
		}
		b.ExtraState = map[string]any{"rowCount": len(s.Body)}
		return b
	case "textblock":
		b := newBlock("nbc_textblock", map[string]any{"NAME": s.Name, "SEP": s.Sep})
		if c, ok := irStmtInput(s.Body); ok {
			b.setInput("BODY", c)
		}
		return b
	}
	return nil
}

// irValueShadow 把 IR 的字符串值转成值输入（文本影子块）。
func irValueShadow(v string) *connectionState {
	return &connectionState{Shadow: newBlock("nebulaText", map[string]any{"TEXT": v})}
}

// irStmtInput 把语句数组转成 input_statement 的连接；空数组返回 false（省略该输入）。
func irStmtInput(stmts []Stmt) (*connectionState, bool) {
	head := irChain(stmts)
	if head == nil {
		return nil, false
	}
	return &connectionState{Block: head}, true
}

// irChain 把语句 IR 数组串成块链。
func irChain(stmts []Stmt) *blockState {
	return irChainBlocks(irStmtBlocks(stmts))
}

// irStmtBlocks 把语句 IR 数组转成块 JSON 数组；连续的注释语句挂到其后方首个积木的内置注释上，
// 末尾无处挂载的注释降级为 // 注释积木链，保证内容不丢。
func irStmtBlocks(stmts []Stmt) []*blockState {
	list := []*blockState{}
	var pending []string
	for _, s := range stmts {
		if s.T == "comment" {
			pending = append(pending, textToComment(s.Text))
			continue
		}
		b := irStmtToBlock(s)
		if b == nil {
			continue
		}
		if len(pending) > 0 {
			attachComment(b, pending)
			pending = nil
		}
		list = append(list, b)
	}
	if tail := commentBlocks(pending); tail != nil {
		list = append(list, tail)
	}
	return list
}

// irChainBlocks 把块 JSON 数组沿 nextConnection 串成链，返回首块。
func irChainBlocks(list []*blockState) *blockState {
	var head, tail *blockState
	for _, b := range list {
		if b == nil {
			continue
		}
		if head == nil {
			head = b
		} else {
			tail.Next = &connectionState{Block: b}
		}
		tail = b
	}
	return head
}

// attachComment 把注释行写入积木 JSON 的内置注释（Blockly 注释图标，序列化键为 "comment"）。
func attachComment(b *blockState, lines []string) {
	if b == nil || len(lines) == 0 {
		return
	}
	if b.Icons == nil {
		b.Icons = map[string]any{}
	}
	b.Icons[commentIconType] = map[string]any{"text": strings.Join(lines, "\n")}
}

// commentBlocks 把注释行转成 // 注释 / 词块积木链（仅在注释无处挂载时兜底，避免内容丢失）。
func commentBlocks(lines []string) *blockState {
	list := make([]*blockState, 0, len(lines))
	for _, l := range lines {
		s := commentStmtOf(l)
		if s.T == "comment" {
			list = append(list, newBlock("nbc_comment", map[string]any{"TEXT": s.Text}))
		} else {
			list = append(list, nbcRawBlock(s.Text))
		}
	}
	return irChainBlocks(list)
}
