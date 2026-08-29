package dic

import (
	"strconv"
	"strings"

	"github.com/cjxpj/nebula/count"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// 本文件仅保留叶子语句（赋值/算术/循环变量步进）的共享语义辅助函数，
// 供字节码 VM 的运行时适配层（bytecode.go 的 dicRuntime.runLeaf 等）复用。
// 旧解释器的 compileProgram / execProgram 状态机已删除，控制流由 dic/bc 与 dic/ast 负责。

// toInt64 把任意值转换为 int64（仅整数类型与整数数字字符串），失败返回 false。
func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int8:
		return int64(n), true
	case int16:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case uint:
		return int64(n), true
	case uint8:
		return int64(n), true
	case uint16:
		return int64(n), true
	case uint32:
		return int64(n), true
	case uint64:
		return int64(n), true
	case string:
		i, err := strconv.ParseInt(n, 10, 64)
		if err != nil {
			return 0, false
		}
		return i, true
	}
	return 0, false
}

// toFloat64 把任意值转换为 float64（整数、浮点与数字字符串），失败返回 false。
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

// int64Min 用于整数乘法溢出检测的 int64 最小值。
const int64Min int64 = -1 << 63

// 数值计算精度边界约定：
//   - 整数变量直存 int64，范围 [-2^63, 2^63-1]（约 ±9.22e18）。
//   - 整数算术溢出后回退 float64，而 float64 仅能精确表示绝对值 ≤ 2^53（约 9.01e15）的整数，
//     超出后低位会静默丢失。因此当前实现不支持任意精度（无上限）整数计算。
//   - 除法结果恒为 float64。
// 如需任意精度整数，需引入 math/big.Int 并重构 num 存储与全部算术函数（本实现未做）。

// numAdd/numSub/numMul 数值直算：优先 int64 原生运算（复用 addInt64/subInt64/mulInt64），
// 溢出或无法整数化时用 float64。返回结果与是否按数值处理；非数值返回 false，由调用方回退旧语义。
func numAdd(a, b any) (any, bool) {
	if ai, ok := toInt64(a); ok {
		if bi, ok := toInt64(b); ok {
			if sum, ok := addInt64(ai, bi); ok {
				return sum, true
			}
			return float64(ai) + float64(bi), true
		}
	}
	if af, aok := toFloat64(a); aok {
		if bf, bok := toFloat64(b); bok {
			return af + bf, true
		}
	}
	return nil, false
}

func numSub(a, b any) (any, bool) {
	if ai, ok := toInt64(a); ok {
		if bi, ok := toInt64(b); ok {
			if diff, ok := subInt64(ai, bi); ok {
				return diff, true
			}
			return float64(ai) - float64(bi), true
		}
	}
	if af, aok := toFloat64(a); aok {
		if bf, bok := toFloat64(b); bok {
			return af - bf, true
		}
	}
	return nil, false
}

func numMul(a, b any) (any, bool) {
	if ai, ok := toInt64(a); ok {
		if bi, ok := toInt64(b); ok {
			if prod, ok := mulInt64(ai, bi); ok {
				return prod, true
			}
			return float64(ai) * float64(bi), true
		}
	}
	if af, aok := toFloat64(a); aok {
		if bf, bok := toFloat64(b); bok {
			return af * bf, true
		}
	}
	return nil, false
}

// numDiv 数值除法，结果始终为 float64。
func numDiv(a, b any) (any, bool) {
	af, aok := toFloat64(a)
	bf, bok := toFloat64(b)
	if aok && bok {
		return af / bf, true
	}
	return nil, false
}

// addInt64/subInt64/mulInt64 带溢出检测的 int64 原生算术，溢出返回 false（调用方回退 float64）。
// 与 numAdd/numSub/numMul 的整数分支逻辑一致。
func addInt64(a, b int64) (int64, bool) {
	sum := a + b
	if (b > 0 && sum < a) || (b < 0 && sum > a) {
		return 0, false
	}
	return sum, true
}

func subInt64(a, b int64) (int64, bool) {
	diff := a - b
	if (b < 0 && diff < a) || (b > 0 && diff > a) {
		return 0, false
	}
	return diff, true
}

func mulInt64(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	if a == int64Min && b == -1 {
		return 0, false
	}
	prod := a * b
	if prod/b != a {
		return 0, false
	}
	return prod, true
}

