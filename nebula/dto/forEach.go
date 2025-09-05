package dto

// 关闭
func (s *LocalDicValueForEach) Close() {
	s.Success = false
	s.IsFor = false
	s.Jump = false
	s.Run = nil
	s.Num = 0
	s.VlaueName = ""
	s.Content = nil
}
