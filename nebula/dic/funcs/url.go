package funcs

import "net/url"

func (f *DicFunc) UrlEn() string {
	if f.Len == 1 {
		return url.QueryEscape(f.Inputs.String(1))
	}
	return ""
}

func (f *DicFunc) UrlDe() string {
	if f.Len == 1 {
		data, err := url.QueryUnescape(f.Inputs.String(1))
		if err != nil {
			return ""
		}
		return data
	}
	return ""

}

func (f *DicFunc) UrlPathEn() string {
	if f.Len == 1 {
		return url.PathEscape(f.Inputs.String(1))
	}
	return ""
}

func (f *DicFunc) UrlPathDe() string {
	if f.Len == 1 {
		data, err := url.PathUnescape(f.Inputs.String(1))
		if err != nil {
			return ""
		}
		return data
	}
	return ""
}
