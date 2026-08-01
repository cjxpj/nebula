package funcs

import (
	"strings"

	"github.com/cjxpj/nebula/dto"
)

func toUpper(d *dto.DicInputs) (any, error) {
	return strings.ToUpper(d.Inputs.String(1)), nil
}

func toLower(d *dto.DicInputs) (any, error) {
	return strings.ToLower(d.Inputs.String(1)), nil
}
