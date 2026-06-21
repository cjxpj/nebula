package funcs

import (
	"strings"

	"github.com/cjxpj/nebula/dto"
)

func (f *DicFunc) ToUpper() string {
	if f.Len == 1 {
		uppercase := strings.ToUpper(f.Inputs.String(1))
		return uppercase
	}
	return ""
}

func (f *DicFunc) ToLower() string {
	if f.Len == 1 {
		lowercase := strings.ToLower(f.Inputs.String(1))
		return lowercase
	}
	return ""
}

func toUpper(d *dto.DicInputs) (any, error) {
	return strings.ToUpper(d.Inputs.String(1)), nil
}

func toLower(d *dto.DicInputs) (any, error) {
	return strings.ToLower(d.Inputs.String(1)), nil
}
