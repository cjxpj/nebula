package bc

import (
	"strconv"
	"strings"

	"github.com/cjxpj/nebula/build"
	"github.com/cjxpj/nebula/dic/ast"
)

// blockKind 编译期执行块类别，与 AST 框块一一对应。
type blockKind uint8

const (
	blockIf      blockKind = iota // 如果> 判断块
	blockFor                      // 循环> 循环块
	blockForEach                  // 遍历> 遍历块
)

// blockCtx 编译期执行块上下文，用于回填 break/continue/skip 的跳转目标。
// 循环（blockFor/blockForEach）记录循环体起始 PC、循环末尾 PC 与弹出帧 PC；
// 判断（blockIf）记录判断块末尾 PC（>跳过 的跳转目标）。
type blockCtx struct {
	kind      blockKind
	bodyStart int   // blockFor/blockForEach：循环体起始 PC
	endPc     int   // blockFor/blockForEach：OpLoopEnd/OpForEachEnd 的 PC（continue 目标）
	popPc     int   // blockFor/blockForEach：OpLoopPop/OpForEachPop 的 PC（break 目标）
	ifEnd     int   // blockIf：判断块末尾 PC（>跳过 目标）
	frameIdx  int   // blockFor/blockForEach：该块帧在 VM 帧栈中的下标（用于跨层 break 截断）
	breaks    []int // 待回填的 break 跳转指令索引
	continues []int // 待回填的 continue 跳转指令索引
	skips     []int // blockIf：待回填的 >跳过 跳转指令索引
}

type compiler struct {
	instrs     []Instr
	blocks     []blockCtx
	frameDepth int // 当前 VM 帧栈深度（循环/遍历帧数）
	// endJumps 记录需跳转到整个程序末尾的跳转指令索引（行内 if 命中分支后 break 到顶层）。
	endJumps []int
}

// Compile 将 AST 节点列表编译为字节码指令序列。
func Compile(nodes []ast.Node) []Instr {
	c := &compiler{}
	c.compileNodes(nodes)
	end := len(c.instrs)
	for _, idx := range c.endJumps {
		c.instrs[idx].Arg = end
	}
	return c.instrs
}

func (c *compiler) emit(in Instr) int {
	c.instrs = append(c.instrs, in)
	return len(c.instrs) - 1
}

func (c *compiler) compileNodes(nodes []ast.Node) {
	for i := 0; i < len(nodes); i++ {
		n := nodes[i]
		if s, ok := n.(*ast.Stmt); ok {
			if cond, isIf := inlineIfStart(s.Text); isIf {
				i = c.compileInlineIf(nodes, i, cond)
				continue
			}
		}
		c.compileNode(n)
	}
}

func (c *compiler) compileNode(n ast.Node) {
	switch v := n.(type) {
	case *ast.Stmt:
		c.compileStmt(v.Line, v.Text)
	case *ast.Block:
		c.compileBlock(v)
	}
}

