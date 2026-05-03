package mailer

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// SmtpMailer отправляет письма через Яндекс.Почту (smtp.yandex.ru:465, TLS).
// Для авторизации требуется пароль приложения — не обычный пароль аккаунта.
type SmtpMailer struct {
	host     string
	port     string
	username string
	password string
	from     string
}

func NewSmtpMailer(host, port, username, password, from string) *SmtpMailer {
	return &SmtpMailer{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
	}
}

func (m *SmtpMailer) Send(ctx context.Context, to, subject, htmlBody, textBody string) error {
	// Яндекс.Почта на порту 465 требует TLS с первого байта (SSL), не STARTTLS.
	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: 10 * time.Second},
		Config: &tls.Config{
			ServerName: m.host,
			MinVersion: tls.VersionTLS12,
		},
	}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(m.host, m.port))
	if err != nil {
		return fmt.Errorf("smtp: tls dial: %w", err)
	}
	defer conn.Close() //nolint:errcheck

	client, err := smtp.NewClient(conn, m.host)
	if err != nil {
		return fmt.Errorf("smtp: new client: %w", err)
	}
	defer client.Quit() //nolint:errcheck

	if err = client.Auth(smtp.PlainAuth("", m.username, m.password, m.host)); err != nil {
		return fmt.Errorf("smtp: auth: %w", err)
	}
	parsed, err := mail.ParseAddress(m.from)
	if err != nil {
		return fmt.Errorf("smtp: parse from address: %w", err)
	}
	if err = client.Mail(parsed.Address); err != nil {
		return fmt.Errorf("smtp: MAIL FROM: %w", err)
	}
	if err = client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp: RCPT TO: %w", err)
	}

	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp: DATA: %w", err)
	}

	msg := buildMIMEMessage(m.from, to, subject, htmlBody, textBody)
	if _, err = fmt.Fprint(wc, msg); err != nil {
		_ = wc.Close()
		return fmt.Errorf("smtp: write body: %w", err)
	}
	// wc.Close() отправляет финальную точку и ждёт 250 от сервера — проверяем.
	if err = wc.Close(); err != nil {
		return fmt.Errorf("smtp: close data: %w", err)
	}
	return nil
}

// buildMIMEMessage собирает multipart/alternative письмо (text + html).
func buildMIMEMessage(from, to, subject, htmlBody, textBody string) string {
	boundary := "==RepricerX_boundary_001=="
	var sb strings.Builder

	sb.WriteString("From: " + from + "\r\n")
	sb.WriteString("To: " + to + "\r\n")
	sb.WriteString("Subject: " + subject + "\r\n")
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString(`Content-Type: multipart/alternative; boundary="` + boundary + `"` + "\r\n\r\n")

	sb.WriteString("--" + boundary + "\r\n")
	sb.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	sb.WriteString(textBody + "\r\n")

	sb.WriteString("--" + boundary + "\r\n")
	sb.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
	sb.WriteString(htmlBody + "\r\n")

	sb.WriteString("--" + boundary + "--\r\n")
	return sb.String()
}
