package dic_dto

import (
	"fmt"

	"github.com/cjxpj/nebula/dto"
)

type Dic struct {
	Data      *dto.BuildValue
	Val       *dto.DicVal
	Id        int16
	Path      string
	FuncText  map[string][]*dto.BuildDic
	ClassText map[string]*dto.DicClass
	// 自义定函数
	MyFunc map[string]dto.DicFunc
}

type WebDic struct {
	Text   string
	Val    *dto.DicVal
	Path   string
	MyFunc map[string]dto.DicFunc
}

// WebHTTPRequest 是一次由宿主下发的网页词库公开访问请求。
//
// 宿主负责用真实请求构建「访问数据」（Access，对应 dto.HTTPRequestInfo 的 JSON）
// 与响应回写；引擎只做执行期编排：重建原始请求、挂载通用内置函数、运行词库并收集输出。
type WebHTTPRequest struct {
	// Path 词库文件绝对路径
	Path string
	// Ext 词库扩展名（.n / .wn）
	Ext string
	// Content 词库文本内容
	Content string
	// WebRoot 「网站根目录」变量取值（空则用 "."）
	WebRoot string
	// Trigger 正文词条的触发词，仅 .n 使用（空则用 Main）。
	// 多文件路由下由宿主解析出具体文件，按 Main 执行；单文件路由下宿主传站内 URL 路径，
	// 使入口词库像路由词库一样按路径分发。
	Trigger string
	// Access 宿主构建的「访问数据」JSON（路径/来源/GET/请求头/IP/Host/POST）
	Access string
	// Body 原始请求体，用于重建 _请求数据_
	Body string
}

// WebHTTPResult 是网页词库执行后的响应状态、头部与正文。
type WebHTTPResult struct {
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers"`
	Body    string              `json:"body"`
}

// DicInfo 词库信息，来自 [函数]词库信息 内用局部变量（名称/价格/描述）声明的元数据。
type DicInfo struct {
	// 名称
	Name string `json:"name"`
	// 描述
	Desc string `json:"desc"`
	// 价格
	Price int64 `json:"price"`
}

// run
type DicEntry struct {
	// 返回信息
	Output  *dto.SingleValue
	Val     *dto.DicVal
	Sys_v   *dto.LocalDicValue
	Trigger bool
	Dic     *dto.BuildValue
	// 当前处理 txt 每行对应的原始文件行号（1-based），与 txt 一一对应
	// 为 nil 时表示无行号映射（如递归调用、非调试场景）
	LineNums []int
}

func (d *DicEntry) Close() {
	d.Output.Clear()
	// 回收局部变量
	d.Val.P.Close()
	// 清空指针
	d.Val = nil
	d.Sys_v = nil
	// 清空回收
	d.Dic.Close()
}

// build
type Build struct {
	Val *dto.DicVal
}

// func
type DicFunc struct {
	// 变量
	Val *dto.DicVal
	// 系统变量
	Sys *dto.LocalDicValue
	// 准备输出内容
	Output *dto.SingleValue
	Dic    *dto.BuildValue
	// 当前执行行号（1-based），用于调试报错定位
	CurLine int
	// Trigger 当前执行是否来自正文触发词匹配。
	// true：正文词条（含头部/初始化/中间件）执行；false：$函数名$/[函数]/[内部]/类方法等函数调用路径。
	// 供 $继续执行$ 等仅在正文触发词下生效的内置函数判断。
	Trigger bool
}

// ReportCountError 实现 count.CountErrorReporter：算术表达式（[...]）求值出错时
// 中断执行、清空已累积输出并写入错误信息，避免静默输出 [原文] 掩盖问题。
func (d *DicFunc) ReportCountError(err error, raw string) {
	d.Sys.Stop.Store(true)
	d.Output.Clear()
	d.Output.Add(fmt.Sprintf("[%s](line:%d)：算术表达式 [%s] 求值失败：%v", d.Val.G.GetStr("_词库路径_"), d.CurLine, raw, err))
}
