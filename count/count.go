package count

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"
)

// 定义令牌类型
const (
	ISINTEGER  = "ISINTEGER" // 整数
	PLUS       = "PLUS"      // 加号
	MINUS      = "MINUS"     // 减号
	MUL        = "MUL"       // 乘号
	DIV        = "DIV"       // 除号
	LPAREN     = "LPAREN"    // 左括号
	RPAREN     = "RPAREN"    // 右括号
	WHITESPACE = " "         // 空格
)

// 令牌结构
type Token struct {
	Type  string      // 类型
	Value interface{} // 值
}

// 词法分析器结构
type Lexer struct {
	Text        string // 文本
	Pos         int    // 当前位置
	CurrentChar byte   // 当前字符
}

// RunCountText 函数用于处理文本
func RunCountText(v *dto.Val, text string) string {
	result := run.ReplaceProcessedContent(text, "[", "]", func(val string) string {
		hasOperator := false // 用于跟踪是否找到了计算符号
		valtext := v.Text(val)
		for _, ch := range valtext {
			if ch >= '0' && ch <= '9' || ch == '(' || ch == ')' || ch == '.' {
				// 这是一个数字或括号，不做特别处理
			} else if ch == '+' || ch == '-' || ch == '*' || ch == '/' {
				// 这是一个计算符号，设置标志并继续检查
				hasOperator = true
			} else {
				// 找到了非法字符，直接返回
				return "[" + val + "]"
			}
		}

		// 检查是否找到了计算符号
		if !hasOperator {
			return "[" + val + "]"
		}

		c, err := Count(valtext)
		if err != nil {
			// fmt.Println(err)
			return "[" + val + "]"
		}

		var strNum string
		switch v := c.(type) {
		case *big.Float:
			strNum = v.Text('f', -1) // 对浮点数使用 'f' 格式输出
		case *big.Int:
			strNum = v.String() // 对整数使用 base 10 格式输出
		default:
			return "[" + val + "]"
		}

		return strNum
	})
	// // 创建正则表达式
	// re := regexp.MustCompile(`\[(.+?)\]`)

	// // 使用 ReplaceAllStringFunc 替换匹配的内容
	// result := re.ReplaceAllStringFunc(text, func(match string) string {
	// 	val := re.FindStringSubmatch(match)[1]
	// 	valtext := v.Text(val)
	// 	re := regexp.MustCompile(`^[+\-*/()0-9]+$`)
	// 	if !re.MatchString(valtext) {
	// 		return match
	// 	}
	// 	c, err := Count(valtext)
	// 	if err != nil {
	// 		return match
	// 	}
	// 	// 将 float64 转换为字符串，使用指数表示法
	// 	strNum := strconv.FormatFloat(c, 'f', -1, 64)

	// 	if strNum == "0" {
	// 		return "0"
	// 	}

	// 	// findstr := strings.Index(strNum, ".")

	// 	// if findstr != -1 {
	// 	// 	// 去除末尾的多余的零
	// 	// 	strNum = strings.TrimRight(strNum, "0")

	// 	// 	// 如果最后一个字符是小数点，也需要去除
	// 	// 	if strNum[len(strNum)-1] == '.' {
	// 	// 		strNum = strNum[:len(strNum)-1]
	// 	// 	}
	// 	// }
	// 	return strNum
	// })
	return result
}

// NewLexer 创建一个新的词法分析器实例
func NewLexer(text string) *Lexer {
	return &Lexer{
		Text:        strings.TrimSpace(text),
		Pos:         0,
		CurrentChar: text[0],
	}
}

// Error 抛出一个错误
func (l *Lexer) Error() {
	panic("发生错误！")
}

// Advance 将词法分析器位置移动到下一个字符
func (l *Lexer) Advance() {
	l.Pos++
	if l.Pos > len(l.Text)-1 {
		l.CurrentChar = 0
	} else {
		l.CurrentChar = l.Text[l.Pos]
	}
}

// SkipWhitespace 跳过任何空白字符
func (l *Lexer) SkipWhitespace() {
	for l.CurrentChar != 0 && l.CurrentChar == ' ' {
		l.Advance()
	}
}

// IsInteger 判断字符串是否为整数
func IsInteger(s string) bool {
	return !strings.Contains(s, ".")
}

// Floats 提取浮点数令牌
func (l *Lexer) Floats() *big.Float {
	var floatPart strings.Builder
	for l.CurrentChar != 0 && (('0' <= l.CurrentChar && l.CurrentChar <= '9') || l.CurrentChar == '.') {
		floatPart.WriteByte(l.CurrentChar)
		l.Advance()
	}
	num, success := new(big.Float).SetString(floatPart.String())
	if !success {
		l.Error()
	}
	return num
}

// Integers 提取整数令牌
func (l *Lexer) Integers() *big.Int {
	var intPart strings.Builder
	for l.CurrentChar != 0 && '0' <= l.CurrentChar && l.CurrentChar <= '9' {
		intPart.WriteByte(l.CurrentChar)
		l.Advance()
	}
	num, success := new(big.Int).SetString(intPart.String(), 10)
	if !success {
		l.Error()
	}
	return num
}

