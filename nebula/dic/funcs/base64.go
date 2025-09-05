package funcs

import "encoding/base64"

func (f *DicFunc) Base64En() string {
	if f.Len == 1 {
		return base64.StdEncoding.EncodeToString([]byte(f.Inputs.String(1)))
	}
	return ""
}

func (f *DicFunc) Base64De() string {
	if f.Len == 1 {
		data, err := base64.StdEncoding.DecodeString(f.Inputs.String(1))
		if err != nil {
			return ""
		}
		return string(data)
	}
	return ""
}
