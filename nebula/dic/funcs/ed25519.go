package funcs

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"errors"
	"math/big"
	"strconv"
	stdjson "encoding/json"

	"github.com/cjxpj/nebula/dto"
	"golang.org/x/crypto/curve25519"
)

var (
	edwardsP, _ = new(big.Int).SetString("7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffed", 16) // 2^255-19
	edwardsD, _ = new(big.Int).SetString("52036cee2b6ffe738cc740797779e89800700a4d4141d8ab75eb4dca135978a3", 16) // -121665/121666 mod p
	edwardsI, _ = new(big.Int).SetString("2b8324804fc1df0b2b4d00993dfbd7a72f431806ad2fe478c4ee1b274a0ea0b0", 16) // sqrt(-1) mod p
)

// edwardsToMontgomery 将 Ed25519 公钥（压缩 Edwards 点）转换为 Curve25519 公钥（Montgomery u 坐标）
func edwardsToMontgomery(pub [32]byte) [32]byte {
	// 1. 提取 y 坐标和 x 符号位
	// Ed25519 公钥是 y 坐标（小端序），最高位是 x 的符号
	pubBI := new(big.Int).SetBytes(reverseBytes(pub[:])) // 大端序读取
	signBit := pubBI.Bit(255)                            // x 的符号
	pubBI.SetBit(pubBI, 255, 0)                          // 清除符号位，得到 y
	y := pubBI

	// 2. 从曲线方程恢复 x 坐标
	// Edwards: -x^2 + y^2 = 1 + d*x^2*y^2 → x^2 = (y^2-1)/(d*y^2+1)
	y2 := new(big.Int).Exp(y, big.NewInt(2), edwardsP)
	num := new(big.Int).Add(y2, big.NewInt(-1))   // y^2 - 1
	den := new(big.Int).Add(new(big.Int).Mul(edwardsD, y2), big.NewInt(1)) // d*y^2 + 1
	den.ModInverse(den, edwardsP)
	x2 := new(big.Int).Mul(num, den)
	x2.Mod(x2, edwardsP)

	// 3. 计算 sqrt(x^2) mod p
	// p ≡ 5 mod 8: sqrt(a) = a^((p+3)/8) if a^((p+1)/4) == 1
	exp := new(big.Int).Add(edwardsP, big.NewInt(3))
	exp.Div(exp, big.NewInt(8))
	x := new(big.Int).Exp(x2, exp, edwardsP)
	// 验证: x^2 mod p
	if new(big.Int).Exp(x, big.NewInt(2), edwardsP).Cmp(x2) != 0 {
		// 乘以 sqrt(-1)
		x.Mul(x, edwardsI).Mod(x, edwardsP)
	}

	// 4. 匹配符号位
	if x.Bit(0) != signBit {
		x.Sub(edwardsP, x)
	}

	// 5. Edwards → Montgomery: u = (1+y)/(1-y)
	u := new(big.Int).Add(big.NewInt(1), y) // 1 + y
	v := new(big.Int).Sub(big.NewInt(1), y) // 1 - y
	if v.Sign() < 0 {
		v.Add(v, edwardsP)
	}
	v.ModInverse(v, edwardsP)
	u.Mul(u, v).Mod(u, edwardsP)

	// 6. 使用 FillBytes 确保恰好 32 字节
	return *(*[32]byte)(u.FillBytes(make([]byte, 32)))
}

func edwardsPrivateToCurve25519Scalar(seed []byte) [32]byte {
	h := sha512.Sum512(seed)
	var s [32]byte
	copy(s[:], h[:32])
	s[0] &= 248
	s[31] &= 127
	s[31] |= 64
	return s
}

// reverseBytes 反转字节切片
func reverseBytes(b []byte) []byte {
	n := len(b)
	r := make([]byte, n)
	for i := range n {
		r[i] = b[n-1-i]
	}
	return r
}

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

	jsonResult, err := stdjson.Marshal(result)
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

	var pub [32]byte
	copy(pub[:], publicKey)
	curvePub := edwardsToMontgomery(pub)
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

	curvePriv := edwardsPrivateToCurve25519Scalar(privateKey[:32])
	return string(curvePriv[:]), nil
}

