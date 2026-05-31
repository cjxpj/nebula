package funcs

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

func (f *DicFunc) QueryJson() (string, error) {
	if !f.Inputs.LenOk(2) {
		return "", errors.New("参数错误")
	}
	var obj any
	if err := json.Unmarshal([]byte(f.Inputs.String(1)), &obj); err != nil {
		return "", errors.New("不是json格式")
	}
	keys := strings.Split(f.Inputs.String(2), "->")
	for _, key := range keys {
		switch d := obj.(type) {
		case string:
			// 字符串直接退出
			break
		case map[string]any:
			if val, ok := d[key]; ok {
				switch num := val.(type) {
				case int:
					obj = strconv.FormatInt(int64(num), 10)
				case int64:
					obj = strconv.FormatInt(num, 10)
				case float64:
					obj = strconv.FormatFloat(num, 'f', -1, 64)
				default:
					obj = num
				}
			} else {
				return "", nil
			}
		case []any:
			if idx, err := strconv.Atoi(key); err == nil && idx >= 0 && idx < len(d) {
				obj = d[idx]
			} else {
				return "", nil
			}
		}
	}
	switch v := obj.(type) {
	case string:
		return v, nil
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (f *DicFunc) IsJson() string {
	if f.Len != 1 {
		return ""
	}
	s := f.Inputs.String(1)
	var m map[string]any
	var a []any
	err1 := json.Unmarshal([]byte(s), &m)
	err2 := json.Unmarshal([]byte(s), &a)
	// fmt.Println("data", s)
	// fmt.Println("err1", err1)
	// fmt.Println("err2", err2)
	if err1 == nil || err2 == nil {
		return "true"
	}
	return "false"
}

func jsonSet(d *dto.DicInputs) (any, error) {
	var obj any
	if err := json.Unmarshal([]byte(d.Inputs.String(1)), &obj); err != nil {
		return nil, errors.New("不是json格式")
	}

	keys := d.Inputs.StringAfterList(2)
	// 去掉最后一个元素
	if keysLen := len(keys); keysLen > 1 {
		keys = keys[:keysLen-1]
	}

	// 取最后一个元素作为设置值
	value := d.Inputs.String(max(d.Inputs.Len(), 3))

	obj = JsonSetValue(obj, keys, value, false)
	b, _ := json.Marshal(obj)
	return string(b), nil
}
func jsonSetString(d *dto.DicInputs) (any, error) {
	var obj any
	if err := json.Unmarshal([]byte(d.Inputs.String(1)), &obj); err != nil {
		return nil, errors.New("不是json格式")
	}

	keys := d.Inputs.StringAfterList(2)
	// 去掉最后一个元素
	if keysLen := len(keys); keysLen > 1 {
		keys = keys[:keysLen-1]
	}

	// 取最后一个元素作为设置值
	value := d.Inputs.String(max(d.Inputs.Len(), 3))

	obj = JsonSetValue(obj, keys, value, true)
	b, _ := json.Marshal(obj)
	return string(b), nil
}

func (f *DicFunc) JsonAdd() string {
	if f.Len != 2 {
		return f.Inputs.String(1)
	}
	var obj any
	if err := json.Unmarshal([]byte(f.Inputs.String(1)), &obj); err != nil {
		return f.Inputs.String(1)
	}
	if arr, ok := obj.([]any); ok {
		var val any
		if err := json.Unmarshal([]byte(f.Inputs.String(2)), &val); err == nil {
			arr = append(arr, val)
		} else {
			arr = append(arr, f.Inputs.List[2])
		}
		b, _ := json.Marshal(arr)
		return string(b)
	}
	return f.Inputs.String(1)
}

func (f *DicFunc) JsonAddString() string {
	if f.Len != 2 {
		return f.Inputs.String(1)
	}
	var obj any
	if err := json.Unmarshal([]byte(f.Inputs.String(1)), &obj); err != nil {
		return f.Inputs.String(1)
	}
	if arr, ok := obj.([]any); ok {
		arr = append(arr, f.Inputs.String(2))
		b, _ := json.Marshal(arr)
		return string(b)
	}
	return f.Inputs.String(1)
}

func (f *DicFunc) JsonDelete() string {
	if f.Len != 2 {
		return f.Inputs.String(1)
	}
	var obj any
	if err := json.Unmarshal([]byte(f.Inputs.String(1)), &obj); err != nil {
		return f.Inputs.String(1)
	}
	switch d := obj.(type) {
	case map[string]any:
		delete(d, f.Inputs.String(2))
	case []any:
		if idx, err := strconv.Atoi(f.Inputs.String(2)); err == nil && idx >= 0 && idx < len(d) {
			d = append(d[:idx], d[idx+1:]...)
			obj = d
		}
	}
	b, _ := json.Marshal(obj)
	return string(b)
}

func (f *DicFunc) JsonIsKey() string {
	if !f.Inputs.LenOk(2) {
		return "false"
	}
	var obj any
	if err := json.Unmarshal([]byte(f.Inputs.String(1)), &obj); err != nil {
		return "false"
	}
	switch d := obj.(type) {
	case map[string]any:
		if _, ok := d[f.Inputs.String(2)]; ok {
			return "true"
		}
	case []any:
		if idx, err := strconv.Atoi(f.Inputs.String(2)); err == nil && idx >= 0 && idx < len(d) {
			return "true"
		}
	}
	return "false"
}

func jsonFindText(d *dto.DicInputs) (any, error) {
	var obj any
	if err := json.Unmarshal([]byte(d.Inputs.String(1)), &obj); err != nil {
		return "-1", nil
	}
	target := d.Inputs.String(2)
	switch v := obj.(type) {
	case map[string]any:
		for k, val := range v {
			if str := utils.AnyToString(val); str == target {
				return k, nil
			}
		}
	case []any:
		for i, val := range v {
			if str := utils.AnyToString(val); str == target {
				return strconv.Itoa(i), nil
			}
		}
	}
	return "-1", nil
}

func jsonFindTextFuzzy(d *dto.DicInputs) (any, error) {
	var obj any
	if err := json.Unmarshal([]byte(d.Inputs.String(1)), &obj); err != nil {
		return "-1", nil
	}
	target := d.Inputs.String(2)
	switch v := obj.(type) {
	case map[string]any:
		for k, val := range v {
			if str := utils.AnyToString(val); strings.Contains(str, target) {
				return k, nil
			}
		}
	case []any:
		for i, val := range v {
			if str := utils.AnyToString(val); strings.Contains(str, target) {
				return strconv.Itoa(i), nil
			}
		}
	}
	return "-1", nil
}

func jsonFindTextRegex(d *dto.DicInputs) (any, error) {
	var obj any
	if err := json.Unmarshal([]byte(d.Inputs.String(1)), &obj); err != nil {
		return "-1", nil
	}
	re, err := regexp.Compile(d.Inputs.String(2))
	if err != nil {
		return "-1", nil
	}
	switch v := obj.(type) {
	case map[string]any:
		for k, val := range v {
			if str := utils.AnyToString(val); re.MatchString(str) {
				return k, nil
			}
		}
	case []any:
		for i, val := range v {
			if str := utils.AnyToString(val); re.MatchString(str) {
				return strconv.Itoa(i), nil
			}
		}
	}
	return "-1", nil
}

func (f *DicFunc) JsonPrettyPrint() (string, error) {
	if !f.Inputs.LenOk(1, 2) {
		return "", errors.New("参数错误")
	}
	var obj any
	if err := json.Unmarshal([]byte(f.Inputs.String(1)), &obj); err != nil {
		return "", errors.New("不是json格式")
	}
	indent := " "
	if n, err := strconv.Atoi(f.Inputs.String(2)); err == nil && n > 0 {
		indent = strings.Repeat(" ", n)
	}
	b, _ := json.MarshalIndent(obj, "", indent)
	return string(b), nil
}

func (f *DicFunc) JsonLen() string {
	if f.Len != 1 {
		return "0"
	}
	var obj any
	if err := json.Unmarshal([]byte(f.Inputs.String(1)), &obj); err != nil {
		return "0"
	}
	switch d := obj.(type) {
	case []any:
		return strconv.Itoa(len(d))
	case map[string]any:
		return strconv.Itoa(len(d))
	}
	return "0"
}

// 根据键列表设置 interface{} 的值
func JsonSetValue(data any, keys []string, dvalue string, str bool) any {
	if len(keys) == 0 {
		return data
	}
	switch v := data.(type) {
	case map[string]any:
		key := keys[0]
		if len(keys) == 1 {
			if str {
				v[key] = dvalue
			} else {
				var value any
				if err := json.Unmarshal([]byte(dvalue), &value); err != nil {
					value = dvalue
				}
				v[key] = value
			}
		} else {
			if v[key] == nil {
				if _, err := strconv.Atoi(keys[1]); err == nil {
					v[key] = []any{}
				} else {
					v[key] = map[string]any{}
				}
			}
			v[key] = JsonSetValue(v[key], keys[1:], dvalue, str)
		}
		return v
	case []any:
		if idx, err := strconv.Atoi(keys[0]); err == nil {
			for idx >= len(v) {
				v = append(v, nil)
			}
			if len(keys) == 1 {
				if str {
					v[idx] = dvalue
				} else {
					var value any
					if err := json.Unmarshal([]byte(dvalue), &value); err != nil {
						value = dvalue
					}
					v[idx] = value
				}
			} else {
				if v[idx] == nil {
					if _, err := strconv.Atoi(keys[1]); err == nil {
						v[idx] = []any{}
					} else {
						v[idx] = map[string]any{}
					}
				}
				v[idx] = JsonSetValue(v[idx], keys[1:], dvalue, str)
			}
			return v
		}
	}
	return data
}