// runNumOperandInt64 仅当右操作数是纯整数变量 %var% 时才返回其 int64（免装箱）。
// 其余情况（float/字符串/函数/[计算]/字面量）一律返回 false，交由通用 any 路径处理，
// 避免把浮点值截断为 int64 造成类型语义变化。
func runNumOperandInt64(d *dic_dto.DicFunc, vSuffix string) (int64, bool) {
	if len(vSuffix) > 2 && vSuffix[0] == '%' && vSuffix[len(vSuffix)-1] == '%' {
		name := vSuffix[1 : len(vSuffix)-1]
		if !strings.Contains(name, "%") {
			return d.Val.GetInt64(name)
		}
	}
	return 0, false
}

// runNumOperand 解析算术右操作数：单变量 %var% 直接取变量值保留数值类型，
// 避免字符串中转后再 ParseFloat；其余（函数/字面量/[计算]）回退 Runs。
func runNumOperand(d *dic_dto.DicFunc, vSuffix string) any {
	if len(vSuffix) > 2 && vSuffix[0] == '%' && vSuffix[len(vSuffix)-1] == '%' {
		name := vSuffix[1 : len(vSuffix)-1]
		if !strings.Contains(name, "%") {
			if val, ok := d.Val.GetVal(name); ok {
				return val
			}
		}
	}
	return Runs(d, utils.AnyToString(count.RunCountText(d.Val, vSuffix)))
}

// loopVarChangedFromValue 根据已读取的循环变量值判断步进（字符串/整数兼容），
// 返回 (新值, 是否被修改, 是否解析失败需中断循环)。
func loopVarChangedFromValue(val any, cur int) (int, bool, bool) {
	switch n := val.(type) {
	case int:
		if n != cur {
			return n, true, false
		}
	case int32:
		if int(n) != cur {
			return int(n), true, false
		}
	case string:
		if n == strconv.Itoa(cur) {
			return cur, false, false
		}
		xi, err := strconv.Atoi(n)
		if err != nil {
			return cur, false, true
		}
		return xi, true, false
	}
	return cur, false, false
}

// loopVarChanged 判断循环变量是否在循环体内被修改。
// 兼容旧脚本的字符串赋值（i:5）与新直存直算的整数赋值。
func loopVarChanged(v *dto.Val, name string, cur int) (int, bool, bool) {
	// 整数变量直读免装箱
	if n, ok := v.GetInt64(name); ok {
		if int(n) != cur {
			return int(n), true, false
		}
		return cur, false, false
	}
	return loopVarChangedFromValue(v.Get(name), cur)
}

// concatLeftStr 自增非数值回退拼接时的左值文本：
// 字符串/数值取其文本，其余（nil、map、切片、Class 等）按旧语义视为空串。
func concatLeftStr(v any) string {
	switch n := v.(type) {
	case nil:
		return ""
	case string:
		return n
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return utils.AnyToString(n)
	default:
		return ""
	}
}

// setInt64OrFloat 整数运算结果写回：ok 为 true 时直存 int64（免装箱），否则以浮点结果回退。
func setInt64OrFloat(v *dto.Val, key string, res int64, ok bool, fallback float64) {
	if ok {
		v.SetInt64(key, res)
	} else {
		v.Set(key, fallback)
	}
}

// setInt64OrFloatSlot 与 setInt64OrFloat 等价，但在 fastSlot 且目标为普通变量（slot>=0）时
// 直写无锁槽（num 映射由字节码块结束统一 FlushSlotsToMap），避免热路径每轮加锁。
func setInt64OrFloatSlot(v *dto.Val, key string, slot int32, res int64, ok bool, fallback float64, fastSlot bool) {
	if fastSlot && slot >= 0 {
		if ok {
			v.SlotSetInt64(slot, res)
		} else {
			v.Set(key, fallback)
		}
		return
	}
	setInt64OrFloat(v, key, res, ok, fallback)
}

