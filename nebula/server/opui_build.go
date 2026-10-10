package dic_server

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cjxpj/nebula/run"
	"github.com/cjxpj/nebula/utils"
)

func opuiAppDir() string {
	if d := utils.GetAppDir(); d != "" {
		return d
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

// nebulaSrcRoot 探测 Nebula 源码根目录（含 go.work 的目录）。
// 优先读取环境变量 NEBULA_SRC；开发模式下（未 -trimpath）从当前源文件反推。
func nebulaSrcRoot() (string, error) {
	if s := strings.TrimSpace(os.Getenv("NEBULA_SRC")); s != "" {
		if fi, err := os.Stat(filepath.Join(s, "go.work")); err == nil && !fi.IsDir() {
			return filepath.Clean(s), nil
		}
	}
	if _, file, _, ok := runtime.Caller(0); ok && file != "" {
		// 忽略 trimpath 编译得到的模块路径（如 github.com/cjxpj/nebula/server/opui.go）
		if !strings.HasPrefix(file, "github.com/") {
			dir := filepath.Dir(file) // server
			dir = filepath.Dir(dir)   // nebula
			dir = filepath.Dir(dir)   // 源码根
			if fi, err := os.Stat(filepath.Join(dir, "go.work")); err == nil && !fi.IsDir() {
				return filepath.Clean(dir), nil
			}
		}
	}
	return "", errors.New("未找到 Nebula 源码目录（需含 go.work），请设置环境变量 NEBULA_SRC 指向源码根目录后重试")
}

// nebulaModuleRoot 定位 Nebula 源码 module 根目录（含 go.mod，即 module github.com/cjxpj/nebula）。
// 仓库根（含 go.work）下的 nebula/ 子目录即为 module 根；同时兼容 NEBULA_SRC 直接指向 module 根的场景。
// 统一此定位后，linux 打包与 android 打包可共用同一个 NEBULA_SRC（仓库根）。
func nebulaModuleRoot() (string, error) {
	// 1) NEBULA_SRC 直接指向 module 根（含 go.mod）
	if s := strings.TrimSpace(os.Getenv("NEBULA_SRC")); s != "" {
		if fi, err := os.Stat(filepath.Join(s, "go.mod")); err == nil && !fi.IsDir() {
			return filepath.Clean(s), nil
		}
	}
	// 2) 仓库根（含 go.work）下的 nebula/ 子目录
	if root, err := nebulaSrcRoot(); err == nil {
		mod := filepath.Join(root, "nebula")
		if fi, err := os.Stat(filepath.Join(mod, "go.mod")); err == nil && !fi.IsDir() {
			return filepath.Clean(mod), nil
		}
	}
	return "", errors.New("未找到 Nebula 源码 module 目录（含 go.mod），请设置环境变量 NEBULA_SRC 指向仓库根（含 go.work 与 nebula/）")
}

// buildTask 词库打包任务的运行状态（异步执行，前端按 taskId 轮询）。
type buildTask struct {
	mu        sync.Mutex
	done      bool
	output    string // 产物绝对路径
	log       string // 构建日志
	err       string // 构建错误
	runOutput string // 运行输出（触发词非空时）
	runErr    string // 运行错误
}

// finish 写入任务结果并标记完成。
func (t *buildTask) finish(output, log, err, runOutput, runErr string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.output = output
	t.log = log
	t.err = err
	t.runOutput = runOutput
	t.runErr = runErr
	t.done = true
}

// snapshot 返回任务的当前状态快照。
func (t *buildTask) snapshot() (done bool, output, log, err, runOutput, runErr string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.done, t.output, t.log, t.err, t.runOutput, t.runErr
}

// buildTasks 词库打包任务表（taskId -> *buildTask）。
var buildTasks sync.Map

// dicExecMainTemplate 生成嵌入「已编译词库产物」的独立可执行程序入口源码（纯 Go，无需引擎源码）。
// 原理：把预编译的 nebula.dll（C ABI 动态库）与词库编译产物（compiled.gob）一起嵌入 exe，
// 运行时释放 dll 后通过 syscall 动态加载，调用导出函数 RunCompiled(data, len, trigger) 执行，
// 运行时不再重新编译、不读取 #引入 依赖文件与 //@资源 资源文件。
func dicExecMainTemplate() string {
	return "//go:build windows\n\n" +
		"package main\n\n" +
		"import (\n" +
		"\t_ \"embed\"\n" +
		"\t\"fmt\"\n" +
		"\t\"os\"\n" +
		"\t\"path/filepath\"\n" +
		"\t\"syscall\"\n" +
		"\t\"unsafe\"\n" +
		")\n\n" +
		"//go:embed nebula.dll\n" +
		"var dllData []byte\n\n" +
		"// compiledData 打包进可执行文件的词库编译产物（gob 字节）。\n" +
		"//go:embed compiled.gob\n" +
		"var compiledData []byte\n\n" +
		"func main() {\n" +
		"\ttmpDir, err := os.MkdirTemp(\"\", \"nebula-dic-*\")\n" +
		"\tif err != nil {\n" +
		"\t\tfmt.Fprintln(os.Stderr, \"创建临时目录失败:\", err)\n" +
		"\t\tos.Exit(1)\n" +
		"\t}\n" +
		"\tdefer os.RemoveAll(tmpDir)\n\n" +
		"\tdllPath := filepath.Join(tmpDir, \"nebula.dll\")\n" +
		"\tif err := os.WriteFile(dllPath, dllData, 0o644); err != nil {\n" +
		"\t\tfmt.Fprintln(os.Stderr, \"释放 nebula.dll 失败:\", err)\n" +
		"\t\tos.Exit(1)\n" +
		"\t}\n\n" +
		"\tdll, err := syscall.LoadLibrary(dllPath)\n" +
		"\tif err != nil {\n" +
		"\t\tfmt.Fprintln(os.Stderr, \"加载 nebula.dll 失败:\", err)\n" +
		"\t\tos.Exit(1)\n" +
		"\t}\n" +
		"\t// 不调用 FreeLibrary：Go c-shared 库的独立 runtime 无法安全卸载，卸载会在\n" +
		"\t// 进程退出时触发访问冲突（0xc0000005）。dll 随进程退出由操作系统回收。\n\n" +
		"\trunCompiled, err := syscall.GetProcAddress(dll, \"RunCompiled\")\n" +
		"\tif err != nil {\n" +
		"\t\tfmt.Fprintln(os.Stderr, \"未找到导出函数 RunCompiled:\", err)\n" +
		"\t\tos.Exit(1)\n" +
		"\t}\n" +
		"\tfreeStr, err := syscall.GetProcAddress(dll, \"FreeString\")\n" +
		"\tif err != nil {\n" +
		"\t\tfmt.Fprintln(os.Stderr, \"未找到导出函数 FreeString:\", err)\n" +
		"\t\tos.Exit(1)\n" +
		"\t}\n\n" +
		"\tvar dataPtr uintptr\n" +
		"\tif len(compiledData) > 0 {\n" +
		"\t\tdataPtr = uintptr(unsafe.Pointer(&compiledData[0]))\n" +
		"\t}\n" +
		"\ttrigB := append([]byte(\"Main\"), 0)\n\n" +
		"\tr, _, callErr := syscall.SyscallN(\n" +
		"\t\trunCompiled,\n" +
		"\t\tdataPtr,\n" +
		"\t\tuintptr(len(compiledData)),\n" +
		"\t\tuintptr(unsafe.Pointer(&trigB[0])),\n" +
		"\t)\n" +
		"\tif callErr != 0 {\n" +
		"\t\tfmt.Fprintln(os.Stderr, \"调用 RunCompiled 失败:\", callErr)\n" +
		"\t\tos.Exit(1)\n" +
		"\t}\n" +
		"\tif r == 0 {\n" +
		"\t\tfmt.Fprintln(os.Stderr, \"RunCompiled 返回空指针\")\n" +
		"\t\tos.Exit(1)\n" +
		"\t}\n\n" +
		"\tresult := cString(r)\n" +
		"\t_, _, _ = syscall.SyscallN(freeStr, r)\n\n" +
		"\tfmt.Println(result)\n" +
		"}\n\n" +
		"// cString 读取以 NUL 结尾的 C 字符串（UTF-8）。\n" +
		"func cString(p uintptr) string {\n" +
		"\tif p == 0 {\n" +
		"\t\treturn \"\"\n" +
		"\t}\n" +
		"\tvar buf []byte\n" +
		"\tfor {\n" +
		"\t\tb := *(*byte)(unsafe.Pointer(p))\n" +
		"\t\tif b == 0 {\n" +
		"\t\t\tbreak\n" +
		"\t\t}\n" +
		"\t\tbuf = append(buf, b)\n" +
		"\t\tp++\n" +
		"\t}\n" +
		"\treturn string(buf)\n" +
		"}\n"
}

// locateNebulaDLL 查找预编译的 nebula.dll（Windows x64 动态库）。
// 查找顺序：环境变量 NEBULA_DLL → 应用数据目录 private/build/dist/nebula.dll
// → 源码根 dist/sdk/windows-amd64/nebula.dll → 当前发布版本下载。
func locateNebulaDLL() (string, error) {
	if s := strings.TrimSpace(os.Getenv("NEBULA_DLL")); s != "" {
		if fi, err := os.Stat(s); err == nil && !fi.IsDir() {
			return s, nil
		}
	}
	if appDLL, err := filepath.Abs(filepath.Join(utils.GetAppDir(), "private", "build", "dist", "nebula.dll")); err == nil {
		if fi, err := os.Stat(appDLL); err == nil && !fi.IsDir() {
			return appDLL, nil
		}
	}
	if src, err := nebulaSrcRoot(); err == nil {
		sdkDLL := filepath.Join(src, "dist", "sdk", "windows-amd64", "nebula.dll")
		if fi, err := os.Stat(sdkDLL); err == nil && !fi.IsDir() {
			return sdkDLL, nil
		}
	}
	// 本地找不到时，从当前发布版本下载 nebula.dll（扩展下载）。
	return downloadNebulaDLL()
}

// pickNebulaDLL 从 release assets 中选出 nebula.dll（Windows x64 SDK 动态库）的下载地址。
func pickNebulaDLL(assets []struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}) (downURL string, ok bool) {
	// 1) 精确匹配 nebula.dll
	for _, a := range assets {
		if strings.EqualFold(a.Name, "nebula.dll") {
			return a.BrowserDownloadURL, true
		}
	}
	// 2) 模糊匹配：.dll 且含 windows/amd64/x64 关键词
	for _, a := range assets {
		lower := strings.ToLower(a.Name)
		if strings.HasSuffix(lower, ".dll") &&
			(strings.Contains(lower, "windows") || strings.Contains(lower, "amd64") || strings.Contains(lower, "x64")) {
			return a.BrowserDownloadURL, true
		}
	}
	// 3) 兜底：以 nebula 开头且以 .dll 结尾
	for _, a := range assets {
		lower := strings.ToLower(a.Name)
		if strings.HasPrefix(lower, "nebula") && strings.HasSuffix(lower, ".dll") {
			return a.BrowserDownloadURL, true
		}
	}
	return "", false
}

// downloadNebulaDLL 从当前发布版本下载 nebula.dll（Gitee 优先，回退 GitHub），
// 保存到 private/build/dist/nebula.dll，返回绝对路径。
func downloadNebulaDLL() (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	var downURL string
	for _, u := range []string{
		"https://gitee.com/api/v5/repos/cjxpj/nebula/releases/latest",
		"https://api.github.com/repos/cjxpj/nebula/releases/latest",
	} {
		_, _, _, assets := fetchReleaseAssets(client, u)
		if u2, ok := pickNebulaDLL(assets); ok {
			downURL = u2
			break
		}
	}
	if downURL == "" {
		return "", errors.New("发布版本中未找到 nebula.dll，请先在发布版本上传 SDK 产物")
	}

	destDir, err := filepath.Abs(filepath.Join(utils.GetAppDir(), "private", "build", "dist"))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("创建产物目录失败: %w", err)
	}
	destPath := filepath.Join(destDir, "nebula.dll")

	// 已存在且非空则直接复用，避免重复下载。
	if fi, err := os.Stat(destPath); err == nil && fi.Size() > 0 {
		return destPath, nil
	}

	if err := utils.NewFileQueue(destPath).DownloadWithMirrors([]string{downURL}, 0, true, nil); err != nil {
		return "", fmt.Errorf("下载 nebula.dll 失败: %w", err)
	}
	return destPath, nil
}

