package dic

import (
	"errors"
	"regexp"
	"strings"

	"github.com/cjxpj/nebula/count"
	"github.com/cjxpj/nebula/debugLog"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"
	"github.com/cjxpj/nebula/utils"

	_ "github.com/go-sql-driver/mysql"
)

// 函数跟变量
func RunLine(d *dto.DicInfoData, text string) any {
	var resA any
	output := run.BuildFuncStr(text, func(valStr []string) (string, bool) {
		input := utils.NewDicInputs()
		input.SetString(valStr)
		resAny, err := RunFuncLine(d, input)
		if err != nil {
			debugLog.Infof("[%s]%s：%v", d.Value.Get("_词库路径_"), valStr[0], err)
		}
		if resStr, ok := resAny.(string); ok {
			return resStr, false
		}
		return "", false
	}, func(s string) (string, bool) {
		resAny := d.Data.Value.Text(d.Value, s)
		if resStr, ok := resAny.(string); ok {
			return resStr, false
		}
		resA = resAny
		return "", true
	})
	if resA != nil {
		return resA
	}
	return output
}

// 函数跟变量
func Runs(d *dic_dto.DicFunc, text string) any {
	var resA any
	output := run.BuildFuncStr(text, func(valStr []string) (string, bool) {
		input := utils.NewDicInputs()
		input.SetString(valStr)
		resAny, err := Funcs(d, &input)
		if err != nil {
			debugLog.Infof("[%s]%s：%v", d.Val.Get("_词库路径_"), valStr[0], err)
		}
		if resStr, ok := resAny.(string); ok {
			return resStr, false
		}
		return "", false
	}, func(s string) (string, bool) {
		resAny := d.Val.Text(s)
		if resStr, ok := resAny.(string); ok {
			return resStr, false
		}
		resA = resAny
		return "", true
	})
	if resA != nil {
		return resA
	}
	return output
}

// 执行，函数跟变量
func RunsAny(d *dic_dto.DicFunc, text string) any {
	// 拦截外部赋予值
	var resA any
	output := run.BuildFuncStr(text, func(valStr []string) (string, bool) {
		input := utils.NewDicInputs()
		input.SetString(valStr)
		resAny, err := Funcs(d, &input)
		if err != nil {
			debugLog.Infof("[%s]%s：%v", d.Val.Get("_词库路径_"), valStr[0], err)
		}
		if resAny == nil {
			return "", false
		}
		if resStr, ok := resAny.(string); ok {
			return resStr, false
		}
		resA = resAny
		return "", true
	}, func(s string) (string, bool) {
		resAny := d.Val.Text(s)
		if resStr, ok := resAny.(string); ok {
			return resStr, false
		}
		resA = resAny
		return "", true
	})
	if resA != nil {
		return resA
	}
	return output
}

// 赋予值执行，函数跟变量
func RunsVal(d *dic_dto.DicFunc, text string, setVal string) (string, bool) {
	// 拦截外部赋予值
	strNo := false
	output := run.BuildFuncStr(text, func(valStr []string) (string, bool) {
		input := utils.NewDicInputs()
		input.SetString(valStr)
		resAny, err := Funcs(d, &input)
		if err != nil {
			debugLog.Infof("[%s]%s：%v", d.Val.Get("_词库路径_"), valStr[0], err)
		}
		if resStr, ok := resAny.(string); ok {
			return resStr, false
		}
		d.Val.P.Set(setVal, resAny)
		strNo = true
		return "", false
	}, func(s string) (string, bool) {
		resAny := d.Val.Text(s)
		if resStr, ok := resAny.(string); ok {
			return resStr, false
		}
		strNo = true
		d.Val.P.Set(setVal, resAny)
		return "", true
	})
	return output, strNo
}

// 纯函数
func Run(d *dic_dto.DicFunc, text string) string {
	output := run.BuildFuncStr(text, func(valStr []string) (string, bool) {
		input := utils.NewDicInputs()
		input.SetString(valStr)
		resAny, err := Funcs(d, &input)
		if err != nil {
			debugLog.Infof("[%s]%s：%v", d.Val.Get("_词库路径_"), valStr[0], err)
		}
		if resStr, ok := resAny.(string); ok {
			return resStr, false
		}
		return "", false
	}, func(s string) (string, bool) {
		return s, false
	})
	return output
}

func RunFuncLine(d *dto.DicInfoData, dic_i utils.DicInputs) (any, error) {
	if dic_i.LenOk(-1) {
		return "$$", nil
	}

	inputs := utils.NewDicInputs()
	inputs.Set(make([]any, dic_i.Len()+1))

	for i, line := range dic_i.List {
		inputs.List[i] = d.Data.Value.Text(d.Value, count.RunCountText(dto.NewDicVals(d.Value, d.Data.Value), line))
	}

	for _, fn := range d.Data.LocalFunc {
		if fn.Name == dic_i.String(0) {
			if fn.Func != nil {
				return fn.Func(d, inputs)
			}
		}
	}

	return "$" + strings.Join(dic_i.StringList(), " ") + "$", nil
}

