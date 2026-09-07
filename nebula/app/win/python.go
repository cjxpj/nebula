//go:build windows

package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
	"golang.org/x/sys/windows"
)

// defaultPyWorkers 为 Python 单进程内线程池的默认最大并发数。
// 运行时可用全局变量 _PythonWorkers_ 覆盖（单位：并发任务数）。
// 线程由 GIL 串行 CPU 密集部分，IO 密集可并行，内存开销远低于多进程。
const defaultPyWorkers = 128

// pythonServerScript 为常驻 Python 服务脚本，内部用线程池并发执行，通过 stdin/stdout 与 Go 侧通信。
// 请求 = 4 字节大端 ID + 4 字节大端长度 + UTF-8 代码；
// 响应 = 4 字节大端 ID + 1 字节状态 + 4 字节大端长度 + 结果。
// __MAX_WORKERS__ 为占位符，Go 侧写入脚本时替换为实际并发上限。
const pythonServerScript = `import os
import io
import contextlib
import traceback
import threading
from concurrent.futures import ThreadPoolExecutor

_executor = ThreadPoolExecutor(max_workers=__MAX_WORKERS__)
_write_lock = threading.Lock()


def _read_exact(n):
    buf = b""
    while len(buf) < n:
        chunk = os.read(0, n - len(buf))
        if not chunk:
            return None
        buf += chunk
    return buf


def _write_all(data):
    view = memoryview(data)
    while view:
        view = view[os.write(1, view):]


def _write_response(rid, status, data):
    with _write_lock:
        _write_all(rid.to_bytes(4, "big"))
        _write_all(bytes([status]))
        _write_all(len(data).to_bytes(4, "big"))
        _write_all(data)


def _error_hint(e):
    name = type(e).__name__
    msg = str(e)
    if name == "ModuleNotFoundError":
        mod = getattr(e, "name", "") or msg
        return "缺少 Python 库 '%s'，内置 Python 为精简版，需先安装该第三方库" % mod
    if name == "ImportError":
        return "导入失败：%s" % msg
    if name == "FileNotFoundError":
        return "文件不存在：%s" % msg
    if name == "SyntaxError":
        return "语法错误：%s" % msg
    if name == "NameError":
        return "名称未定义：%s" % msg
    if name == "TypeError":
        return "类型错误：%s" % msg
    if name == "ValueError":
        return "值错误：%s" % msg
    if name == "KeyError":
        return "键不存在：%s" % msg
    if name == "IndexError":
        return "索引越界：%s" % msg
    if name == "AttributeError":
        return "属性/方法不存在：%s" % msg
    if name == "ZeroDivisionError":
        return "除零错误"
    if name == "IndentationError":
        return "缩进错误：%s" % msg
    return "执行出错（%s）：%s" % (name, msg)


def _format_exc(e):
    frames = traceback.extract_tb(e.__traceback__)
    # 去掉服务脚本自身的调用帧，只保留用户代码调用栈
    while frames and frames[0].filename == __file__:
        frames = frames[1:]
    return "".join(["Traceback (most recent call last):\n"] +
                   traceback.format_list(frames) +
                   traceback.format_exception_only(type(e), e))


def _error_result(e, out, err):
    result = out.getvalue()
    if err.getvalue():
        result += ("\n" if result else "") + err.getvalue()
    result += ("\n" if result else "") + _format_exc(e)
    return _error_hint(e) + "\n\n" + result


def _handle(rid, code):
    out = io.StringIO()
    err = io.StringIO()
    status = 0
    try:
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            exec(compile(code, "<python>", "exec"), {"__name__": "__main__"})
        result = out.getvalue()
        if err.getvalue():
            result += ("\n" if result else "") + err.getvalue()
    except ModuleNotFoundError as e:
        mod = getattr(e, "name", None)
        if mod:
            # 状态 2：缺少模块，data 只放模块名，供上层自动安装后重跑
            result = mod
            status = 2
        else:
            result = _error_result(e, out, err)
            status = 1
    except BaseException as e:
        result = _error_result(e, out, err)
        status = 1
    _write_response(rid, status, result.encode("utf-8"))


while True:
    rid_buf = _read_exact(4)
    if rid_buf is None:
        break
    rid = int.from_bytes(rid_buf, "big")
    len_buf = _read_exact(4)
    if len_buf is None:
        break
    code_len = int.from_bytes(len_buf, "big")
    code = _read_exact(code_len)
    if code is None:
        break
    _executor.submit(_handle, rid, code.decode("utf-8"))
`

// pyWorker 为一个常驻 Python 子进程。
type pyWorker struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
}

// pyResponse 为单次执行的响应帧。
type pyResponse struct {
	status byte
	data   string
}