// findGoExe 定位「扩展部署」的 Go 工具链（private/extensions/go/bin/go.exe）。
// 该目录由 OPUI「扩展部署」下载到应用数据目录；扩展 Go 缺失时上层用 resolveGoExe
// 回退 PATH 中的 go（词库运行器在独立 module 源码现编、仅依赖标准库，无需特定 Go 版本）。
// 返回绝对路径，规避 exec 在子进程工作目录下解析相对路径失败的问题。
func findGoExe() (string, error) {
	extGo := filepath.Join(utils.GetAppDir(), "private", "extensions", "go", "bin", "go.exe")
	if abs, err := filepath.Abs(extGo); err == nil {
		extGo = abs
	}
	if fileExists(extGo) {
		return extGo, nil
	}
	return "", errors.New("未检测到词库编译环境（Go 工具链），请先在「扩展部署」下载词库编译环境")
}

// resolveGoExe 定位可用 Go 工具链：优先「扩展部署」的 go.exe，
// 其次 PATH 中的 go（开发机自带 / 源码现编场景），供 exe / linux 打包共用。
func resolveGoExe() (string, error) {
	if ext, err := findGoExe(); err == nil {
		return ext, nil
	}
	if p, err := exec.LookPath("go"); err == nil {
		return p, nil
	}
	return "", errors.New("未检测到 Go 工具链：扩展部署 Go 与 PATH 中的 go 均不可用")
}

