package appfiles

import (
	"embed"
	"fmt"
)

//go:embed static/*
var content embed.FS

func GetFileString(filename string) (string, error) {
	data, err := content.ReadFile("static/" + filename)
	if err != nil {
		return "", fmt.Errorf("read embedded file %s: %w", filename, err)
	}
	return string(data), nil
}

func GetFile(filename string) ([]byte, error) {
	data, err := content.ReadFile("static/" + filename)
	if err != nil {
		return nil, fmt.Errorf("read embedded file %s: %w", filename, err)
	}
	return data, nil
}

// 秘钥（固定，32 字节用于 AES-256）
var Key []byte = []byte("cjxpj2960965389 nebula0052 juice")

// 版本号
var Version string = "16.18.4"
