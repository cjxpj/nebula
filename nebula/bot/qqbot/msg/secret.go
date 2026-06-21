package qqbot_msg

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// 签名
func GenerateValidationResult(botSecret, eventTs, plainToken string) (string, error) {
	// QQ Bot 官方算法：botSecret 作为原始字符串，不足 32 字节时 repeat 扩展
	seed := botSecret
	for len(seed) < ed25519.SeedSize {
		seed = strings.Repeat(seed, 2)
	}
	seed = seed[:ed25519.SeedSize]

	// 使用 seed 通过 NewKeyFromSeed 派生密钥对
	privateKey := ed25519.NewKeyFromSeed([]byte(seed))

	// 拼接签名内容: eventTs + plainToken
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

// VerifyWebhookSignature 验证 QQ Bot Webhook 消息签名
// signatureHex: X-Signature-Ed25519 头的值
// timestamp:    X-Signature-Timestamp 头的值
// body:         HTTP 请求体的原始字节
func VerifyWebhookSignature(botSecret, signatureHex, timestamp string, body []byte) bool {
	// QQ Bot 官方算法：botSecret 作为原始字符串派生密钥对
	seed := botSecret
	for len(seed) < ed25519.SeedSize {
		seed = strings.Repeat(seed, 2)
	}
	seed = seed[:ed25519.SeedSize]

	privateKey := ed25519.NewKeyFromSeed([]byte(seed))
	publicKey := privateKey.Public().(ed25519.PublicKey)

	sigBytes, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false
	}

	// QQ Bot Webhook 签名原文: timestamp + body
	var msg bytes.Buffer
	msg.WriteString(timestamp)
	msg.Write(body)

	return ed25519.Verify(publicKey, msg.Bytes(), sigBytes)
}
