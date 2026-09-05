//go:build windows

package extloader

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

// winLib 基于 syscall.LoadLibrary 的 Windows 动态库句柄（纯 Go，无需 CGO）。
type winLib struct {
	dll      *syscall.LazyDLL
	extInit  *syscall.LazyProc
	extClose *syscall.LazyProc
	extCount *syscall.LazyProc
	extName  *syscall.LazyProc
	extL     *syscall.LazyProc
	extCall  *syscall.LazyProc
}

func openNative(path string) (nativeLib, error) {
	dll := syscall.NewLazyDLL(path)
	if err := dll.Load(); err != nil {
		return nil, fmt.Errorf("加载 %s 失败：%w", path, err)
	}
	n := &winLib{dll: dll}
	n.extInit = dll.NewProc("ext_init")
	n.extClose = dll.NewProc("ext_close")
	n.extCount = dll.NewProc("ext_count")
	n.extName = dll.NewProc("ext_name")
	n.extL = dll.NewProc("ext_l")
	n.extCall = dll.NewProc("ext_call")
	for _, p := range []*syscall.LazyProc{n.extInit, n.extClose, n.extCount, n.extName, n.extL, n.extCall} {
		if err := p.Find(); err != nil {
			return nil, fmt.Errorf("找不到导出符号：%w", err)
		}
	}
	return n, nil
}

// cstrToGo 把 C 字符串指针（UTF-8）复制为 Go 字符串。
// p 来自 syscall.LazyProc.Call 返回的指针值（uintptr 往返），故禁用 checkptr。
//
//go:nocheckptr
func cstrToGo(p uintptr) string {
	if p == 0 {
		return ""
	}
	ptr := unsafe.Pointer(p)
	n := 0
	for *(*byte)(unsafe.Add(ptr, n)) != 0 {
		n++
	}
	return string(unsafe.Slice((*byte)(ptr), n))
}

func (n *winLib) init() error {
	r, _, _ := n.extInit.Call()
	if r != 0 {
		return fmt.Errorf("扩展初始化失败（返回 %d）", r)
	}
	return nil
}

func (n *winLib) close() error {
	n.extClose.Call()
	return nil
}

func (n *winLib) count() int {
	r, _, _ := n.extCount.Call()
	return int(r)
}

func (n *winLib) name(index int) string {
	r, _, _ := n.extName.Call(uintptr(index))
	return cstrToGo(r)
}

func (n *winLib) l(index int) string {
	r, _, _ := n.extL.Call(uintptr(index))
	return cstrToGo(r)
}

func (n *winLib) call(name string, args []string) (string, error) {
	nameBuf := append([]byte(name), 0)
	argPtrs := make([]uintptr, len(args))
	argBufs := make([][]byte, len(args))
	for i, a := range args {
		b := append([]byte(a), 0)
		argBufs[i] = b
		argPtrs[i] = uintptr(unsafe.Pointer(&b[0]))
	}
	var argsPtr uintptr
	if len(argPtrs) > 0 {
		argsPtr = uintptr(unsafe.Pointer(&argPtrs[0]))
	}
	var errPtr *byte
	// 临时缓冲区转成 uintptr 后 GC 无法追踪，必须 KeepAlive 到跨 C 调用结束
	defer runtime.KeepAlive(nameBuf)
	defer runtime.KeepAlive(argBufs)
	defer runtime.KeepAlive(argPtrs)
	r, _, _ := n.extCall.Call(
		uintptr(unsafe.Pointer(&nameBuf[0])),
		argsPtr,
		uintptr(len(args)),
		uintptr(unsafe.Pointer(&errPtr)),
	)
	if errPtr != nil {
		return "", fmt.Errorf("%s", cstrToGo(uintptr(unsafe.Pointer(errPtr))))
	}
	return cstrToGo(r), nil
}
