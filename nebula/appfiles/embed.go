package appfiles

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

//go:embed static/*
var content embed.FS

func GetFileString(filename string) (string, error) {
	data, err := content.ReadFile(path.Join("static", filename))
	if err != nil {
		return "", fmt.Errorf("read embedded file %s: %w", filename, err)
	}
	return string(data), nil
}

func GetFile(filename string) ([]byte, error) {
	data, err := content.ReadFile(path.Join("static", filename))
	if err != nil {
		return nil, fmt.Errorf("read embedded file %s: %w", filename, err)
	}
	return data, nil
}

// ListFiles 列出 embed 资源中某个目录下（含各级子目录）的全部文件，
// 返回相对 static 的路径（以 / 分隔），按路径字典序排序。
func ListFiles(dir string) ([]string, error) {
	root := path.Join("static", dir)
	names := []string{}
	err := fs.WalkDir(content, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		names = append(names, strings.TrimPrefix(p, "static/"))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list embedded dir %s: %w", dir, err)
	}
	sort.Strings(names)
	return names, nil
}

// 秘钥（固定，32 字节用于 AES-256）
var Key []byte = []byte("cjxpj2960965389 nebula0052 juice")

// 版本号
var Version string = "20.3.1"
