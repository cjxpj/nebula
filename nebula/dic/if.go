package dic

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/cjxpj/nebula/count"
	"github.com/cjxpj/nebula/debugLog"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/utils"
)

// ifExpr 一条解析后的判断项：text 非空表示括号/逻辑符号，否则为操作数或比较。
type ifExpr struct {
	a, b, c, text string
}

func Pd(dic *dic_dto.DicFunc, str string) bool {
	it := &IfText{}
	runstr := it.Run(str)
	if it.Error {
		dic.Output.Add("条件表达式解析异常: " + str)
		dic.Sys.Stop.Store(true)
		return false
	}
	pdstr := it.Evaluate(dic, runstr)
	sendstr := it.EvaluateExpression(pdstr)
	return sendstr
}

func (it *IfText) Run(input string) []ifExpr {
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
				!(input[i] == ' ' && i+3 < inputLen && input[i+1] == 'i' && input[i+2] == 'n' && input[i+3] == ' ') {
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
			debugLog.Infof("错误判断: %v", input)
			it.Error = true
			return nil
		}
	}

	return out
}

func (it *IfText) Evaluate(dic *dic_dto.DicFunc, parsed []ifExpr) string {
	var b strings.Builder
	for _, p := range parsed {
		if p.text != "" {
			b.WriteString(p.text)
			continue
		}
		a := utils.AnyToString(Runs(dic, utils.AnyToString(count.RunCountText(dic.Val, p.a))))
		if p.b == "" {
			switch a {
			case "true", "1":
				b.WriteByte('1')
			case "false", "0":
				b.WriteByte('0')
			}
			continue
		}
		c := utils.AnyToString(Runs(dic, utils.AnyToString(count.RunCountText(dic.Val, p.c))))
		if evalCmp(a, c, p.b) {
			b.WriteByte('1')
		} else {
			b.WriteByte('0')
		}
	}
	return b.String()
}

// 评估一个表达式的真假值，支持 ! 取反、& 与、| 或、() 分组。
func (it *IfText) EvaluateExpression(expression string) bool {
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
	if A, err := strconv.ParseFloat(a, 64); err == nil {
		if C, err2 := strconv.ParseFloat(c, 64); err2 == nil {
			switch op {
			case ">":
				return A > C
			case "<":
				return A < C
			case ">=":
				return A >= C
			case "<=":
				return A <= C
			}
		}
	}
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
	case "~":
		m, _ := regexp.MatchString("^"+regexp.QuoteMeta(a)+"$", c)
		return !m
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
