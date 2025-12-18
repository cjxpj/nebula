package funcs

import "github.com/cjxpj/nebula/dto"

func ifNull(d *dto.DicInputs) (any, error) {
	switch d.Inputs.String(1) {
	case "null",
		"nil",
		"false",
		"{}",
		"[]",
		"空",
		"NaN",
		"undefined",
		" ",
		"":
		return "true", nil
	}
	return "false", nil
}

func ifNONull(d *dto.DicInputs) (any, error) {
	switch d.Inputs.String(1) {
	case "null",
		"nil",
		"false",
		"{}",
		"[]",
		"空",
		"NaN",
		"undefined",
		" ",
		"":
		return "false", nil
	}
	return "true", nil
}
