package dic_server

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"

	"github.com/cjxpj/nebula/utils"
)

func TestGenerateSelfSignedCert(t *testing.T) {
	certPEM, keyPEM, err := generateSelfSignedCert("localhost", 3650)
	if err != nil {
		t.Fatalf("生成失败: %v", err)
	}
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		t.Fatalf("证书校验失败: %v", err)
	}
}

func TestWriteCertFiles(t *testing.T) {
	dir := t.TempDir()
	utils.SetAppDir(dir)
	defer utils.SetAppDir("")

	certPEM, keyPEM, _ := generateSelfSignedCert("localhost", 3650)
	certPath, keyPath, err := writeCertFiles(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(certPath))); err != nil {
		t.Fatalf("证书文件不存在: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(keyPath))); err != nil {
		t.Fatalf("密钥文件不存在: %v", err)
	}
}

