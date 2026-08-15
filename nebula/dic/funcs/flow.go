package funcs

import (
	"errors"
	"fmt"

	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// ========== 基础工具 ==========

func captureOutput(d *dto.DicInputs) (any, error) {
	if d.Inputs.LenOk(0) && d.Output != nil {
		return d.Output.Get(), nil
	}
	return "", nil
}

func interceptOutput(d *dto.DicInputs) (any, error) {
	if d.Output != nil {
		d.Output.Clear()
	}
	return "", nil
}

func stopProgram(d *dto.DicInputs) (any, error) {
	if d.Output != nil {
		fmt.Print(d.Output.Get())
	}
	// 结束程序：打印输出后直接退出整个进程（wasm 环境下无操作）
	exitProcess()
	return "", errors.New("stop")
}

func encodeDic(d *dto.DicInputs) (any, error) {
	setpath := d.Inputs.String(1)
	file := utils.NewFileQueue(setpath)
	if file.ReadFileExt() != ".n" {
		return "false", nil
	}
	filedata, err := file.ReadFromFile()
	if err != nil {
		return "false", nil
	}
	file.SetPath("encode/" + d.Inputs.String(1))
	encodeDic, err := utils.Encrypt(filedata, appfiles.Key)
	if err != nil {
		return "false", nil
	}
	encodeDic = `// ` + appfiles.Version + "\n" + encodeDic
	file.WriteToFile(encodeDic)
	return "true", nil
}

// ========== GC回收 ==========

func gcCollect(d *dto.DicInputs) (any, error) {
	return nil, nil
}
