//go:build !windows

package funcs

import (
	"github.com/cjxpj/nebula/dto"
)

// 获取文件属性：非 Windows 平台不支持，返回空对象。
func fileAttributeGet(d *dto.DicInputs) (any, error) {
	return "{}", nil
}

// 设置文件属性：非 Windows 平台不支持。
func fileAttributeSet(d *dto.DicInputs) (any, error) {
	return "false", nil
}
