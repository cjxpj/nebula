package funcs

import (
	"strconv"

	"github.com/cjxpj/nebula/dto"
)

func stringSliceLen(d *dto.DicInputs) (any, error) {
	r := []rune(d.Inputs.String(1))
	return strconv.Itoa(len(r)), nil
}

func stringLen(d *dto.DicInputs) (any, error) {
	return strconv.Itoa(len(d.Inputs.String(1))), nil
}
