package funcs

import (
	"bytes"
	"fmt"
	"mime"
	"net/smtp"
	"strings"

	"github.com/cjxpj/nebula/dto"
)

// EmailConfig 邮件连接配置
type EmailConfig struct {
	Addr string // host:port
	From string
	Auth smtp.Auth
}

func getEmailConfig(d *dto.DicInputs) *EmailConfig {
	if v := d.Inputs.Get(1); v != nil {
		if cfg, ok := v.(*EmailConfig); ok {
			return cfg
		}
	}
	return nil
}

// 创建邮件 链接 端口 账号 密码
func emailCreate(d *dto.DicInputs) (any, error) {
	smtpHost := d.Inputs.String(1)
	smtpPort := d.Inputs.String(2)
	from := d.Inputs.String(3)
	password := d.Inputs.String(4)

	return newEmailClass(&EmailConfig{
		Addr: smtpHost + ":" + smtpPort,
		From: from,
		Auth: smtp.PlainAuth("", from, password, smtpHost),
	}), nil
}

// newEmailClass 将邮件配置包装为面对像 Class，方法闭包捕获同一配置实例。
func newEmailClass(cfg *EmailConfig) *dto.DicClass {
	instance := &dto.DicClass{
		LocalValue: dto.NewVal().Set("_邮件_", cfg),
	}
	instance.Fn = map[string]dto.DicFunc{
		"发送":     wrapObj(cfg, emailSend, "3"),
		"发送HTML": wrapObj(cfg, emailSendHTML, "3"),
	}
	return instance
}

func doSendMail(cfg *EmailConfig, to string, subject string, body string, isHTML bool) error {
	contentType := "text/plain"
	if isHTML {
		contentType = "text/html"
	}

	var msg bytes.Buffer
	msg.Grow(256 + len(body))

	msg.WriteString("From: ")
	msg.WriteString(cfg.From)
	msg.WriteString("\r\nTo: ")
	msg.WriteString(to)
	msg.WriteString("\r\nSubject: ")
	writeSubject(&msg, subject)
	msg.WriteString("\r\nMIME-Version: 1.0\r\nContent-Type: ")
	msg.WriteString(contentType)
	msg.WriteString("; charset=\"UTF-8\"\r\n\r\n")
	msg.WriteString(body)

	rcpts := strings.Split(to, ",")
	for i, addr := range rcpts {
		rcpts[i] = strings.TrimSpace(addr)
	}
	err := smtp.SendMail(cfg.Addr, cfg.Auth, cfg.From, rcpts, msg.Bytes())
	if err != nil {
		return fmt.Errorf("发送邮件失败: %v", err)
	}
	return nil
}

func writeSubject(msg *bytes.Buffer, subject string) {
	for i := 0; i < len(subject); i++ {
		if subject[i] > 127 {
			msg.WriteString(mime.BEncoding.Encode("UTF-8", subject))
			return
		}
	}
	msg.WriteString(subject)
}

// $a.发送 收件人 标题 文本
func emailSend(d *dto.DicInputs) (any, error) {
	cfg := getEmailConfig(d)
	if cfg == nil {
		return "", fmt.Errorf("未创建邮件连接")
	}
	if err := doSendMail(cfg, d.Inputs.String(2), d.Inputs.String(3), d.Inputs.String(4), false); err != nil {
		return "", err
	}
	return "true", nil
}

// $a.发送HTML 收件人 标题 HTML文本
func emailSendHTML(d *dto.DicInputs) (any, error) {
	cfg := getEmailConfig(d)
	if cfg == nil {
		return "", fmt.Errorf("未创建邮件连接")
	}
	if err := doSendMail(cfg, d.Inputs.String(2), d.Inputs.String(3), d.Inputs.String(4), true); err != nil {
		return "", err
	}
	return "true", nil
}
