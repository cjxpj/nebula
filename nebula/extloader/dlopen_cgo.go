//go:build !windows && cgo

package extloader

/*
#cgo linux,!ohos LDFLAGS: -ldl
#include <dlfcn.h>
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// unixLib 基于 dlopen 的动态库句柄（Linux / Android / 鸿蒙，需 CGO）。
type unixLib struct {
	handle   unsafe.Pointer
	extInit  unsafe.Pointer
	extClose unsafe.Pointer
	extCount unsafe.Pointer
	extName  unsafe.Pointer
	extL     unsafe.Pointer
	extCall  unsafe.Pointer
}

type (
	cInit  func() C.int
	cClose func()
	cCount func() C.int
	cName  func(C.int) *C.char
	cL     func(C.int) *C.char
	cCall  func(*C.char, **C.char, C.int, **C.char) *C.char
)

func openNative(path string) (nativeLib, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	h := C.dlopen(cpath, C.RTLD_NOW)
	if h == nil {
		return nil, fmt.Errorf("dlopen %s 失败：%s", path, C.GoString(C.dlerror()))
	}
	n := &unixLib{handle: h}
	for _, s := range []struct {
		name string
		dst  *unsafe.Pointer
	}{
		{"ext_init", &n.extInit},
		{"ext_close", &n.extClose},
		{"ext_count", &n.extCount},
		{"ext_name", &n.extName},
		{"ext_l", &n.extL},
		{"ext_call", &n.extCall},
	} {
		p, err := n.sym(s.name)
		if err != nil {
			C.dlclose(h)
			return nil, err
		}
		*s.dst = p
	}
	return n, nil
}

func (n *unixLib) sym(name string) (unsafe.Pointer, error) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	p := C.dlsym(n.handle, cname)
	if p == nil {
		return nil, fmt.Errorf("找不到导出符号 %s", name)
	}
	return p, nil
}

func (n *unixLib) init() error {
	fn := *(*cInit)(unsafe.Pointer(&n.extInit))
	if r := fn(); r != 0 {
		return fmt.Errorf("扩展初始化失败（返回 %d）", int(r))
	}
	return nil
}

func (n *unixLib) close() error {
	fn := *(*cClose)(unsafe.Pointer(&n.extClose))
	fn()
	C.dlclose(n.handle)
	return nil
}

func (n *unixLib) count() int {
	fn := *(*cCount)(unsafe.Pointer(&n.extCount))
	return int(fn())
}

func (n *unixLib) name(index int) string {
	fn := *(*cName)(unsafe.Pointer(&n.extName))
	p := fn(C.int(index))
	if p == nil {
		return ""
	}
	return C.GoString(p)
}

func (n *unixLib) l(index int) string {
	fn := *(*cL)(unsafe.Pointer(&n.extL))
	p := fn(C.int(index))
	if p == nil {
		return ""
	}
	return C.GoString(p)
}

func (n *unixLib) call(name string, args []string) (string, error) {
	fn := *(*cCall)(unsafe.Pointer(&n.extCall))
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	cargs := make([]*C.char, len(args))
	for i, a := range args {
		cargs[i] = C.CString(a)
	}
	defer func() {
		for _, p := range cargs {
			C.free(unsafe.Pointer(p))
		}
	}()

	var cargsPtr **C.char
	if len(cargs) > 0 {
		cargsPtr = &cargs[0]
	}
	var cerr *C.char
	r := fn(cname, cargsPtr, C.int(len(args)), &cerr)
	if cerr != nil {
		return "", fmt.Errorf("%s", C.GoString(cerr))
	}
	if r == nil {
		return "", nil
	}
	return C.GoString(r), nil
}
