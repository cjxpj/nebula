package dic

import dic_api "github.com/cjxpj/nebula/dic/api"

func init() {
	dic_api.Api = &dicImpl{}
}

type dicImpl struct{}

// if
type IfText struct{}
