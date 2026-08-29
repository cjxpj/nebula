package cond

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	jsoniter "github.com/json-iterator/go"
)

// json 与 dic 包保持一致的 JSON 配置（禁用 HTML 转义），保证 in 判断序列化语义一致。
var json = jsoniter.Config{
	EscapeHTML: false,
}.Froze()

// ifExpr 一条解析后的判断项：text 非空表示括号/逻辑符号，否则为操作数或比较。
type ifExpr struct {
	a, b, c, text string
}

// Eval 解析并求值条件表达式。
// resolve 用于把操作数解析为字符串值（%变量%/[算术]/$函数$ 的解析由调用方完成）。
// 返回条件真假；解析异常时返回 error。
func Eval(condition string, resolve func(operand string) string) (bool, error) {
	parsed, err := parse(condition)
	if err != nil {
		return false, err
	}
	var b strings.Builder
	for _, p := range parsed {
		if p.text != "" {
			b.WriteString(p.text)
			continue
		}
		a := resolve(p.a)
		if p.b == "" {
			switch a {
			case "true", "1":
				b.WriteByte('1')
			case "false", "0":
				b.WriteByte('0')
			}
			continue
		}
		c := resolve(p.c)
		if evalCmp(a, c, p.b) {
			b.WriteByte('1')
		} else {
			b.WriteByte('0')
		}
	}
	return evalExpression(b.String()), nil
}

// spaceWrappedLen 检测位置 i 是否为「空格 + 词 + 空格」，命中返回总字节长（含两侧空格），未命中返回 0。
// words 按长词优先传入，避免「大于等于」被「大于」、「或者」被「或」抢先匹配。
func spaceWrappedLen(s string, i int, words ...string) int {
	if i >= len(s) || s[i] != ' ' {
		return 0
	}
	rest := s[i+1:]
	for _, w := range words {
		if strings.HasPrefix(rest, w) {
			l := len(w)
			if i+1+l < len(s) && s[i+1+l] == ' ' {
				return 1 + l + 1
			}
		}
	}
	return 0
}

// cnLogicWordLen 检测位置 i 是否为「空格包裹的中文逻辑连接词」（或/或者 → |，且/并且 → &）。
func cnLogicWordLen(s string, i int) int {
	return spaceWrappedLen(s, i, "或者", "并且", "或", "且")
}

// cnCmpWordLen 检测位置 i 是否为「空格包裹的中文比较运算符」
// （等于→==、不等于→!=、大于→>、小于→<、大于等于→>=、小于等于→<=）。
func cnCmpWordLen(s string, i int) int {
	return spaceWrappedLen(s, i, "大于等于", "小于等于", "不等于", "等于", "大于", "小于")
}

// cnCmpOp 中文比较运算符 → 符号。
func cnCmpOp(w string) string {
	switch w {
	case "等于":
		return "=="
	case "不等于":
		return "!="
	case "大于":
		return ">"
	case "小于":
		return "<"
	case "大于等于":
		return ">="
	case "小于等于":
		return "<="
	}
	return ""
}

func parse(input string) ([]ifExpr, error) {
	type tokenType uint8

	const (
		tokOperand tokenType = iota // 操作数
		tokOp                       // 运算符
		tokJump                     // 括号 / 逻辑
	)

	type token struct {
		typ   tokenType
		value string
	}

	// 去掉整体首尾空白，避免 `如果> %x%==1 或 %x%==2` 这类条件
	// 首尾空格被当作操作数的一部分，导致比较结果错误。内部空格（如中文连接词两侧）不受影响。
	input = strings.TrimSpace(input)

	var tokens []token
	inputLen := len(input)

	i := 0
	lastWasOperand := false
	for i < inputLen {
		switch {
		case input[i] == '(' || input[i] == ')' || input[i] == '&' || input[i] == '|':
			tokens = append(tokens, token{typ: tokJump, value: string(input[i])})
			lastWasOperand = input[i] == ')'
			i++
		case cnLogicWordLen(input, i) > 0:
			wlen := cnLogicWordLen(input, i)
			w := input[i+1 : i+wlen-1]
			if w == "或" || w == "或者" {
				tokens = append(tokens, token{typ: tokJump, value: "|"})
			} else {
				tokens = append(tokens, token{typ: tokJump, value: "&"})
			}
			lastWasOperand = false
			i += wlen
		case cnCmpWordLen(input, i) > 0:
			wlen := cnCmpWordLen(input, i)
			w := input[i+1 : i+wlen-1]
			tokens = append(tokens, token{typ: tokOp, value: cnCmpOp(w)})
			lastWasOperand = false
			i += wlen
		case input[i] == '=' || input[i] == '!' || input[i] == '>' || input[i] == '<' || input[i] == '~':
			if i+1 < inputLen && input[i+1] == '=' {
				tokens = append(tokens, token{typ: tokOp, value: input[i : i+2]})
				i += 2
			} else if input[i] == '!' && !lastWasOperand {
				tokens = append(tokens, token{typ: tokJump, value: "!"})
				i++
			} else {
				tokens = append(tokens, token{typ: tokOp, value: string(input[i])})
				i++
			}
			lastWasOperand = false
		case input[i] == ' ' && i+3 < inputLen && input[i+1] == 'i' && input[i+2] == 'n' && input[i+3] == ' ':
			tokens = append(tokens, token{typ: tokOp, value: input[i : i+4]})
			i += 4
			lastWasOperand = false
		default:
			start := i
			for i < inputLen &&
				input[i] != '(' &&
				input[i] != ')' &&
				input[i] != '&' &&
				input[i] != '|' &&
				input[i] != '=' &&
				input[i] != '!' &&
				input[i] != '>' &&
				input[i] != '<' &&
				input[i] != '~' &&
				!(input[i] == ' ' && i+3 < inputLen && input[i+1] == 'i' && input[i+2] == 'n' && input[i+3] == ' ') &&
				cnLogicWordLen(input, i) == 0 &&
				cnCmpWordLen(input, i) == 0 {
				i++
			}
			tokens = append(tokens, token{typ: tokOperand, value: input[start:i]})
			lastWasOperand = true
		}
	}

	out := make([]ifExpr, 0, len(tokens))
	dieNum := 0
	for i = 0; i < len(tokens); {
		switch tokens[i].typ {
		case tokOperand:
			e := ifExpr{a: tokens[i].value}
			i++
			if i < len(tokens) && tokens[i].typ == tokOp {
				e.b = tokens[i].value
				i++
				if i < len(tokens) && tokens[i].typ == tokOperand {
					e.c = tokens[i].value
					i++
				}
			}
			out = append(out, e)
		case tokJump:
			out = append(out, ifExpr{text: tokens[i].value})
			i++
		default:
			i++
		}
		dieNum++
		if dieNum > 5000 {
			return nil, fmt.Errorf("条件表达式解析异常: %s", input)
		}
	}

	return out, nil
}

