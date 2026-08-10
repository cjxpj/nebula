package funcs

import (
	"bytes"
	"errors"
	"regexp"
	stdjson "encoding/json"
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
	raw := d.Inputs.String(1)
	if raw == "" {
		return nil, errors.New("不是json格式")
	}
	var obj any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
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
	raw := d.Inputs.String(1)
	if raw == "" {
		return nil, errors.New("不是json格式")
	}
	var obj any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
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
	raw := f.Inputs.String(1)
	if raw == "" {
		return "", errors.New("不是json格式")
	}
	if !stdjson.Valid([]byte(raw)) {
		return "", errors.New("不是json格式")
	}
	indent := " "
	if n, err := strconv.Atoi(f.Inputs.String(2)); err == nil && n > 0 {
		indent = strings.Repeat(" ", n)
	}
	var buf bytes.Buffer
	if err := stdjson.Indent(&buf, []byte(raw), "", indent); err != nil {
		return "", err
	}
	return buf.String(), nil
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

// 根据键列表设置 any 的值
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

// 不指定值时：$JSON重名解析 [json] [键->子键...]$ 等同于 JSON解析
// jsonUnmarshalDup 解析含重名key的JSON，重名值合并为数组
func jsonUnmarshalDup(raw string) (any, error) {
	dec := stdjson.NewDecoder(strings.NewReader(raw))
	return parseValue(dec)
}

func parseValue(dec *stdjson.Decoder) (any, error) {
	t, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch v := t.(type) {
	case stdjson.Delim:
		if v == '{' {
			return parseObject(dec)
		}
		if v == '[' {
			var arr []any
			for dec.More() {
				elem, err := parseValue(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, elem)
			}
			return arr, nil
		}
	case bool, float64, string:
		return v, nil
	case nil:
		return nil, nil
	}
	return t, nil
}

func parseObject(dec *stdjson.Decoder) (any, error) {
	result := make(map[string]any)
	dupKeys := make(map[string]bool)
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key := kt.(string)
		val, err := parseValue(dec)
		if err != nil {
			return nil, err
		}
		if existing, ok := result[key]; ok {
			if !dupKeys[key] {
				result[key] = []any{existing, val}
				dupKeys[key] = true
			} else {
				result[key] = append(result[key].([]any), val)
			}
		} else {
			result[key] = val
		}
	}
	return result, nil
}

