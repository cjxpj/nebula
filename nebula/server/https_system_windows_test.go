//go:build windows

package dic_server

import "testing"

func TestListSystemCerts(t *testing.T) {
	certs, err := listSystemCerts()
	if err != nil {
		t.Fatalf("枚举系统证书失败: %v", err)
	}
	t.Logf("系统证书数量: %d", len(certs))
}

func TestExtractSystemCertInvalid(t *testing.T) {
	// 无效指纹应报格式错误
	if _, _, err := extractSystemCert("not-hex!"); err == nil {
		t.Fatal("应返回指纹格式错误")
	}
	// 合法格式但不存在的指纹，应报未找到（不报解析错误）
	if _, _, err := extractSystemCert("0000000000000000000000000000000000000000"); err == nil {
		t.Fatal("不存在的指纹应报错")
	}
}
