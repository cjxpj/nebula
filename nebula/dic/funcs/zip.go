package funcs

import (
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// 压缩
func (f *DicFunc) ZipFolder() string {
	path := f.Inputs.String(1)
	path2 := f.Inputs.String(2)
	if utils.NewFileQueue(path).ZipFolder(path2) {
		return "true"
	}
	return "false"
}

// 解压
func (f *DicFunc) UnZip() string {
	path := f.Inputs.String(1)
	path2 := f.Inputs.String(2)
	if utils.NewFileQueue(path).UnZip(path2) {
		return "true"
	}
	return "false"
}

func zipCompress(d *dto.DicInputs) (any, error) {
	// 源文件夹与目标压缩包路径均需限制在词库目录内
	if err := checkFuncPath(d, 1); err != nil {
		return "", err
	}
	if err := checkFuncPath(d, 2); err != nil {
		return "", err
	}
	path := d.Inputs.String(1)
	path2 := d.Inputs.String(2)
	if utils.NewFileQueue(path).ZipFolder(path2) {
		return "true", nil
	}
	return "false", nil
}

func zipDecompress(d *dto.DicInputs) (any, error) {
	// 压缩包与解压目标目录路径均需限制在词库目录内
	if err := checkFuncPath(d, 1); err != nil {
		return "", err
	}
	if err := checkFuncPath(d, 2); err != nil {
		return "", err
	}
	path := d.Inputs.String(1)
	path2 := d.Inputs.String(2)
	if utils.NewFileQueue(path).UnZip(path2) {
		return "true", nil
	}
	return "false", nil
}
