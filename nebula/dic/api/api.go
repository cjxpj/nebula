package dic_api

import (
	"time"

	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
)

// Api 词库执行引擎的全局公开入口，由引擎初始化时注入实现。
var Api DicApi

// DicRunner 普通词库执行。
type DicRunner interface {
	// 执行词库
	DicRun(D *dic_dto.Dic, trigger string) string
	// 执行词库（带超时）：超时后强行打断执行并返回当前结果，timedOut=true 表示已超时
	DicRunTimeout(D *dic_dto.Dic, trigger string, timeout time.Duration) (result string, timedOut bool)
}

// DicPrivateRunner 内部触发执行。
type DicPrivateRunner interface {
	// 执行词库内部
	DicRunPrivate(D *dic_dto.Dic, trigger string) string
	// 执行词库内部-自义定变量
	DicRunPrivateVal(D *dic_dto.Dic, trigger string, v *dto.DicVal) string
}

// DicEventRunner 特殊事件触发执行。
type DicEventRunner interface {
	// 执行词库特殊触发
	DicRunEvent(D *dic_dto.Dic, event string, trigger string) string
	// 执行词库特殊触发-自义定变量
	DicRunEventVal(D *dic_dto.Dic, event string, trigger string, v *dto.DicVal) string
}

// DicEntryRunner 词条执行。
type DicEntryRunner interface {
	// 执行词条
	DicRunLine(r *dic_dto.DicEntry, txt []string) string
	// 新建执行词条
	NewDicRunLine(D *dic_dto.DicEntry, txt []string) string
}

// WebDicRunner 网页词库执行。
type WebDicRunner interface {
	// 执行网页词库（处理 <?n ... ?> 代码块）
	WebPHPDicRun(WD *dic_dto.WebDic) string
	// 执行网页词库（解析 HTML 与模板渲染）
	WebDicRun(WD *dic_dto.WebDic) string
}

// DicApi 词库执行公开接口，按功能分类聚合，便于按需依赖与维护。
type DicApi interface {
	DicRunner
	DicPrivateRunner
	DicEventRunner
	DicEntryRunner
	WebDicRunner
}
