package utils

import (
	"math/big"
	"reflect"
	"strconv"
	"strings"
)

type DicInputs struct {
	List []any
}

func NewDicInputs() *DicInputs {
	return &DicInputs{}
}

func (i *DicInputs) Len() int {
	return len(i.List) - 1
}

// LenOk 判断输入长度是否满足任意一条规则
// 支持：1, "5..", "1|2|3..", []string{"1","2","3.."}
func (i *DicInputs) LenOk(rules ...any) bool {
	l := len(i.List) - 1

	for _, rule := range rules {
		switch v := rule.(type) {
		case int:
			if l == v {
				return true
			}

		case string:
			if ok := matchOne(l, v); ok {
				return true
			}

		case []string:
			for _, s := range v {
				if ok := matchOne(l, s); ok {
					return true
				}
			}
		}
	}
	return false
}

// matchOne 处理单条规则，支持:
//   - 精确数字 "3"
//   - 区间 "..5", "5.."
//   - 组合 "1|2|3.."
func matchOne(l int, rule string) bool {
	// 先按 | 切分组合规则
	for _, part := range strings.Split(rule, "|") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		switch {
		case strings.HasSuffix(part, ".."):
			min, err := strconv.Atoi(strings.TrimSuffix(part, ".."))
			if err == nil && l >= min {
				return true
			}

		case strings.HasPrefix(part, ".."):
			max, err := strconv.Atoi(strings.TrimPrefix(part, ".."))
			if err == nil && l <= max {
				return true
			}

		default:
			exact, err := strconv.Atoi(part)
			if err == nil && l == exact {
				return true
			}
		}
	}
	return false
}

func (i *DicInputs) Set(input []any) {
	i.List = input
}

func (i *DicInputs) SetString(input []string) {
	i.List = make([]any, len(input))
	for ii, v := range input {
		i.List[ii] = v
	}
}

func (i *DicInputs) Get(ii int) any {
	if ii >= len(i.List) {
		return nil
	}
	return i.List[ii]
}

func (i *DicInputs) GetType(ii int) string {
	if ii >= len(i.List) {
		return ""
	}
	// 返回类型
	return reflect.TypeOf(i.List[ii]).String()
}

func (i *DicInputs) String(ii int) string {
	if ii >= len(i.List) {
		return ""
	}
	if res, ok := i.List[ii].(string); ok {
		return res
	}
	return ""
}

func (i *DicInputs) Bool(ii int) bool {
	if ii >= len(i.List) {
		return false
	}
	if res, ok := i.List[ii].(bool); ok {
		return res
	}
	if res, ok := i.List[ii].(string); ok {
		if res == "true" || res == "1" {
			return true
		}
	}
	return false
}

// 读取字符串，不存在返回默认值
func (i *DicInputs) StringDefault(ii int, def string) string {
	if ii >= len(i.List) {
		return def
	}
	if res, ok := i.List[ii].(string); ok {
		return res
	}
	return def
}

// 获取string后面全部文本
func (i *DicInputs) StringAfterList(ii int) []string {
	if ii >= len(i.List) {
		return nil
	}

	var str []string
	// 推断全部String
	for _, v := range i.List[ii:] {
		if s, ok := v.(string); ok {
			str = append(str, s)
		} else {
			str = append(str, "")
		}
	}
	return str
}

// 获取全部[]string
func (i *DicInputs) StringList() []string {
	var str []string
	// 推断全部String
	for _, v := range i.List {
		if s, ok := v.(string); ok {
			str = append(str, s)
		} else {
			str = append(str, "")
		}
	}
	return str
}

// 获取后面全部文本
func (i *DicInputs) StringAfter(ii int) string {
	if ii >= len(i.List) {
		return ""
	}

	var str []string
	// 推断全部String
	for _, v := range i.List[ii:] {
		if s, ok := v.(string); ok {
			str = append(str, s)
		} else {
			str = append(str, "")
		}
	}
	return strings.Join(str, " ")
}

func (i *DicInputs) StringOk(ii int) (string, bool) {
	if ii >= len(i.List) {
		return "", false
	}
	if res, ok := i.List[ii].(string); ok {
		return res, true
	}
	return "", false
}

func (i *DicInputs) Int64(ii int) int64 {
	return int64(i.Int(ii))
}

func (i *DicInputs) Int(ii int) int {
	if ii >= len(i.List) {
		return 0
	}
	if res, ok := i.List[ii].(int); ok {
		return res
	}
	if res, ok := i.List[ii].(string); ok {
		if val, err := strconv.Atoi(res); err == nil {
			return val
		}
	}
	return 0
}

func (i *DicInputs) IntOk(ii int) (int, bool) {
	if ii >= len(i.List) {
		return 0, false
	}
	if res, ok := i.List[ii].(int); ok {
		return res, true
	}
	if res, ok := i.List[ii].(string); ok {
		if val, err := strconv.Atoi(res); err == nil {
			return val, true
		}
	}
	return 0, false
}

func (i *DicInputs) IntDefault(ii int, def int) int {
	if ii >= len(i.List) {
		return def
	}
	if res, ok := i.List[ii].(int); ok {
		return res
	}
	if res, ok := i.List[ii].(string); ok {
		if val, err := strconv.Atoi(res); err == nil {
			return val
		}
	}
	return def
}

func (i *DicInputs) BigFloat(ii int) *big.Float {
	if ii >= len(i.List) {
		return big.NewFloat(0)
	}
	if res, ok := i.List[ii].(*big.Float); ok {
		return res
	}
	if res, ok := i.List[ii].(string); ok {
		if val, ok := new(big.Float).SetString(res); ok {
			return val
		}
	}
	return big.NewFloat(0)
}

func (i *DicInputs) Float64(ii int) float64 {
	if ii >= len(i.List) {
		return 0
	}
	if res, ok := i.List[ii].(float64); ok {
		return res
	}
	if res, ok := i.List[ii].(string); ok {
		if val, err := strconv.ParseFloat(res, 64); err == nil {
			return val
		}
	}
	return 0
}

func (i *DicInputs) Float64Ok(ii int) (float64, bool) {
	if ii >= len(i.List) {
		return 0, false
	}
	if res, ok := i.List[ii].(float64); ok {
		return res, true
	}
	if res, ok := i.List[ii].(string); ok {
		if val, err := strconv.ParseFloat(res, 64); err == nil {
			return val, true
		}
	}
	return 0, false
}
