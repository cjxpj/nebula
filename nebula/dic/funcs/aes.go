package funcs

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/cjxpj/nebula/dto"
)

func aesCBCEncrypt(d *dto.DicInputs) (any, error) {
	key := []byte(d.Inputs.String(1))
	iv := []byte(d.Inputs.String(2))
	plaintext := d.Inputs.String(3)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("创建加密器失败: %v", err)
	}

	if len(iv) != aes.BlockSize {
		return "", fmt.Errorf("IV长度必须为%d字节", aes.BlockSize)
	}

	padding := aes.BlockSize - len(plaintext)%aes.BlockSize
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	plaintext = string(plaintext) + string(padText)
	ciphertext := make([]byte, len(plaintext))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, []byte(plaintext))

	return hex.EncodeToString(ciphertext), nil
}

func aesCBCDecrypt(d *dto.DicInputs) (any, error) {
	key := []byte(d.Inputs.String(1))
	iv := []byte(d.Inputs.String(2))
	ciphertext, err := hex.DecodeString(d.Inputs.String(3))
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	if len(iv) != aes.BlockSize {
		return "", fmt.Errorf("IV长度必须为%d字节", aes.BlockSize)
	}

	if len(ciphertext) < aes.BlockSize {
		return "", errors.New("密文太短")
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(ciphertext, ciphertext)
	padding := int(ciphertext[len(ciphertext)-1])
	plaintext := ciphertext[:len(ciphertext)-padding]

	return string(plaintext), nil
}

func aesCFBEncrypt(d *dto.DicInputs) (any, error) {
	key := []byte(d.Inputs.String(1))
	iv := []byte(d.Inputs.String(2))
	plaintext := d.Inputs.String(3)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("创建加密器失败: %v", err)
	}

	if len(iv) != aes.BlockSize {
		return "", fmt.Errorf("IV长度必须为%d字节", aes.BlockSize)
	}

	ciphertext := make([]byte, len(plaintext))
	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(ciphertext, []byte(plaintext))

	return hex.EncodeToString(ciphertext), nil
}

func aesCFBDecrypt(d *dto.DicInputs) (any, error) {
	key := []byte(d.Inputs.String(1))
	iv := []byte(d.Inputs.String(2))
	ciphertext, err := hex.DecodeString(d.Inputs.String(3))
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	if len(iv) != aes.BlockSize {
		return "", fmt.Errorf("IV长度必须为%d字节", aes.BlockSize)
	}

	if len(ciphertext) < aes.BlockSize {
		return "", errors.New("密文太短")
	}

	stream := cipher.NewCFBDecrypter(block, iv)
	stream.XORKeyStream(ciphertext, ciphertext)
	plaintext := ciphertext

	return string(plaintext), nil
}

func aesGCMEncrypt(d *dto.DicInputs) (any, error) {
	key := []byte(d.Inputs.String(1))
	plaintext := d.Inputs.String(2)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("创建加密器失败: %v", err)
	}

	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("生成随机数失败: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("创建GCM加密器失败: %v", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	return hex.EncodeToString(ciphertext), nil
}

func aesGCMDecrypt(d *dto.DicInputs) (any, error) {
	key := []byte(d.Inputs.String(1))
	ciphertext, err := hex.DecodeString(d.Inputs.String(2))
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	if len(ciphertext) < 28 {
		return "", errors.New("密文太短")
	}
	nonce := ciphertext[:12]
	ciphertext = ciphertext[12:]
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("创建GCM解密器失败: %v", err)
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("GCM解密失败: %v", err)
	}

	return string(plaintext), nil
}

func aesCTREncrypt(d *dto.DicInputs) (any, error) {
	key := []byte(d.Inputs.String(1))
	iv := []byte(d.Inputs.String(2))
	plaintext := d.Inputs.String(3)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("创建加密器失败: %v", err)
	}

	if len(iv) != aes.BlockSize {
		return "", fmt.Errorf("IV长度必须为%d字节", aes.BlockSize)
	}

	ciphertext := make([]byte, len(plaintext))
	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(ciphertext, []byte(plaintext))

	return hex.EncodeToString(ciphertext), nil
}

func aesCTRDecrypt(d *dto.DicInputs) (any, error) {
	key := []byte(d.Inputs.String(1))
	iv := []byte(d.Inputs.String(2))
	ciphertext, err := hex.DecodeString(d.Inputs.String(3))
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	if len(iv) != aes.BlockSize {
		return "", fmt.Errorf("IV长度必须为%d字节", aes.BlockSize)
	}

	if len(ciphertext) < aes.BlockSize {
		return "", errors.New("密文太短")
	}

	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(ciphertext, ciphertext)
	plaintext := ciphertext

	return string(plaintext), nil
}