// runSimpleAssign 处理简单赋值/算术行（vType 1,2,3,4,5,7,8），返回 true 表示该行已处理完毕。
// 这些行不改变状态机、不涉及 JSON 框/连续执行等复杂分支，可在普通状态下走快速路径直接执行，
// 语义与原解释器的 switch vType 分支完全一致。
func runSimpleAssign(r *dic_dto.DicEntry, funcV *dic_dto.DicFunc, vType int8, vPrefix, vSuffix string, prefixSlot, suffixSlot int32, fastSlot bool) bool {
	// 整数直算快速路径：左右操作数均为整数时直接 int64 原生运算，免装箱、免字符串中转。
	// 仅当左值为整数变量且右操作数为整数变量时生效；否则回退下方通用 any 逻辑，语义不变。
	if vType == 1 || vType == 2 || vType == 7 || vType == 8 {
		var left int64
		var lok bool
		if prefixSlot >= 0 {
			left, lok = r.Val.P.SlotGetInt64(prefixSlot)
		} else {
			left, lok = r.Val.P.GetInt64(vPrefix)
		}
		if lok {
			var right int64
			var rok bool
			if suffixSlot >= 0 {
				right, rok = funcV.Val.GetSlotInt64(suffixSlot)
			} else {
				right, rok = runNumOperandInt64(funcV, vSuffix)
			}
			if rok {
				switch vType {
				case 1: // 自减
					res, ok := subInt64(left, right)
					setInt64OrFloatSlot(r.Val.P, vPrefix, prefixSlot, res, ok, float64(left)-float64(right), fastSlot)
					return true
				case 2: // 自增
					res, ok := addInt64(left, right)
					setInt64OrFloatSlot(r.Val.P, vPrefix, prefixSlot, res, ok, float64(left)+float64(right), fastSlot)
					return true
				case 7: // 乘法
					res, ok := mulInt64(left, right)
					setInt64OrFloatSlot(r.Val.P, vPrefix, prefixSlot, res, ok, float64(left)*float64(right), fastSlot)
					return true
				case 8: // 除法
					r.Val.P.Set(vPrefix, float64(left)/float64(right))
					return true
				}
			}
		}
	}

	var vSetData any
	switch vType {
	case 1, 2, 7, 8:
		vSetData = runNumOperand(funcV, vSuffix)
	}

	switch vType {
	case 1: // 自减
		leftVal := r.Val.P.Get(vPrefix)
		if res, ok := numSub(leftVal, vSetData); ok {
			r.Val.P.Set(vPrefix, res)
		}
		return true
	case 2: // 自增
		leftVal := r.Val.P.Get(vPrefix)
		// 追加 JSON 数组/对象（仅当左值为 JSON 字符串时）
		if str, ok := leftVal.(string); ok {
			if j := utils.IsJSONResult(str); j != nil {
				vSetStr := utils.AnyToString(vSetData)
				if j, ok := j.([]any); ok {
					j = append(j, vSetStr)
					if j, err := json.Marshal(j); err == nil {
						r.Val.P.Set(vPrefix, string(j))
						return true
					}
				}
				if j, ok := j.(map[string]any); ok {
					j[strconv.Itoa(len(j))] = vSetStr
					if j, err := json.Marshal(j); err == nil {
						r.Val.P.Set(vPrefix, string(j))
						return true
					}
				}
			}
		}
		if res, ok := numAdd(leftVal, vSetData); ok {
			r.Val.P.Set(vPrefix, res)
			return true
		}
		// 非数值回退字符串拼接
		r.Val.P.Set(vPrefix, concatLeftStr(leftVal)+utils.AnyToString(vSetData))
		return true
	case 7: // 乘法
		leftVal := r.Val.P.Get(vPrefix)
		if res, ok := numMul(leftVal, vSetData); ok {
			r.Val.P.Set(vPrefix, res)
			return true
		}
		// 复读字符串
		if n, ok := toFloat64(vSetData); ok && n >= 0 {
			r.Val.P.Set(vPrefix, strings.Repeat(concatLeftStr(leftVal), int(n)))
		}
		return true
	case 8: // 除法
		leftVal := r.Val.P.Get(vPrefix)
		if res, ok := numDiv(leftVal, vSetData); ok {
			r.Val.P.Set(vPrefix, res)
		}
		return true
	case 3: // 执行函数
		r.Val.P.Set(vPrefix, Runs(funcV, vSuffix))
		return true
	case 4: // 执行变量
		r.Val.P.Set(vPrefix, r.Val.Text(vSuffix))
		return true
	case 5: // 纯文本赋值（绝对文本，不支持转义）
		r.Val.P.Set(vPrefix, vSuffix)
		return true
	}
	return false
}