// linuxDicMainTemplate 生成 Linux 独立运行器入口源码：在 Nebula 源码 module 内编译，
// 直接 import 引擎包执行内嵌的词库 gob（源码现编，不需要 .so/dll/dlopen，产物为单文件 ELF）。
func linuxDicMainTemplate() string {
	return `package main

import (
	_ "embed"
	"fmt"
	"os"

	_ "github.com/cjxpj/nebula/dic" // 触发引擎初始化：注入 dic_api.Api 并注册内置函数
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"
)

//go:embed compiled.gob
var compiledData []byte

func main() {
	trigger := "Main"
	if len(os.Args) > 1 && os.Args[1] != "" {
		trigger = os.Args[1]
	}
	bv, err := run.UnmarshalBuildValue(compiledData)
	if err != nil {
		fmt.Fprintln(os.Stderr, "编译产物加载失败:", err)
		os.Exit(1)
	}
	d := &dic_dto.Dic{Data: bv, Val: dto.NewDicVal(), MyFunc: bv.MyFunc}
	fmt.Println(dic_api.Api.DicRun(d, trigger))
}
`
}

// buildLinuxDicBundle 把词库打包为 Linux 独立单文件运行器（源码现编）：
//  1. 词库编译产物（gob）与运行器 main.go 一起写入 Nebula 源码内的临时包目录；
//  2. 用 goExe 在该源码 module 中编译（import 引擎包，go.mod/go.sum 齐备即可，无需联网下载）；
//  3. 纯 Go 静态 ELF（CGO_ENABLED=0），可 Windows 交叉编译，运行时不依赖 .so/dll，
//     直接以 Main/命令行触发词执行。
func buildLinuxDicBundle(dicPath, goExe, goarch string) (outPath, logText string, err error) {
	content, err := utils.NewFileQueue(dicPath).ReadFromFile()
	if err != nil {
		return "", "", fmt.Errorf("词库读取失败: %w", err)
	}
	buildValue := run.BuildDic(dicPath, content)
	gobData, err := run.MarshalBuildValue(buildValue)
	if err != nil {
		return "", "", err
	}

	// 源码 module 根：编译运行器必须在 Nebula module 内，才能 import 引擎包。
	srcRoot, err := nebulaModuleRoot()
	if err != nil {
		return "", "", err
	}
	if srcRoot, err = filepath.Abs(srcRoot); err != nil {
		return "", "", fmt.Errorf("解析源码 module 根失败: %w", err)
	}

	// 产物输出到应用数据目录 private/build/dist/。
	appBuildDir, err := filepath.Abs(filepath.Join(utils.GetAppDir(), "private", "build", "dist"))
	if err != nil {
		return "", "", fmt.Errorf("解析产物目录失败: %w", err)
	}
	if err = os.MkdirAll(appBuildDir, 0o755); err != nil {
		return "", "", fmt.Errorf("创建产物目录失败: %w", err)
	}
	outPath = filepath.Join(appBuildDir, "nebula-dic")

	// 源码内临时运行器包目录（必须位于 module 内才能 import 引擎包；构建后删除）。
	pkgName := fmt.Sprintf("dicrun_tmp_%d", time.Now().UnixNano())
	pkgDir := filepath.Join(srcRoot, pkgName)
	if err = os.MkdirAll(pkgDir, 0o755); err != nil {
		return "", "", fmt.Errorf("创建运行器目录失败: %w", err)
	}
	defer os.RemoveAll(pkgDir)

	if err = os.WriteFile(filepath.Join(pkgDir, "main.go"), []byte(linuxDicMainTemplate()), 0o644); err != nil {
		return "", "", fmt.Errorf("写入 main.go 失败: %w", err)
	}
	if err = os.WriteFile(filepath.Join(pkgDir, "compiled.gob"), gobData, 0o644); err != nil {
		return "", "", fmt.Errorf("写入 compiled.gob 失败: %w", err)
	}

	cmd := exec.Command(goExe, "build", "-trimpath", "-ldflags", "-s -w", "-o", outPath, "./"+pkgName)
	cmd.Dir = srcRoot
	// 强制 GOOS=linux：即使打包动作发生在 Windows（纯 Go 交叉编译，CGO_ENABLED=0 无需 gcc），
	// 产物也是 ELF。GOARCH 取调用方指定的目标架构。
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOOS=linux", "GOARCH="+goarch, "CGO_ENABLED=0")
	combined, err := cmd.CombinedOutput()
	logText = string(combined)
	if err != nil {
		return "", logText, fmt.Errorf("go build 失败: %v", err)
	}
	return outPath, logText, nil
}

