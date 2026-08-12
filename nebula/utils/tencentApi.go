package utils

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"
)

type TencentAPI struct {
	SecretId  string
	SecretKey string
	Token     string

	Host    string
	Service string
	Action  string
	Version string
	Region  string
}

func sha256Hex(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

func hmacSha256(key []byte, msg string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(msg))
	return h.Sum(nil)
}

func (api *TencentAPI) getDate(timestamp int64) string {
	return time.Unix(timestamp, 0).UTC().Format("2006-01-02")
}

func (api *TencentAPI) Request(payload map[string]any) ([]byte, error) {
	timestamp := time.Now().Unix()
	date := api.getDate(timestamp)

	// 步骤 1：拼接 Canonical Request
	httpRequestMethod := "POST"
	canonicalUri := "/"
	canonicalQueryString := ""
	canonicalHeaders := fmt.Sprintf("content-type:application/json; charset=utf-8\nhost:%s\n", api.Host)
	signedHeaders := "content-type;host"

	payloadJson, _ := Json.Marshal(payload)
	hashedRequestPayload := sha256Hex(string(payloadJson))

	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		httpRequestMethod,
		canonicalUri,
		canonicalQueryString,
		canonicalHeaders,
		signedHeaders,
		hashedRequestPayload,
	)

	// 步骤 2：拼接字符串待签名
	algorithm := "TC3-HMAC-SHA256"
	credentialScope := fmt.Sprintf("%s/%s/tc3_request", date, api.Service)
	hashedCanonicalRequest := sha256Hex(canonicalRequest)

	stringToSign := fmt.Sprintf("%s\n%d\n%s\n%s",
		algorithm,
		timestamp,
		credentialScope,
		hashedCanonicalRequest,
	)

	// 步骤 3：计算签名
	secretDate := hmacSha256([]byte("TC3"+api.SecretKey), date)
	secretService := hmacSha256(secretDate, api.Service)
	secretSigning := hmacSha256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSha256(secretSigning, stringToSign))

	// 步骤 4：拼接 Authorization 头
	authorization := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm,
		api.SecretId,
		credentialScope,
		signedHeaders,
		signature,
	)

	// 请求头
	reqUrl := fmt.Sprintf("https://%s", api.Host)
	req, err := http.NewRequest("POST", reqUrl, bytes.NewReader(payloadJson))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", authorization)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Host", api.Host)
	req.Header.Set("X-TC-Action", api.Action)
	req.Header.Set("X-TC-Timestamp", fmt.Sprintf("%d", timestamp))
	req.Header.Set("X-TC-Version", api.Version)
	if api.Region != "" {
		req.Header.Set("X-TC-Region", api.Region)
	}
	if api.Token != "" {
		req.Header.Set("X-TC-Token", api.Token)
	}

	// 执行请求
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}
