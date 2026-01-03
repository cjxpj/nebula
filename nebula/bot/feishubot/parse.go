package feishubot

import (
	"encoding/json"

	feishubot_msg "github.com/cjxpj/nebula/bot/feishubot/msg"
)

// parseAndDecrypt 读取 body 并解密（如有），返回解析好的外壳
func parseAndDecrypt(body []byte) (*feishubot_msg.SlackURLVerification, error) {
	plain, err := decryptIfNeeded(body)
	if err != nil {
		return nil, err
	}

	var e feishubot_msg.SlackURLVerification
	if err = json.Unmarshal(plain, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// 解析群消息
func parseGroupMessage(body []byte) (*feishubot_msg.ImMessageReceiveV1, error) {
	plain, err := decryptIfNeeded(body)
	if err != nil {
		return nil, err
	}

	var e feishubot_msg.ImMessageReceiveV1
	if err = json.Unmarshal(plain, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// decryptIfNeeded 若配置了 EncryptKey 则解密，否则原样返回
func decryptIfNeeded(cipher []byte) ([]byte, error) {
	// 快速判断：飞书加密事件一定是 {"encrypt":"..."}
	var tmp map[string]json.RawMessage
	if err := json.Unmarshal(cipher, &tmp); err != nil {
		return nil, err
	}
	if encrypt, ok := tmp["encrypt"]; ok {
		// 这里调用你的 AES-CBC 解密函数，把 encrypt 字段解密成明文
		// 例如：return decryptAES(encrypt, yourEncryptKey)
		// 下面给出占位实现：
		return decryptAES(encrypt)
	}
	return cipher, nil
}

// decryptAES 占位：AES-256-CBC 解密，key 为你后台配置的 EncryptKey
func decryptAES(cipherField json.RawMessage) ([]byte, error) {
	// 把引号去掉拿到 base64 密文
	var b64 string
	_ = json.Unmarshal(cipherField, &b64)
	// 在此实现 AES-CBC 解密，返回解密后的 JSON 明文
	// 若无需支持加密，可直接 return nil, fmt.Errorf("encrypt not supported")
	// 需要完整代码可再喊我
	return []byte("decrypted-json-placeholder"), nil
}