// buildDicBundle 把指定词库打包为独立可执行文件，按目标平台分发：
//   - windows：编译嵌入 nebula.dll + 词库 gob 的运行器（复用已发布 DLL，加载器纯标准库）
//   - linux：在 Nebula 源码目录内编译「直连引擎」运行器（import 引擎包执行词库 gob），
//     需要环境变量 NEBULA_SRC 指向 Nebula 源码根、可用 Go 工具链（源码现编，无 .so/dll）
func buildDicBundle(dicPath, goos, goarch string) (outPath, logText string, err error) {
	if goos == "linux" {
		goExe, gerr := resolveGoExe()
		if gerr != nil {
			return "", "", gerr
		}
		return buildLinuxDicBundle(dicPath, goExe, goarch)
	}
	// Windows：nebula.dll 为 windows-amd64，加载器使用 syscall.LoadLibrary
	if goos != "windows" {
		return "", "", errors.New("仅支持 Windows / Linux 平台打包")
	}
	dllPath, err := locateNebulaDLL()
	if err != nil {
		return "", "", err
	}
	goExe, err := resolveGoExe()
	if err != nil {
		return "", "", err
	}
	content, err := utils.NewFileQueue(dicPath).ReadFromFile()
	if err != nil {
		return "", "", fmt.Errorf("词库读取失败: %w", err)
	}

	// 打包前先编译词库：此时 #引入 依赖文件与 //@资源 资源文件均在磁盘上，可直接读入；
	// 编译产物序列化为 gob 后嵌入 exe，运行时反序列化执行，不再重新编译或读取外部文件。
	buildValue := run.BuildDic(dicPath, content)
	gobData, err := run.MarshalBuildValue(buildValue)
	if err != nil {
		return "", "", err
	}

	// 读取 nebula.dll（参与打包指纹，dll 更新时需重新打包）
	dllData, err := os.ReadFile(dllPath)
	if err != nil {
		return "", "", fmt.Errorf("读取 nebula.dll 失败: %w", err)
	}

	// 产物输出到应用数据目录 private/build/dist/，与词库 private/build/dic/ 分开。
	appBuildDir, err := filepath.Abs(filepath.Join(utils.GetAppDir(), "private", "build", "dist"))
	if err != nil {
		return "", "", fmt.Errorf("解析产物目录失败: %w", err)
	}
	if err = os.MkdirAll(appBuildDir, 0o755); err != nil {
		return "", "", fmt.Errorf("创建产物目录失败: %w", err)
	}
	outPath = filepath.Join(appBuildDir, "nebula-dic.exe")

	// 打包指纹 = sha256(所有依赖文件内容 hash + nebula.dll)：词库（含 #引入 / //@资源）与 dll 均无变化时，
	// 直接复用已有产物，跳过耗时的 go build。依赖 hash 来自编译期记录（Deps，含主文件与全部依赖文件），
	// 按路径排序后拼接，保证相同输入得到稳定指纹（gob 的 map 序列化顺序不稳定，不能用作指纹）。
	fpHash := sha256.New()
	deps := buildValue.Deps
	depsPaths := make([]string, 0, len(deps))
	for p := range deps {
		depsPaths = append(depsPaths, p)
	}
	sort.Strings(depsPaths)
	for _, p := range depsPaths {
		fpHash.Write([]byte(p))
		fpHash.Write([]byte{0})
		fpHash.Write([]byte(deps[p]))
		fpHash.Write([]byte{0})
	}
	fpHash.Write(dllData)
	fingerprint := hex.EncodeToString(fpHash.Sum(nil))
	cachePath := filepath.Join(appBuildDir, "nebula-dic.exe.sha256")
	if old, rerr := os.ReadFile(cachePath); rerr == nil && strings.TrimSpace(string(old)) == fingerprint {
		if fi, serr := os.Stat(outPath); serr == nil && fi.Size() > 0 {
			return outPath, "缓存命中，未重新编译，直接运行已有产物", nil
		}
	}

	// 临时构建目录（独立 module，仅依赖标准库 + 内嵌 nebula.dll / compiled.gob）
	buildDir, err := os.MkdirTemp("", "nebula-dic-build-*")
	if err != nil {
		return "", "", fmt.Errorf("创建构建目录失败: %w", err)
	}
	defer os.RemoveAll(buildDir)

	if err = os.WriteFile(filepath.Join(buildDir, "main.go"), []byte(dicExecMainTemplate()), 0o644); err != nil {
		return "", "", fmt.Errorf("写入 main.go 失败: %w", err)
	}
	if err = os.WriteFile(filepath.Join(buildDir, "compiled.gob"), gobData, 0o644); err != nil {
		return "", "", fmt.Errorf("写入 compiled.gob 失败: %w", err)
	}
	if err = os.WriteFile(filepath.Join(buildDir, "go.mod"), []byte("module nebuladic\n\ngo 1.18\n"), 0o644); err != nil {
		return "", "", fmt.Errorf("写入 go.mod 失败: %w", err)
	}
	if err = os.WriteFile(filepath.Join(buildDir, "nebula.dll"), dllData, 0o644); err != nil {
		return "", "", fmt.Errorf("复制 nebula.dll 失败: %w", err)
	}

	cmd := exec.Command(goExe, "build", "-trimpath", "-ldflags", "-s -w", "-o", outPath, ".")
	cmd.Dir = buildDir
	// GOWORK=off 保证临时目录作为独立 module 构建，不受外层 go.work 影响。
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=windows", "GOARCH=amd64", "GOWORK=off")
	combined, err := cmd.CombinedOutput()
	logText = string(combined)
	if err != nil {
		return "", logText, fmt.Errorf("go build 失败: %v", err)
	}

	// 记录本次打包指纹，供下次编译做缓存命中判断。
	_ = os.WriteFile(cachePath, []byte(fingerprint), 0o644)

	return outPath, logText, nil
}

