package funcs

import (
	"slices"
	"strconv"
	"strings"

	"github.com/cjxpj/nebula/dto"
)

func split(d *dto.DicInputs) (any, error) {
	if d.Inputs.LenOk(3) {
		var z int
		if zz, err := strconv.Atoi(d.Inputs.String(3)); err == nil {
			z = zz
		}
		parts := strings.SplitN(d.Inputs.String(2), d.Inputs.String(1), z)
		resS, err := json.Marshal(parts)
		if err != nil {
			return "[]", nil
		}
		return string(resS), nil
	}
	parts := strings.Split(d.Inputs.String(2), d.Inputs.String(1))

	resS, err := json.Marshal(parts)
	if err != nil {
		return "[]", nil
	}
	return string(resS), nil
}

// 分割匹配
func splitMatch(d *dto.DicInputs) (any, error) {
	parts := strings.Split(d.Inputs.String(2), d.Inputs.String(1))
	isStr := d.Inputs.String(3)
	// 匹配是否存在
	if slices.Contains(parts, isStr) {
		return "true", nil
	}
	return "false", nil
}

func stringSlice(d *dto.DicInputs) (any, error) {
	r := []rune(d.Inputs.String(1))
	stringlist := make([]string, len(r))
	for i, r := range r {
		stringlist[i] = string(r)
	}

	var jsonBuilder strings.Builder
	encoder := json.NewEncoder(&jsonBuilder)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(stringlist)
	if err != nil {
		return "[]", nil
	}

	return jsonBuilder.String(), nil
}
