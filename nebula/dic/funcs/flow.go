package funcs

import (
	"errors"
	"fmt"
	"path/filepath"

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

// throwError 主动抛错：$报错$ 或 $报错 错误信息$。
// 返回 error 触发统一报错处理，可被 测试> 框捕获；无参时用默认错误文案。
func throwError(d *dto.DicInputs) (any, error) {
	msg := d.Inputs.String(1)
	if msg == "" {
		msg = "主动报错"
	}
	return nil, errors.New(msg)
}

func encodeDic(d *dto.DicInputs) (any, error) {
	raw := d.Inputs.String(1)
	// 读入路径：限定在词库目录内
	inPath, err := resolveDicPath(d, raw)
	if err != nil || inPath == "" {
		return "false", nil
	}
	file := utils.NewFileQueue(inPath)
	if file.ReadFileExt() != ".n" {
		return "false", nil
	}
	filedata, err := file.ReadFromFile()
	if err != nil {
		return "false", nil
	}
	// 输出路径：词库目录下的 encode/ 子目录，同样限制在词库目录内
	outPath, err := resolveDicPath(d, filepath.Join("encode", raw))
	if err != nil || outPath == "" {
		return "false", nil
	}
	file.SetPath(outPath)
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
