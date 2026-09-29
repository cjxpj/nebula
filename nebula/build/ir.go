package build

import "strings"

// ============== 词库 JSON 中间表示（IR）→ .n 源码 ==============
//
// 积木编程与词库调试 AI 协作共用这一套：前端 / AI 产出结构化 JSON IR，
// 后端统一序列化为 .n 源码，保证格式（缩进、空行、块结构）正确，
// 避免各处各自拼接 .n 文本导致的格式错误。

const irIndent = "    " // 每层缩进 4 空格

// assignSuffix 赋值操作符 → 冒号后缀（赋值 名:值 / 追加 名+:值 / 相减 名-:值 / 原样 名::值）。
var irAssignSuffix = map[string]string{"set": ":", "add": "+:", "sub": "-:", "raw": "::"}

// IR 顶层结构：头部（可选）+ 词条列表。
type IR struct {
	Header  []Stmt  `json:"header"`
	Entries []Entry `json:"entries"`
}

// Entry 一条词条。
type Entry struct {
	Kind      string `json:"kind"`                // normal|func|inner|func_class|inner_class
	Name      string `json:"name"`                // 触发词 / 方法名
	ClassName string `json:"className,omitempty"` // 类方法时的类名
	Rule      string `json:"rule,omitempty"`      // 函数的参数数量规则（如 1、1|2、2..），仅函数类别使用
	Body      []Stmt `json:"body"`                // 正文语句
}

// Stmt 单条语句：用 t 字段区分类型，其余字段按类型按需使用。
// 字段命名与前端（积木编程 workspaceToIr）产出的 JSON 保持一致。
type Stmt struct {
	T      string     `json:"t"`
	V      string     `json:"v,omitempty"`      // output / assign / kv 的值
	Name   string     `json:"name,omitempty"`   // assign / json / textblock 的变量名
	Op     string     `json:"op,omitempty"`     // assign: set|add|sub|raw
	Text   string     `json:"text,omitempty"`   // raw / comment 文本
	Path   string     `json:"path,omitempty"`   // import 路径
	Cond   string     `json:"cond,omitempty"`   // if / loopWhile 条件
	Do     []Stmt     `json:"do,omitempty"`     // if 分支
	Elifs  []IfBranch `json:"elifs,omitempty"`  // if 的「否则如果」分支（按顺序）
	Else   []Stmt     `json:"else,omitempty"`   // if 否则
	Expr   string     `json:"expr,omitempty"`   // match 主体表达式
	Cases  []Case     `json:"cases,omitempty"`  // match 分支
	Def    []Stmt     `json:"def,omitempty"`    // match 默认分支
	Var    string     `json:"var,omitempty"`    // loopCount / loopRange 循环变量
	Count  string     `json:"count,omitempty"`  // loopCount 次数
	From   string     `json:"from,omitempty"`   // loopRange 起始
	To     string     `json:"to,omitempty"`     // loopRange 结束
	Mode   string     `json:"mode,omitempty"`   // foreach: kv|v；kv: =|:=
	Key    string     `json:"key,omitempty"`    // foreach / kv 的键
	Val    string     `json:"val,omitempty"`    // foreach 的值
	Target string     `json:"target,omitempty"` // foreach 目标
	Kind   string     `json:"kind,omitempty"`   // json: obj|arr
	Sep    string     `json:"sep,omitempty"`    // textblock 分隔符
	Body   []Stmt     `json:"body,omitempty"`   // 各种块的 body
}

// Case 匹配框的一个分支。
type Case struct {
	V    string `json:"v"`
	Body []Stmt `json:"body"`
}

// IfBranch 判断框的一个「否则如果:」分支。
type IfBranch struct {
	Cond string `json:"cond"`
	Body []Stmt `json:"body"`
}

