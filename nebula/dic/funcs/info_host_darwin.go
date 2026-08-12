//go:build darwin

package funcs

import (
	"errors"

	"github.com/cjxpj/nebula/dto"
)

func host_information(d *dto.DicInputs) (any, error) {
	return nil, errors.New("不支持 macOS")
}
