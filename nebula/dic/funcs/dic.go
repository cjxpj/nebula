package funcs

import (
	"github.com/cjxpj/nebula/appfiles"
	"github.com/cjxpj/nebula/utils"
)

func (f *DicFunc) EncodeDic() string {
	if f.Len == 1 {
		setpath := f.Inputs.String(1)
		file := utils.NewFileQueue(setpath)
		if file.ReadFileExt() != ".n" {
			return "false"
		}
		filedata, err := file.ReadFromFile()
		if err != nil {
			return "false"
		}
		file.SetPath("encode/" + f.Inputs.String(1))
		encodeDic, err := utils.Encrypt(filedata, appfiles.Key)
		if err != nil {
			return "false"
		}
		encodeDic = `// ` + appfiles.Version + "\n" + encodeDic
		file.WriteToFile(encodeDic)
		return "true"
	}
	return ""
}