// compileStmt 编译叶子语句：识别终止/循环控制关键字，无副作用赋值下沉为 OpAssign，
// 其余交给 Runtime.Line。line 为语句真实源文件行号（1-based，0 表示未知）。
func (c *compiler) compileStmt(line int, text string) {
	if cond, offset, ok := parseJumpRel(text); ok {
		c.emit(Instr{Op: OpJumpRel, Text: cond, Expr: offset, Line: line})
		return
	}
	// 畸形 >跳行(（有内容但缺 )>> 偏移）：解释器 execProgram 命中「>跳行( 前缀且前缀非空」
	// 后 continue，即静默消费该行（无输出、不跳转）。>跳行(（空前缀）除外，按普通行处理。
	if rest, has := strings.CutPrefix(text, ">跳行("); has && rest != "" {
		c.emit(Instr{Op: OpNop})
		return
	}
	switch {
	case text == ">终止":
		c.emit(Instr{Op: OpHalt})
	case strings.HasPrefix(text, ">终止 ") && len(text) > len(">终止 "):
		c.emit(Instr{Op: OpHaltOut, Text: text[len(">终止 "):]})
	case text == ">终止循环", text == ">中断":
		if c.emitBreak(blockFor) {
			return
		}
		c.emit(Instr{Op: OpLine, Line: line, Text: text})
	case text == ">终止遍历":
		if c.emitBreak(blockForEach) {
			return
		}
		c.emit(Instr{Op: OpLine, Line: line, Text: text})
	case text == ">跳过":
		if c.emitSkip() {
			return
		}
		c.emit(Instr{Op: OpLine, Line: line, Text: text})
	default:
		if vt, vp, vs, ok := assignableLeaf(text); ok {
			c.emit(Instr{Op: OpAssign, Line: line, Text: text, VType: vt, Prefix: vp, Suffix: vs})
			return
		}
		c.emit(Instr{Op: OpLine, Line: line, Text: text})
	}
}

