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
	Addr     string // host:port
	From     string
	Nickname string // 发件人显示名，空则只显示邮箱地址
	Auth     smtp.Auth
}

func getEmailConfig(d *dto.DicInputs) *EmailConfig {
	if v := d.Inputs.Get(1); v != nil {
		if cfg, ok := v.(*EmailConfig); ok {
			return cfg
		}
	}
	return nil
}

// 创建邮件 链接 端口 账号 密码 [昵称]
func emailCreate(d *dto.DicInputs) (any, error) {
	smtpHost := d.Inputs.String(1)
	smtpPort := d.Inputs.String(2)
	from := d.Inputs.String(3)
	password := d.Inputs.String(4)

	return newEmailClass(&EmailConfig{
		Addr:     smtpHost + ":" + smtpPort,
		From:     from,
		Nickname: d.Inputs.String(5), // 不填则不发件人昵称
		Auth:     smtp.PlainAuth("", from, password, smtpHost),
	}), nil
}

// newEmailClass 将邮件配置包装为面对像 Class，方法闭包捕获同一配置实例。
func newEmailClass(cfg *EmailConfig) *dto.DicClass {
	instance := &dto.DicClass{
		LocalValue: dto.NewVal().Set("_邮件_", cfg),
	}
	instance.Fn = map[string]dto.DicFunc{
		"设置昵称":   wrapObj(cfg, emailSetNickname, "1"),
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
	writeFrom(&msg, cfg.Nickname, cfg.From)
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

// writeFrom 写发件人地址，有昵称时输出「昵称 <邮箱>」
func writeFrom(msg *bytes.Buffer, nickname string, addr string) {
	if nickname == "" {
		msg.WriteString(addr)
		return
	}
	msg.WriteString(encodeDisplayName(nickname))
	msg.WriteString(" <")
	msg.WriteString(addr)
	msg.WriteString(">")
}

// encodeDisplayName 编码发件人显示名：含非 ASCII 用 RFC 2047 编码，含特殊字符用引号包裹
func encodeDisplayName(name string) string {
	for i := 0; i < len(name); i++ {
		if name[i] > 127 {
			return mime.BEncoding.Encode("UTF-8", name)
		}
	}
	if strings.ContainsAny(name, `()<>@,;:\".[]`) {
		return `"` + strings.ReplaceAll(name, `"`, `\"`) + `"`
	}
	return name
}

// $a.设置昵称 昵称
func emailSetNickname(d *dto.DicInputs) (any, error) {
	cfg := getEmailConfig(d)
	if cfg == nil {
		return "", fmt.Errorf("未创建邮件连接")
	}
	cfg.Nickname = d.Inputs.String(2)
	return "true", nil
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