// IRToNebula 把 IR 序列化为 .n 源码。
// 结构：头部（可选）→ 空行 → 词条列表（词条之间空行分隔）。
func IRToNebula(ir IR) string {
	var lines []string
	if len(ir.Header) > 0 {
		lines = append(lines, stmtsToLines(ir.Header, 0)...)
		lines = append(lines, "")
	} else if len(ir.Entries) > 0 {
		// 无头部时开头补空行：词库引擎把「文件开头到第一个空行」当头部，
		// 不补空行会把第一个词条吞进头部，导致运行结果错误。
		lines = append(lines, "")
	}
	for i, e := range ir.Entries {
		lines = append(lines, entryTrigger(e))
		lines = append(lines, stmtsToLines(e.Body, 1)...)
		if i < len(ir.Entries)-1 {
			lines = append(lines, "")
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// entryTrigger 词条触发词行（普通 / 函数 / 内部 / 类方法）。
// 函数类别带参数规则时补上 [类别|规则] 后缀（如 [函数|2]名、[函数:类|2]名）。
func entryTrigger(e Entry) string {
	rule := ""
	if e.Rule != "" {
		rule = "|" + e.Rule
	}
	switch e.Kind {
	case "func":
		return "[函数" + rule + "]" + e.Name
	case "inner":
		return "[内部]" + e.Name
	case "func_class":
		return "[函数:" + e.ClassName + rule + "]" + e.Name
	case "inner_class":
		return "[内部:" + e.ClassName + "]" + e.Name
	default:
		return e.Name
	}
}

// stmtsToLines 把语句数组序列化为行数组（indent 为缩进层数）。
func stmtsToLines(stmts []Stmt, indent int) []string {
	pad := strings.Repeat(irIndent, indent)
	var lines []string
	pushBlock := func(body []Stmt) {
		lines = append(lines, stmtsToLines(body, indent+1)...)
	}
	for _, s := range stmts {
		switch s.T {
		case "output":
			lines = append(lines, pad+s.V)
		case "assign":
			sfx := irAssignSuffix[s.Op]
			if sfx == "" {
				sfx = ":"
			}
			lines = append(lines, pad+s.Name+sfx+s.V)
		case "raw":
			if s.Text == "" {
				lines = append(lines, "")
			} else {
				lines = append(lines, pad+s.Text)
			}
		case "comment":
			lines = append(lines, pad+"// "+s.Text)
		case "import":
			// 赋予值形式：变量:$引入 路径$（把整个包打包成实例赋给变量）
			if s.Name != "" {
				lines = append(lines, pad+s.Name+":$引入 "+s.Path+"$")
			} else {
				lines = append(lines, pad+"$引入 "+s.Path+"$")
			}
		case "importGlobal":
			lines = append(lines, pad+"#引入="+s.Path)
		case "if":
			lines = append(lines, pad+"如果>"+s.Cond)
			pushBlock(s.Do)
			for _, e := range s.Elifs {
				lines = append(lines, pad+">否则如果:"+e.Cond)
				pushBlock(e.Body)
			}
			if len(s.Else) > 0 {
				lines = append(lines, pad+">否则")
				pushBlock(s.Else)
			}
			lines = append(lines, pad+"<如果")
		case "match":
			lines = append(lines, pad+"匹配>"+s.Expr)
			for _, c := range s.Cases {
				lines = append(lines, pad+"如果是:"+c.V)
				lines = append(lines, stmtsToLines(c.Body, indent+1)...)
			}
			if len(s.Def) > 0 {
				lines = append(lines, pad+"如果不是")
				lines = append(lines, stmtsToLines(s.Def, indent+1)...)
			}
			lines = append(lines, pad+"<匹配")
		case "loopCount":
			lines = append(lines, pad+"循环>"+s.Var+"="+s.Count)
			pushBlock(s.Body)
			lines = append(lines, pad+"<循环")
		case "loopRange":
			lines = append(lines, pad+"循环>"+s.Var+"="+s.From+"~"+s.To)
			pushBlock(s.Body)
			lines = append(lines, pad+"<循环")
		case "loopWhile":
			lines = append(lines, pad+"判断循环>"+s.Cond)
			pushBlock(s.Body)
			lines = append(lines, pad+"<循环")
		case "foreach":
			head := s.Val
			if s.Mode == "kv" {
				head = s.Key + "," + s.Val
			}
			lines = append(lines, pad+"遍历>"+head+"="+s.Target)
			pushBlock(s.Body)
			lines = append(lines, pad+"<遍历")
		case "break":
			lines = append(lines, pad+">中断")
		case "continue":
			lines = append(lines, pad+">跳过")
		case "stop":
			lines = append(lines, pad+">终止")
		case "stopLoop":
			lines = append(lines, pad+">终止循环")
		case "stopForeach":
			lines = append(lines, pad+">终止遍历")
		case "json":
			kind := "{}"
			if s.Kind == "arr" {
				kind = "[]"
			}
			lines = append(lines, pad+"JSON>"+s.Name+"="+kind)
			pushBlock(s.Body)
			lines = append(lines, pad+"<JSON")
		case "kv":
			lines = append(lines, pad+s.Key+s.Mode+s.V)
		case "textblock":
			lines = append(lines, pad+s.Name+":文本>"+s.Sep)
			pushBlock(s.Body)
			lines = append(lines, pad+"<文本")
		}
	}
	return lines
}