func jsonQueryByName(d *dto.DicInputs) (any, error) {
	if !d.Inputs.LenOk(2) {
		return "", errors.New("参数错误")
	}
	var obj any
	parsed, err := jsonUnmarshalDup(d.Inputs.String(1))
	if err != nil {
		return "", errors.New("不是json格式")
	}
	obj = parsed
	keys := strings.Split(d.Inputs.String(2), "->")

	switch data := obj.(type) {
	case []any:
		// 第一段是数字 → 按索引访问: 0->age, 1->name
		if idx, err := strconv.Atoi(keys[0]); err == nil {
			if idx < 0 || idx >= len(data) {
				return "", errors.New("索引超出范围")
			}
			obj = data[idx]
			for _, k := range keys[1:] {
				switch sub := obj.(type) {
				case map[string]any:
					obj = sub[k]
				case []any:
					if idx2, err := strconv.Atoi(k); err == nil && idx2 >= 0 && idx2 < len(sub) {
						obj = sub[idx2]
					} else {
						return "", nil
					}
				default:
					return "", nil
				}
			}
		} else {
			// 否则按 key=value 匹配: name->Bob->age
			if len(keys) < 2 {
				return "", errors.New("数组查询至少需要 键->值")
			}
			seekKey := keys[0]
			seekVal := keys[1]
			for _, elem := range data {
				if m, ok := elem.(map[string]any); ok {
					if v, exists := m[seekKey]; exists {
						if utils.AnyIsString(v) == seekVal {
							obj = elem
							for _, k := range keys[2:] {
								switch sub := obj.(type) {
								case map[string]any:
									obj = sub[k]
								case []any:
									if idx, err := strconv.Atoi(k); err == nil && idx >= 0 && idx < len(sub) {
										obj = sub[idx]
									} else {
										return "", nil
									}
								default:
									return "", nil
								}
							}
							break
						}
					}
				}
			}
		}
	default:
		// 非数组，等同于 JSON解析
		for _, key := range keys {
			switch d := obj.(type) {
			case map[string]any:
				if val, ok := d[key]; ok {
					obj = val
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
	}

	switch v := obj.(type) {
	case string:
		return v, nil
	case bool:
		return strconv.FormatBool(v), nil
	case map[string]any, []any:
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	return "", nil
}

func queryJson(d *dto.DicInputs) (any, error) {
	if !d.Inputs.LenOk(2) {
		return "", errors.New("参数错误")
	}
	raw := d.Inputs.String(1)
	if raw == "" {
		return "", errors.New("不是json格式")
	}
	var obj any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return "", errors.New("不是json格式")
	}
	keys := strings.Split(d.Inputs.String(2), "->")
	for _, key := range keys {
		switch d := obj.(type) {
		case map[string]any:
			if val, ok := d[key]; ok {
				obj = val
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
	case bool:
		return strconv.FormatBool(v), nil
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10), nil
		}
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case map[string]any, []any:
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	return "", nil
}

func isJson(d *dto.DicInputs) (any, error) {
	var js any
	if err := json.Unmarshal([]byte(d.Inputs.String(1)), &js); err != nil {
		return "false", nil
	}
	return "true", nil
}

func jsonAdd(d *dto.DicInputs) (any, error) {
	raw := d.Inputs.String(1)
	if raw == "" {
		return "", errors.New("不是json格式")
	}

	var obj any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return "", errors.New("不是json格式")
	}

	switch v := obj.(type) {
	case map[string]any:
		keys := strings.Split(d.Inputs.String(2), ".")
		curr := v
		for i, key := range keys {
			if i == len(keys)-1 {
				if d.Inputs.Len() > 2 {
					var val any
					if err := json.Unmarshal([]byte(d.Inputs.String(3)), &val); err != nil {
						curr[key] = d.Inputs.String(3)
					} else {
						curr[key] = val
					}
				} else {
					curr[key] = nil
				}
			} else {
				if next, ok := curr[key]; ok {
					if m, ok := next.(map[string]any); ok {
						curr = m
					} else {
						return "", errors.New("路径中存在非对象类型")
					}
				} else {
					newObj := make(map[string]any)
					curr[key] = newObj
					curr = newObj
				}
			}
		}
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(b), nil

	case []any:
		var val any
		if err := json.Unmarshal([]byte(d.Inputs.String(2)), &val); err != nil {
			v = append(v, d.Inputs.String(2))
		} else {
			v = append(v, val)
		}
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(b), nil

	default:
		return "", errors.New("JSON追加仅支持对象或数组")
	}
}

func jsonAddString(d *dto.DicInputs) (any, error) {
	raw := d.Inputs.String(1)
	if raw == "" {
		return "", errors.New("不是json格式")
	}

	var obj any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return "", errors.New("不是json格式")
	}

	switch v := obj.(type) {
	case map[string]any:
		keys := strings.Split(d.Inputs.String(2), ".")
		curr := v
		for i, key := range keys {
			if i == len(keys)-1 {
				if d.Inputs.Len() > 2 {
					curr[key] = d.Inputs.String(3)
				} else {
					curr[key] = ""
				}
			} else {
				if next, ok := curr[key]; ok {
					if m, ok := next.(map[string]any); ok {
						curr = m
					} else {
						return "", errors.New("路径中存在非对象类型")
					}
				} else {
					newObj := make(map[string]any)
					curr[key] = newObj
					curr = newObj
				}
			}
		}
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(b), nil

	case []any:
		v = append(v, d.Inputs.String(2))
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(b), nil

	default:
		return "", errors.New("JSON追加仅支持对象或数组")
	}
}

func jsonDelete(d *dto.DicInputs) (any, error) {
	raw := d.Inputs.String(1)
	if raw == "" {
		return "", errors.New("不是json格式")
	}

	var obj any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return "", errors.New("不是json格式")
	}

	switch v := obj.(type) {
	case map[string]any:
		keys := strings.Split(d.Inputs.String(2), ".")
		curr := v
		for i, key := range keys {
			if i == len(keys)-1 {
				delete(curr, key)
			} else {
				if next, ok := curr[key]; ok {
					if m, ok := next.(map[string]any); ok {
						curr = m
					} else {
						return "", errors.New("路径中存在非对象类型")
					}
				} else {
					return "", errors.New("路径不存在")
				}
			}
		}
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(b), nil

	case []any:
		idx, err := strconv.Atoi(d.Inputs.String(2))
		if err != nil || idx < 0 || idx >= len(v) {
			return "", errors.New("数组索引无效")
		}
		v = append(v[:idx], v[idx+1:]...)
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(b), nil

	default:
		return "", errors.New("JSON删仅支持对象或数组")
	}
}

func jsonIsKey(d *dto.DicInputs) (any, error) {
	raw := d.Inputs.String(1)
	if raw == "" {
		return "false", nil
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return "false", nil
	}
	keys := strings.Split(d.Inputs.String(2), ".")
	curr := obj
	for i, key := range keys {
		if i == len(keys)-1 {
			if _, ok := curr[key]; ok {
				return "true", nil
			}
			return "false", nil
		}
		if next, ok := curr[key]; ok {
			if m, ok := next.(map[string]any); ok {
				curr = m
			} else {
				return "false", nil
			}
		} else {
			return "false", nil
		}
	}
	return "false", nil
}

func jsonLen(d *dto.DicInputs) (any, error) {
	raw := d.Inputs.String(1)
	if raw == "" {
		return "", errors.New("不是json格式")
	}
	var obj any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return "", errors.New("不是json格式")
	}
	switch v := obj.(type) {
	case map[string]any:
		return strconv.Itoa(len(v)), nil
	case []any:
		return strconv.Itoa(len(v)), nil
	}
	return "0", nil
}

func jsonPrettyPrint(d *dto.DicInputs) (any, error) {
	raw := d.Inputs.String(1)
	if raw == "" {
		return "", errors.New("不是json格式")
	}
	if !stdjson.Valid([]byte(raw)) {
		return "", errors.New("不是json格式")
	}
	indent := "  "
	if d.Inputs.LenOk(2) {
		indent = d.Inputs.String(2)
	}
	var buf bytes.Buffer
	if err := stdjson.Indent(&buf, []byte(raw), "", indent); err != nil {
		return "", err
	}
	return buf.String(), nil
}
