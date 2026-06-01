package utils

import (
	"github.com/cjxpj/nebula/debugLog"
)

func FmtResult(i any) any {
	debugLog.Infof("%v", i)
	return i
}