func Funcs(d *dic_dto.DicFunc, dic_i *utils.DicInputs) (any, error) {
	if dic_i.LenOk(-1) {
		return "$$", nil
	}

	// 面对象
	if className := dic_i.String(0); dic_i.LenOk("2..") && len(className) > 1 && (className[0] == '.' || className[0] == '%') {
		classType := className[0]
		className := className[1:]

		var isV bool
		if classType == '%' && !strings.HasSuffix(className, "%") {
			isV = true
		}
		if className == "自己" {
			className = d.Val.P.Get("Class").(string)
		}
		classData := d.Dic.LocalClass[className]
		if classData == nil {
			return "", errors.New("非整合包")
		}

		if isV {
			if dic_i.LenOk(3) {
				resVT := utils.AnyIsString(d.Val.Text(count.RunCountText(d.Val, dic_i.String(1))))
				resVTs := utils.AnyIsString(d.Val.Text(count.RunCountText(d.Val, dic_i.String(2))))
				classData.LocalValue.Set(resVT, resVTs)
				return "", nil
			}
			if dic_i.LenOk(2) {
				resVT := utils.AnyIsString(d.Val.Text(count.RunCountText(d.Val, dic_i.String(1))))
				resV, _ := classData.LocalValue.Get(resVT).(string)
				return resV, nil
			}
			return "", nil
		}

		if dic_i.LenOk(0, 1) {
			return "未知整合包方法", nil
		}

		TStr := strings.Join(dic_i.StringAfterList(1), " ")
		// 整合包局部函数
		if str, Tstr, _, regex := run.RunFor(classData.LocalFunc, TStr, 0); regex != nil {
			funcv := dto.NewVal()
			funcv.Set("触发", Tstr)
			funcv.Set("触发词", TStr)
			funcv.Set("Class", className)
			dto.ValRunTrigger(TStr, Tstr, d.Val.NewDicVal(funcv), d.Val)
			RunDic := dic_dto.NewRunDicEntry().
				CloseTrigger().
				SetGlobal_v(d.Val.G).
				Set_v(funcv).
				SetDic_v(d.Dic.Clone())
			RunDic.ClearDicFuncs()
			resRunDic := dic_api.Api.DicRunLine(RunDic, str)
			return resRunDic, nil
		}
	} else {
		text := strings.Join(dic_i.StringList(), " ")
		// 局部函数
		if str, Tstr, _, regex, tparts := run.RunFors(d.Dic.LocalFunc, text, 0); regex != nil {
			funcv := dto.NewVal()
			give, ok := d.Val.P.Get("_继承_").(string)
			if ok && give != "" {
				for v := range strings.SplitSeq(give, ",") {
					set, ok := d.Val.P.Get(v).(string)
					if ok {
						funcv.Set(v, set)
					}
				}
				d.Val.P.Set("_继承_", "")
			}
			funcv.Set("触发", Tstr)
			funcv.Set("触发词", text)
			dto.ValRunTrigger(text, Tstr, d.Val.NewDicVal(funcv), d.Val)
			RunDic := dic_dto.NewRunDicEntry().
				CloseTrigger().
				SetGlobal_v(d.Val.G).
				Set_v(funcv).
				SetDic_v(d.Dic.Clone())
			RunDic.ClearDicFuncs()

			resRunDic := dic_api.Api.DicRunLine(RunDic, str)
			if tparts != "" {
				subParts := strings.SplitSeq(tparts, ",")
				for setv := range subParts {
					getv := RunDic.Val.P.Get(setv)
					d.Val.P.Set(setv, getv)
				}
			}
			return resRunDic, nil
		}
	}

	lines := make([]any, dic_i.Len()+1)
	inputs := utils.NewDicInputs()
	inputs.Set(make([]any, dic_i.Len()+1))

	for i, line := range dic_i.List {
		lines[i] = line
		inputs.List[i] = d.Val.Text(count.RunCountText(d.Val, line))
	}

	if funcName := dic_i.String(0); strings.HasPrefix(funcName, "%") && strings.HasSuffix(funcName, "%") && len(funcName) > 2 {
		Tstr := dic_i.StringAfter(1)
		funcName = funcName[1 : len(funcName)-1]
		if f, ok := d.Val.P.Get(funcName).(*dto.FuncBox); ok && f != nil {
			matches := []string{}
			if f.Trigger != "" {
				matches = regexp.MustCompile("^" + f.Trigger + "$").FindStringSubmatch(Tstr)
			}
			if len(matches) > 0 || f.Trigger == "" {
				funcv := dto.NewVal().
					Reset(d.Val.P.GetAll()).
					Set("触发", f.Trigger).
					Set("触发词", Tstr)
				resDics := dic_dto.NewRunDicEntry().
					SetGlobal_v(d.Val.G).
					Set_v(funcv).
					SetDic_v(d.Dic)
				return dic_api.Api.DicRunLine(resDics, f.Content), nil
			}
		}
	}

	if fn, ok := d.Dic.MyFunc[dic_i.String(0)]; ok {
		if inputs.LenOk(fn.L) {
			return fn.Fn(dto.NewDicInputsWithOutput(d.Dic, d.Val, &inputs, d.Output))
		}
	}

	if fnInfo, ok := funcs.GetFunc(dic_i.String(0)); ok {
		if inputs.LenOk(fnInfo.L) {
			return fnInfo.Fn(dto.NewDicInputsWithOutput(d.Dic, d.Val, &inputs, d.Output))
		}
	}

	return "$" + strings.Join(dic_i.StringList(), " ") + "$", nil
}
