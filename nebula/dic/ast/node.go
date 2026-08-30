package ast

// 本包定义星云词库的 AST（抽象语法树）节点类型。
// 阶段 A 仅建模「结构层」：词条 → 框块 → 语句叶子，为后续字节码编译打基础。
// 语句叶子暂以原始文本（Stmt）表示，细粒度的表达式/函数调用拆解在后续阶段补充。

// Pos 源码位置，Line 为 1-based 行号（0 表示未知）。
type Pos struct {
	Line int
}

// Kind 节点类别。
type Kind uint8

const (
	KindEntry Kind = iota
	KindBlock
	KindStmt
)

// Node 是所有 AST 节点的公共接口。
type Node interface {
	Pos() Pos
	Kind() Kind
}

// BlockKind 框块类型，与运行时状态机（State*）及 run 包编译检查的 blockKind 一一对应。
type BlockKind uint8

const (
	BlockFunc BlockKind = iota // 函数> ... <函数
	BlockIf                     // 如果> ... <如果
	BlockMatch                  // 匹配> ... <匹配
	BlockFor                    // 循环> ... <循环
	BlockForEach                // 遍历> ... <遍历
	BlockText                   // 文本>/纯文本> ... <文本
	BlockJson                   // JSON> ... <JSON
	BlockNewJson                // JSON>{ / JSON>[ ... 平衡括号
	BlockVarNewJson             // 变量:{ / 变量:[ ... 平衡括号（赋值到变量）
	BlockValChain               // 变量:>>> ... <<<（连续执行框）
	BlockValText                // 变量:""" ... """（赋值文本框，内容 %变量% 插值）
	BlockValTextr               // 变量:''' ... '''（赋值文本框，内容原样）
	BlockNodeJs                 // --js ... --end（JS 代码框）
)

func (k BlockKind) String() string {
	switch k {
	case BlockFunc:
		return "函数"
	case BlockIf:
		return "如果"
	case BlockMatch:
		return "匹配"
	case BlockFor:
		return "循环"
	case BlockForEach:
		return "遍历"
	case BlockText:
		return "文本"
	case BlockJson:
		return "JSON"
	case BlockNewJson:
		return "新建JSON"
	case BlockVarNewJson:
		return "赋值新建JSON"
	case BlockValChain:
		return "连续执行"
	case BlockValText:
		return "赋值文本"
	case BlockValTextr:
		return "赋值原文本"
	case BlockNodeJs:
		return "JS代码"
	}
	return "未知框"
}

// Entry 一条词条：触发词 + 正文语句列表。
type Entry struct {
	Trigger     string
	TriggerLine int
	Body        []Node
}

func (e *Entry) Pos() Pos { return Pos{Line: e.TriggerLine} }
func (e *Entry) Kind() Kind { return KindEntry }

// Block 一个框块（函数/判断/循环/遍历/文本/JSON），OpenKind 区分具体类型。
// CloseLine 为 0 表示框未闭合。
type Block struct {
	Open      string
	OpenKind  BlockKind
	OpenLine  int
	Children  []Node
	CloseLine int

	// Raw 该框的原始行（开启行到关闭行，含首尾；未闭合时到末尾）。
	// 供字节码编译器把尚未下沉为跳转指令的复杂框（文本/JSON/函数框/遍历/JS 等）
	// 整段委托给原解释器执行，保证语义等价。
	Raw []string
	// RawLineNums 与 Raw 平行，记录每行对应的原始文件行号（1-based，缺失回退正文下标）。
	// 供 函数> 框存储 FuncBox.LineNums，保持调用函数框时的报错定位与解释器一致。
	RawLineNums []int

	depth    int // 仅新建 JSON 框使用：括号平衡深度
	rawStart int // 解析期：框开启行在 body 中的下标
}

func (b *Block) Pos() Pos { return Pos{Line: b.OpenLine} }
func (b *Block) Kind() Kind { return KindBlock }

// Stmt 语句叶子，暂存原始行文本与行号。
type Stmt struct {
	Line int
	Text string
}

func (s *Stmt) Pos() Pos { return Pos{Line: s.Line} }
func (s *Stmt) Kind() Kind { return KindStmt }
