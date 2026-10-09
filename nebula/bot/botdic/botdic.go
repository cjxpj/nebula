// Package botdic 提供各机器人适配器共用的「遍历词库并执行」逻辑。
//
// QQ / NapCat / 飞书 / 隐匿 / 云湖等适配器原先各自把同一套骨架写了很多遍：
// 列词库目录（或单文件）→ 过滤 .n → 逐个读取 → 建库（注入全局变量与内置函数）→
// 执行（消息 / 事件 / 内部触发）→ 结果送达。这里收敛为一份实现，
// 各适配器只需给出「数据、内置函数、触发方式、送达方式」这些真正不同的部分。
package botdic

import (
	"path"
	"strings"
	"sync"
	"time"

	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// ExtraDic 描述一个挂在机器人上的「外部词库」：内容从 Real（真实绝对路径，位于账号目录之外，
// 如平台公共目录）读取，但以 Virtual（账号布局内的绝对虚拟路径）作为执行路径，
// 使账号上下文（私有目录 / 数据库 / 储存 / 文件边界）仍归属该账号；
// 且该词库完全绕过磁盘编译缓存——内容与编译产物都不落在账号目录里。
type ExtraDic struct {
	Virtual string
	Real    string
}

var (
	extraMu    sync.RWMutex
	extraByDir = map[string][]ExtraDic{}
)

// SetExtraDics 为某机器人（按词库来源目录 filePath 标识）设置外部词库列表，覆盖旧值；
// 传空列表即清除。由宿主在机器人启动后下发。
func SetExtraDics(filePath string, dics []ExtraDic) {
	if filePath == "" {
		return
	}
	extraMu.Lock()
	defer extraMu.Unlock()
	if len(dics) == 0 {
		delete(extraByDir, filePath)
		return
	}
	extraByDir[filePath] = append([]ExtraDic(nil), dics...)
}

// extraOf 返回某来源目录已注册的外部词库。
func extraOf(filePath string) []ExtraDic {
	extraMu.RLock()
	defer extraMu.RUnlock()
	return extraByDir[filePath]
}

// Run 一次「遍历词库并执行」的配置。
type Run struct {
	// FilePath 机器人账号路径；SingleFile 为 true 时即单个词库文件路径。
	FilePath string
	// SingleFile 单文件模式：不列目录，只执行 FilePath 指向的词库。
	SingleFile bool
	// Val 注入子词库的全局变量（SetGlobal_v）；为 nil 时不注入。
	Val *dto.Val
	// Funcs 注入子词库的内置函数（各适配器的发送/回复函数等），可为 nil。
	Funcs map[string]dto.DicFunc
	// Prepare 建库后、挂「调用」前的额外设置（如 SetPushContext、自定义方法），可为 nil。
	Prepare func(*dic_dto.Dic)
	// Trigger 触发词。
	Trigger string
	// Event 事件名；非空时按事件执行（EventMsg 为事件参数），并忽略 Private。
	Event string
	// EventMsg 事件参数。
	EventMsg string
	// Private 为 true 时走内部触发（DicRunPrivate），否则走 DicRun。
	Private bool
	// Parallel 为 true 时每个词库各起一个 goroutine，否则按文件顺序逐个执行。
	Parallel bool
	// AsyncCall 挂「调用」异步回调（延迟回复），送达方式同 Deliver。
	AsyncCall bool
	// DeliverOnEmpty 为 true 时，结果文本为空也调用 Deliver（供需按产出变量单独送达的适配器，
	// 如 QQ 频道「只设置 发送图片 而无文本」的场景）；默认 false 时空结果忽略。
	DeliverOnEmpty bool
	// Deliver 结果送达（msg 已把 "\\r" 替换为 "\n"）；val 为产出该结果的值集合
	// （异步回调为回调变量，其余为词库全局变量），供送达时按变量取图等使用；为 nil 则忽略结果。
	Deliver func(msg string, val *dto.DicVal)
}

// dicSource 一个待执行词库的来源。
type dicSource struct {
	path    string // 执行路径：磁盘路径，或外部词库的虚拟路径（账号布局内）
	real    string // 非空表示内容取自该真实绝对路径（外部词库，不在账号目录内）
	noCache bool   // 是否强制不使用磁盘编译缓存
}

// Exec 遍历词库并逐个执行。
func (r Run) Exec() {
	for _, s := range r.listSources() {
		if r.Parallel {
			go r.runOne(s)
			continue
		}
		r.runOne(s)
	}
}

// ListFiles 返回待执行的词库路径：单文件模式返回该文件本身，
// 否则返回词库目录下的全部 .n，以及该机器人已注册的外部词库（虚拟路径）。
// 适配器可用它提前判断是否有词库可执行。
func ListFiles(filePath string, singleFile bool) []string {
	src := listSources(filePath, singleFile)
	out := make([]string, 0, len(src))
	for _, s := range src {
		out = append(out, s.path)
	}
	return out
}

// listSources 汇总待执行的词库来源：词库目录下的 .n + 已注册的外部词库。
func listSources(filePath string, singleFile bool) []dicSource {
	if singleFile {
		return []dicSource{{path: filePath}}
	}
	dir := filePath
	if dir != "" {
		dir = path.Join(dir, utils.DicDirName())
	}
	out := make([]dicSource, 0, 4)
	if list, err := utils.NewFileQueue(dir).GetFileList(); err == nil {
		for _, v := range list {
			if strings.HasSuffix(v, ".n") {
				out = append(out, dicSource{path: path.Join(dir, v)})
			}
		}
	}
	for _, e := range extraOf(filePath) {
		if e.Virtual == "" || e.Real == "" {
			continue
		}
		out = append(out, dicSource{path: e.Virtual, real: e.Real, noCache: true})
	}
	return out
}

// listSources 返回本次待执行的词库来源。
func (r Run) listSources() []dicSource {
	return listSources(r.FilePath, r.SingleFile)
}

// runOne 读取单个词库并执行。
func (r Run) runOne(s dicSource) {
	var data string
	var err error
	if s.real != "" {
		// 外部词库：内容取自公共目录的真实文件，执行路径仍用虚拟路径（账号上下文）
		data, err = utils.NewFileQueue(s.real).ReadFromFile()
	} else {
		data, err = utils.NewFileQueue(s.path).ReadFromFile()
	}
	if err != nil {
		return
	}

	var dic *dic_dto.Dic
	if s.noCache {
		// 外部词库不落盘：强制绕过磁盘编译缓存
		dic = dic_dto.NewDicNoCache(s.path, data)
	} else {
		dic = dic_dto.NewDic(s.path, data)
	}
	if r.Val != nil {
		dic.SetGlobal_v(r.Val)
	}
	if r.Prepare != nil {
		r.Prepare(dic)
	}
	dic.AddFuncs(r.Funcs)

	if r.AsyncCall {
		dic.SetFunc("调用", dto.DicFunc{
			L: "2..",
			Fn: func(d *dto.DicInputs) (any, error) {
				go func() {
					qqVal := dic.NewDicVal()
					time.Sleep(time.Duration(d.Inputs.Int(1)) * time.Millisecond)
					r.deliver(dic_api.Api.DicRunPrivateVal(dic, d.Inputs.StringAfter(2), qqVal), qqVal)
				}()
				return "", nil
			}})
	}

	switch {
	case r.Event != "":
		r.deliver(dic_api.Api.DicRunEvent(dic, r.Event, r.EventMsg), dic.Val)
	case r.Private:
		r.deliver(dic_api.Api.DicRunPrivate(dic, r.Trigger), dic.Val)
	default:
		r.deliver(dic_api.Api.DicRun(dic, r.Trigger), dic.Val)
	}
}

// deliver 把结果的 "\\r" 转为换行后送达；结果为空（且未开启 DeliverOnEmpty）或未配置送达时忽略。
func (r Run) deliver(msg string, val *dto.DicVal) {
	if r.Deliver == nil || (msg == "" && !r.DeliverOnEmpty) {
		return
	}
	r.Deliver(strings.ReplaceAll(msg, "\\r", "\n"), val)
}
