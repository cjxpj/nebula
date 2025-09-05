package funcs

import (
	"fmt"

	"github.com/cjxpj/nebula/utils"
)

// TencentParseDomain

func (f *DicFunc) TencentGetApi() (*utils.TencentAPI, error) {
	if !f.Inputs.LenOk(6, 7) {
		return nil, fmt.Errorf("参数数量错误")
	}
	api := &utils.TencentAPI{
		SecretId:  f.Inputs.String(1),
		SecretKey: f.Inputs.String(2),
		Host:      f.Inputs.String(3),
		Service:   f.Inputs.String(4),
		Action:    f.Inputs.String(5),
		Version:   f.Inputs.String(6),
		Region:    f.Inputs.String(7),
	}
	return api, nil
}

func (f *DicFunc) TencentGetApiCall() (string, error) {
	if !f.Inputs.LenOk(2) {
		return "", fmt.Errorf("参数数量错误")
	}
	if api, ok := f.Inputs.Get(1).(*utils.TencentAPI); ok {
		// 参数2转map[string]any
		var payload map[string]any
		if err := json.Unmarshal([]byte(f.Inputs.Get(2).(string)), &payload); err != nil {
			result, err := api.Request(payload)
			if err != nil {
				return "", err
			}
			return string(result), nil
		}
	}
	return "", fmt.Errorf("参数类型错误")
}
