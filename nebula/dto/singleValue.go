package dto

// 设置值
func (s *SingleValue) Set(v string) *SingleValue {
	s.Data.Reset()
	s.Data.WriteString(v)
	return s
}

// 追加值
func (s *SingleValue) Add(v string) *SingleValue {
	s.Data.WriteString(v)
	return s
}

// 清空值
func (s *SingleValue) Clear() {
	s.Data.Reset()
}

// 获取值
func (s *SingleValue) Get() string {
	return s.Data.String()
}
