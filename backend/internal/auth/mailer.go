package auth

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"

	"github.com/ydfk/measure-trail/backend/internal/config"
)

type Notifier interface {
	SendVerification(context.Context, string, string) error
	SendPasswordReset(context.Context, string, string) error
}

func NewNotifier(mailConfig config.Mail, publicBaseURL string) (Notifier, error) {
	if strings.EqualFold(mailConfig.Mode, "log") {
		return logNotifier{}, nil
	}
	if !strings.EqualFold(mailConfig.Mode, "smtp") || mailConfig.Host == "" || mailConfig.From == "" {
		return nil, fmt.Errorf("SMTP 邮件配置不完整")
	}
	port, err := strconv.Atoi(mailConfig.Port)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("SMTP 端口无效")
	}
	if unsafeMailHeader(mailConfig.From) {
		return nil, fmt.Errorf("SMTP 发件人格式无效")
	}
	return smtpNotifier{mail: mailConfig, baseURL: strings.TrimRight(publicBaseURL, "/")}, nil
}

type logNotifier struct{}

func (logNotifier) SendVerification(_ context.Context, email string, _ string) error {
	log.Printf("开发邮件已生成：purpose=verify_email recipient=%s", email)
	return nil
}

func (logNotifier) SendPasswordReset(_ context.Context, email string, _ string) error {
	log.Printf("开发邮件已生成：purpose=reset_password recipient=%s", email)
	return nil
}

type smtpNotifier struct {
	mail    config.Mail
	baseURL string
}

func (notifier smtpNotifier) SendVerification(ctx context.Context, email string, token string) error {
	return notifier.send(ctx, email, "验证你的量迹账号", "/verify-email", token)
}

func (notifier smtpNotifier) SendPasswordReset(ctx context.Context, email string, token string) error {
	return notifier.send(ctx, email, "重置你的量迹密码", "/reset-password", token)
}

func (notifier smtpNotifier) send(ctx context.Context, recipient string, subject string, path string, token string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if unsafeMailHeader(recipient) {
		return fmt.Errorf("收件人格式无效")
	}
	link, err := url.Parse(notifier.baseURL + path)
	if err != nil {
		return err
	}
	query := link.Query()
	query.Set("token", token)
	link.RawQuery = query.Encode()
	body := "To: " + recipient + "\r\nSubject: " + subject + "\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + link.String() + "\r\n"
	var auth smtp.Auth
	if notifier.mail.Username != "" {
		auth = smtp.PlainAuth("", notifier.mail.Username, notifier.mail.Password, notifier.mail.Host)
	}
	return sendStartTLS(ctx, net.JoinHostPort(notifier.mail.Host, notifier.mail.Port), notifier.mail.Host, auth, notifier.mail.From, recipient, []byte(body))
}

func sendStartTLS(ctx context.Context, address string, host string, auth smtp.Auth, sender string, recipient string, body []byte) error {
	dialer := net.Dialer{}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	defer connection.Close()

	client, err := smtp.NewClient(connection, host)
	if err != nil {
		return err
	}
	defer client.Quit()
	if ok, _ := client.Extension("STARTTLS"); !ok {
		return fmt.Errorf("SMTP 服务不支持 STARTTLS")
	}
	if err := client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
		return err
	}
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(sender); err != nil {
		return err
	}
	if err := client.Rcpt(recipient); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(body); err != nil {
		writer.Close()
		return err
	}
	return writer.Close()
}

func unsafeMailHeader(value string) bool {
	return value == "" || strings.ContainsAny(value, "\r\n")
}
