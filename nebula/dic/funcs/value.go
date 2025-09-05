package funcs

import (
	"strings"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// 线程变量
func threadVar(d *dto.DicInputs) (any, error) {
	if d.Inputs.LenOk("2") {
		dto.GV.Set(d.Inputs.String(1), d.Inputs.Get(2))
		return "", nil
	}
	if res := dto.GV.Get(d.Inputs.String(1)); res != nil {
		return res, nil
	}
	return "", nil
}

// 局部变量
func localVar(d *dto.DicInputs) (any, error) {
	if d.Inputs.LenOk("2") {
		d.V.P.Set(d.Inputs.String(1), d.Inputs.Get(2))
		return "", nil
	}
	if res := d.V.P.Get(d.Inputs.String(1)); res != nil {
		return res, nil
	}
	return "", nil
}

// 全局变量
func globalVar(d *dto.DicInputs) (any, error) {
	if d.Inputs.LenOk("2") {
		d.V.G.Set(d.Inputs.String(1), d.Inputs.Get(2))
		return "", nil
	}
	if res := d.V.G.Get(d.Inputs.String(1)); res != nil {
		return res, nil
	}
	return "", nil
}

// 局部变量锁
func localVarLock(d *dto.DicInputs) (any, error) {
	str := strings.Split(d.Inputs.String(1), ",")
	for _, s := range str {
		d.V.P.SetLock(s, true)
	}
	return "", nil
}

// 局部变量文本
func localVarText(d *dto.DicInputs) (any, error) {
	return utils.AnyIsString(d.V.P.Get(d.Inputs.String(1))), nil
}
