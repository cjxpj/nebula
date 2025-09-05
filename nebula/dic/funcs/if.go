package funcs

func (f *DicFunc) IfNull() string {
	if f.Len == 1 {
		switch f.Inputs.String(1) {
		case "null",
			"nil",
			"false",
			"{}",
			"[]",
			"空",
			"NaN",
			"undefined",
			" ",
			"":
			return "true"
		}
	}
	return "false"
}

func (f *DicFunc) IfNONull() string {
	if f.Len == 1 {
		switch f.Inputs.String(1) {
		case "null",
			"nil",
			"false",
			"{}",
			"[]",
			"空",
			"NaN",
			"undefined",
			" ",
			"":
			return "false"
		}
	}
	return "true"
}
