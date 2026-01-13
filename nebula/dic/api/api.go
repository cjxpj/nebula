package dic_api

import (
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
)

var Api DicApi

type DicApi = interface {
	// 执行词库
	DicRun(D *dic_dto.Dic, trigger string) string
	// 执行词库内部
	DicRunPrivate(D *dic_dto.Dic, trigger string) string
	// 执行词库内部-自义定变量
	DicRunPrivateVal(D *dic_dto.Dic, trigger string, v *dto.DicVal) string
	// 执行词条
	DicRunLine(r *dic_dto.DicEntry, txt []string) string
	// 新建执行词条
	NewDicRunLine(D *dic_dto.DicEntry, txt []string) string
	// 执行网页词库
	WebPHPDicRun(WD *dic_dto.WebDic) string
	// 执行网页词库
	WebDicRun(WD *dic_dto.WebDic) string
}
