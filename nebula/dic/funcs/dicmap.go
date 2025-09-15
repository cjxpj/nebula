package funcs

import (
	"errors"
	"fmt"

	"github.com/cjxpj/nebula/dto"
)

func newMapData(d *dto.DicInputs) (any, error) {
	var result map[string]any
	err := json.Unmarshal([]byte(d.Inputs.String(1)), &result)
	if err != nil {
		return result, nil
	}
	return result, nil
}

func setMapData(d *dto.DicInputs) (any, error) {
	m, ok := d.Inputs.Get(1).(map[string]any)
	if !ok {
		return nil, errors.New("不是字典数据类型")
	}

	// 跳过第一个(map)，取出 keys+value
	keys := d.Inputs.List[2:]

	// 最后一个是 value，其余的是路径
	path := make([]string, 0, len(keys)-1)
	for _, k := range keys[:len(keys)-1] {
		path = append(path, fmt.Sprint(k)) // 统一转 string
	}
	value := keys[len(keys)-1]

	// 遍历路径并设置
	curr := m
	for i, k := range path {
		if i == len(path)-1 {
			curr[k] = value
		} else {
			if _, ok := curr[k]; !ok {
				curr[k] = make(map[string]any)
			}
			next, ok := curr[k].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("path conflict at %s", k)
			}
			curr = next
		}
	}

	return m, nil
}

func getMapData(d *dto.DicInputs) (any, error) {
	m, ok := d.Inputs.Get(1).(map[string]any)
	if !ok {
		return nil, errors.New("不是字典数据类型")
	}

	r, err := json.Marshal(m)
	return string(r), err
}
