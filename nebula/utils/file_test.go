package utils

import (
	"path/filepath"
	"testing"

	"github.com/cjxpj/nebula/log"
)

func TestDebug(t *testing.T) {
	t.Run("TestFileExists", func(t *testing.T) {
		path := "a/b/c.txt"
		newName := "ok"
		// 获取原文件所在目录
		dir := filepath.Dir(path)

		// 构造新文件完整路径
		newPath := filepath.Join(dir, newName)
		log.Debug(newPath)
	})

	t.Run("TestFileExists", func(t *testing.T) {
		path := "a/b/c.txt"
		newName := "ok"

		newPath := filepath.Join(newName, filepath.Base(path))
		log.Debug(newPath)
	})
}
