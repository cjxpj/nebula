package count

import (
	"errors"
	"math/big"
	"strings"

	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"
)

/* ===================== Token ===================== */

const (
	INT   = "INT"
	FLOAT = "FLOAT"

	PLUS  = "PLUS"
	MINUS = "MINUS"
	MUL   = "MUL"
	DIV   = "DIV"
	POW   = "POW"

	SHL = "SHL"
	SHR = "SHR"

	LPAREN = "LPAREN"
	RPAREN = "RPAREN"
	EOF    = "EOF"
)

type Token struct {
	Type  string
	Value any
}

/* ===================== Lexer ===================== */

type Lexer struct {
	Text        string
	Pos         int
	CurrentChar byte
}

func NewLexer(text string) *Lexer {
	text = strings.TrimSpace(text)
	l := &Lexer{Text: text}
	if text != "" {
		l.CurrentChar = text[0]
	}
	return l
}

func (l *Lexer) Advance() {
	l.Pos++
	if l.Pos >= len(l.Text) {
		l.CurrentChar = 0
	} else {
		l.CurrentChar = l.Text[l.Pos]
	}
}

func (l *Lexer) SkipWhitespace() {
	for l.CurrentChar == ' ' {
		l.Advance()
	}
}

func (l *Lexer) GetNextToken() (*Token, error) {
	for l.CurrentChar != 0 {
		if l.CurrentChar == ' ' {
			l.SkipWhitespace()
			continue
		}

		if l.CurrentChar >= '0' && l.CurrentChar <= '9' {
			start := l.Pos
			for l.CurrentChar >= '0' && l.CurrentChar <= '9' {
				l.Advance()
			}
			if l.CurrentChar == '.' {
				l.Pos = start
				l.CurrentChar = l.Text[l.Pos]
				return readFloat(l)
			}
			l.Pos = start
			l.CurrentChar = l.Text[l.Pos]
			return readInt(l)
		}

		switch l.CurrentChar {
		case '+':
			l.Advance()
			return &Token{Type: PLUS}, nil
		case '-':
			l.Advance()
			return &Token{Type: MINUS}, nil
		case '*':
			l.Advance()
			return &Token{Type: MUL}, nil
		case '/':
			l.Advance()
			return &Token{Type: DIV}, nil
		case '^':
			l.Advance()
			return &Token{Type: POW}, nil
		case '<':
			l.Advance()
			if l.CurrentChar == '<' {
				l.Advance()
				return &Token{Type: SHL}, nil
			}
			return nil, errors.New("非法 <")
		case '>':
			l.Advance()
			if l.CurrentChar == '>' {
				l.Advance()
				return &Token{Type: SHR}, nil
			}
			return nil, errors.New("非法 >")
		case '(':
			l.Advance()
			return &Token{Type: LPAREN}, nil
		case ')':
			l.Advance()
			return &Token{Type: RPAREN}, nil
		}

		return nil, errors.New("非法字符")
	}

	return &Token{Type: EOF}, nil
}

func readInt(l *Lexer) (*Token, error) {
	var sb strings.Builder
	for l.CurrentChar >= '0' && l.CurrentChar <= '9' {
		sb.WriteByte(l.CurrentChar)
		l.Advance()
	}
	n, ok := new(big.Int).SetString(sb.String(), 10)
	if !ok {
		return nil, errors.New("整数解析失败")
	}
	return &Token{Type: INT, Value: n}, nil
}

func readFloat(l *Lexer) (*Token, error) {
	var sb strings.Builder
	for (l.CurrentChar >= '0' && l.CurrentChar <= '9') || l.CurrentChar == '.' {
		sb.WriteByte(l.CurrentChar)
		l.Advance()
	}
	n, ok := new(big.Float).SetString(sb.String())
	if !ok {
		return nil, errors.New("浮点解析失败")
	}
	return &Token{Type: FLOAT, Value: n}, nil
}

/* ===================== Interpreter ===================== */

type Interpreter struct {
	Lexer        *Lexer
	CurrentToken *Token
	Err          error
}

