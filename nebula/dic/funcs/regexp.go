package funcs

import (
	"errors"
	"regexp"
	"strconv"

	"github.com/cjxpj/nebula/dto"
)

// 正则
func regexpFind(d *dto.DicInputs) (any, error) {
	matcheA, err := regexp.Compile(d.Inputs.String(1))
	if err != nil {
		return "", nil
	}
	matches := matcheA.FindStringSubmatch(d.Inputs.String(2))

	resS, err := json.Marshal(matches)

	if err != nil {
		return "", nil
	}
	return string(resS), nil
}

// 正则匹配
func regexpMatche(d *dto.DicInputs) (any, error) {
	matches, _ := regexp.MatchString("^"+d.Inputs.String(1)+"$", d.Inputs.String(2))
	if matches {
		return "true", nil
	}
	return "false", nil
}

// 正则替换
func regexReplace(d *dto.DicInputs) (any, error) {
	if !d.Inputs.LenOk(3, 4) {
		return "", nil
	}

	src := d.Inputs.String(1)
	pattern := d.Inputs.String(2)
	repl := d.Inputs.String(3)

	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", errors.New("正则表达式错误")
	}

	// 指定替换次数
	if d.Inputs.LenOk(4) {
		n, err := strconv.Atoi(d.Inputs.String(4))
		if err != nil {
			return "", errors.New("非数字")
		}
		if n <= 0 {
			return src, nil
		}

		res := src
		for range n {
			loc := re.FindStringIndex(res)
			if loc == nil {
				break
			}
			res = res[:loc[0]] + re.ReplaceAllString(res[loc[0]:loc[1]], repl) + res[loc[1]:]
		}
		return res, nil
	}

	// 全部替换
	return re.ReplaceAllString(src, repl), nil
}
