package dto

// 设置值
func (s *SingleValue) Set(v string) *SingleValue {
	s.Data = v
	return s
}

// 追加值
func (s *SingleValue) Add(v string) *SingleValue {
	s.Data += v
	return s
}

// 反着追加值
func (s *SingleValue) Prepend(v string) *SingleValue {
	s.Data = v + s.Data
	return s
}

// 清空值
func (s *SingleValue) Clear() *SingleValue {
	s.Data = ""
	return s
}

// 获取值
func (s *SingleValue) Get() string {
	return s.Data
}