// androidProjectRoot 定位 Android 工程根目录（含 app/build.gradle）。
// APK 打包是「打包机本机打包」：需在包含 android/ 与 go.work 的仓库根上跑 gradle。
// 候选路径：环境变量 NEBULA_SRC/android → 源码反推的仓库根/android。
func androidProjectRoot() (string, error) {
	var candidates []string
	if s := strings.TrimSpace(os.Getenv("NEBULA_SRC")); s != "" {
		candidates = append(candidates, filepath.Join(s, "android"))
	}
	if r, err := nebulaSrcRoot(); err == nil {
		candidates = append(candidates, filepath.Join(r, "android"))
	}
	seen := make(map[string]bool)
	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil || seen[abs] {
			continue
		}
		seen[abs] = true
		if fi, statErr := os.Stat(filepath.Join(abs, "app", "build.gradle")); statErr == nil && !fi.IsDir() {
			return abs, nil
		}
	}
	return "", errors.New("未找到 Android 工程目录（app/build.gradle）：请设置 NEBULA_SRC 环境变量指向仓库根目录（含 android/ 与 go.work）")
}

// apkToolchain 探测 APK 打包所需的 JDK 与 Android SDK（优先环境变量，其次常见默认路径）。
func apkToolchain() (javaHome, sdkRoot string, err error) {
	javaHome = strings.TrimSpace(os.Getenv("JAVA_HOME"))
	if javaHome == "" {
		javaHome = `C:\Program Files\Java\jdk-17`
	}
	if fi, statErr := os.Stat(filepath.Join(javaHome, "bin", "java.exe")); statErr != nil || fi.IsDir() {
		return "", "", fmt.Errorf("未找到 JDK（%s\\bin\\java.exe）：请安装 JDK 17 或设置环境变量 JAVA_HOME", javaHome)
	}
	sdkRoot = strings.TrimSpace(os.Getenv("ANDROID_SDK_ROOT"))
	if sdkRoot == "" {
		sdkRoot = strings.TrimSpace(os.Getenv("ANDROID_HOME"))
	}
	if sdkRoot == "" {
		sdkRoot = `E:\android-sdk`
	}
	if fi, statErr := os.Stat(filepath.Join(sdkRoot, "platforms")); statErr != nil || !fi.IsDir() {
		return "", "", fmt.Errorf("未找到 Android SDK（%s）：请设置环境变量 ANDROID_SDK_ROOT", sdkRoot)
	}
	return javaHome, sdkRoot, nil
}

