package dic

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/cjxpj/nebula/count"
)

func Pd(dic *DicTools, path, str string) bool {

	it := &IfText{}
	runstr := it.Run(str)
	pdstr := it.Evaluate(dic, path, runstr)
	sendstr := it.EvaluateExpression(pdstr)
	return sendstr
}

func (it *IfText) Run(input string) []map[string]string {
	type Token struct {
		Type  string
		Value string
	}

	var tokens []Token
	var parsed []map[string]string
	inputLen := len(input)

	i := 0
	for i < inputLen {
		switch {
		case input[i] == '(' || input[i] == ')' || input[i] == '&' || input[i] == '|':
			tokens = append(tokens, Token{Type: "jump", Value: string(input[i])})
			i++
		case input[i] == '=' || input[i] == '!' || input[i] == '>' || input[i] == '<' || input[i] == '~':
			if i+1 < inputLen && input[i+1] == '=' {
				tokens = append(tokens, Token{Type: "b", Value: string(input[i : i+2])})
				i += 2
			} else {
				tokens = append(tokens, Token{Type: "b", Value: string(input[i])})
				i++
			}
		case input[i] == ' ' && i+3 < inputLen && input[i+1] == 'i' && input[i+2] == 'n' && input[i+3] == ' ':
			tokens = append(tokens, Token{Type: "b", Value: string(input[i : i+4])})
			i += 4
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
			tokens = append(tokens, Token{Type: "a", Value: input[start:i]})
		}
	}

	dieNum := 0
	i = 0
	tokensLen := len(tokens)
	for i < tokensLen {
		current := make(map[string]string)
		if tokens[i].Type == "a" {
			current["a"] = tokens[i].Value
			i++
			if i < tokensLen && tokens[i].Type == "b" {
				current["b"] = tokens[i].Value
				i++
				if len(tokens) > i {
					if tokens[i].Type == "a" {
						current["c"] = tokens[i].Value
						i++
						if i < len(tokens) && tokens[i].Type == "jump" {
							current["jump"] = tokens[i].Value
							i++
						}
					}
				} else {
					current["c"] = ""
					i++
				}
			}
		} else if tokens[i].Type == "jump" {
			current["text"] = tokens[i].Value
			i++
		}
		parsed = append(parsed, current)
		dieNum++
		if dieNum > 5000 {
			fmt.Println("错误判断", input)
			return []map[string]string{
				{
					"a": "",
					"b": "!=",
					"c": "",
				},
			}
		}
	}

	return parsed
}

func (it *IfText) Evaluate(dic *DicTools, path string, parsed []map[string]string) string {
	var result string
	yes := "1"
	no := "0"
	for _, p := range parsed {
		if p["text"] != "" {
			result += p["text"]
			continue
		}
		a := dic.Func.Runs(count.RunCountText(dic.LocalVariable, p["a"]))
		if len(p) == 1 {
			switch a {
			case "true", "1":
				result += yes
			case "false", "0":
				result += no
			}

			if p["jump"] != "" {
				result += p["jump"]
			}
			continue
		}

		c := dic.Func.Runs(count.RunCountText(dic.LocalVariable, p["c"]))

		switch p["b"] {
		case " in ":
			var jsonOk bool
			var jsonMap []interface{}
			if err := json.Unmarshal([]byte(a), &jsonMap); err == nil {
				for _, v := range jsonMap {
					switch jv := v.(type) {
					case string:
						if jv == c {
							jsonOk = true
							break
						}
					case []interface{}, map[string]interface{}:
						if jvv, err := json.Marshal(jv); err == nil {
							if string(jvv) == c {
								jsonOk = true
								break
							}
						}
					default:
						if fmt.Sprintf("%v", v) == c {
							jsonOk = true
							break
						}
					}
				}
			} else {
				if err := json.Unmarshal([]byte(c), &jsonMap); err == nil {
					for _, v := range jsonMap {
						switch jv := v.(type) {
						case string:
							if jv == a {
								jsonOk = true
								break
							}
						case []interface{}, map[string]interface{}:
							if jvv, err := json.Marshal(jv); err == nil {
								if string(jvv) == a {
									jsonOk = true
									break
								}
							}
						default:
							if fmt.Sprintf("%v", v) == a {
								jsonOk = true
								break
							}
						}
					}
				}
			}
			if jsonOk {
				result += yes
			} else {
				result += no
			}
		case "~=":
			matches, _ := regexp.MatchString("^"+a+"$", c)
			if matches {
				result += yes
			} else {
				result += no
			}
		case "==":
			if a == c {
				result += yes
			} else {
				result += no
			}
		case "!=":
			if a != c {
				result += yes
			} else {
				result += no
			}
		case ">=":
			if A, err := strconv.ParseFloat(a, 64); err == nil {
				if C, err2 := strconv.ParseFloat(c, 64); err2 == nil {
					if A >= C {
						result += yes
					} else {
						result += no
					}
				} else {
					if a >= c {
						result += yes
					} else {
						result += no
					}
				}
			} else {
				if a >= c {
					result += yes
				} else {
					result += no
				}
			}
		case "<=":
			if A, err := strconv.ParseFloat(a, 64); err == nil {
				if C, err2 := strconv.ParseFloat(c, 64); err2 == nil {
					if A <= C {
						result += yes
					} else {
						result += no
					}
				} else {
					if a <= c {
						result += yes
					} else {
						result += no
					}
				}
			} else {
				if a <= c {
					result += yes
				} else {
					result += no
				}
			}
		case "~":
			matches, _ := regexp.MatchString("^"+a+"$", c)
			if !matches {
				result += yes
			} else {
				result += no
			}
		case "!":
			if len(a) == len(c) {
				result += yes
			} else {
				result += no
			}
		case "<":
			if A, err := strconv.ParseFloat(a, 64); err == nil {
				if C, err2 := strconv.ParseFloat(c, 64); err2 == nil {
					if A < C {
						result += yes
					} else {
						result += no
					}
				} else {
					if a < c {
						result += yes
					} else {
						result += no
					}
				}
			} else {
				if a < c {
					result += yes
				} else {
					result += no
				}
			}
		case ">":
			if A, err := strconv.ParseFloat(a, 64); err == nil {
				if C, err2 := strconv.ParseFloat(c, 64); err2 == nil {
					if A > C {
						result += yes
					} else {
						result += no
					}
				} else {
					if a > c {
						result += yes
					} else {
						result += no
					}
				}
			} else {
				if a > c {
					result += yes
				} else {
					result += no
				}
			}
		}

		if p["jump"] != "" {
			result += p["jump"]
		}
	}

	return result
}

