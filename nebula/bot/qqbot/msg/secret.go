package qqbot_msg

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// 签名
func GenerateValidationResult(botSecret, eventTs, plainToken string) (string, error) {
	// 补齐或截断 seed
	seed := botSecret
	// for len(seed) < ed25519.SeedSize {
	// 	seed += botSecret
	// }
	// seed = seed[:ed25519.SeedSize]
	// 不足长度拦截
	if len(seed) > ed25519.SeedSize {
		return "", fmt.Errorf("botSecret too long")
	}
	// 不足长度拦截
	if len(seed) < ed25519.SeedSize {
		return "", fmt.Errorf("botSecret too short")
	}

	// 使用 seed 派生私钥
	privateKey := ed25519.NewKeyFromSeed([]byte(seed))

	// 拼接签名内容
	var msg bytes.Buffer
	msg.WriteString(eventTs)
	msg.WriteString(plainToken)

	// 生成签名
	signature := hex.EncodeToString(ed25519.Sign(privateKey, msg.Bytes()))

	// 构造结果
	result := ValidationResult{
		PlainToken: plainToken,
		Signature:  signature,
	}

	// 转 JSON 字符串
	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshal result failed: %w", err)
	}

	return string(jsonBytes), nil
}
