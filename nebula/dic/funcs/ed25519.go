package funcs

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"strconv"

	"github.com/cjxpj/nebula/dto"
	"golang.org/x/crypto/curve25519"
)

func ed25519_SeedSize(d *dto.DicInputs) (any, error) {
	return strconv.Itoa(ed25519.SeedSize), nil
}

func ed25519_GenerateKey(d *dto.DicInputs) (any, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", errors.New("生成密钥失败")
	}

	result := map[string]string{
		"公钥": string(publicKey),
		"私钥": string(privateKey),
	}

	jsonResult, err := json.Marshal(result)
	if err != nil {
		return "", errors.New("转换 JSON 失败")
	}

	return string(jsonResult), nil
}

func (f *DicFunc) Ed25519_NewKeyFromSeed() (map[string]string, error) {
	if f.Len != 1 {
		return nil, errors.New("参数数量错误")
	}

	seed := []byte(f.Inputs.String(1))
	if len(seed) != ed25519.SeedSize {
		return nil, errors.New("seed 长度错误，必须为32字节")
	}

	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)

	result := map[string]string{
		"公钥": string(publicKey),
		"私钥": string(privateKey),
	}

	return result, nil
	// jsonResult, err := json.Marshal(result)
	// if err != nil {
	// 	return "", errors.New("转换 JSON 失败")
	// }

	// return string(jsonResult), nil
}

func (f *DicFunc) Ed25519_Sign() (string, error) {
	if f.Len != 2 {
		return "", errors.New("参数数量错误")
	}

	// 获取参数1：私钥，支持 string 或 ed25519.PrivateKey 类型
	var privateKey ed25519.PrivateKey

	switch v := f.Inputs.Get(1).(type) {
	case ed25519.PrivateKey:
		privateKey = v
	case string:
		data := []byte(v)
		switch len(data) {
		case ed25519.SeedSize:
			privateKey = ed25519.NewKeyFromSeed(data)
		case ed25519.PrivateKeySize:
			privateKey = ed25519.PrivateKey(data)
		default:
			return "", errors.New("字符串私钥长度必须为32或64字节")
		}
	default:
		return "", errors.New("参数1必须是字符串或ed25519.PrivateKey类型")
	}

	// 获取参数2：消息
	message := []byte(f.Inputs.String(2))

	// 执行签名
	signature := ed25519.Sign(privateKey, message)

	return string(signature), nil
}

func (f *DicFunc) Ed25519_Verify() (string, error) {
	if f.Len != 3 {
		return "", errors.New("参数数量错误")
	}

	publicKey := []byte(f.Inputs.String(1))
	message := []byte(f.Inputs.String(2))
	signature := []byte(f.Inputs.String(3))

	if len(publicKey) != ed25519.PublicKeySize {
		return "", errors.New("公钥长度错误，必须为32字节")
	}
	if len(signature) != ed25519.SignatureSize {
		return "", errors.New("签名长度错误，必须为64字节")
	}

	verified := ed25519.Verify(publicKey, message, signature)
	if verified {
		return "true", nil
	}
	return "false", nil
}

func (f *DicFunc) Ed25519_PublicKeyToCurve25519() (string, error) {
	if f.Len != 1 {
		return "", errors.New("参数数量错误")
	}

	publicKey := []byte(f.Inputs.String(1))
	if len(publicKey) != 32 {
		return "", errors.New("公钥长度必须为32字节")
	}

	var curvePub [32]byte
	copy(curvePub[:], publicKey)
	return string(curvePub[:]), nil
}

func (f *DicFunc) Ed25519_PrivateKeyToCurve25519() (string, error) {
	if f.Len != 1 {
		return "", errors.New("参数数量错误")
	}

	privateKey := []byte(f.Inputs.String(1))
	if len(privateKey) < 32 {
		return "", errors.New("私钥长度不足，必须至少32字节")
	}

	var curvePriv [32]byte
	copy(curvePriv[:], privateKey[:32])
	return string(curvePriv[:]), nil
}

func (f *DicFunc) Ed25519_NewKeyFromCurve25519() (string, error) {
	if f.Len != 2 {
		return "", errors.New("参数数量错误")
	}

	c25519Pub := []byte(f.Inputs.String(1))
	c25519Priv := []byte(f.Inputs.String(2))

	if len(c25519Pub) != 32 || len(c25519Priv) != 32 {
		return "", errors.New("curve25519 公私钥必须都是32字节")
	}

	pub, err := curve25519.X25519(c25519Priv, c25519Pub)
	if err != nil {
		return "", errors.New("生成 ed25519 公钥失败")
	}

	result := map[string]string{
		"公钥": string(pub),
		"私钥": string(c25519Priv),
	}

	jsonResult, err := json.Marshal(result)
	if err != nil {
		return "", errors.New("转换 JSON 失败")
	}

	return string(jsonResult), nil
}