// 评估一个表达式的真假值
func (it *IfText) EvaluateExpression(expression string) bool {

	// evaluateBinaryOperation 函数用于评估一个二元操作
	evaluateBinaryOperation := func(left, right bool, operator rune) bool {
		switch operator {
		case '&':
			return left && right // 如果操作符为 '&'，返回左右操作数的逻辑与结果
		case '|':
			return left || right // 如果操作符为 '|'，返回左右操作数的逻辑或结果
		default:
			return false // 默认返回 false
		}
	}

	// 用于存储操作数和操作符的栈
	var operands []bool
	var operators []rune

	// performPendingOperations 函数用于执行优先级较高或相等的待处理操作
	performPendingOperations := func(operator rune) {
		for len(operators) > 0 && (operators[len(operators)-1] == '&' || operators[len(operators)-1] == '|') {
			prevOperator := operators[len(operators)-1]
			if (operator == '|' && prevOperator == '&') || operator == prevOperator && len(operands) != 0 {
				left := false  // 左操作数
				right := false // 右操作数
				if len(operands)-1 > 0 {
					right = operands[len(operands)-1]
					operands = operands[:len(operands)-1]
					left = operands[len(operands)-1]
					operands = operands[:len(operands)-1]
				}

				operands = append(operands, evaluateBinaryOperation(left, right, prevOperator)) // 计算并将结果压入操作数栈

				operators = operators[:len(operators)-1] // 弹出操作符
			} else {
				break
			}
		}
	}

	// 遍历表达式
	for _, char := range expression {
		switch char {
		case '1':
			operands = append(operands, true) // 将 true 压入操作数栈
		case '0':
			operands = append(operands, false) // 将 false 压入操作数栈
		case '&', '|':
			performPendingOperations(char)      // 执行待处理操作
			operators = append(operators, char) // 将当前操作符压入操作符栈
		case '(':
			operators = append(operators, '(') // 将左括号压入操作符栈
		case ')':
			for operators[len(operators)-1] != '(' {
				operator := operators[len(operators)-1]
				operators = operators[:len(operators)-1]

				right := operands[len(operands)-1] // 右操作数
				operands = operands[:len(operands)-1]

				left := operands[len(operands)-1] // 左操作数
				operands = operands[:len(operands)-1]

				operands = append(operands, evaluateBinaryOperation(left, right, operator)) // 计算并将结果压入操作数栈
			}
			operators = operators[:len(operators)-1] // 弹出左括号
		}
	}

	// 执行剩余的操作
	for len(operators) > 0 && len(operands) != 0 {
		left := false  // 左操作数
		right := false // 右操作数
		operator := operators[len(operators)-1]
		if len(operands)-1 > 0 {
			operators = operators[:len(operators)-1]

			right = operands[len(operands)-1]
			operands = operands[:len(operands)-1]

			left = operands[len(operands)-1]
			operands = operands[:len(operands)-1]
		}
		operands = append(operands, evaluateBinaryOperation(left, right, operator)) // 计算并将结果压入操作数栈

	}

	// 结果应在操作数栈的顶部
	if len(operands) > 0 {
		return operands[0] // 返回最终结果
	}
	return false
}
