package dic_api

import "github.com/cjxpj/nebula/dto"

type Dic struct {
	text      string
	Val       *dto.DicVal
	id        int16
	Path      string
	FuncText  map[string][]*dto.BuildDic
	ClassText map[string]*dto.DicClass
	// 自义定函数
	MyFunc map[string]dto.DicFunc
}
