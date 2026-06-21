package funcs

import (
	"strconv"
	stdjson "encoding/json"

	"github.com/cjxpj/nebula/dto"
)

func (f *DicFunc) Range() string {
	if f.Len == 2 {
		min, err1 := strconv.Atoi(f.Inputs.String(1))
		max, err2 := strconv.Atoi(f.Inputs.String(2))
		if err1 != nil || err2 != nil {
			return ""
		}

		// 生成从 min 到 max 的切片
		if min > max {
			return ""
		}
		result := make([]int, max-min+1)
		for i := range result {
			result[i] = min + i
		}

		// 将切片转换为字符串返回
		jsonResult, err := json.Marshal(result)
		if err != nil {
			return ""
		}
		return string(jsonResult)
	}
	return ""
}

func doRange(d *dto.DicInputs) (any, error) {
	min, err1 := strconv.Atoi(d.Inputs.String(1))
	max, err2 := strconv.Atoi(d.Inputs.String(2))
	if err1 != nil || err2 != nil {
		return "", nil
	}
	if min > max {
		return "", nil
	}
	result := make([]int, max-min+1)
	for i := range result {
		result[i] = min + i
	}
	jsonResult, err := stdjson.Marshal(result)
	if err != nil {
		return "", nil
	}
	return string(jsonResult), nil
}
