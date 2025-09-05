package funcs

import "strings"

func (f *DicFunc) ToUpper() string {
	if f.Len == 1 {
		uppercase := strings.ToUpper(f.Inputs.String(1))
		return uppercase
	}
	return ""
}

func (f *DicFunc) ToLower() string {
	if f.Len == 1 {
		lowercase := strings.ToLower(f.Inputs.String(1))
		return lowercase
	}
	return ""
}
