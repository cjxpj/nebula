package funcs

import (
	"regexp"
)

func (f *DicFunc) Regexp() string {
	if f.Len == 2 {
		matcheA, err := regexp.Compile(f.Inputs.String(1))
		if err != nil {
			return ""
		}
		matches := matcheA.FindStringSubmatch(f.Inputs.String(2))

		resS, err := json.Marshal(matches)

		if err != nil {
			return ""
		}
		return string(resS)
	}
	return "null"
}

func (f *DicFunc) RegexpMatche() string {
	if f.Len == 2 {
		matches, _ := regexp.MatchString("^"+f.Inputs.String(1)+"$", f.Inputs.String(2))
		if matches {
			return "true"
		}
		return "false"
	}
	return "false"
}
