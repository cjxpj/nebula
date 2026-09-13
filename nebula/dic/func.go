package dic

import (
	"errors"
	"fmt"
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

	// 捕获报错调用：$!函数名 参数$，报错时写入「报错」变量并返回错误文本，不停止后续执行
	captureErr := false
	if name := dic_i.String(0); strings.HasPrefix(name, "!") {
		captureErr = true
		dic_i.List[0] = name[1:]
		d.Val.P.Set("报错", "")
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
					if res, ok := runClassMethod(d, classData, methodArgs, captureErr); ok {
						return res, nil
					}
					// 变量确为面对象实例，但方法不存在：直接报错，不再回退为普通函数名静默吞掉。
					return handleFuncError(d, s, fmt.Errorf("面对象不存在方法：%s", methodArgs[0]), captureErr), nil
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
		if res, ok := runClassMethod(d, classData, dic_i.StringAfterList(1), captureErr); ok {
			return res, nil
		}
	} else {
		text := strings.Join(dic_i.StringList(), " ")
		// 局部函数
		if str, Tstr, tparts, errRule, ok := run.RunFuncIndexed(d.Dic.GetFuncIndex(), dic_i.String(0), dic_i.Len()); ok {
			// 系统内置函数禁止覆盖：同名 [函数] 定义调用时直接报错。
			if _, isBuiltin := funcs.GetFunc(dic_i.String(0)); isBuiltin {
				return handleFuncError(d, dic_i.String(0), fmt.Errorf("禁止覆盖系统内置函数：%s", dic_i.String(0)), captureErr), nil
			}
			funcv := dto.NewVal()
			giveRaw, _ := d.Val.P.GetRaw("_继承_")
			give, ok := giveRaw.(string)
			if ok && give != "" {
				for v := range strings.SplitSeq(give, ",") {
					set, ok := d.Val.P.Get(v).(string)
					if ok {
						funcv.Set(v, set)
					}
				}
				d.Val.P.SetRaw("_继承_", "")
			}
			funcv.Set("触发", Tstr)
			funcv.Set("触发词", text)
			// 参数需先求值 [算术]（如 [%参数1%+1]）与 %变量%，再拆分写入 参数N，保证递归/传参语义正确
			dto.ValRunTrigger(utils.AnyToString(count.RunCountText(d.Val, text)), Tstr, d.Val.NewDicVal(funcv), d.Val)
			RunDic := dic_dto.NewRunDicEntry().
				CloseTrigger().
				SetGlobal_v(d.Val.G).
				Set_v(funcv).
				SetDic_v(d.Dic)

			resRunDic := dic_api.Api.DicRunLine(RunDic, str)
			if captureErr && RunDic.Sys_v.Stop.Load() {
				d.Val.P.Set("报错", resRunDic)
				return "", nil
			}
			if tparts != "" {
				subParts := strings.SplitSeq(tparts, ",")
				for setv := range subParts {
					getv := RunDic.Val.P.Get(setv)
					d.Val.P.Set(setv, getv)
				}
			}
			return resRunDic, nil
		} else if errRule != "" {
			return handleFuncError(d, dic_i.String(0), fmt.Errorf("参数数量错误(需要%s，实际%d)", errRule, dic_i.Len()), captureErr), nil
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
			// 无参调用时使用默认参数，否则使用调用方传入的参数。
			arg := Tstr
			if arg == "" {
				arg = f.Default
			}
			funcv := dto.NewVal().
				Reset(d.Val.P.GetAll()).
				Set("触发", f.Default).
				Set("触发词", arg)
			resDics := dic_dto.NewRunDicEntry().
				SetGlobal_v(d.Val.G).
				Set_v(funcv).
				SetDic_v(d.Dic)
			resDics.LineNums = f.LineNums
			return dic_api.Api.DicRunLine(resDics, f.Content), nil
		}
	}

	if fn, ok := d.Dic.MyFunc[dic_i.String(0)]; ok {
		if !inputs.LenOk(fn.L) {
			return handleFuncError(d, dic_i.String(0), fmt.Errorf("参数数量错误(需要%s，实际%d)", fn.L, inputs.Len()), captureErr), nil
		}
		res, err := fn.Fn(dto.NewDicInputsWithOutput(d.Dic, d.Val, &inputs, d.Output))
		if err != nil {
			return handleFuncError(d, dic_i.String(0), err, captureErr), nil
		}
		return res, nil
	}

	if fnInfo, ok := funcs.GetFunc(dic_i.String(0)); ok {
		if !inputs.LenOk(fnInfo.L) {
			return handleFuncError(d, dic_i.String(0), fmt.Errorf("参数数量错误(需要%s，实际%d)", fnInfo.L, inputs.Len()), captureErr), nil
		}
		di := dto.NewDicInputsWithOutput(d.Dic, d.Val, &inputs, d.Output)
		di.Raw = dic_i
		res, err := fnInfo.Fn(di)
		if err != nil {
			return handleFuncError(d, dic_i.String(0), err, captureErr), nil
		}
		return res, nil
	}

	return "$" + strings.Join(dic_i.StringList(), " ") + "$", nil
}

