package dto

// 关闭
func (s *LocalDicValueForEach) Close() {
	s.IsFor = false
	s.Jump = false
	s.Run = nil
	s.Num = 0
	s.ValueName = ""
	s.Content = nil
	s.LineNums = nil
}
