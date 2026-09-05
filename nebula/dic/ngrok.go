package dic

import (
	"fmt"
	"strconv"

	"github.com/cjxpj/nebula/dto"
	dic_server "github.com/cjxpj/nebula/server"
)

// 设置Ngrok 词库函数：$设置Ngrok 开关 密钥 域名 [服务器地址]$
// 配置全局 Ngrok 隧道并立即启停生效。
// 开关默认 true；密钥/域名默认空（空域名使用随机域名）；服务器地址默认空（转发核心服务器）。
func setNgrok(d *dto.DicInputs) (any, error) {
	open := true
	if d.Inputs.Len() >= 1 {
		open = d.Inputs.Bool(1)
	}
	token := d.Inputs.StringDefault(2, "")
	domain := d.Inputs.StringDefault(3, "")
	serverAddr := d.Inputs.StringDefault(4, "")

	// 保存到合并配置的 Ngrok 节
	cfg, err := dto.LoadConfigFile()
	if err != nil {
		return nil, fmt.Errorf("设置Ngrok：读取系统配置失败: %v", err)
	}
	sec := cfg.Section("Ngrok")
	sec.Key("启用").SetValue(strconv.FormatBool(open))
	sec.Key("密钥").SetValue(token)
	sec.Key("访问链接").SetValue(domain)
	sec.Key("服务器").SetValue(serverAddr)
	if err := cfg.Save(); err != nil {
		return nil, fmt.Errorf("设置Ngrok：保存系统配置失败: %v", err)
	}

	// 立即生效
	if open {
		if dto.ServerConfig.NgrokListener != nil || dto.ServerConfig.NgrokCancel != nil {
			dic_server.StopNgrok()
		}
		dto.ServerConfig.Ngrok = &dto.NgrokConfig{
			Addr:       domain,
			Token:      token,
			ServerAddr: serverAddr,
		}
		if _, err := dic_server.StartNgrok(token, domain); err != nil {
			return nil, fmt.Errorf("设置Ngrok：%v", err)
		}
	} else {
		dic_server.StopNgrok()
		dto.ServerConfig.Ngrok = nil
	}
	return nil, nil
}
