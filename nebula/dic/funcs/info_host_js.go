//go:build js

package funcs

import (
	"errors"

	"github.com/cjxpj/nebula/dto"
)

// host_information 主机信息：浏览器（wasm）环境无法读取系统信息。
func host_information(d *dto.DicInputs) (any, error) {
	return "", errors.New("主机信息在 wasm 环境不可用")
}
