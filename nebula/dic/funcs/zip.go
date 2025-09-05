package funcs

import "github.com/cjxpj/nebula/utils"

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