var (
	pyOnce    sync.Once
	pyInitErr error

	pyProcPtr atomic.Pointer[pyWorker] // 当前常驻进程
	pyStateMu sync.Mutex               // 保护 pyReqMap 与进程重启
	pyWriteMu sync.Mutex               // 串行化 stdin 写入
	pyReqMap  map[uint32]chan pyResponse
	pySeq     uint32
	pySem     chan struct{} // 并发许可证，限制同时在途的请求数

	pyExec    string
	pyScript  string
	pyJob     windows.Handle
	pyJobOnce sync.Once
)

// runPythonCode 执行一段 Python 脚本（单进程内线程池并发执行）。
// autoInstall 为 true 时，遇到缺失模块会先 pip 安装再重跑一次。
func runPythonCode(_ *dto.DicInputs, code string, autoInstall bool) (string, error) {
	pyOnce.Do(func() {
		pyInitErr = initPyPool()
	})
	if pyInitErr != nil {
		return "", pyInitErr
	}

	result, err := runPythonOnce(code)
	if err != nil && autoInstall {
		var missing *pyMissingModuleError
		if errors.As(err, &missing) && missing.module != "" {
			if installErr := pipInstallModule(missing.module); installErr != nil {
				return result, installErr
			}
			return runPythonOnce(code)
		}
	}
	return result, err
}

// runPythonOnce 提交一个请求并等待对应 ID 的响应返回。
func runPythonOnce(code string) (string, error) {
	pySem <- struct{}{}
	defer func() { <-pySem }()

	id := atomic.AddUint32(&pySeq, 1)
	ch := make(chan pyResponse, 1)

	pyStateMu.Lock()
	pyReqMap[id] = ch
	pyStateMu.Unlock()

	if err := sendPyRequest(id, code); err != nil {
		pyStateMu.Lock()
		delete(pyReqMap, id)
		pyStateMu.Unlock()
		return "", err
	}

	resp := <-ch
	if resp.status == 2 {
		return resp.data, &pyMissingModuleError{module: resp.data}
	}
	if resp.status != 0 {
		return resp.data, &pyExecError{output: resp.data}
	}
	return resp.data, nil
}

// pyWorkerLimit 返回 Python 线程池最大并发数，默认 defaultPyWorkers，可用 _PythonWorkers_ 覆盖。
func pyWorkerLimit() int {
	if n := dto.GV.GetINT("_PythonWorkers_"); n > 0 {
		return n
	}
	if s := dto.GV.GetStr("_PythonWorkers_"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	return defaultPyWorkers
}

// sendPyRequest 写一帧请求；进程已死时重启并重试一次。
func sendPyRequest(id uint32, code string) error {
	pyWriteMu.Lock()
	defer pyWriteMu.Unlock()

	werr := writePyFrame(id, code)
	if werr == nil {
		return nil
	}

	// 写失败说明进程已退出，串行化重启后重试一次。
	pyStateMu.Lock()
	rerr := restartPyProc()
	pyStateMu.Unlock()
	if rerr != nil {
		return fmt.Errorf("写入 Python 请求失败: %v；重启进程失败: %v", werr, rerr)
	}
	return writePyFrame(id, code)
}

// writePyFrame 写一帧请求：4 字节 ID + 4 字节长度 + 代码。
func writePyFrame(id uint32, code string) error {
	w := pyProcPtr.Load()
	if w == nil {
		return errors.New("Python 进程未就绪")
	}

	var idBuf [4]byte
	binary.BigEndian.PutUint32(idBuf[:], id)
	if _, err := w.stdin.Write(idBuf[:]); err != nil {
		return fmt.Errorf("写入 Python 请求失败: %v", err)
	}

	data := []byte(code)
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data)))
	if _, err := w.stdin.Write(lenBuf[:]); err != nil {
		return fmt.Errorf("写入 Python 请求失败: %v", err)
	}
	if _, err := w.stdin.Write(data); err != nil {
		return fmt.Errorf("写入 Python 代码失败: %v", err)
	}
	return nil
}

// restartPyProc 关闭旧进程并启动新进程（调用方需持有 pyStateMu）。
func restartPyProc() error {
	if old := pyProcPtr.Swap(nil); old != nil {
		old.close()
	}
	w, err := startPyWorker()
	if err != nil {
		return err
	}
	pyProcPtr.Store(w)
	startPyReadLoop(w)
	return nil
}

