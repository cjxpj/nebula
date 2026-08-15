//go:build js

package funcs

import (
	"errors"

	"github.com/cjxpj/nebula/dto"
)

// errWasmUnsupported 浏览器（wasm）环境下终端相关功能不可用。
var errWasmUnsupported = errors.New("该功能在 wasm 环境不可用")

// restart 重启进程：wasm 环境无独立进程，直接返回错误。
func restart(d *dto.DicInputs) (any, error) {
	return "", errWasmUnsupported
}

func runCommandNew(d *dto.DicInputs) (any, error) {
	return nil, errWasmUnsupported
}

func runCommandShellNew(d *dto.DicInputs) (any, error) {
	return nil, errWasmUnsupported
}

func runCommandDir(d *dto.DicInputs) (any, error) {
	return "", errWasmUnsupported
}

func runCommand(d *dto.DicInputs) (any, error) {
	return "", errWasmUnsupported
}

func runCommandAsync(d *dto.DicInputs) (any, error) {
	return "", errWasmUnsupported
}

func runCommandDecoder(d *dto.DicInputs) (any, error) {
	return "", errWasmUnsupported
}

func runCommandVar(d *dto.DicInputs) (any, error) {
	return "", errWasmUnsupported
}

func runCommandClose(d *dto.DicInputs) (any, error) {
	return "false", nil
}

func runCommandInputText(d *dto.DicInputs) (any, error) {
	return "", errWasmUnsupported
}
