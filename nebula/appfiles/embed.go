package appfiles

import "embed"

//go:embed static/*
var content embed.FS

func GetFileString(filename string) string {
	data, _ := content.ReadFile("static/" + filename)
	return string(data)
}

func GetFile(filename string) []byte {
	data, _ := content.ReadFile("static/" + filename)
	return data
}

// 秘钥
var Key []byte = []byte("cjxpj2960965389 nebula0047 juice") // 32 字节用于 AES-256

// 版本号
var Version string = "15.0.1"