func NewInterpreter(l *Lexer) *Interpreter {
	t, err := l.GetNextToken()
	if t == nil {
		t = &Token{Type: EOF}
	}
	return &Interpreter{
		Lexer:        l,
		CurrentToken: t,
		Err:          err,
	}
}

func (i *Interpreter) next() {
	if i.Err != nil {
		i.CurrentToken = &Token{Type: EOF}
		return
	}
	t, err := i.Lexer.GetNextToken()
	if err != nil || t == nil {
		i.Err = err
		i.CurrentToken = &Token{Type: EOF}
		return
	}
	i.CurrentToken = t
}

func (i *Interpreter) Eat(t string) {
	if i.Err != nil || i.CurrentToken == nil {
		return
	}
	if i.CurrentToken.Type != t {
		i.Err = errors.New("语法错误")
		i.CurrentToken = &Token{Type: EOF}
		return
	}
	i.next()
}

/* ---------- 工具 ---------- */

func intToFloat(i *big.Int) *big.Float {
	return new(big.Float).SetInt(i)
}

func floatToIntRound(f *big.Float) *big.Int {
	tmp := new(big.Float).Copy(f)
	if f.Sign() >= 0 {
		tmp.Add(tmp, big.NewFloat(0.5))
	} else {
		tmp.Sub(tmp, big.NewFloat(0.5))
	}
	i, _ := tmp.Int(nil)
	return i
}

func toIntRound(v any) (*big.Int, bool) {
	switch n := v.(type) {
	case *big.Int:
		return n, true
	case *big.Float:
		return floatToIntRound(n), true
	default:
		return nil, false
	}
}

/* ---------- Factor ---------- */

func (i *Interpreter) Factor() any {
	if i.Err != nil || i.CurrentToken == nil {
		return nil
	}

	t := i.CurrentToken

	if t.Type == MINUS {
		i.Eat(MINUS)
		v := i.Factor()
		switch n := v.(type) {
		case *big.Int:
			return new(big.Int).Neg(n)
		case *big.Float:
			return new(big.Float).Neg(n)
		default:
			i.Err = errors.New("非法负号")
			return nil
		}
	}

	switch t.Type {
	case INT, FLOAT:
		i.Eat(t.Type)
		return t.Value
	case LPAREN:
		i.Eat(LPAREN)
		r := i.Expr()
		i.Eat(RPAREN)
		return r
	}

	i.Err = errors.New("Factor 错误")
	return nil
}

/* ---------- Power ---------- */

func (i *Interpreter) Power() any {
	left := i.Factor()
	if i.Err != nil || i.CurrentToken == nil {
		return left
	}

	if i.CurrentToken.Type == POW {
		i.Eat(POW)
		right := i.Power()

		var lInt, rInt *big.Int

		switch v := left.(type) {
		case *big.Int:
			lInt = v
		case *big.Float:
			lInt = floatToIntRound(v)
		default:
			i.Err = errors.New("^ 左操作数非法")
			return nil
		}

		switch v := right.(type) {
		case *big.Int:
			rInt = v
		case *big.Float:
			rInt = floatToIntRound(v)
		default:
			i.Err = errors.New("^ 右操作数非法")
			return nil
		}

		if rInt.Sign() < 0 {
			i.Err = errors.New("幂指数不能为负数")
			return nil
		}

		return new(big.Int).Exp(lInt, rInt, nil)
	}

	return left
}

/* ---------- Term ---------- */

