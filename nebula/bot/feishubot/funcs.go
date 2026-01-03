package feishubot

import "github.com/cjxpj/nebula/dto"

var Funcs = map[string]dto.DicFunc{
	"图片": {
		L: "2",
		Fn: func(d *dto.DicInputs) (any, error) {
			list, err := GetImageMsg(d.Inputs.String(1), d.Inputs.String(2))
			return string(list), err
		},
	},
}
