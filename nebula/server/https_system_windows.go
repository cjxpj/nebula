//go:build windows

package dic_server

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os/exec"
	"strings"

	"golang.org/x/crypto/pkcs12"
)

// jsonUnmarshal 统一 JSON 解码入口
func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// randomHex 生成指定字节数的随机十六进制字符串
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "00000000000000000000000000000000"
	}
	return hex.EncodeToString(b)
}

// listSystemCerts 枚举 Windows 系统证书库（当前用户 + 本地计算机 的个人证书，仅含私钥）
// 返回: [{thumbprint, subject, issuer, not_after, has_private_key, store}]
func listSystemCerts() ([]map[string]string, error) {
	ps := `$r = @(); Get-ChildItem Cert:\CurrentUser\My, Cert:\LocalMachine\My -ErrorAction SilentlyContinue | Where-Object { $_.HasPrivateKey } | ForEach-Object { $r += [PSCustomObject]@{ thumbprint=$_.Thumbprint; subject=$_.Subject; issuer=$_.Issuer; not_after=$_.NotAfter.ToString('yyyy-MM-dd'); has_private_key=$_.HasPrivateKey.ToString(); store=if($_.PSPath -match 'CurrentUser'){'user'}else{'machine'} } }; $r | ConvertTo-Json -Compress -Depth 3`
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps).Output()
	if err != nil {
		return nil, errors.New("读取系统证书库失败: " + err.Error())
	}
	text := strings.TrimSpace(string(out))
	if text == "" || text == "null" {
		return []map[string]string{}, nil
	}
	if !strings.HasPrefix(text, "[") {
		text = "[" + text + "]"
	}
	var list []map[string]string
	if err := jsonUnmarshal([]byte(text), &list); err != nil {
		return nil, errors.New("解析系统证书失败: " + err.Error())
	}
	return list, nil
}

// extractSystemCert 从 Windows 系统证书库按指纹导出证书与私钥（PEM）
func extractSystemCert(thumbprint string) (certPEM, keyPEM []byte, err error) {
	thumbprint = strings.TrimSpace(strings.ToUpper(thumbprint))
	if thumbprint == "" {
		return nil, nil, errors.New("证书指纹不能为空")
	}
	// 只允许十六进制指纹，防止 PowerShell 命令注入
	for _, c := range thumbprint {
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'F')) {
			return nil, nil, errors.New("证书指纹格式不正确")
		}
	}
	// 随机密码导出 PFX（仅用于内存中转）
	pwd := "Nebula_" + strings.ToUpper(randomHex(16))
	ps := "$ErrorActionPreference='Stop'; " +
		"$c = Get-ChildItem Cert:\\CurrentUser\\My, Cert:\\LocalMachine\\My -ErrorAction SilentlyContinue | Where-Object { $_.Thumbprint -eq '" + thumbprint + "' } | Select-Object -First 1; " +
		"if ($null -eq $c) { Write-Output 'NOT_FOUND'; exit 1 }; " +
		"if (-not $c.HasPrivateKey) { Write-Output 'NO_KEY'; exit 1 }; " +
		"$pwd = ConvertTo-SecureString -String '" + pwd + "' -Force -AsPlainText; " +
		"$bytes = $c.Export([System.Security.Cryptography.X509Certificates.X509ContentType]::Pfx, $pwd); " +
		"[Convert]::ToBase64String($bytes)"
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps).Output()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(msg, "NOT_FOUND") {
			return nil, nil, errors.New("证书库中未找到该指纹的证书")
		}
		if strings.Contains(msg, "NO_KEY") {
			return nil, nil, errors.New("该证书不包含私钥，无法用于 HTTPS")
		}
		return nil, nil, errors.New("导出证书失败: " + msg)
	}
	pfxData, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
	if err != nil {
		return nil, nil, errors.New("解析证书数据失败: " + err.Error())
	}
	blocks, err := pkcs12.ToPEM(pfxData, pwd)
	if err != nil {
		return nil, nil, errors.New("解析证书私钥失败: " + err.Error())
	}
	// 拆分证书块与私钥块
	var certBlocks, keyBlock []byte
	for _, b := range blocks {
		switch b.Type {
		case "CERTIFICATE":
			certBlocks = append(certBlocks, pem.EncodeToMemory(b)...)
		case "PRIVATE KEY", "RSA PRIVATE KEY", "EC PRIVATE KEY", "ENCRYPTED PRIVATE KEY":
			keyBlock = pem.EncodeToMemory(b)
		}
	}
	if len(certBlocks) == 0 || len(keyBlock) == 0 {
		return nil, nil, errors.New("证书库导出内容不完整（缺证书或私钥）")
	}
	return certBlocks, keyBlock, nil
}