func (i *Interpreter) Term() any {
	result := i.Power()
	if i.Err != nil {
		return nil
	}

	for i.CurrentToken != nil &&
		(i.CurrentToken.Type == MUL || i.CurrentToken.Type == DIV) {

		op := i.CurrentToken.Type
		i.Eat(op)
		rhs := i.Power()

		switch l := result.(type) {

		case *big.Int:
			switch r := rhs.(type) {
			case *big.Int:
				if op == MUL {
					result = l.Mul(l, r)
				} else {
					result = l.Div(l, r)
				}
			case *big.Float:
				lf := intToFloat(l)
				if op == MUL {
					result = lf.Mul(lf, r)
				} else {
					result = lf.Quo(lf, r)
				}
			default:
				i.Err = errors.New("非法乘除")
				return nil
			}

		case *big.Float:
			var rf *big.Float
			switch r := rhs.(type) {
			case *big.Float:
				rf = r
			case *big.Int:
				rf = intToFloat(r)
			default:
				i.Err = errors.New("非法乘除")
				return nil
			}

			if op == MUL {
				result = l.Mul(l, rf)
			} else {
				result = l.Quo(l, rf)
			}

		default:
			i.Err = errors.New("非法 Term")
			return nil
		}
	}

	return result
}

/* ---------- Expr ---------- */

func (i *Interpreter) Expr() any {
	result := i.Term()
	if i.Err != nil {
		return nil
	}

	for i.CurrentToken != nil &&
		(i.CurrentToken.Type == PLUS || i.CurrentToken.Type == MINUS) {

		op := i.CurrentToken.Type
		i.Eat(op)
		rhs := i.Term()

		switch l := result.(type) {

		case *big.Int:
			switch r := rhs.(type) {
			case *big.Int:
				if op == PLUS {
					result = l.Add(l, r)
				} else {
					result = l.Sub(l, r)
				}
			case *big.Float:
				lf := intToFloat(l)
				if op == PLUS {
					result = lf.Add(lf, r)
				} else {
					result = lf.Sub(lf, r)
				}
			default:
				i.Err = errors.New("非法加减")
				return nil
			}

		case *big.Float:
			var rf *big.Float
			switch r := rhs.(type) {
			case *big.Float:
				rf = r
			case *big.Int:
				rf = intToFloat(r)
			default:
				i.Err = errors.New("非法加减")
				return nil
			}

			if op == PLUS {
				result = l.Add(l, rf)
			} else {
				result = l.Sub(l, rf)
			}

		default:
			i.Err = errors.New("非法 Expr")
			return nil
		}
	}

	return result
}

/* ---------- Shift ---------- */

func (i *Interpreter) Shift() any {
	result := i.Expr()
	if i.Err != nil {
		return nil
	}

	for i.CurrentToken != nil &&
		(i.CurrentToken.Type == SHL || i.CurrentToken.Type == SHR) {

		op := i.CurrentToken.Type
		i.Eat(op)
		rhs := i.Expr()

		l, ok1 := toIntRound(result)
		r, ok2 := toIntRound(rhs)
		if !ok1 || !ok2 {
			i.Err = errors.New("位移操作数非法")
			return nil
		}

		if r.Sign() < 0 {
			i.Err = errors.New("位移位数不能为负")
			return nil
		}

		if op == SHL {
			result = new(big.Int).Lsh(l, uint(r.Int64()))
		} else {
			result = new(big.Int).Rsh(l, uint(r.Int64()))
		}
	}

	return result
}

/* ===================== API ===================== */

func Count(text string) (any, error) {
	i := NewInterpreter(NewLexer(text))
	res := i.Shift()
	if i.Err != nil {
		return nil, i.Err
	}
	return res, nil
}

/* ===================== 文本处理 ===================== */

func RunCountText(v *dto.DicVal, content any) any {
	text, ok := content.(string)
	if !ok || text == "" {
		return text
	}

	return run.ReplaceProcessedContent(text, "[", "]", func(val string) string {
		raw, ok := v.Text(val).(string)
		if !ok {
			return "[" + val + "]"
		}
		res, err := Count(raw)
		if err != nil {
			switch err.Error() {
			case "非法字符", "Factor 错误":
				return "[" + val + "]"
			}
			if strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]") {
				debugLog.Infof("Count 报错内容：%v", val)
				debugLog.Errorf("Count 失败: %v", err)
				return "[" + val + "]"
			}
			return "[" + val + "]"
		}
		switch n := res.(type) {
		case *big.Int:
			return n.String()
		case *big.Float:
			return n.Text('f', -1)
		}
		return "[" + val + "]"
	})
}
