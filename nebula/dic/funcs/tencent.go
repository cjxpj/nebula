package funcs

import (
	stdjson "encoding/json"
	"fmt"

	"github.com/cjxpj/nebula/dto"
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
		payloadStr, ok := f.Inputs.Get(2).(string)
		if !ok {
			return "", fmt.Errorf("参数2必须是字符串")
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(payloadStr), &payload); err == nil {
			result, err := api.Request(payload)
			if err != nil {
				return "", err
			}
			return string(result), nil
		}
		return "", fmt.Errorf("json解析失败")
	}
	return "", fmt.Errorf("参数类型错误")
}

func tencentGetApi(d *dto.DicInputs) (any, error) {
	api := &utils.TencentAPI{
		SecretId:  d.Inputs.String(1),
		SecretKey: d.Inputs.String(2),
		Host:      d.Inputs.String(3),
		Service:   d.Inputs.String(4),
		Action:    d.Inputs.String(5),
		Version:   d.Inputs.String(6),
		Region:    d.Inputs.String(7),
	}
	return newTencentClass(api), nil
}

// newTencentClass 将腾讯云 API 包装为面对像 Class，方法闭包捕获同一实例。
func newTencentClass(api *utils.TencentAPI) *dto.DicClass {
	instance := &dto.DicClass{
		LocalValue: dto.NewVal().Set("_腾讯_", api),
	}
	instance.Fn = map[string]dto.DicFunc{
		"调用": wrapObj(api, tencentGetApiCall, "1"),
	}
	return instance
}

func tencentGetApiCall(d *dto.DicInputs) (any, error) {
	if api, ok := d.Inputs.Get(1).(*utils.TencentAPI); ok {
		var payload map[string]any
		if err := stdjson.Unmarshal([]byte(d.Inputs.String(2)), &payload); err != nil {
			return "", fmt.Errorf("json解析失败: %v", err)
		}
		result, err := api.Request(payload)
		if err != nil {
			return "", err
		}
		return string(result), nil
	}
	return "", fmt.Errorf("参数类型错误")
}
