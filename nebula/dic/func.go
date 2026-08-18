package dic

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/cjxpj/nebula/count"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"
	"github.com/cjxpj/nebula/utils"

	_ "github.com/go-sql-driver/mysql"
)

// 函数跟变量
func Runs(d *dic_dto.DicFunc, text string) any {
	var resA any
	output := run.BuildFuncStr(text, func(valStr []string) (string, bool) {
		input := utils.NewDicInputs()
		input.SetString(valStr)
		resAny, _ := Funcs(d, &input)
		if resStr, ok := resAny.(string); ok {
			return resStr, false
		}
		if resAny != nil {
			return utils.AnyToString(resAny), false
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
		resAny, _ := Funcs(d, &input)
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
		resAny, _ := Funcs(d, &input)
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
		resAny, _ := Funcs(d, &input)
		if resStr, ok := resAny.(string); ok {
			return resStr, false
		}
		return "", false
	}, func(s string) (string, bool) {
		return s, false
	})
	return output
}

func Funcs(d *dic_dto.DicFunc, dic_i *utils.DicInputs) (any, error) {
	if dic_i.LenOk(-1) {
		return "$$", nil
	}

	if d.Sys.Stop.Load() {
		return "", nil
	}

	// 创建 Class 实例
	if dic_i.String(0) == "new" {
		return newClassInstance(d, dic_i)
	}

	// 实例方法调用：$变量.方法 参数$（变量值为实例）
	if s := dic_i.String(0); len(s) > 2 && s[0] != '.' && s[0] != '%' {
		if dot := strings.IndexByte(s, '.'); dot > 0 && dot < len(s)-1 {
			if value, ok := d.Val.GetVal(s[:dot]); ok {
				if classData, isClass := value.(*dto.DicClass); isClass {
					methodArgs := append([]string{s[dot+1:]}, dic_i.StringAfterList(1)...)
					if res, ok := runClassMethod(d, classData, methodArgs); ok {
						return res, nil
					}
				}
			}
		}
	}

	// 面对象
	if className := dic_i.String(0); dic_i.LenOk("2..") && len(className) > 1 && (className[0] == '.' || className[0] == '%') {
		classType := className[0]
		className := className[1:]

		var isV bool
		if classType == '%' && !strings.HasSuffix(className, "%") {
			isV = true
		}
		var classData *dto.DicClass
		if className == "自己" {
			classData = d.Dic.ResolveClassData(d.Val.P.Get("Class"))
		} else {
			classData = d.Dic.ResolveClassData(className)
			if classData == nil {
				// 变量间接：className 是变量，值为类名或实例
				if value, ok := d.Val.GetVal(className); ok {
					classData = d.Dic.ResolveClassData(value)
				}
			}
		}
		if classData == nil {
			return "", errors.New("非Class")
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
			return "未知Class方法", nil
		}

		// Class 局部函数
		if res, ok := runClassMethod(d, classData, dic_i.StringAfterList(1)); ok {
			return res, nil
		}
	} else {
		text := strings.Join(dic_i.StringList(), " ")
		// 局部函数
		if str, Tstr, tparts, errRule, ok := run.RunFunc(d.Dic.DicFuncs["函数"], dic_i.String(0), dic_i.Len()); ok {
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
				SetDic_v(d.Dic).
				WithRecursionDepth(d.RecursionDepth)
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
		} else if errRule != "" {
			return "", paramCountError(d, dic_i.String(0), errRule, dic_i.Len())
		}
	}

	inputs := utils.NewDicInputs()
	inputs.Set(make([]any, dic_i.Len()+1))

	for i, line := range dic_i.List {
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
					SetDic_v(d.Dic).
					WithRecursionDepth(d.RecursionDepth)
				return dic_api.Api.DicRunLine(resDics, f.Content), nil
			}
		}
	}

	if fn, ok := d.Dic.MyFunc[dic_i.String(0)]; ok {
		if !inputs.LenOk(fn.L) {
			return "", paramCountError(d, dic_i.String(0), fn.L, inputs.Len())
		}
		res, err := fn.Fn(dto.NewDicInputsWithOutput(d.Dic, d.Val, &inputs, d.Output))
		if err != nil {
			d.Sys.Stop.Store(true)
			if err.Error() != "stop" {
				d.Output.Clear()
				d.Output.Add(fmt.Sprintf("[%s]%s(line:%d)：%v", d.Val.Get("_词库路径_"), dic_i.String(0), d.CurLine, err))
			}
		}
		return res, err
	}

	if fnInfo, ok := funcs.GetFunc(dic_i.String(0)); ok {
		if !inputs.LenOk(fnInfo.L) {
			return "", paramCountError(d, dic_i.String(0), fnInfo.L, inputs.Len())
		}
		res, err := fnInfo.Fn(dto.NewDicInputsWithOutput(d.Dic, d.Val, &inputs, d.Output))
		if err != nil {
			d.Sys.Stop.Store(true)
			if err.Error() != "stop" {
				d.Output.Clear()
				d.Output.Add(fmt.Sprintf("[%s]%s(line:%d)：%v", d.Val.Get("_词库路径_"), dic_i.String(0), d.CurLine, err))
			}
		}
		return res, err
	}

	return "$" + strings.Join(dic_i.StringList(), " ") + "$", nil
}

