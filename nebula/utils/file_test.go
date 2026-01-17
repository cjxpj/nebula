package utils

import (
	"path/filepath"
	"testing"

	"github.com/cjxpj/nebula/debugLog"
)

func TestDebug(t *testing.T) {
	t.Run("TestFileExists", func(t *testing.T) {
		path := "a/b/c.txt"
		newName := "ok"
		// 获取原文件所在目录
		dir := filepath.Dir(path)

		// 构造新文件完整路径
		newPath := filepath.Join(dir, newName)
		debugLog.Debug(newPath)
	})

	t.Run("TestFileExists", func(t *testing.T) {
		path := "a/b/c.txt"
		newName := "ok"

		newPath := filepath.Join(newName, filepath.Base(path))
		debugLog.Debug(newPath)
	})

	t.Run("TestMd", func(t *testing.T) {
		raw := `1. 这是*强调*、_下划线_、[链接](url)、` + "`" + `代码` + "`" + `、# 标题
2. 反斜杠\、管道|、尖括号<tag>、加减-和+、点号.、感叹号!`
		debugLog.Debug(MDEscape(raw))
	})
}
