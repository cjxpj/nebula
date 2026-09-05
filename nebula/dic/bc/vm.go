package bc

// Runtime 是字节码 VM 与具体语言运行时之间的接口。
// 叶子语句（$函数$ 调用、%变量% 插值、赋值、算术等）的执行语义由实现方提供，
// VM 只负责控制流（跳转、循环、分支）与变量槽（循环变量）的读写。
type Runtime interface {
	// Line 执行一条叶子语句，返回应追加到最终输出的文本（可为空）。
	// line 为该语句的真实源文件行号（1-based，0 表示未知），用于运行时报错定位。
	Line(line int, text string) string
	// Append 将文本追加到共享输出缓冲。VM 不自行持有输出缓冲，而是写入运行时共享缓冲，
	// 使运行时在函数报错等场景清空共享缓冲时能一并清空已累积的输出。
	Append(s string)
	// Output 返回共享输出缓冲的当前内容。
	Output() string
	// Cond 求值条件表达式真假。
	Cond(expr string) bool
	// Resolve 求值表达式（%变量%/$函数$/[算术] 等）为字符串，供 匹配> 框一次性求值主体与 case 值。
	Resolve(expr string) string
	// JumpRelOffset 求值 跳行 偏移表达式（可为 %变量%/字面量），返回解析后的整数与是否解析成功。
	// 语义与解释器 >跳行 一致：偏移经 %变量% 插值后按整数解析，解析失败视为未命中（不跳转）。
	JumpRelOffset(expr string) (int, bool)
	// SetVarInt 写入整数循环变量（直接写 int64，避免字符串往返装箱）。
	SetVarInt(name string, value int)
	// GetVar 读取循环变量当前值。
	GetVar(name string) string
	// LoopCount 求值动态循环次数表达式（%变量%/$函数$ 等），返回循环次数。
	// 语义与解释器 opFor 一致：表达式求值后按整数解析，解析失败回退为 1。
	LoopCount(expr string) int
	// LoopRange 求值范围循环的 "起始表达式~结束表达式"（各经 %变量%/$函数$ 求值后按整数解析，
	// 解析失败回退 1），返回起始值与结束值。与 LoopCount 不同，边界值保留负数语义。
	LoopRange(expr string) (start, end int)
	// Stop 是否收到全局终止指令。
	Stop() bool
	// Halt 发出全局终止指令（>终止 / >终止 文案），使外层上下文同样停止。
	Halt()
	// LoopVarChanged 循环变量步进检查：给定循环变量名与上一轮序号，返回
	// 变量是否被脚本改写、改写后的新值，以及是否因改写为非法值而需终止循环。
	// 语义与解释器 loopVarChanged 一致（整数直读 / 字符串兼容）。
	LoopVarChanged(name string, cur int) (newVal int, changed bool, breakLoop bool)
	// TextBlock 执行 文本>/纯文本> 框：text 为开启行，lines 为内容行，
	// pure 为 true 表示纯文本（内容原样输出），false 表示文本（内容做 %变量% 插值）。
	// 返回应追加到最终输出的文本（赋值到变量时返回空）。
	TextBlock(text string, lines []string, pure bool) string
	// JsonBlock 执行 JSON> 框：text 为开启行（含初始 JSON），lines 为内容行（key=value 设置键值）。
	// 返回应追加到最终输出的文本（赋值到变量时返回空）。
	JsonBlock(text string, lines []string) string
	// NewJsonBlock 执行 JSON>{ / JSON>[ 框：text 为开启行（"JSON>{" 或 "JSON>["），
	// lines 为内容行（原样 JSON 文本累积）。返回应追加到最终输出的文本。
	NewJsonBlock(text string, lines []string) string
	// FuncBlock 执行 变量:函数> 框：text 为开启行（变量:函数> 或 变量:函数>默认参数），
	// lines 为内容行，lineNums 为内容行对应的原始文件行号。
	// 将内容存入 FuncBox 赋给变量（返回空）。
	FuncBlock(text string, lines []string, lineNums []int) string
	// ExecFuncBlock 执行 变量:执行函数> 框：text 为开启行（变量:执行函数> 或 变量:执行函数>默认参数），
	// lines 为内容行，lineNums 为内容行对应的原始文件行号。
	// 立即以新局部作用域执行内容，并把返回内容写入变量（返回空）。
	ExecFuncBlock(text string, lines []string, lineNums []int) string
	// ForEachInit 求值 遍历> 框的遍历源并准备迭代：text 为开启行（遍历>k,v=表达式 或 遍历>k）。
	// depth 为该遍历帧在 VM 帧栈中的下标（用于嵌套遍历时隔离各层迭代状态）。
	// 运行时内部按 depth 记录键值变量名与物化的迭代项；返回迭代项数量（0 表示无迭代）。
	ForEachInit(depth int, text string) int
	// ForEachNext 设置第 idx 项（0-based）的键值变量，返回是否在范围内（越界返回 false）。
	// depth 为该遍历帧在 VM 帧栈中的下标。
	ForEachNext(depth int, idx int) bool
	// VarNewJsonBlock 执行 变量:{ / 变量:[ 框：text 为开启行（含变量名与 { 或 [），
	// lines 为内容行（原样 JSON 文本累积）。闭合后将 NewJson 结果赋值给变量，返回空。
	VarNewJsonBlock(text string, lines []string) string
	// ValChainBlock 执行 变量:>>> 框：text 为开启行（含变量名），lines 为内容行。
	// 逐行执行内容并写回变量（空行跳过，支持 ?: 回退与 @json 路径），返回空。
	ValChainBlock(text string, lines []string) string
	// ValTextBlock 执行 变量:""" / 变量:''' 框：text 为开启行（含变量名），lines 为内容行，
	// raw 为 true 表示内容原样（'''），false 表示内容做 %变量% 插值（"""）。
	// 内容行以 \n 连接后赋值给变量，返回空。
	ValTextBlock(text string, lines []string, raw bool) string
	// NodeJsBlock 执行 --js ... --end 框：lines 为 JS 内容行，line 为关闭行行号（报错定位）。
	// 运行 JS 并返回结果字符串（undefined 返回空）。
	NodeJsBlock(lines []string, line int) string
	// Assign 执行一条无副作用赋值/算术叶子（已由编译期识别为结构化信息），
	// 返回应追加到最终输出的文本（赋值语句通常返回空）。text 为原始行文本，供防御性回退。
	// line 为该语句的真实源文件行号，供防御性回退到 Line 时保持报错定位。
	Assign(line int, text string, vType int8, prefix, suffix string) string
}