// evalExpression 评估一个表达式的真假值，支持 ! 取反、& 与、| 或、() 分组。
func evalExpression(expression string) bool {
	n := len(expression)
	idx := 0

	var parseOr func() bool
	var parseAnd func() bool
	var parseNot func() bool
	var parsePrimary func() bool

	parseOr = func() bool {
		left := parseAnd()
		for idx < n && expression[idx] == '|' {
			idx++
			right := parseAnd()
			left = left || right
		}
		return left
	}
	parseAnd = func() bool {
		left := parseNot()
		for idx < n && expression[idx] == '&' {
			idx++
			right := parseNot()
			left = left && right
		}
		return left
	}
	parseNot = func() bool {
		if idx < n && expression[idx] == '!' {
			idx++
			return !parseNot()
		}
		return parsePrimary()
	}
	parsePrimary = func() bool {
		if idx >= n {
			return false
		}
		switch expression[idx] {
		case '1':
			idx++
			return true
		case '0':
			idx++
			return false
		case '(':
			idx++
			v := parseOr()
			if idx < n && expression[idx] == ')' {
				idx++
			}
			return v
		}
		return false
	}

	return parseOr()
}

// cmpNumOrStr 优先按数字比较，无法解析时回退为字符串比较。
func cmpNumOrStr(a, c, op string) bool {
	if A, errA := strconv.ParseFloat(a, 64); errA == nil {
		if C, errC := strconv.ParseFloat(c, 64); errC == nil {
			return cmpOrdered(A, C, op)
		}
	}
	return cmpOrdered(a, c, op)
}

// cmpOrdered 按运算符比较两个可排序值（数字或字符串）。
func cmpOrdered[T ~float64 | ~string](a, c T, op string) bool {
	switch op {
	case ">":
		return a > c
	case "<":
		return a < c
	case ">=":
		return a >= c
	case "<=":
		return a <= c
	}
	return false
}

// evalCmp 计算二元比较结果。
func evalCmp(a, c, op string) bool {
	switch op {
	case " in ":
		return containsJSON(a, c)
	case "~=":
		m, _ := regexp.MatchString("^"+regexp.QuoteMeta(a)+"$", c)
		return m
	case "==":
		return a == c
	case "!=":
		return a != c
	case ">=", "<=", "<", ">":
		return cmpNumOrStr(a, c, op)
	case "!":
		return len(a) == len(c)
	}
	return false
}

// containsJSON 判断 a、c 是否构成数组包含关系（in 判断）。
func containsJSON(a, c string) bool {
	var arr []any
	if json.Unmarshal([]byte(a), &arr) == nil {
		for _, v := range arr {
			if jsonElemEquals(v, c) {
				return true
			}
		}
		return false
	}
	if json.Unmarshal([]byte(c), &arr) == nil {
		for _, v := range arr {
			if jsonElemEquals(v, a) {
				return true
			}
		}
	}
	return false
}

// jsonElemEquals 判断 JSON 数组元素是否等于目标值。
func jsonElemEquals(v any, target string) bool {
	switch jv := v.(type) {
	case string:
		return jv == target
	case []any, map[string]any:
		if b, err := json.Marshal(jv); err == nil {
			return string(b) == target
		}
	default:
		return fmt.Sprintf("%v", v) == target
	}
	return false
}
