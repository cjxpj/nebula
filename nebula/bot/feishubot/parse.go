package feishubot

import (
	"encoding/json"
	"fmt"

	"github.com/cjxpj/nebula/dto"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
)

// decryptIfNeeded 若为加密事件（{"encrypt":"..."}）则解密，否则原样返回
func decryptIfNeeded(cipher []byte) ([]byte, error) {
	var tmp map[string]json.RawMessage
	if err := json.Unmarshal(cipher, &tmp); err != nil {
		return nil, err
	}
	encryptField, ok := tmp["encrypt"]
	if !ok {
		return cipher, nil
	}

	var encrypt string
	if err := json.Unmarshal(encryptField, &encrypt); err != nil {
		return nil, err
	}

	secret := ""
	if bot := dto.ServerConfig.FeiShuBot; bot != nil {
		secret = bot.EncryptKey
	}
	if secret == "" {
		return nil, fmt.Errorf("收到加密事件，但未配置飞书「加密密钥」")
	}
	// AES-256-CBC，key = sha256(加密密钥)，密文前 16 字节为 IV
	return larkevent.EventDecrypt(encrypt, secret)
}

// checkToken 校验事件订阅验证令牌；未配置验证令牌时不做校验
func checkToken(token string) bool {
	if bot := dto.ServerConfig.FeiShuBot; bot != nil && bot.VerificationToken != "" {
		return token == bot.VerificationToken
	}
	return true
}
