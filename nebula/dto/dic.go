package dto

import "github.com/cjxpj/nebula/utils"

func NewDicInputs(dic *BuildValue, v *DicVal, i *utils.DicInputs) *DicInputs {
	return &DicInputs{
		Dic:    dic,
		V:      v,
		Inputs: i,
	}
}
