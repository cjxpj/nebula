package dic

import dic_api "github.com/cjxpj/nebula/dic/api"

func init() {
	dic_api.Api = &dicImpl{}
}

type dicImpl struct{}

// if
type IfText struct {
	Error bool // 条件表达式解析异常
}