func (f *DicFunc) Ed25519_NewKeyFromCurve25519() (string, error) {
	if f.Len != 2 {
		return "", errors.New("参数数量错误")
	}

	c25519Priv := []byte(f.Inputs.String(1))
	c25519Pub := []byte(f.Inputs.String(2))

	if len(c25519Priv) != 32 || len(c25519Pub) != 32 {
		return "", errors.New("curve25519 公私钥必须都是32字节")
	}

	sharedSecret, err := curve25519.X25519(c25519Priv, c25519Pub)
	if err != nil {
		return "", errors.New("X25519 密钥协商失败")
	}

	result := map[string]string{
		"共享密钥": string(sharedSecret),
		"私钥":   string(c25519Priv),
	}

	jsonResult, err := stdjson.Marshal(result)
	if err != nil {
		return "", errors.New("转换 JSON 失败")
	}

	return string(jsonResult), nil
}

// ========== Ed25519 free functions ==========

func ed25519NewKeyFromSeed(d *dto.DicInputs) (any, error) {
	seed := []byte(d.Inputs.String(1))
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
}

func ed25519Sign(d *dto.DicInputs) (any, error) {
	var privateKey ed25519.PrivateKey
	switch v := d.Inputs.Get(1).(type) {
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
	message := []byte(d.Inputs.String(2))
	signature := ed25519.Sign(privateKey, message)
	return string(signature), nil
}

func ed25519Verify(d *dto.DicInputs) (any, error) {
	publicKey := []byte(d.Inputs.String(1))
	message := []byte(d.Inputs.String(2))
	signature := []byte(d.Inputs.String(3))
	if len(publicKey) != ed25519.PublicKeySize {
		return "", errors.New("公钥长度错误，必须为32字节")
	}
	if len(signature) != ed25519.SignatureSize {
		return "", errors.New("签名长度错误，必须为64字节")
	}
	if ed25519.Verify(publicKey, message, signature) {
		return "true", nil
	}
	return "false", nil
}

func ed25519PublicKeyToCurve25519(d *dto.DicInputs) (any, error) {
	publicKey := []byte(d.Inputs.String(1))
	if len(publicKey) != 32 {
		return "", errors.New("公钥长度必须为32字节")
	}
	var pub [32]byte
	copy(pub[:], publicKey)
	curvePub := edwardsToMontgomery(pub)
	return string(curvePub[:]), nil
}

func ed25519PrivateKeyToCurve25519(d *dto.DicInputs) (any, error) {
	privateKey := []byte(d.Inputs.String(1))
	if len(privateKey) < 32 {
		return "", errors.New("私钥长度不足，必须至少32字节")
	}
	curvePriv := edwardsPrivateToCurve25519Scalar(privateKey[:32])
	return string(curvePriv[:]), nil
}

func ed25519NewKeyFromCurve25519(d *dto.DicInputs) (any, error) {
	if d.Inputs.Len() != 2 {
		return "", errors.New("参数数量错误")
	}
	c25519Priv := []byte(d.Inputs.String(1))
	c25519Pub := []byte(d.Inputs.String(2))
	if len(c25519Priv) != 32 || len(c25519Pub) != 32 {
		return "", errors.New("curve25519 公私钥必须都是32字节")
	}
	sharedSecret, err := curve25519.X25519(c25519Priv, c25519Pub)
	if err != nil {
		return "", errors.New("X25519 密钥协商失败")
	}
	result := map[string]string{
		"共享密钥": string(sharedSecret),
		"私钥":   string(c25519Priv),
	}
	jsonResult, err := stdjson.Marshal(result)
	if err != nil {
		return "", errors.New("转换 JSON 失败")
	}
	return string(jsonResult), nil
}
