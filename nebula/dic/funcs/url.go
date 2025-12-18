package funcs

import (
	"net/url"

	"github.com/cjxpj/nebula/dto"
)

func urlEn(d *dto.DicInputs) (any, error) {
	return url.QueryEscape(d.Inputs.String(1)), nil
}

func urlDe(d *dto.DicInputs) (any, error) {
	data, err := url.QueryUnescape(d.Inputs.String(1))
	if err != nil {
		return "", nil
	}
	return data, nil
}

func urlPathEn(d *dto.DicInputs) (any, error) {
	return url.PathEscape(d.Inputs.String(1)), nil
}

func urlPathDe(d *dto.DicInputs) (any, error) {
	data, err := url.PathUnescape(d.Inputs.String(1))
	if err != nil {
		return "", nil
	}
	return data, nil
}
