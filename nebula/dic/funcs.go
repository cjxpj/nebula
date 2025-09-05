package dic

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"
	"github.com/cjxpj/nebula/utils"
)

// 执行词库
func runDic(d *dto.DicInputs) (any, error) {
	data := d.Inputs.String(1)
	if data == "" {
		return "", nil
	}
	dicPath := "执行"

	// 触发
	chufa := d.Inputs.StringDefault(2, "Main")

	// 执行模式
	dicType := d.Inputs.StringDefault(3, "独立")

	calldicrun := NewDic(dicPath, data).
		SetGlobal_v(d.V.G)
	calldicrun.MyFunc = d.Dic.MyFunc
	calldicrun.SetFunc("调用", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("2..") {
			return "", errors.New("调用参数错误")
		}
		go func() {
			sleepTime := inputs.Int(1)
			time.Sleep(time.Duration(sleepTime) * time.Millisecond)
			rMsg := calldicrun.RunPrivate(inputs.StringAfter(2))
			if rMsg != "" {
				fmt.Println(rMsg)
			}
		}()
		return "", nil
	})
	calldicrun.ClassText = d.Dic.LocalClass
	calldicrun.Val.P.Set("_词库路径_", dicPath)

	switch dicType {
	case "继承":
		fv := dto.NewVal()
		fv.Reset(d.V.P.GetAll())
		fv.Set("_词库路径_", dicPath)
		calldicrun.Set_v(fv)
		calldicrun.FuncText = d.Dic.LocalFunc
	case "继承函数":
		calldicrun.FuncText = d.Dic.LocalFunc
	case "互通":
		d.V.P.Set("_词库路径_", dicPath)
		calldicrun.Set_v(d.V.P)
		calldicrun.FuncText = d.Dic.LocalFunc
	}

	DicRes := calldicrun.Run(chufa)
	return DicRes, nil
}

// 执行词库文件
func runDicFile(d *dto.DicInputs) (any, error) {
	dicPath := d.Inputs.String(1)
	data, err := utils.NewFileQueue(dicPath).ReadFromFile()
	if err != nil {
		return "", nil
	}

	// 触发
	chufa := d.Inputs.StringDefault(2, "Main")

	// 执行模式
	dicType := d.Inputs.StringDefault(3, "独立")

	calldicrun := NewDic(dicPath, data).
		SetGlobal_v(d.V.G)
	calldicrun.MyFunc = d.Dic.MyFunc
	calldicrun.SetFunc("调用", func(val *dto.DicVal, inputs *utils.DicInputs) (any, error) {
		if !inputs.LenOk("2..") {
			return "", errors.New("调用参数错误")
		}
		go func() {
			sleepTime := inputs.Int(1)
			time.Sleep(time.Duration(sleepTime) * time.Millisecond)
			rMsg := calldicrun.RunPrivate(inputs.StringAfter(2))
			if rMsg != "" {
				fmt.Println(rMsg)
			}
		}()
		return "", nil
	})
	calldicrun.ClassText = d.Dic.LocalClass
	calldicrun.Val.P.Set("_词库路径_", dicPath)

	switch dicType {
	case "继承":
		fv := dto.NewVal()
		fv.Reset(d.V.P.GetAll())
		fv.Set("_词库路径_", dicPath)
		calldicrun.Set_v(fv)
		calldicrun.FuncText = d.Dic.LocalFunc
	case "继承函数":
		calldicrun.FuncText = d.Dic.LocalFunc
	case "互通":
		d.V.P.Set("_词库路径_", dicPath)
		calldicrun.Set_v(d.V.P)
		calldicrun.FuncText = d.Dic.LocalFunc
	}

	DicRes := calldicrun.Run(chufa)
	return DicRes, nil
}

// 回调词库
func callDic(d *dto.DicInputs) (any, error) {
	var triggerParts []string
	for _, part := range d.Inputs.List[1:] {
		if strPart, ok := part.(string); ok {
			triggerParts = append(triggerParts, strPart)
		}
	}
	trigger := strings.Join(triggerParts, " ")

	// 判断是否在整合包中执行
	if classN, ok := d.V.P.Get("Class").(string); ok {
		classData := d.Dic.LocalClass[classN]
		if classData != nil {
			GetDic, GetDicTrigger, _, _ := run.RunFor(classData.LocalStatic, trigger, 0)
			funcV := dto.NewVal()
			funcV.Reset(d.V.P.GetAll())
			funcV.Set("触发词", trigger)
			funcV.Set("触发", GetDicTrigger)
			RunDics := NewRunDicEntry().
				SetGlobal_v(d.V.G).
				Set_v(funcV).
				SetDic_v(d.Dic)
			RunDic := RunDics.Run(GetDic)
			return RunDic, nil
		}
	}
	GetDic, GetDicTrigger, _, _ := run.RunFor(d.Dic.LocalStatic, trigger, 0)
	funcV := dto.NewVal()
	funcV.Reset(d.V.P.GetAll())
	funcV.Set("触发词", trigger)
	funcV.Set("触发", GetDicTrigger)
	RunDics := NewRunDicEntry().
		SetGlobal_v(d.V.G).
		Set_v(funcV).
		SetDic_v(d.Dic)
	RunDic := RunDics.Run(GetDic)
	return RunDic, nil
}

// 执行网页词库
func runWebDic(d *dto.DicInputs) (any, error) {
	data := d.Inputs.String(1)
	if data == "" {
		return "", nil
	}
	dicPath := "执行"
	webdic := NewWebDic(dicPath, data).
		SetGlobal_v(d.V.G)
	webdic.MyFunc = d.Dic.MyFunc
	webdicRes := webdic.Run()
	return webdicRes, nil
}

// 执行网页词库文件
func runWebDicFile(d *dto.DicInputs) (any, error) {
	dicPath := d.Inputs.String(1)
	data, err := utils.NewFileQueue(dicPath).ReadFromFile()
	if err != nil {
		return "", nil
	}
	webdic := NewWebDic(dicPath, data).
		SetGlobal_v(d.V.G)
	webdic.MyFunc = d.Dic.MyFunc
	webdicRes := webdic.Run()
	return webdicRes, nil
}
