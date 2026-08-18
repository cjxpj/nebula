package funcs

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"

	"github.com/cjxpj/nebula/dto"
)

// RSA生成密钥：可选参数为密钥位数，默认 2048
func rsaGenerateKey(d *dto.DicInputs) (any, error) {
	bits := d.Inputs.IntDefault(1, 2048)
	if bits < 1024 || bits > 4096 {
		return "", errors.New("密钥位数必须在 1024~4096 之间")
	}

	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return "", fmt.Errorf("生成RSA密钥失败: %v", err)
	}

	privateDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", fmt.Errorf("编码私钥失败: %v", err)
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})

	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", fmt.Errorf("编码公钥失败: %v", err)
	}
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})

	result := map[string]string{
		"公钥": string(publicPEM),
		"私钥": string(privatePEM),
	}
	jsonResult, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("转换JSON失败: %v", err)
	}
	return string(jsonResult), nil
}

// RSA加密：参数1公钥(PEM)，参数2明文，返回 base64 密文
func rsaEncrypt(d *dto.DicInputs) (any, error) {
	pub, err := parseRSAPublicKey(d.Inputs.String(1))
	if err != nil {
		return "", err
	}

	ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, pub, []byte(d.Inputs.String(2)))
	if err != nil {
		return "", fmt.Errorf("RSA加密失败: %v", err)
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// RSA解密：参数1私钥(PEM)，参数2 base64 密文，返回明文
func rsaDecrypt(d *dto.DicInputs) (any, error) {
	priv, err := parseRSAPrivateKey(d.Inputs.String(1))
	if err != nil {
		return "", err
	}

	ciphertext, err := base64.StdEncoding.DecodeString(d.Inputs.String(2))
	if err != nil {
		return "", fmt.Errorf("密文base64解码失败: %v", err)
	}

	plaintext, err := rsa.DecryptPKCS1v15(rand.Reader, priv, ciphertext)
	if err != nil {
		return "", fmt.Errorf("RSA解密失败: %v", err)
	}
	return string(plaintext), nil
}

// parseRSAPublicKey 解析 PEM 公钥（支持 PKIX 与 PKCS1 两种格式）
func parseRSAPublicKey(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("公钥解析失败：不是有效的PEM格式")
	}

	if pub, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		if rsaPub, ok := pub.(*rsa.PublicKey); ok {
			return rsaPub, nil
		}
		return nil, errors.New("公钥不是RSA公钥")
	}

	if pub, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return pub, nil
	}

	return nil, errors.New("公钥解析失败：不支持的公钥格式")
}

// parseRSAPrivateKey 解析 PEM 私钥（支持 PKCS8 与 PKCS1 两种格式）
func parseRSAPrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("私钥解析失败：不是有效的PEM格式")
	}

	if priv, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaPriv, ok := priv.(*rsa.PrivateKey); ok {
			return rsaPriv, nil
		}
		return nil, errors.New("私钥不是RSA私钥")
	}

	if priv, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return priv, nil
	}

	return nil, errors.New("私钥解析失败：不支持的私钥格式")
}
