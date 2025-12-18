package funcs

import (
	"encoding/base64"

	"github.com/cjxpj/nebula/dto"
)

func base64En(d *dto.DicInputs) (any, error) {
	return base64.StdEncoding.EncodeToString([]byte(d.Inputs.String(1))), nil
}

func base64De(d *dto.DicInputs) (any, error) {
	data, err := base64.StdEncoding.DecodeString(d.Inputs.String(1))
	if err != nil {
		return "", nil
	}
	return string(data), nil
}