// parseJumpRel 识别 >跳行(条件)>>偏移 语句，返回条件表达式与偏移表达式。
// 语义与解释器 execProgram 的跳行分支一致：对原始行做 CutPrefix(">跳行(")，
// 再 SplitN(rest, ")>>", 2)；解析失败（缺 )>> 或空前缀）返回 ok=false，由调用方当普通行处理。
func parseJumpRel(text string) (cond, offset string, ok bool) {
	rest, has := strings.CutPrefix(text, ">跳行(")
	if !has || rest == "" {
		return "", "", false
	}
	parts := strings.SplitN(rest, ")>>", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// assignableLeaf 判断叶子语句是否为「无副作用的赋值/算术」，返回其结构化信息。
// 仅下沉执行变量赋值（vType 4）、纯文本赋值（vType 5）与右操作数无副作用的算术（vType 1/2/7/8），
// 这些语句不产生输出、不触发函数调用副作用，可在 VM 内直接执行，跳过旧解释器框架。
func assignableLeaf(text string) (int8, string, string, bool) {
	vt, vp, vs := build.ValTextTest(text)
	switch vt {
	case 4: // 执行变量 :%: 插值（仅变量读取，无函数副作用、无输出）
		return vt, vp, vs, true
	case 5: // 纯文本赋值 n:字面量
		return vt, vp, vs, true
	case 1, 2, 7, 8: // 算术，仅右操作数为纯变量 %var% 或纯字面量（不含 %/$）时无副作用
		if isSafeArithOperand(vs) {
			return vt, vp, vs, true
		}
	}
	return 0, "", "", false
}

// isSafeArithOperand 判断算术右操作数是否无副作用（不产生输出、不触发函数/变量插值）：
//   - 纯变量 %var%：直接取变量值，不执行函数；
//   - 纯字面量（不含 % 与 $）：不触发任何插值。
func isSafeArithOperand(s string) bool {
	if s == "" {
		return false
	}
	if len(s) > 2 && s[0] == '%' && s[len(s)-1] == '%' && !strings.Contains(s[1:len(s)-1], "%") {
		return true
	}
	return !strings.ContainsAny(s, "%$")
}

// findBlock 从栈顶向下查找第一个指定类型的执行块，返回其下标（-1 表示未找到）。
func (c *compiler) findBlock(kind blockKind) int {
	for i := len(c.blocks) - 1; i >= 0; i-- {
		if c.blocks[i].kind == kind {
			return i
		}
	}
	return -1
}

// emitBreak 发射 >中断 / >终止循环 / >终止遍历 的统一中断指令：跳出最内层指定类别（循环/遍历）执行块。
// 三者共用 OpBreak，仅目标块类别不同（>中断 与 >终止循环 均跳循环）；命中后由块关闭逻辑回填截断帧深度与弹出指令 PC。
// 返回是否成功发射（无对应类别执行块时返回 false，交由调用方当作普通行）。
func (c *compiler) emitBreak(kind blockKind) bool {
	i := c.findBlock(kind)
	if i < 0 {
		return false
	}
	idx := c.emit(Instr{Op: OpBreak})
	c.blocks[i].breaks = append(c.blocks[i].breaks, idx)
	return true
}

// emitSkip 发射 >跳过 跳转。语义为「结束最内层执行块」：
//   - 最内层为判断块：跳到判断块末尾（跳过剩余分支），与解释器 IfFunc.Jump 一致；
//   - 最内层为循环/遍历块：跳到其 OpLoopEnd/OpForEachEnd（continue）。
//
// 返回是否成功发射（无任何执行块时返回 false，交由调用方当作普通行）。
func (c *compiler) emitSkip() bool {
	if len(c.blocks) == 0 {
		return false
	}
	top := len(c.blocks) - 1
	if c.blocks[top].kind == blockIf {
		idx := c.emit(Instr{Op: OpJump})
		c.blocks[top].skips = append(c.blocks[top].skips, idx)
		return true
	}
	idx := c.emit(Instr{Op: OpJump})
	c.blocks[top].continues = append(c.blocks[top].continues, idx)
	return true
}

// emitInlineBreak 发射行内 if 命中分支后「结束当前执行块」的跳转：
//   - 最内层为判断块：跳到判断块末尾（等价 >跳过）；
//   - 最内层为循环/遍历块：跳到其循环末尾（continue）；
//   - 顶层：跳到整个程序末尾。
//
// 与解释器 execProgram 行内 if 命中后遇到 elif/else 时的 break 语义一致（return 当前 execProgram）。
func (c *compiler) emitInlineBreak() {
	if c.emitSkip() {
		return
	}
	idx := c.emit(Instr{Op: OpJump})
	c.endJumps = append(c.endJumps, idx)
}

// inlineIfStart 判断语句是否为行内判断起始（如果:/if:），返回条件表达式。
func inlineIfStart(text string) (string, bool) {
	if len(text) > 7 && strings.HasPrefix(text, "如果:") {
		return text[7:], true
	}
	if len(text) > 3 && strings.HasPrefix(text, "if:") {
		return text[3:], true
	}
	return "", false
}

// inlineElifCond 判断语句是否为行内判断的 elif 分支（否则如果:/elif:），返回条件表达式。
func inlineElifCond(text string) (string, bool) {
	if len(text) > 13 && strings.HasPrefix(text, "否则如果:") {
		return text[13:], true
	}
	if len(text) > 5 && strings.HasPrefix(text, "elif:") {
		return text[5:], true
	}
	return "", false
}

// inlineIsElse 判断语句是否为行内判断的 else 分支。
func inlineIsElse(text string) bool {
	return text == "else" || text == "否则"
}

// inlineIsEndif 判断语句是否为行内判断的终止标记（如果尾/end）。
func inlineIsEndif(text string) bool {
	return text == "如果尾" || text == "end"
}

// inlineBranch 行内判断的一个分支（cond 为空表示 else 分支）。
// breakAfter 为 true 表示该分支（由 返回+如果尾 收尾）命中后需 break 到当前执行块末尾，
// 与解释器 matched 状态下遇到 返回+如果尾 的 break 语义一致。
type inlineBranch struct {
	cond       string
	body       []ast.Node
	breakAfter bool
}

// compileInlineIf 编译一条行内判断链，语义与解释器 execProgram 的 isif/lock 状态机一致：
//
//	如果:/if: cond ... [否则如果:/elif: cond ...] [否则/else ...] [如果尾/end]
//	- 命中分支后执行其正文，遇到下一个分支标记（否则如果:/elif:/否则/else）时结束当前执行块（break）；
//	- 命中分支到 如果尾/end 时正常继续后续语句；
//	- 无分支命中时执行 else 正文（若有）。
//
// start 为「如果:/if:」语句下标，返回最后一个已消费节点下标（调用方 for 循环 i++ 后推进到下一未消费节点）。
func (c *compiler) compileInlineIf(nodes []ast.Node, start int, cond string) int {
	var branches []inlineBranch
	cur := inlineBranch{cond: cond}
	i := start + 1
	endIndex := len(nodes) - 1

parse:
	for i < len(nodes) {
		s, ok := nodes[i].(*ast.Stmt)
		if !ok {
			cur.body = append(cur.body, nodes[i])
			i++
			continue
		}
		t := s.Text
		if c2, ok := inlineElifCond(t); ok {
			branches = append(branches, cur)
			cur = inlineBranch{cond: c2}
			i++
			continue
		}
		if inlineIsElse(t) {
			branches = append(branches, cur)
			// else 正文为之后全部节点（含可能的 如果尾/end 等，均按普通文本执行，与解释器一致）。
			branches = append(branches, inlineBranch{body: nodes[i+1:]})
			endIndex = len(nodes) - 1
			break parse
		}
		if inlineIsEndif(t) {
			branches = append(branches, cur)
			endIndex = i
			break parse
		}
		if t == "返回" && i+1 < len(nodes) {
			if nxt, ok := nodes[i+1].(*ast.Stmt); ok && inlineIsEndif(nxt.Text) {
				cur.breakAfter = true
				branches = append(branches, cur)
				endIndex = i + 1
				break parse
			}
		}
		cur.body = append(cur.body, nodes[i])
		i++
	}
	if i >= len(nodes) {
		branches = append(branches, cur)
	}

	// 编译分支：末尾分支命中后自然落到底部继续；非末尾分支命中后 break 到当前执行块末尾。
	for idx := range branches {
		br := branches[idx]
		last := idx == len(branches)-1
		if br.cond == "" {
			c.compileNodes(br.body)
			continue
		}
		jf := c.emit(Instr{Op: OpJumpIfFalse, Text: br.cond})
		c.compileNodes(br.body)
		if br.breakAfter || !last {
			c.emitInlineBreak()
		}
		c.instrs[jf].Arg = len(c.instrs)
	}
	return endIndex
}

func (c *compiler) compileBlock(b *ast.Block) {
	switch b.OpenKind {
	case ast.BlockIf:
		c.compileIf(b)
	case ast.BlockFor:
		c.compileFor(b)
	case ast.BlockText:
		c.compileText(b)
	case ast.BlockJson:
		c.compileJson(b)
	case ast.BlockNewJson:
		c.compileNewJson(b)
	case ast.BlockVarNewJson:
		c.compileVarNewJson(b)
	case ast.BlockValChain:
		c.compileValChain(b)
	case ast.BlockValText:
		c.compileValText(b, false)
	case ast.BlockValTextr:
		c.compileValText(b, true)
	case ast.BlockNodeJs:
		c.compileNodeJs(b)
	case ast.BlockFunc:
		c.compileFunc(b)
	case ast.BlockForEach:
		c.compileForEach(b)
	}
}

// compileText 编译 文本>/纯文本> 框为 OpTextBlock：内容行由运行时按
// 纯文本（原样）或文本（%变量% 插值）拼接，换行符由开启行后缀插值得到。
func (c *compiler) compileText(b *ast.Block) {
	pure := strings.HasPrefix(b.Open, "纯文本>")
	arg := 0
	if pure {
		arg = 1
	}
	lines := make([]string, 0, len(b.Children))
	for _, child := range b.Children {
		if s, ok := child.(*ast.Stmt); ok {
			lines = append(lines, s.Text)
		}
	}
	c.emit(Instr{Op: OpTextBlock, Text: b.Open, Lines: lines, Arg: arg})
}

// compileJson 编译 JSON> 框为 OpJsonBlock。
func (c *compiler) compileJson(b *ast.Block) {
	lines := make([]string, 0, len(b.Children))
	for _, child := range b.Children {
		if s, ok := child.(*ast.Stmt); ok {
			lines = append(lines, s.Text)
		}
	}
	c.emit(Instr{Op: OpJsonBlock, Text: b.Open, Lines: lines})
}

// compileNewJson 编译 JSON>{ / JSON>[ 框为 OpNewJsonBlock。
// 注意：使用 b.Raw[1:] 而非 b.Children，因为闭合括号行（} / ]）不在 Children 中，
// 但运行时需要它完成括号平衡与文本累积。
func (c *compiler) compileNewJson(b *ast.Block) {
	var lines []string
	if len(b.Raw) > 1 {
		lines = b.Raw[1:]
	}
	c.emit(Instr{Op: OpNewJsonBlock, Text: b.Open, Lines: lines})
}

// compileVarNewJson 编译 变量:{ / 变量:[ 框为 OpVarNewJsonBlock。
// 与 compileNewJson 一致使用 b.Raw[1:] 作为内容行（含闭合括号行），Text 携带变量名。
func (c *compiler) compileVarNewJson(b *ast.Block) {
	var lines []string
	if len(b.Raw) > 1 {
		lines = b.Raw[1:]
	}
	c.emit(Instr{Op: OpVarNewJsonBlock, Text: b.Open, Lines: lines})
}

// compileValChain 编译 变量:>>> 框为 OpValChainBlock：内容行为开启行与关闭行之间的原始行
// （不含 <<< 关闭行，与解释器 StateValChain 逐行处理 content 一致），Text 携带变量名。
func (c *compiler) compileValChain(b *ast.Block) {
	lines := make([]string, 0, len(b.Children))
	for _, child := range b.Children {
		if s, ok := child.(*ast.Stmt); ok {
			lines = append(lines, s.Text)
		}
	}
	c.emit(Instr{Op: OpValChainBlock, Text: b.Open, Lines: lines})
}

// compileValText 编译 变量:""" / 变量:”' 框为 OpValTextBlock：
// 内容行不含关闭行，raw 为 true 表示内容原样（”'），false 表示内容 %变量% 插值（"""）。
func (c *compiler) compileValText(b *ast.Block, raw bool) {
	arg := 0
	if raw {
		arg = 1
	}
	lines := make([]string, 0, len(b.Children))
	for _, child := range b.Children {
		if s, ok := child.(*ast.Stmt); ok {
			lines = append(lines, s.Text)
		}
	}
	c.emit(Instr{Op: OpValTextBlock, Text: b.Open, Lines: lines, Arg: arg})
}

// compileNodeJs 编译 --js 框为 OpNodeJsBlock：内容行为开启行与关闭行之间的原始行
// （不含 --end 关闭行），Line 记录关闭行行号供 JS 报错定位。
func (c *compiler) compileNodeJs(b *ast.Block) {
	lines := make([]string, 0, len(b.Children))
	for _, child := range b.Children {
		if s, ok := child.(*ast.Stmt); ok {
			lines = append(lines, s.Text)
		}
	}
	c.emit(Instr{Op: OpNodeJsBlock, Lines: lines, Line: b.CloseLine})
}

// compileFunc 编译 函数> 框为 OpFuncBlock：内容行为开启行与关闭行之间的原始行
// （含嵌套函数框的 函数>/<函数 行，与解释器 StateFunc 累积的 content 一致），
// LineNums 记录内容行的原始文件行号，供存储 FuncBox 后调用时定位报错。
func (c *compiler) compileFunc(b *ast.Block) {
	content := b.Raw
	contentNums := b.RawLineNums
	if len(content) > 0 {
		content = content[1:] // 去掉开启行
		contentNums = contentNums[1:]
	}
	if b.CloseLine != 0 && len(content) > 0 {
		content = content[:len(content)-1] // 去掉关闭行
		contentNums = contentNums[:len(contentNums)-1]
	}
	c.emit(Instr{Op: OpFuncBlock, Text: b.Open, Lines: content, LineNums: contentNums})
}

// compileForEach 编译 遍历> 框为原生遍历循环：OpForEachInit 求值遍历源并入帧，
// 遍历体（b.Children）内联编译，循环控制（>终止遍历/>终止循环/>跳过）通过统一执行块栈
// 解析为跳转指令，实现嵌套遍历/循环/判断的穿越语义。
func (c *compiler) compileForEach(b *ast.Block) {
	initIdx := c.emit(Instr{Op: OpForEachInit, Text: b.Open})
	bodyStart := len(c.instrs)

	c.blocks = append(c.blocks, blockCtx{kind: blockForEach, bodyStart: bodyStart, frameIdx: c.frameDepth})
	c.frameDepth++

	c.compileNodes(b.Children)

	top := c.blocks[len(c.blocks)-1] // 弹出前取当前遍历 ctx（含编译期间回填的 break/continue 索引）
	c.blocks = c.blocks[:len(c.blocks)-1]
	c.frameDepth--

	endPc := c.emit(Instr{Op: OpForEachEnd, Arg: bodyStart})
	popPc := c.emit(Instr{Op: OpForEachPop})
	c.instrs[initIdx].End = popPc

	for _, idx := range top.breaks {
		c.instrs[idx].Arg = popPc
		c.instrs[idx].End = top.frameIdx + 1
	}
	for _, idx := range top.continues {
		c.instrs[idx].Arg = endPc
	}
}

// branch 一个判断分支：条件（否则为空）与正文节点。
type branch struct {
	cond string
	body []ast.Node
}

// splitIfBranches 按 >否则如果:/ >否则 把判断框的子节点切成分支。
func splitIfBranches(open string, children []ast.Node) []branch {
	cond := strings.TrimPrefix(open, "如果>")
	branches := []branch{{cond: cond}}
	cur := &branches[len(branches)-1]
	for _, n := range children {
		if s, ok := n.(*ast.Stmt); ok {
			switch {
			case strings.HasPrefix(s.Text, ">否则如果:"):
				branches = append(branches, branch{cond: s.Text[len(">否则如果:"):]})
				cur = &branches[len(branches)-1]
				continue
			case s.Text == ">否则":
				branches = append(branches, branch{})
				cur = &branches[len(branches)-1]
				continue
			}
		}
		cur.body = append(cur.body, n)
	}
	return branches
}

func (c *compiler) compileIf(b *ast.Block) {
	branches := splitIfBranches(b.Open, b.Children)
	var endJumps []int

	c.blocks = append(c.blocks, blockCtx{kind: blockIf})

	for i, br := range branches {
		hasCond := br.cond != ""
		var jumpIfFalse int
		if hasCond {
			jumpIfFalse = c.emit(Instr{Op: OpJumpIfFalse, Text: br.cond})
		}
		c.compileNodes(br.body)
		// 非最后一个分支：执行完跳到整个判断块末尾
		if i != len(branches)-1 {
			endJumps = append(endJumps, c.emit(Instr{Op: OpJump}))
		}
		if hasCond {
			c.instrs[jumpIfFalse].Arg = len(c.instrs)
		}
	}

	end := len(c.instrs)
	for _, idx := range endJumps {
		c.instrs[idx].Arg = end
	}

	// 回填 >跳过 到判断块末尾
	top := c.blocks[len(c.blocks)-1]
	c.blocks = c.blocks[:len(c.blocks)-1]
	for _, idx := range top.skips {
		c.instrs[idx].Arg = end
	}
}

func (c *compiler) compileFor(b *ast.Block) {
	if strings.HasPrefix(b.Open, "判断循环>") {
		c.compileForWhile(b)
		return
	}

	rest := strings.TrimPrefix(b.Open, "循环>")
	// 循环入口指令：默认 OpLoop 无限循环（Arg=-1）。
	instr := Instr{Op: OpLoop, Text: rest, Arg: -1}

	// 「循环>变量=次数」：字面量整数下沉为 OpLoop；变量/函数等运行时求值次数下沉为 OpLoopDyn。
	// 「循环>变量=起始~结束」：范围循环下沉为 OpLoopRange（字面量起止用 Start/Arg，动态用 Expr）。
	if before, after, ok := strings.Cut(rest, "="); ok {
		instr.Text = before
		expr := after
		if startExpr, endExpr, ok := strings.Cut(expr, "~"); ok {
			instr.Op = OpLoopRange
			s1, err1 := strconv.Atoi(startExpr)
			s2, err2 := strconv.Atoi(endExpr)
			if err1 == nil && err2 == nil {
				instr.Start, instr.Arg = s1, s2
			} else {
				instr.Expr = expr
			}
		} else if n, err := strconv.Atoi(expr); err == nil {
			// 负数次数等价 0 次（解释器 for i:=1; i<=count 不执行），
			// 避免与 OpLoop 的无限循环哨兵（-1）冲突。
			if n < 0 {
				n = 0
			}
			instr.Arg = n
		} else {
			instr.Op, instr.Expr = OpLoopDyn, expr
		}
	}

	loopIdx := c.emit(instr)
	bodyStart := len(c.instrs)

	c.blocks = append(c.blocks, blockCtx{kind: blockFor, bodyStart: bodyStart, frameIdx: c.frameDepth})
	c.frameDepth++

	c.compileNodes(b.Children)

	top := c.blocks[len(c.blocks)-1] // 弹出前取当前循环 ctx（含编译期间回填的 break/continue 索引）
	c.blocks = c.blocks[:len(c.blocks)-1]
	c.frameDepth--

	endPc := c.emit(Instr{Op: OpLoopEnd, Arg: bodyStart})
	popPc := c.emit(Instr{Op: OpLoopPop})
	c.instrs[loopIdx].End = popPc

	for _, idx := range top.breaks {
		c.instrs[idx].Arg = popPc
		c.instrs[idx].End = top.frameIdx + 1
	}
	for _, idx := range top.continues {
		c.instrs[idx].Arg = endPc
	}
}

// compileForWhile 编译 判断循环>条件 ... <循环 块为 while 循环：
// OpWhile 先压入循环帧，随后 OpJumpIfFalse 每轮判断条件（条件为假跳到 OpLoopPop 退出）；
// 循环体末尾 OpJump 无条件跳回条件判断；>终止循环/>跳过 复用 blockFor 的 break/continue 回填。
func (c *compiler) compileForWhile(b *ast.Block) {
	cond := strings.TrimPrefix(b.Open, "判断循环>")

	c.emit(Instr{Op: OpWhile})
	condPc := len(c.instrs)
	jumpIfFalse := c.emit(Instr{Op: OpJumpIfFalse, Text: cond})
	bodyStart := len(c.instrs)

	c.blocks = append(c.blocks, blockCtx{kind: blockFor, bodyStart: bodyStart, frameIdx: c.frameDepth})
	c.frameDepth++

	c.compileNodes(b.Children)

	top := c.blocks[len(c.blocks)-1] // 弹出前取当前循环 ctx（含编译期间回填的 break/continue 索引）
	c.blocks = c.blocks[:len(c.blocks)-1]
	c.frameDepth--

	// 循环体结束：无条件跳回条件判断（continue 目标同为 condPc）。
	c.emit(Instr{Op: OpJump, Arg: condPc})

	popPc := c.emit(Instr{Op: OpLoopPop})
	c.instrs[jumpIfFalse].Arg = popPc

	for _, idx := range top.breaks {
		c.instrs[idx].Arg = popPc
		c.instrs[idx].End = top.frameIdx + 1
	}
	for _, idx := range top.continues {
		c.instrs[idx].Arg = condPc
	}
}
