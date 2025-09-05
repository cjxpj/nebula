package funcs

import (
	"fmt"
	"net/smtp"
	"strings"

	"github.com/cjxpj/nebula/dto"
)

// xwatvdftgpzkdghe

func sendMail(d *dto.DicInputs) (any, error) {

	// SMTP 服务器地址
	smtpHost := d.Inputs.String(1)
	smtpPort := d.Inputs.String(2)

	// 发件人的邮箱和密码
	from := d.Inputs.String(3)
	password := d.Inputs.String(4)

	// 收件人的邮箱
	to := d.Inputs.String(5)

	// 邮件内容
	msg := d.Inputs.String(6)

	// 认证信息
	auth := smtp.PlainAuth("", from, password, smtpHost)

	// 发送邮件
	err := smtp.SendMail(smtpHost+":"+smtpPort, auth, from, strings.Split(to, ","), []byte(msg))
	if err != nil {
		return "", fmt.Errorf("发送邮件失败: %v", err)
	}
	return "true", nil
}