// buildApkBundle 把指定词库预置进 Android 工程 assets 并调用 gradle 打包 APK：
//  1. 读取词库文件（.n 源文本，与 exe/linux 打包共用同一份文件；三端产物运行时都执行该词库）；
//     词库是否带「$设置工作目录$」（初始化/头部）都不影响打包：Android 默认工作目录即 Documents/Nebula，
//     词库数据会直接写入该目录；带了该指令则把工作目录切到其下的子目录；
//  2. 写入 android/app/src/main/assets/nebula/start.n 与版本标记 dic.version（内容 md5），
//     App 首启时由 MainActivity.syncBundledDic() 同步到 Documents/Nebula/start.n 顶替默认模板；
//     同内容时跳过覆盖，避免每次启动都改写用户已有词库；
//  3. 用本机 gradle（JAVA_HOME + ANDROID_SDK_ROOT）执行 assembleRelease；
//  4. 复制 app-release.apk 到 private/build/dist/nebula.apk。
//
// APK 引擎 .so 已发布在 android jniLibs，本流程不改动引擎，仅内置词库。
func buildApkBundle(dicPath string) (outPath, logText string, err error) {
	content, err := utils.NewFileQueue(dicPath).ReadFromFile()
	if err != nil {
		return "", "", fmt.Errorf("词库读取失败: %w", err)
	}
	if strings.TrimSpace(content) == "" {
		return "", "", errors.New("词库内容为空，无法打包")
	}
	if !strings.Contains(content, "设置工作目录") {
		logText = "说明：词库未含「$设置工作目录 …$」头部。Android 端默认工作目录即为 Documents/Nebula，数据将直接写入该目录，无需特殊处理；如需把数据放入 NebulaData 子目录，可在词库头部添加该指令。\n"
	}

	androidRoot, err := androidProjectRoot()
	if err != nil {
		return "", "", err
	}
	javaHome, sdkRoot, err := apkToolchain()
	if err != nil {
		return "", "", err
	}

	// 1) 写入 assets/nebula/：start.n + dic.version（md5 标记，App 首启幂等同步用；
	//    注意用普通文件名，aapt2 打包时会过滤点开头的隐藏文件）
	assetsDir := filepath.Join(androidRoot, "app", "src", "main", "assets", "nebula")
	if err = os.MkdirAll(assetsDir, 0o755); err != nil {
		return "", "", fmt.Errorf("创建 assets 目录失败: %w", err)
	}
	if err = os.WriteFile(filepath.Join(assetsDir, "start.n"), []byte(content), 0o644); err != nil {
		return "", "", fmt.Errorf("写入 assets/nebula/start.n 失败: %w", err)
	}
	sum := md5.Sum([]byte(content))
	if err = os.WriteFile(filepath.Join(assetsDir, "dic.version"), []byte(hex.EncodeToString(sum[:])), 0o644); err != nil {
		return "", "", fmt.Errorf("写入词库版本标记失败: %w", err)
	}

	// 2) gradle 打包 APK（release 已走调试签名，产物为 app/build/outputs/apk/release/app-release.apk）
	gradlew := filepath.Join(androidRoot, "gradlew.bat")
	if _, statErr := os.Stat(gradlew); statErr != nil {
		return "", "", fmt.Errorf("Android 工程缺少 gradlew.bat（%s）", gradlew)
	}
	cmd := exec.Command(gradlew, "assembleRelease")
	cmd.Dir = androidRoot
	cmd.Env = append(os.Environ(),
		"JAVA_HOME="+javaHome,
		"ANDROID_SDK_ROOT="+sdkRoot,
		"ANDROID_HOME="+sdkRoot,
	)
	combined, err := cmd.CombinedOutput()
	logText = string(combined)
	if err != nil {
		return "", logText, fmt.Errorf("gradle 打包失败: %v", err)
	}
	apkSrc := filepath.Join(androidRoot, "app", "build", "outputs", "apk", "release", "app-release.apk")
	if _, statErr := os.Stat(apkSrc); statErr != nil {
		return "", logText, fmt.Errorf("未找到打包产物 %s", apkSrc)
	}

	// 3) 复制到产物目录 private/build/dist/nebula.apk（与 exe/linux 运行器同一目录）
	appBuildDir, err := filepath.Abs(filepath.Join(utils.GetAppDir(), "private", "build", "dist"))
	if err != nil {
		return "", logText, fmt.Errorf("解析产物目录失败: %w", err)
	}
	if err = os.MkdirAll(appBuildDir, 0o755); err != nil {
		return "", logText, fmt.Errorf("创建产物目录失败: %w", err)
	}
	outPath = filepath.Join(appBuildDir, "nebula.apk")
	apkData, err := os.ReadFile(apkSrc)
	if err != nil {
		return "", logText, fmt.Errorf("读取 APK 失败: %w", err)
	}
	if err = os.WriteFile(outPath, apkData, 0o644); err != nil {
		return "", logText, fmt.Errorf("复制 APK 失败: %w", err)
	}
	return outPath, logText, nil
}

// runDicBundle 运行已打包的可执行文件，传入触发词并捕获标准输出/错误（限时 60 秒）。
// 返回标准输出与标准错误（进程异常退出时错误信息并入标准错误）。
func runDicBundle(exePath, trigger string) (stdout, stderr string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exePath, trigger)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if errBuf.Len() > 0 {
			return outBuf.String(), errBuf.String()
		}
		return outBuf.String(), err.Error()
	}
	return outBuf.String(), errBuf.String()
}

// openSqliteOpui 打开应用目录下的 SQLite 数据库文件（modernc 纯 Go 驱动），
// 设置忙碌等待与单连接，避免与词库写入并发时出现 database is locked
