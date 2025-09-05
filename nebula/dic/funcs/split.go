package funcs

import (
	"errors"
	"strconv"
	"strings"
)

func (f *DicFunc) Split() (string, error) {
	if f.Len == 3 {
		var z int
		if zz, err := strconv.Atoi(f.Inputs.String(3)); err == nil {
			z = zz
		}
		parts := strings.SplitN(f.Inputs.String(2), f.Inputs.String(1), z)
		resS, err := json.Marshal(parts)
		if err != nil {
			return "[]", nil
		}
		return string(resS), nil
	}
	if f.Len == 2 {
		parts := strings.Split(f.Inputs.String(2), f.Inputs.String(1))

		resS, err := json.Marshal(parts)
		if err != nil {
			return "[]", nil
		}
		return string(resS), nil
	}
	return "", errors.New("参数数量不正确")
}

func (f *DicFunc) StringSlice() string {
	if f.Len == 1 {
		r := []rune(f.Inputs.String(1))
		stringlist := make([]string, len(r))
		for i, r := range r {
			stringlist[i] = string(r)
		}

		var jsonBuilder strings.Builder
		encoder := json.NewEncoder(&jsonBuilder)
		encoder.SetEscapeHTML(false)
		err := encoder.Encode(stringlist)
		if err != nil {
			return "[]"
		}

		return jsonBuilder.String()
	}
	return ""
}
