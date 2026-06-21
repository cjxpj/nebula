package appfiles

import (
	"embed"
	"fmt"
	"os"
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

// 秘钥（可通过环境变量 NEBULA_KEY 覆盖）
var Key []byte = initKey()

func initKey() []byte {
	if envKey := os.Getenv("NEBULA_KEY"); envKey != "" {
		k := []byte(envKey)
		if len(k) == 32 {
			return k
		}
	}
	return []byte("cjxpj2960965389 nebula0051 juice") // 32 字节用于 AES-256
}

// 版本号
var Version string = "16.13.0"