// startPyReadLoop 启动读循环，解析响应帧并按 ID 分发到对应等待者。
func startPyReadLoop(w *pyWorker) {
	go func() {
		defer func() {
			// 进程退出：通知所有等待中的请求失败并清空映射。
			pyStateMu.Lock()
			for id, ch := range pyReqMap {
				select {
				case ch <- pyResponse{status: 1, data: "Python 进程已退出"}:
				default:
				}
				delete(pyReqMap, id)
			}
			if pyProcPtr.Load() == w {
				pyProcPtr.Store(nil)
			}
			pyStateMu.Unlock()
		}()
		for {
			var idBuf [4]byte
			if _, err := io.ReadFull(w.stdout, idBuf[:]); err != nil {
				return
			}
			id := binary.BigEndian.Uint32(idBuf[:])

			var statusBuf [1]byte
			if _, err := io.ReadFull(w.stdout, statusBuf[:]); err != nil {
				return
			}

			var lenBuf [4]byte
			if _, err := io.ReadFull(w.stdout, lenBuf[:]); err != nil {
				return
			}
			n := binary.BigEndian.Uint32(lenBuf[:])
			data := make([]byte, n)
			if _, err := io.ReadFull(w.stdout, data); err != nil {
				return
			}

			pyStateMu.Lock()
			ch, ok := pyReqMap[id]
			delete(pyReqMap, id)
			pyStateMu.Unlock()
			if ok {
				select {
				case ch <- pyResponse{status: statusBuf[0], data: string(data)}:
				default:
				}
			}
		}
	}()
}

// pipInstallModule 使用内置 Python 安装缺失的第三方模块。
func pipInstallModule(module string) error {
	cmd := exec.Command(pyExec, "-m", "pip", "install", module)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		// 内置 Python 未带 pip 时，python -m pip 会输出 "No module named pip"
		if strings.Contains(msg, "No module named pip") {
			return fmt.Errorf("内置 Python 未安装 pip，无法自动安装模块 '%s'，请手动安装该模块，或在管理后台「扩展部署」页面部署带 pip 的 Python 环境", module)
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("自动安装模块 %s 失败: %s", module, msg)
	}
	return nil
}

// initPyPool 写入服务脚本并启动单个常驻 Python 进程。
func initPyPool() error {
	appDir := utils.GetAppDir()
	pythonDir := filepath.Join(appDir, "private", "extensions", "python")
	if err := os.MkdirAll(pythonDir, 0755); err != nil {
		return fmt.Errorf("创建 Python 目录失败: %v", err)
	}

	n := pyWorkerLimit()
	pyScript = filepath.Join(pythonDir, "_nebula_python_server.py")
	script := strings.Replace(pythonServerScript, "__MAX_WORKERS__", strconv.Itoa(n), 1)
	if err := os.WriteFile(pyScript, []byte(script), 0644); err != nil {
		return fmt.Errorf("写入 Python 服务脚本失败: %v", err)
	}

	pyExec = dto.GV.GetStr("_Python_")
	if pyExec == "" {
		return fmt.Errorf("未设置 Python 执行路径 (_Python_)")
	}
	if _, err := exec.LookPath(pyExec); err != nil {
		return fmt.Errorf("Python 启动失败: 未找到 Python 运行环境 (%s)", pyExec)
	}

	initPyJob()

	pySem = make(chan struct{}, n)
	pyReqMap = make(map[uint32]chan pyResponse)

	if err := restartPyProc(); err != nil {
		return fmt.Errorf("启动 Python 进程失败: %v", err)
	}
	return nil
}

// startPyWorker 启动一个常驻 Python 子进程。
func startPyWorker() (*pyWorker, error) {
	cmd := exec.Command(pyExec, pyScript)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// 绑定到 Job Object，Go 崩溃时 OS 自动终止子进程。
	if pyJob != 0 {
		if procHandle, herr := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.PROCESS_SET_QUOTA, false, uint32(cmd.Process.Pid)); herr == nil {
			_ = windows.AssignProcessToJobObject(pyJob, procHandle)
			windows.CloseHandle(procHandle)
		}
	}

	return &pyWorker{cmd: cmd, stdin: stdin, stdout: bufio.NewReader(stdout)}, nil
}

// pyExecError 表示 Python 代码执行失败（进程本身健康）。
type pyExecError struct {
	output string
}

func (e *pyExecError) Error() string {
	return "Python 执行失败:\n" + e.output
}

// pyMissingModuleError 表示缺少第三方模块（进程健康，可安装后重跑）。
type pyMissingModuleError struct {
	module string
}

func (e *pyMissingModuleError) Error() string {
	return "缺少 Python 库 '" + e.module + "'"
}

// close 终止 worker 并释放资源。
func (w *pyWorker) close() {
	if w.cmd.Process != nil {
		_ = w.cmd.Process.Kill()
	}
	if w.stdin != nil {
		_ = w.stdin.Close()
	}
}

// initPyJob 创建带 KILL_ON_JOB_CLOSE 的 Job Object。
func initPyJob() {
	pyJobOnce.Do(func() {
		job, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			return
		}
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
			BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
				LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
			},
		}
		if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
			windows.CloseHandle(job)
			return
		}
		pyJob = job
	})
}

// shutdownPyPool 关闭 Python 进程。
func shutdownPyPool() {
	if w := pyProcPtr.Load(); w != nil {
		w.close()
		pyProcPtr.Store(nil)
	}
	if pyJob != 0 {
		windows.CloseHandle(pyJob)
		pyJob = 0
	}
}

// ShutdownPython 关闭 Python 进程（main 退出前调用）。
func ShutdownPython() {
	shutdownPyPool()
}
