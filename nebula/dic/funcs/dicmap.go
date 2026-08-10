package funcs

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cjxpj/nebula/dto"
	"github.com/iancoleman/orderedmap"
)

// newMapData 把 JSON 字符串或 map[string]any 转成 orderedmap
func newMapData(d *dto.DicInputs) (any, error) {
	result := orderedmap.New()
	val := d.Inputs.Get(1)

	switch v := val.(type) {
	case map[string]any:
		// 如果已经是 map，直接转换
		for k, vv := range v {
			result.Set(k, vv)
		}
		result.SetEscapeHTML(false)
		return result, nil

	case string:
		// 如果是字符串，尝试解析为 JSON
		err := json.Unmarshal([]byte(v), &result)
		result.SetEscapeHTML(false)
		if err != nil {
			// 解析失败，返回空 orderedmap
			return result, nil
		}
		return result, nil

	default:
		// 其他类型不支持
		return nil, fmt.Errorf("unsupported input type: %T", v)
	}
}

// setMapData 在 orderedmap 中设置嵌套值
func setMapData(d *dto.DicInputs) (any, error) {
	m, ok := d.Inputs.Get(1).(*orderedmap.OrderedMap)
	if !ok {
		return nil, errors.New("不是字典数据类型")
	}

	// 跳过第一个(map)，取出 keys+value
	keys := d.Inputs.List[2:]
	if len(keys) < 2 {
		return nil, errors.New("至少需要 key 和 value")
	}

	// 最后一个是 value，其余的是路径
	path := make([]string, 0, len(keys)-1)
	for _, k := range keys[:len(keys)-1] {
		path = append(path, fmt.Sprint(k))
	}
	value := keys[len(keys)-1]

	// 遍历路径并设置
	curr := m
	for i, k := range path {
		if i == len(path)-1 {
			curr.Set(k, value)
		} else {
			v, ok := curr.Get(k)
			if !ok {
				// 不存在则新建
				next := orderedmap.New()
				next.SetEscapeHTML(false)
				curr.Set(k, next)
				curr = next
			} else {
				next, ok := v.(*orderedmap.OrderedMap)
				if !ok {
					return nil, fmt.Errorf("path conflict at %s", k)
				}
				curr = next
			}
		}
	}

	return "", nil
}

// getMapData 把 orderedmap 转 JSON 字符串
func getMapData(d *dto.DicInputs) (any, error) {
	m, ok := d.Inputs.Get(1).(*orderedmap.OrderedMap)
	if !ok {
		return nil, errors.New("不是字典数据类型")
	}

	r, err := json.MarshalToString(m)
	// 去掉换行
	r = strings.ReplaceAll(r, "\n", "")
	return r, err
}
