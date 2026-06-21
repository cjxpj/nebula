package funcs

import (
	"sort"
	"strconv"
	stdjson "encoding/json"

	"github.com/cjxpj/nebula/dto"
)

func (f *DicFunc) Sort() string {
	if f.Len >= 2 {
		data := f.Inputs.String(2)
		sortKey := f.Inputs.String(1)

		// 检查是否有第三个输入来判断正序或反序
		isDescending := false
		if f.Len >= 3 && f.Inputs.String(3) == "true" {
			isDescending = true
		}

		// 将 JSON 数据解析为 []map[string]interface{} 切片
		var list []map[string]interface{}
		err := json.Unmarshal([]byte(data), &list)
		if err != nil {
			return "null"
		}

		// 按照指定的键排序
		sort.Slice(list, func(i, j int) bool {
			valueI := list[i][sortKey]
			valueJ := list[j][sortKey]

			var result bool

			switch vI := valueI.(type) {
			case string:
				intI, errI := strconv.Atoi(vI)
				if errI == nil {
					switch vJ := valueJ.(type) {
					case string:
						intJ, errJ := strconv.Atoi(vJ)
						if errJ == nil {
							result = intI < intJ
						} else {
							result = vI < vJ
						}
					case float64:
						result = float64(intI) < vJ
					}
				} else {
					switch vJ := valueJ.(type) {
					case string:
						result = vI < vJ
					case float64:
						// 无法直接比较，按照默认顺序
						result = true
					}
				}
			case float64:
				switch vJ := valueJ.(type) {
				case string:
					intJ, errJ := strconv.Atoi(vJ)
					if errJ == nil {
						result = vI < float64(intJ)
					} else {
						// 无法直接比较，按照默认顺序
						result = false
					}
				case float64:
					result = vI < vJ
				}
			}

			// 如果是反序，则反转结果
			if isDescending {
				return !result
			}
			return result
		})

		// 排序后的结果
		sortedJsonData, err := json.Marshal(list)
		if err != nil {
			return data
		}

		return string(sortedJsonData)
	}
	return ""
}

func doSort(d *dto.DicInputs) (any, error) {
	l := d.Inputs.Len()
	if l >= 2 {
		data := d.Inputs.String(2)
		sortKey := d.Inputs.String(1)

		isDescending := false
		if l >= 3 && d.Inputs.String(3) == "true" {
			isDescending = true
		}

		var list []map[string]interface{}
		if err := stdjson.Unmarshal([]byte(data), &list); err != nil {
			return "null", nil
		}

		sort.Slice(list, func(i, j int) bool {
			valueI := list[i][sortKey]
			valueJ := list[j][sortKey]

			var result bool
			switch vI := valueI.(type) {
			case string:
				intI, errI := strconv.Atoi(vI)
				if errI == nil {
					switch vJ := valueJ.(type) {
					case string:
						intJ, errJ := strconv.Atoi(vJ)
						if errJ == nil {
							result = intI < intJ
						} else {
							result = vI < vJ
						}
					case float64:
						result = float64(intI) < vJ
					}
				} else {
					switch vJ := valueJ.(type) {
					case string:
						result = vI < vJ
					case float64:
						result = true
					}
				}
			case float64:
				switch vJ := valueJ.(type) {
				case string:
					intJ, errJ := strconv.Atoi(vJ)
					if errJ == nil {
						result = vI < float64(intJ)
					} else {
						result = false
					}
				case float64:
					result = vI < vJ
				}
			}

			if isDescending {
				return !result
			}
			return result
		})

		sortedJsonData, err := stdjson.Marshal(list)
		if err != nil {
			return data, nil
		}
		return string(sortedJsonData), nil
	}
	return "", nil
}
