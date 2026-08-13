//go:build js

package dic

import (
	"errors"

	"github.com/cjxpj/nebula/dto"
)

// 终端.监听执行在 wasm 环境不可用
func cmdListenRun(d *dto.DicInputs) (any, error) {
	return "", errors.New("终端监听执行在 wasm 环境不可用")
}
