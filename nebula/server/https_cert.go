package dic_server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cjxpj/nebula/utils"
)

// httpsCertDir 返回 HTTPS 证书存储目录（private/https）
func httpsCertDir() string {
	dir := filepath.Join(utils.GetAppDir(), "private", "https")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		utils.Error("创建证书目录失败>" + err.Error())
	}
	return dir
}

// certRelPath 返回证书文件相对应用目录的路径（配置中保存用）
func certRelPath(name string) string {
	return filepath.ToSlash(filepath.Join("private", "https", name))
}

// generateSelfSignedCert 生成自签名证书（PEM），返回 cert/key 字节
func generateSelfSignedCert(cn string, days int) (certPEM, keyPEM []byte, err error) {
	if cn == "" {
		cn = "localhost"
	}
	if days <= 0 {
		days = 3650
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}
	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   cn,
			Organization: []string{"Nebula"},
		},
		DNSNames:              []string{cn, "localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Duration(days) * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	return certPEM, keyPEM, nil
}

// writeCertFiles 校验并写入证书/密钥文件
func writeCertFiles(certPEM, keyPEM []byte) (certPath, keyPath string, err error) {
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return "", "", errors.New("证书与密钥不匹配: " + err.Error())
	}
	certPath = certRelPath("cert.pem")
	keyPath = certRelPath("key.pem")
	dir := httpsCertDir()
	if err := os.WriteFile(filepath.Join(dir, "cert.pem"), certPEM, 0o644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "key.pem"), keyPEM, 0o600); err != nil {
		return "", "", err
	}
	return certPath, keyPath, nil
}

// opuiGenHttpsCert 一键生成自签名证书并保存（API: gen_https_cert）
func opuiGenHttpsCert(w http.ResponseWriter, _ *http.Request, h *HttpOpUiData) {
	var j struct {
		CommonName string `json:"cn"`
		Days       int    `json:"days"`
	}
	if len(h.Data) > 0 {
		_ = json.Unmarshal(h.Data, &j)
	}
	certPEM, keyPEM, err := generateSelfSignedCert(j.CommonName, j.Days)
	if err != nil {
		http.Error(w, `{"status":"error","error":`+strconvQuote("证书生成失败: "+err.Error())+`}`, http.StatusBadRequest)
		return
	}
	certPath, keyPath, err := writeCertFiles(certPEM, keyPEM)
	if err != nil {
		http.Error(w, `{"status":"error","error":`+strconvQuote("证书保存失败: "+err.Error())+`}`, http.StatusBadRequest)
		return
	}
	jsonResp, _ := json.Marshal(map[string]string{
		"status":    "ok",
		"cert_file": certPath,
		"key_file":  keyPath,
	})
	w.Write(jsonResp)
}

// opuiImportHttpsCert 上传导入证书（API: import_https_cert）
func opuiImportHttpsCert(w http.ResponseWriter, _ *http.Request, h *HttpOpUiData) {
	var j struct {
		CertPEM string `json:"cert"`
		KeyPEM  string `json:"key"`
	}
	if err := json.Unmarshal(h.Data, &j); err != nil {
		http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if !strings.Contains(j.CertPEM, "-----BEGIN CERTIFICATE-----") || !strings.Contains(j.KeyPEM, "-----BEGIN") {
		http.Error(w, `{"status":"error","error":"证书内容格式不正确"}`, http.StatusBadRequest)
		return
	}
	certPath, keyPath, err := writeCertFiles([]byte(j.CertPEM), []byte(j.KeyPEM))
	if err != nil {
		http.Error(w, `{"status":"error","error":`+strconvQuote(err.Error())+`}`, http.StatusBadRequest)
		return
	}
	jsonResp, _ := json.Marshal(map[string]string{
		"status":    "ok",
		"cert_file": certPath,
		"key_file":  keyPath,
	})
	w.Write(jsonResp)
}

// strconvQuote 简单的字符串 JSON 转义（避免 import strconv 展开）
func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// opuiListSystemCerts 枚举系统证书库（API: list_system_certs）
func opuiListSystemCerts(w http.ResponseWriter, _ *http.Request, _ *HttpOpUiData) {
	certs, err := listSystemCerts()
	if err != nil {
		http.Error(w, `{"status":"error","error":`+strconvQuote(err.Error())+`}`, http.StatusBadRequest)
		return
	}
	jsonResp, _ := json.Marshal(map[string]any{"status": "ok", "certs": certs})
	w.Write(jsonResp)
}

// opuiExtractSystemCert 从系统证书库提取证书（API: extract_system_cert）
func opuiExtractSystemCert(w http.ResponseWriter, _ *http.Request, h *HttpOpUiData) {
	var j struct {
		Thumbprint string `json:"thumbprint"`
	}
	if err := json.Unmarshal(h.Data, &j); err != nil {
		http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	certPEM, keyPEM, err := extractSystemCert(j.Thumbprint)
	if err != nil {
		http.Error(w, `{"status":"error","error":`+strconvQuote(err.Error())+`}`, http.StatusBadRequest)
		return
	}
	certPath, keyPath, err := writeCertFiles(certPEM, keyPEM)
	if err != nil {
		http.Error(w, `{"status":"error","error":`+strconvQuote(err.Error())+`}`, http.StatusBadRequest)
		return
	}
	jsonResp, _ := json.Marshal(map[string]string{
		"status":    "ok",
		"cert_file": certPath,
		"key_file":  keyPath,
	})
	w.Write(jsonResp)
}