// Run 执行字节码，返回累积输出。
func Run(instrs []Instr, rt Runtime) string {
	// debugLog.Info("Run", utils.AnyToString(instrs))
	// defer debugLog.Info("Run end")
	type frame struct {
		varName string // 循环变量名（遍历帧为空）
		count   int    // 循环结束值/次数；遍历帧为迭代项数；while 为 -1
		i       int    // 当前值/游标
	}
	var frames []frame
	var switchVals []string

	for pc := 0; pc < len(instrs); pc++ {
		in := instrs[pc]
		switch in.Op {
		case OpNop:
		case OpLine:
			rt.Append(rt.Line(in.Line, in.Text))
			if rt.Stop() {
				return rt.Output()
			}
		case OpAssign:
			rt.Append(rt.Assign(in.Line, in.Text, in.VType, in.Prefix, in.Suffix))
			if rt.Stop() {
				return rt.Output()
			}
		case OpJump:
			pc = in.Arg - 1 // 循环末尾 pc++ 抵消
		case OpJumpIfFalse:
			if !rt.Cond(in.Text) {
				pc = in.Arg - 1
			}
		case OpJumpRel:
			// 相对跳转：条件命中时求值偏移并相对当前 PC 跳转。
			// 语义与解释器 >跳行 的 index = index + seti 完全一致：
			//   - 偏移 >= 0：跳过 offset 行（pc += offset，循环末尾 pc++ 抵消）；
			//   - 偏移 < 0：回退 -offset 行（解释器 seti<0 时先 seti-=1，等价于 pc += offset-1）。
			if rt.Cond(in.Text) {
				if offset, ok := rt.JumpRelOffset(in.Expr); ok {
					if offset >= 0 {
						pc += offset
					} else {
						pc += offset - 1
					}
				}
			}
		case OpLoop:
			frames = append(frames, frame{varName: in.Text, count: in.Arg, i: 1})
			if in.Arg == 0 {
				// 0 次：跳过循环体（含 OpLoopEnd），直接落到 OpLoopPop 弹出帧。
				pc = in.End - 1
			} else {
				rt.SetVarInt(in.Text, 1)
			}
		case OpLoopDyn:
			count := rt.LoopCount(in.Expr)
			frames = append(frames, frame{varName: in.Text, count: count, i: 1})
			if count < 1 {
				// 次数 ≤0：跳过循环体（含 OpLoopEnd），直接落到 OpLoopPop 弹出帧。
				pc = in.End - 1
			} else {
				rt.SetVarInt(in.Text, 1)
			}
		case OpLoopRange:
			start, end := in.Start, in.Arg
			if in.Expr != "" {
				start, end = rt.LoopRange(in.Expr)
			}
			frames = append(frames, frame{varName: in.Text, count: end, i: start})
			if end < start {
				// 空范围（起始值 > 结束值）：跳过循环体（含 OpLoopEnd），直接落到 OpLoopPop 弹出帧。
				pc = in.End - 1
			} else {
				rt.SetVarInt(in.Text, start)
			}
		case OpWhile:
			// while 循环：仅压入一个无限循环帧，实际是否进入循环体由紧随其后的 OpJumpIfFalse 判断。
			frames = append(frames, frame{varName: "", count: -1})
		case OpLoopEnd:
			f := &frames[len(frames)-1]
			if newVal, changed, breakLoop := rt.LoopVarChanged(f.varName, f.i); breakLoop {
				// 变量被改写为无法解析的值：结束循环，落到 OpLoopPop
			} else if changed {
				f.i = newVal
			}
			f.i++
			if f.count < 0 || f.i <= f.count {
				rt.SetVarInt(f.varName, f.i)
				pc = in.Arg - 1
			}
			// 正常退出：落到下一条 OpLoopPop
		case OpLoopPop:
			frames = frames[:len(frames)-1]
		case OpBreak:
			// 跳出循环/遍历（可能跨多层）：先把帧栈截断到目标帧深度（保留目标循环帧，
			// 交由紧随其后的 OpLoopPop/OpForEachPop 弹出），再跳转到对应弹出指令。
			frames = frames[:in.End]
			pc = in.Arg - 1
		case OpHalt:
			rt.Halt()
			return rt.Output()
		case OpHaltOut:
			rt.Append(in.Text)
			rt.Halt()
			return rt.Output()
		case OpTextBlock:
			rt.Append(rt.TextBlock(in.Text, in.Lines, in.Arg == 1))
			if rt.Stop() {
				return rt.Output()
			}
		case OpJsonBlock:
			rt.Append(rt.JsonBlock(in.Text, in.Lines))
			if rt.Stop() {
				return rt.Output()
			}
		case OpNewJsonBlock:
			rt.Append(rt.NewJsonBlock(in.Text, in.Lines))
			if rt.Stop() {
				return rt.Output()
			}
		case OpFuncBlock:
			rt.Append(rt.FuncBlock(in.Text, in.Lines, in.LineNums))
			if rt.Stop() {
				return rt.Output()
			}
		case OpExecFuncBlock:
			rt.Append(rt.ExecFuncBlock(in.Text, in.Lines, in.LineNums))
			if rt.Stop() {
				return rt.Output()
			}
		case OpForEachInit:
			depth := len(frames)
			count := rt.ForEachInit(depth, in.Text)
			frames = append(frames, frame{count: count, i: 0})
			if count < 1 {
				// 0 项：跳过循环体（含 OpForEachEnd），直接落到 OpForEachPop 弹出帧。
				pc = in.End - 1
			} else {
				rt.ForEachNext(depth, 0)
			}
		case OpForEachEnd:
			f := &frames[len(frames)-1]
			f.i++
			if f.i < f.count {
				rt.ForEachNext(len(frames)-1, f.i)
				pc = in.Arg - 1 // 跳回循环体起始
			}
			// 正常退出：落到下一条 OpForEachPop
		case OpForEachPop:
			frames = frames[:len(frames)-1]
		case OpVarNewJsonBlock:
			rt.Append(rt.VarNewJsonBlock(in.Text, in.Lines))
			if rt.Stop() {
				return rt.Output()
			}
		case OpValChainBlock:
			rt.Append(rt.ValChainBlock(in.Text, in.Lines))
			if rt.Stop() {
				return rt.Output()
			}
		case OpValTextBlock:
			rt.Append(rt.ValTextBlock(in.Text, in.Lines, in.Arg == 1))
			if rt.Stop() {
				return rt.Output()
			}
		case OpNodeJsBlock:
			rt.Append(rt.NodeJsBlock(in.Lines, in.Line))
			if rt.Stop() {
				return rt.Output()
			}
		case OpSwitch:
			// 求值匹配主体一次并压栈，后续 case 只与栈顶值做相等比较。
			switchVals = append(switchVals, rt.Resolve(in.Text))
		case OpSwitchCase:
			// 与栈顶匹配值不相等则跳到下一分支（Arg 指向下一 case/默认正文/OpSwitchPop）。
			if switchVals[len(switchVals)-1] != rt.Resolve(in.Text) {
				pc = in.Arg - 1
			}
		case OpSwitchPop:
			switchVals = switchVals[:len(switchVals)-1]
		}
	}
	return rt.Output()
}
