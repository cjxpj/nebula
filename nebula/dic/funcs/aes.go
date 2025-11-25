package funcs

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/cjxpj/nebula/dto"
)

func aesEncrypt(d *dto.DicInputs) (any, error) {
	mode := d.Inputs.String(1)
	key := []byte(d.Inputs.String(2))
	plaintext := d.Inputs.String(3)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("创建加密器失败: %v", err)
	}

	var ciphertext []byte
	switch mode {
	case "CFB":
		ciphertext = make([]byte, aes.BlockSize+len(plaintext))
		iv := ciphertext[:aes.BlockSize]
		stream := cipher.NewCFBEncrypter(block, iv)
		stream.XORKeyStream(ciphertext[aes.BlockSize:], []byte(plaintext))
	case "CBC":
		padding := aes.BlockSize - len(plaintext)%aes.BlockSize
		padText := bytes.Repeat([]byte{byte(padding)}, padding)
		plaintext = string(plaintext) + string(padText)
		ciphertext = make([]byte, len(plaintext))
		iv := make([]byte, aes.BlockSize)
		mode := cipher.NewCBCEncrypter(block, iv)
		mode.CryptBlocks(ciphertext, []byte(plaintext))
	default:
		return "", errors.New("不支持的模式")
	}

	return hex.EncodeToString(ciphertext), nil
}

func aesDecrypt(d *dto.DicInputs) (any, error) {
	mode := d.Inputs.String(1)
	key := []byte(d.Inputs.String(2))
	ciphertext, err := hex.DecodeString(d.Inputs.String(3))
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	var plaintext []byte
	switch mode {
	case "CFB":
		if len(ciphertext) < aes.BlockSize {
			return "", errors.New("密文太短")
		}
		iv := ciphertext[:aes.BlockSize]
		ciphertext = ciphertext[aes.BlockSize:]
		stream := cipher.NewCFBDecrypter(block, iv)
		stream.XORKeyStream(ciphertext, ciphertext)
		plaintext = ciphertext
	case "CBC":
		if len(ciphertext) < aes.BlockSize {
			return "", errors.New("密文太短")
		}
		iv := make([]byte, aes.BlockSize)
		mode := cipher.NewCBCDecrypter(block, iv)
		mode.CryptBlocks(ciphertext, ciphertext)
		padding := int(ciphertext[len(ciphertext)-1])
		plaintext = ciphertext[:len(ciphertext)-padding]
	default:
		return "", errors.New("不支持的模式")
	}

	return string(plaintext), nil
}
