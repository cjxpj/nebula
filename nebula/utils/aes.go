package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"sync"
)

var cipherPool = sync.Pool{
	New: func() any {
		return &cipherData{}
	},
}

type cipherData struct {
	block cipher.Block
	gcm   cipher.AEAD
}

// 加密函数
func Encrypt(plaintext string, key []byte) (string, error) {
	data := cipherPool.Get().(*cipherData)
	defer cipherPool.Put(data)

	var err error
	if data.block == nil {
		data.block, err = aes.NewCipher(key)
		if err != nil {
			return "", err
		}
	}
	data.gcm, err = cipher.NewGCM(data.block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, data.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := data.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// 解密函数
func Decrypt(ciphertext string, key []byte) (string, error) {
	ciphertextBytes, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	data := cipherPool.Get().(*cipherData)
	defer cipherPool.Put(data)

	if data.block == nil {
		data.block, err = aes.NewCipher(key)
		if err != nil {
			return "", err
		}
	}
	data.gcm, err = cipher.NewGCM(data.block)
	if err != nil {
		return "", err
	}

	nonceSize := data.gcm.NonceSize()
	if len(ciphertextBytes) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertextBytes := ciphertextBytes[:nonceSize], ciphertextBytes[nonceSize:]
	plaintext, err := data.gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