// GetNextToken 从词法分析器中获取下一个令牌
func (l *Lexer) GetNextToken() *Token {
	for l.CurrentChar != 0 {
		if l.CurrentChar == ' ' {
			l.SkipWhitespace()
			continue
		}

		if '0' <= l.CurrentChar && l.CurrentChar <= '9' {
			// 判断是否是整数，尝试使用 big.Float 处理
			if IsInteger(l.Text[l.Pos:]) {
				value := l.Integers()
				floatValue := new(big.Float).SetInt(value)
				if floatValue.Cmp(new(big.Float).SetInt(value)) == 0 {
					return &Token{Type: "FLOAT", Value: floatValue}
				}
				return &Token{Type: "INT", Value: value}
			} else {
				return &Token{Type: "FLOAT", Value: l.Floats()}
			}
		}

		if l.CurrentChar == '+' {
			l.Advance()
			return &Token{Type: "PLUS", Value: '+'}
		}

		if l.CurrentChar == '-' {
			l.Advance()
			return &Token{Type: "MINUS", Value: '-'}
		}

		if l.CurrentChar == '*' {
			l.Advance()
			return &Token{Type: "MUL", Value: '*'}
		}

		if l.CurrentChar == '/' {
			l.Advance()
			return &Token{Type: "DIV", Value: '/'}
		}

		if l.CurrentChar == '(' {
			l.Advance()
			return &Token{Type: "LPAREN", Value: '('}
		}

		if l.CurrentChar == ')' {
			l.Advance()
			return &Token{Type: "RPAREN", Value: ')'}
		}

		l.Error()
		return nil
	}

	return &Token{Type: "EOF", Value: nil}
}

// Interpreter 解释器结构
type Interpreter struct {
	Lexer        *Lexer
	CurrentToken *Token
}

// NewInterpreter 创建一个新的解释器实例
func NewInterpreter(lexer *Lexer) *Interpreter {
	return &Interpreter{
		Lexer:        lexer,
		CurrentToken: lexer.GetNextToken(),
	}
}

// Eat 如果当前令牌匹配指定类型，则消耗该令牌
func (i *Interpreter) Eat(tokenType string) {
	if i.CurrentToken.Type == tokenType {
		i.CurrentToken = i.Lexer.GetNextToken()
	} else {
		i.Error()
	}
}

// Error 抛出一个错误
func (i *Interpreter) Error() {
	panic("计算")
}

// Factor 解析因子表达式，增加负数处理
func (i *Interpreter) Factor() interface{} {
	token := i.CurrentToken

	// 检查是否是负数
	if token.Type == "MINUS" {
		i.Eat("MINUS")
		factor := i.Factor() // 递归调用 Factor 以解析后面的数字
		if floatResult, ok := factor.(*big.Float); ok {
			return new(big.Float).Neg(floatResult) // 返回负的浮点数
		} else if intResult, ok := factor.(*big.Int); ok {
			return new(big.Int).Neg(intResult) // 返回负的整数
		}
	}

	if token.Type == "FLOAT" {
		i.Eat("FLOAT")
		return token.Value.(*big.Float)
	} else if token.Type == "INT" {
		i.Eat("INT")
		return token.Value.(*big.Int)
	} else if token.Type == "LPAREN" {
		i.Eat("LPAREN")
		result := i.Expr()
		i.Eat("RPAREN")
		return result
	}

	i.Error()
	return nil
}

// Term 解析项表达式
func (i *Interpreter) Term() interface{} {
	result := i.Factor()
	for i.CurrentToken.Type == "MUL" || i.CurrentToken.Type == "DIV" {
		token := i.CurrentToken
		if token.Type == "MUL" {
			i.Eat("MUL")
			if floatResult, ok := result.(*big.Float); ok {
				result = floatResult.Mul(floatResult, i.Factor().(*big.Float))
			} else if intResult, ok := result.(*big.Int); ok {
				result = intResult.Mul(intResult, i.Factor().(*big.Int))
			}
		} else if token.Type == "DIV" {
			i.Eat("DIV")
			if floatResult, ok := result.(*big.Float); ok {
				result = floatResult.Quo(floatResult, i.Factor().(*big.Float))
			} else if intResult, ok := result.(*big.Int); ok {
				result = intResult.Div(intResult, i.Factor().(*big.Int))
			}
		}
	}
	return result
}

// Expr 解析表达式
func (i *Interpreter) Expr() interface{} {
	result := i.Term()
	for i.CurrentToken.Type == "PLUS" || i.CurrentToken.Type == "MINUS" {
		token := i.CurrentToken
		if token.Type == "PLUS" {
			i.Eat("PLUS")
			if floatResult, ok := result.(*big.Float); ok {
				result = floatResult.Add(floatResult, i.Term().(*big.Float))
			} else if intResult, ok := result.(*big.Int); ok {
				result = intResult.Add(intResult, i.Term().(*big.Int))
			}
		} else if token.Type == "MINUS" {
			i.Eat("MINUS")
			if floatResult, ok := result.(*big.Float); ok {
				result = floatResult.Sub(floatResult, i.Term().(*big.Float))
			} else if intResult, ok := result.(*big.Int); ok {
				result = intResult.Sub(intResult, i.Term().(*big.Int))
			}
		}
	}
	return result
}

// Count 评估表达式并返回结果
func Count(text string) (result interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("发生错误：%v", r)
		}
	}()

	lexer := NewLexer(text)
	interpreter := NewInterpreter(lexer)
	result = interpreter.Expr()
	return result, err
}