// handleFuncError 统一处理函数调用报错。
// captureErr 为 true（$!函数名$ 调用）时把错误写入「报错」变量、清除 Stop 并返回空串（函数不返回错误文本）；
// 否则按原逻辑停止执行并把格式化错误写入输出，返回空串。
func handleFuncError(d *dic_dto.DicFunc, name string, err error, captureErr bool) string {
	if captureErr {
		d.Val.P.Set("报错", err.Error())
		d.Sys.Stop.Store(false)
		return ""
	}
	d.Sys.Stop.Store(true)
	if err.Error() != "stop" {
		d.Output.Clear()
		d.Output.Add(fmt.Sprintf("[%s]%s(line:%d)：%v", d.Val.G.GetStr("_词库路径_"), name, d.CurLine, err))
	}
	return ""
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
	if str, Tstr, _, _, ok := run.RunFuncIndexed(classData.GetFuncIndex(), "new", 0); ok {
		funcv := dto.NewVal().
			Set("触发", Tstr).
			Set("触发词", "new").
			Set("Class", instance)
		dto.ValRunTrigger("new", Tstr, d.Val.NewDicVal(funcv), d.Val)
		RunDic := dic_dto.NewRunDicEntry().
			CloseTrigger().
			SetGlobal_v(d.Val.G).
			Set_v(funcv).
			SetDic_v(d.Dic)
		d.Output.Add(dic_api.Api.DicRunLine(RunDic, str))
	}

	return instance, nil
}

// runClassMethod 执行类方法（函数）：methodArgs 为 [方法名, 参数...]。
// 优先匹配 Class.Fn 自定义函数，再回退到 BuildDic 函数。
// 返回执行结果与是否命中方法。
func runClassMethod(d *dic_dto.DicFunc, classData *dto.DicClass, methodArgs []string, captureErr bool) (any, bool) {
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
			SetDic_v(d.Dic)
		res := dic_api.Api.DicRunLine(RunDic, str)
		if captureErr && RunDic.Sys_v.Stop.Load() {
			d.Val.P.Set("报错", res)
			return "", true
		}
		return res, true
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
			return handleFuncError(d, methodArgs[0], fmt.Errorf("参数数量错误(需要%s，实际%d)", fn.L, inputs.Len()), captureErr), true
		}
		funcv := dto.NewVal().
			Set("触发", methodArgs[0]).
			Set("触发词", strings.Join(methodArgs, " ")).
			Set("Class", classData)
		newV := d.Val.NewDicVal(funcv)
		res, err := fn.Fn(dto.NewDicInputsWithOutput(d.Dic, newV, &inputs, d.Output))
		if err != nil {
			return handleFuncError(d, methodArgs[0], err, captureErr), true
		}
		return res, true
	}

	TStr := strings.Join(methodArgs, " ")
	str, Tstr, _, errRule, ok := run.RunFuncIndexed(classData.GetFuncIndex(), methodArgs[0], len(methodArgs)-1)
	if !ok {
		if errRule != "" {
			return handleFuncError(d, methodArgs[0], fmt.Errorf("参数数量错误(需要%s，实际%d)", errRule, len(methodArgs)-1), captureErr), true
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
		SetDic_v(d.Dic)
	res := dic_api.Api.DicRunLine(RunDic, str)
	if captureErr && RunDic.Sys_v.Stop.Load() {
		d.Val.P.Set("报错", res)
		return "", true
	}
	return res, true
}