// paramCountError 参数数量校验失败：输出错误并停止执行，避免静默返回原文
func paramCountError(d *dic_dto.DicFunc, name, rule string, actual int) error {
	err := fmt.Errorf("参数数量错误(需要%s，实际%d)", rule, actual)
	d.Sys.Stop.Store(true)
	d.Output.Clear()
	d.Output.Add(fmt.Sprintf("[%s]%s(line:%d)：%v", d.Val.Get("_词库路径_"), name, d.CurLine, err))
	return err
}

// newClassInstance 创建 Class 实例并执行构造函数：$new 类名$
// 返回实例数据（*DicClass），可赋值给变量后用 %变量.成员% 读取、$.变量 函数$ 调用。
func newClassInstance(d *dic_dto.DicFunc, dic_i *utils.DicInputs) (any, error) {
	className := dic_i.String(1)
	if className == "" {
		return "", errors.New("new 参数错误：$new 类名$")
	}
	classData := d.Dic.Class[className]
	if classData == nil {
		return "", fmt.Errorf("非Class：%s", className)
	}

	newVal := dto.NewVal()
	if classData.LocalValue != nil {
		newVal.NewObj(classData.LocalValue.GetAll())
	}
	instance := &dto.DicClass{
		LocalValue: newVal,
		DicFuncs:   classData.DicFuncs,
		Fn:         classData.Fn,
	}

	// 执行构造函数 [函数:类名]new
	if str, Tstr, _, _, ok := run.RunFunc(classData.DicFuncs["函数"], "new", 0); ok {
		funcv := dto.NewVal().
			Set("触发", Tstr).
			Set("触发词", "new").
			Set("Class", instance)
		dto.ValRunTrigger("new", Tstr, d.Val.NewDicVal(funcv), d.Val)
		RunDic := dic_dto.NewRunDicEntry().
			CloseTrigger().
			SetGlobal_v(d.Val.G).
			Set_v(funcv).
			SetDic_v(d.Dic).
			WithRecursionDepth(d.RecursionDepth)
		RunDic.ClearDicFuncs()
		d.Output.Add(dic_api.Api.DicRunLine(RunDic, str))
	}

	return instance, nil
}

// runClassMethod 执行类方法（函数）：methodArgs 为 [方法名, 参数...]。
// 优先匹配 Class.Fn 自定义函数，再回退到 BuildDic 函数。
// 返回执行结果与是否命中方法。
func runClassMethod(d *dic_dto.DicFunc, classData *dto.DicClass, methodArgs []string) (any, bool) {
	// 内置回调：$变量.回调 名称$ 触发类内 [内部]名称
	if methodArgs[0] == "回调" {
		trigger := strings.Join(methodArgs[1:], " ")
		str, Tstr, _, _ := run.RunFor(classData.DicFuncs["内部"], trigger, 0)
		funcv := dto.NewVal().
			Set("触发", Tstr).
			Set("触发词", trigger).
			Set("Class", classData)
		dto.ValRunTrigger(strings.Join(methodArgs, " "), Tstr, d.Val.NewDicVal(funcv), d.Val)
		RunDic := dic_dto.NewRunDicEntry().
			CloseTrigger().
			SetGlobal_v(d.Val.G).
			Set_v(funcv).
			SetDic_v(d.Dic).
			WithRecursionDepth(d.RecursionDepth)
		RunDic.ClearDicFuncs()
		return dic_api.Api.DicRunLine(RunDic, str), true
	}

	// 自定义函数优先
	if fn, ok := classData.Fn[methodArgs[0]]; ok {
		inputs := utils.NewDicInputs()
		list := make([]any, len(methodArgs))
		list[0] = methodArgs[0]
		for i := 1; i < len(methodArgs); i++ {
			list[i] = d.Val.Text(count.RunCountText(d.Val, methodArgs[i]))
		}
		inputs.Set(list)
		if !inputs.LenOk(fn.L) {
			paramCountError(d, methodArgs[0], fn.L, inputs.Len())
			return "", true
		}
		funcv := dto.NewVal().
			Set("触发", methodArgs[0]).
			Set("触发词", strings.Join(methodArgs, " ")).
			Set("Class", classData)
		newV := d.Val.NewDicVal(funcv)
		res, err := fn.Fn(dto.NewDicInputsWithOutput(d.Dic, newV, &inputs, d.Output))
		if err != nil {
			d.Sys.Stop.Store(true)
			if err.Error() != "stop" {
				d.Output.Clear()
				d.Output.Add(fmt.Sprintf("[%s]%s(line:%d)：%v", d.Val.Get("_词库路径_"), methodArgs[0], d.CurLine, err))
			}
		}
		return res, true
	}

	TStr := strings.Join(methodArgs, " ")
	str, Tstr, _, errRule, ok := run.RunFunc(classData.DicFuncs["函数"], methodArgs[0], len(methodArgs)-1)
	if !ok {
		if errRule != "" {
			paramCountError(d, methodArgs[0], errRule, len(methodArgs)-1)
			return "", true
		}
		return nil, false
	}
	funcv := dto.NewVal()
	funcv.Set("触发", Tstr)
	funcv.Set("触发词", TStr)
	funcv.Set("Class", classData)
	dto.ValRunTrigger(TStr, Tstr, d.Val.NewDicVal(funcv), d.Val)
	RunDic := dic_dto.NewRunDicEntry().
		CloseTrigger().
		SetGlobal_v(d.Val.G).
		Set_v(funcv).
		SetDic_v(d.Dic).
		WithRecursionDepth(d.RecursionDepth)
	RunDic.ClearDicFuncs()
	return dic_api.Api.DicRunLine(RunDic, str), true
}
