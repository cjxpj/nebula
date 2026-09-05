//go:build !windows && !cgo

package extloader

import "fmt"

// stubLib 非 CGO 构建下的占位实现：不支持加载动态库扩展。
type stubLib struct{}

func openNative(path string) (nativeLib, error) {
	return nil, fmt.Errorf("当前平台/构建未启用 CGO，不支持加载动态库扩展")
}

func (n *stubLib) init() error                           { return fmt.Errorf("不支持") }
func (n *stubLib) close() error                          { return nil }
func (n *stubLib) count() int                            { return 0 }
func (n *stubLib) name(int) string                       { return "" }
func (n *stubLib) l(int) string                          { return "" }
func (n *stubLib) call(string, []string) (string, error) { return "", fmt.Errorf("不支持") }
