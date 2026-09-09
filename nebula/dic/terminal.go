//go:build !js && !dll

package dic

import (
	"github.com/cjxpj/nebula/appfiles"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
)

// RunFile 加载并执行指定词库文件（触发词固定 Main），返回执行结果。
func RunFile(path string) (string, error) {
	infoDic, err := dic_dto.NewDicFile(path)
	if err != nil {
		return "", err
	}
	GV := dto.NewVal()
	GV.Set("版本", appfiles.Version)
	infoDic.SetGlobal_v(GV)
	return dic_api.Api.DicRun(infoDic, "Main"), nil
}

// CheckFile 预编译检测指定词库文件，返回警告与报错（level 为 error）。
func CheckFile(path string) (warns, errs []dto.BuildWarning, err error) {
	infoDic, err := dic_dto.NewDicFileNoCache(path)
	if err != nil {
		return nil, nil, err
	}
	for _, w := range infoDic.Data.Warnings {
		if w.Level == "error" {
			errs = append(errs, w)
		} else {
			warns = append(warns, w)
		}
	}
	return warns, errs, nil
}
