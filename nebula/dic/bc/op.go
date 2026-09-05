package bc

// 字节码指令集。栈式控制流 VM，覆盖：叶子语句执行、条件分支、固定/无限次数循环、终止。
// 叶子语句（函数调用 $...$、%变量% 插值、赋值、算术等）的执行语义由 Runtime 注入，
// 编译器只负责把「结构」（框、分支、循环）编译成跳转指令。

// Op 操作码。
type Op uint8

const (
	OpNop         Op = iota // 空操作
	OpLine                  // 执行叶子语句，Text 为行文本，返回输出追加到结果
	OpJump                  // 无条件跳转，Arg 为目标 PC
	OpJumpIfFalse           // 条件为假跳转，Text 为条件表达式，Arg 为目标 PC
	OpJumpRel               // 相对跳转（>跳行(条件)>>偏移）：Text 为条件表达式，Expr 为偏移表达式（可为 %变量%）
	OpLoop                  // 循环入口，Text 为循环变量名，Arg 为次数（<0 表示无限）
	OpLoopDyn               // 循环入口（次数为运行时表达式），Text 为循环变量名，Expr 为次数表达式
	OpLoopRange             // 范围循环入口（循环>变量=起始~结束）：Text 为循环变量名，Start/Arg 为字面量起止值（Expr 非空时为动态 "起~止" 表达式）
	OpWhile                 // while 循环入口（判断循环>条件）：push 无限循环帧（无循环变量），随后由 OpJumpIfFalse 判断条件
	OpLoopEnd               // 循环体末尾：变量递增，未达次数则跳回 Arg（循环体起始 PC）
	OpLoopPop               // 弹出循环帧（正常退出与 break 的汇聚点）
	OpBreak                 // 跳出循环/遍历（可能跨多层）：End 为截断后的帧深度，Arg 为目标 PC（对应 OpLoopPop/OpForEachPop）
	OpHalt                  // 终止执行（无额外输出）
	OpHaltOut               // 终止执行并输出 Text
	OpAssign                // 执行无副作用赋值/算术叶子（VType/Prefix/Suffix 携带结构化信息）
	OpTextBlock             // 执行 文本>/纯文本> 框：Text 为开启行，Lines 为内容行，Arg 0=文本(变量插值) 1=纯文本(原样)
	OpJsonBlock             // 执行 JSON> 框：Text 为开启行，Lines 为内容行（key=value 设置键值）
	OpNewJsonBlock          // 执行 JSON>{ / JSON>[ 框：Text 为开启行，Lines 为内容行（原样 JSON 文本累积）
	OpFuncBlock             // 执行 变量:函数> 框：Text 为开启行，Lines 为内容行，LineNums 为内容行行号
	OpExecFuncBlock         // 执行 变量:执行函数> 框：立即执行内容并把返回内容写入变量，Text 为开启行
	OpForEachInit           // 遍历> 框入口：Text 为开启行（遍历>k,v=表达式），求值遍历源并入帧，End 为空遍历跳转目标（OpForEachPop）
	OpForEachEnd            // 遍历> 框末尾：递增游标，未到末尾则跳回 Arg（循环体起始 PC）
	OpForEachPop            // 弹出遍历帧（正常退出与 break 的汇聚点）
	OpVarNewJsonBlock       // 执行 变量:{ / 变量:[ 框：Text 为开启行（含变量名），Lines 为内容行（原样 JSON 文本累积）
	OpValChainBlock         // 执行 变量:>>> 框：Text 为开启行（含变量名），Lines 为内容行（逐行 runValSet 写回）
	OpValTextBlock          // 执行 变量:""" / 变量:''' 框：Text 为开启行（含变量名），Lines 为内容行，Arg 0=插值 1=原样
	OpNodeJsBlock           // 执行 --js ... --end 框：Lines 为 JS 内容行，Line 为关闭行行号（报错定位）
	OpSwitch                // 匹配> 框入口：Text 为匹配主体表达式，求值一次后压入匹配值栈
	OpSwitchCase            // 匹配分支：Text 为 case 值表达式，与栈顶匹配值不相等则跳转 Arg（下一 case/默认正文/OpSwitchPop）
	OpSwitchPop             // 匹配框末尾：弹出匹配值栈顶
)

// Instr 一条字节码指令。
type Instr struct {
	Op    Op
	Text  string   // OpLine/OpJumpIfFalse/OpJumpRel(条件)/OpLoop/OpLoopDyn/OpLoopRange(变量名)/OpHaltOut/OpAssign(原始行)/OpForEachInit(遍历开启行) 使用
	Arg   int      // OpJump/OpJumpIfFalse/OpLoopEnd/OpForEachEnd 使用（目标 PC 或循环次数）；OpLoopRange 使用：结束值（Expr 为空时）
	Start int      // OpLoopRange 使用：起始值（Expr 为空时）
	Expr  string   // OpLoopDyn 使用：循环次数运行时表达式（%变量%/$函数$ 等）；OpLoopRange 使用：动态 "起~止" 表达式；OpJumpRel 使用：偏移表达式（可为 %变量%）
	End   int      // OpLoop/OpLoopDyn/OpLoopRange/OpForEachInit 使用：循环退出目标 PC（OpLoopPop/OpForEachPop），用于 0 次循环跳过循环体；OpBreak 使用：截断后的帧深度
	Line  int      // OpLine/OpAssign 使用：语句真实源文件行号（1-based，0 表示未知），用于运行时报错定位；OpNodeJsBlock 使用：关闭行行号
	Lines []string // 各类框指令使用：内容行（OpTextBlock/OpJsonBlock/OpNewJsonBlock/OpFuncBlock/OpExecFuncBlock/OpVarNewJsonBlock/OpValChainBlock/OpValTextBlock/OpNodeJsBlock）
	// OpFuncBlock/OpExecFuncBlock 使用：内容行对应的原始文件行号（与 Lines 平行，1-based）。
	LineNums []int
	// OpAssign 使用：结构化赋值信息（Text 仍保留原始行文本，供运行时防御性回退）。
	VType  int8
	Prefix string
	Suffix string
}
