package funcs

import (
	"errors"
	"strconv"
	"strings"
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
	if json.Unmarshal([]byte(s), &m) == nil || json.Unmarshal([]byte(s), &a) == nil {
		return "true"
	}
	return "false"
}

func (f *DicFunc) JsonSet() string {
	if f.Len < 3 {
		return f.Inputs.String(1)
	}
	var obj any
	if err := json.Unmarshal([]byte(f.Inputs.String(1)), &obj); err != nil {
		return f.Inputs.String(1)
	}
	keys := make([]string, 0, f.Len-2)
	for _, key := range f.Inputs.List[3:] {
		if s, ok := key.(string); ok {
			keys = append(keys, s)
		}
	}
	obj = JsonSetValue(obj, keys, f.Inputs.String(2), false)
	b, _ := json.Marshal(obj)
	return string(b)
}

func (f *DicFunc) JsonSetString() string {
	if f.Len < 3 {
		return f.Inputs.String(1)
	}
	var obj any
	if err := json.Unmarshal([]byte(f.Inputs.String(1)), &obj); err != nil {
		return f.Inputs.String(1)
	}
	keys := make([]string, 0, f.Len-2)
	for _, key := range f.Inputs.List[2:] {
		if s, ok := key.(string); ok {
			keys = append(keys, s)
		}
	}
	obj = JsonSetValue(obj, keys, f.Inputs.String(2), true)
	b, _ := json.Marshal(obj)
	return string(b)
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
