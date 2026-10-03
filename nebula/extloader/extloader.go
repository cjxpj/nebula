// Package extloader 在程序启动时加载扩展动态库（Windows .dll / 其他平台 .so），
// 并将其中的字典函数注册到词库。扩展以原生 C ABI 导出固定符号，服务端据此枚举
// 函数列表并调用，使词库可用 $函数名$ 调用扩展能力。
package extloader

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// FuncInfo 扩展声明的一个字典函数。
type FuncInfo struct {
	Name string `json:"name"`
	L    string `json:"l"`
}

// nativeLib 平台相关的动态库句柄抽象，由各平台文件实现。
type nativeLib interface {
	init() error
	close() error
	count() int
	name(index int) string
	l(index int) string
	call(name string, args []string) (string, error)
}

// Lib 一个已加载的扩展。
type Lib struct {
	name   string
	native nativeLib
	Funcs  []FuncInfo

	// callMu 保证单个扩展的调用串行执行：进程内 C 扩展使用静态返回缓冲区，
	// 独立进程后端亦为「一次一个请求」的协议，二者都要求服务端串行调用。
	callMu sync.Mutex
}

var (
	mu   sync.Mutex
	libs = map[string]*Lib{} // 文件名 -> 已加载扩展（已注册函数）
)

// pluginsDir 返回扩展动态库所在目录。
func pluginsDir() string {
	return filepath.Join(utils.GetAppDir(), "private", "plugins")
}

// Load 加载指定扩展：动态库文件（.dll/.so）走进程内后端，含 plugin.json 的目录走独立进程后端。
// 随后解析其函数列表并执行初始化。
func Load(path string) (*Lib, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	var n nativeLib
	if info.IsDir() {
		n, err = openIPC(path)
	} else {
		n, err = openNative(path)
	}
	if err != nil {
		return nil, err
	}
	if err := n.init(); err != nil {
		n.close()
		return nil, err
	}
	cnt := n.count()
	if cnt < 0 {
		n.close()
		return nil, fmt.Errorf("扩展返回非法函数数量 %d", cnt)
	}
	funcs := make([]FuncInfo, 0, cnt)
	for i := 0; i < cnt; i++ {
		funcs = append(funcs, FuncInfo{Name: n.name(i), L: n.l(i)})
	}
	return &Lib{name: filepath.Base(path), native: n, Funcs: funcs}, nil
}

// Call 调用扩展中的函数，返回其执行结果。参数与返回值均为字符串。
// 对单个扩展串行加锁，避免并发调用破坏扩展的静态缓冲区/单请求协议。
func (l *Lib) Call(funcName string, args []any) (any, error) {
	strArgs := make([]string, len(args))
	for i, a := range args {
		strArgs[i] = anyToString(a)
	}
	l.callMu.Lock()
	defer l.callMu.Unlock()
	return l.native.call(funcName, strArgs)
}

// Close 关闭扩展并释放动态库。
func (l *Lib) Close() error {
	return l.native.close()
}

// scanDir 扫描扩展目录，返回其中的扩展条目名列表：
// 当前平台支持的动态库文件（.dll/.so），以及内含 plugin.json 的子目录（独立进程扩展）。
func scanDir() ([]string, error) {
	dir := pluginsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			if fileExists(filepath.Join(dir, e.Name(), ipcManifestName)) {
				names = append(names, e.Name())
			}
			continue
		}
		if isExtFile(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// fileExists 判断路径是否存在且为普通文件。
func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

// registerLib 注册某个扩展的全部函数到词库。
func registerLib(lib *Lib) {
	for _, f := range lib.Funcs {
		if err := funcs.Register(f.Name, f.L, wrapFunc(lib, f.Name)); err != nil {
			debugLog.Errorf("扩展 %s 注册函数 %s 失败：%v", lib.name, f.Name, err)
		}
	}
}

// InitAll 扫描扩展目录，加载全部扩展并注册其字典函数。程序启动时调用一次。
func InitAll() {
	names, err := scanDir()
	if err != nil {
		debugLog.Errorf("扫描扩展目录失败：%v", err)
		return
	}
	mu.Lock()
	defer mu.Unlock()
	for _, name := range names {
		lib, err := Load(filepath.Join(pluginsDir(), name))
		if err != nil {
			debugLog.Errorf("加载扩展 %s 失败：%v", name, err)
			continue
		}
		registerLib(lib)
		libs[name] = lib
	}
}

// CloseAll 关闭全部扩展并释放动态库。程序退出时调用一次。
func CloseAll() {
	mu.Lock()
	list := make([]*Lib, 0, len(libs))
	for _, l := range libs {
		list = append(list, l)
	}
	libs = map[string]*Lib{}
	mu.Unlock()
	for _, l := range list {
		if err := l.Close(); err != nil {
			debugLog.Errorf("关闭扩展 %s 失败：%v", l.name, err)
		}
	}
}

// isExtFile 判断是否为当前平台支持的扩展动态库文件。
func isExtFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if runtime.GOOS == "windows" {
		return ext == ".dll"
	}
	return ext == ".so"
}

// wrapFunc 把扩展函数封装为词库字典函数。
func wrapFunc(lib *Lib, name string) func(d *dto.DicInputs) (any, error) {
	return func(d *dto.DicInputs) (any, error) {
		var args []any
		if d.Inputs != nil && len(d.Inputs.List) > 1 {
			args = d.Inputs.List[1:]
		}
		return lib.Call(name, args)
	}
}

// anyToString 把词库传入的参数转换为字符串传给 C 扩展。
// 字符串原样返回；数字转为十进制；布尔转为 true/false；其余回退到 fmt.Sprint。
func anyToString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 32)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case bool:
		return strconv.FormatBool(x)
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}
