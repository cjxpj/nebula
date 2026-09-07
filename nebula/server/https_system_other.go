//go:build !windows

package dic_server

import "errors"

// listSystemCerts 非 Windows 平台暂不支持系统证书库
func listSystemCerts() ([]map[string]string, error) {
	return nil, errors.New("系统证书库提取仅支持 Windows 平台")
}

// extractSystemCert 非 Windows 平台暂不支持系统证书库
func extractSystemCert(_ string) (certPEM, keyPEM []byte, err error) {
	return nil, nil, errors.New("系统证书库提取仅支持 Windows 平台")
}
