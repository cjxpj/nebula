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

func (f *DicFunc) AesEn(V *dto.Val) (string, error) {
	if f.Len == 3 {
		mode := f.Inputs.String(1)
		key := []byte(f.Inputs.String(2))
		plaintext := f.Inputs.String(3)

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
	return "", errors.New("参数数量不正确")
}

func (f *DicFunc) AesDe(V *dto.Val) string {
	if f.Len == 3 {
		V.Set("报错", "null")
		mode := f.Inputs.String(1)
		key := []byte(f.Inputs.String(2))
		ciphertext, err := hex.DecodeString(f.Inputs.String(3))
		if err != nil {
			V.Set("报错", err.Error())
			return ""
		}

		block, err := aes.NewCipher(key)
		if err != nil {
			V.Set("报错", err.Error())
			return ""
		}

		var plaintext []byte
		switch mode {
		case "CFB":
			if len(ciphertext) < aes.BlockSize {
				V.Set("报错", "密文太短")
				return ""
			}
			iv := ciphertext[:aes.BlockSize]
			ciphertext = ciphertext[aes.BlockSize:]
			stream := cipher.NewCFBDecrypter(block, iv)
			stream.XORKeyStream(ciphertext, ciphertext)
			plaintext = ciphertext
		case "CBC":
			if len(ciphertext) < aes.BlockSize {
				V.Set("报错", "密文太短")
				return ""
			}
			iv := make([]byte, aes.BlockSize)
			mode := cipher.NewCBCDecrypter(block, iv)
			mode.CryptBlocks(ciphertext, ciphertext)
			padding := int(ciphertext[len(ciphertext)-1])
			plaintext = ciphertext[:len(ciphertext)-padding]
		default:
			V.Set("报错", "不支持的模式")
			return ""
		}

		return string(plaintext)
	}
	return ""
}
