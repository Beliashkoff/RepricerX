// Package mailer предоставляет интерфейс отправки email и две реализации:
// SmtpMailer (Яндекс.Почта) и LogMailer (dev-режим, письма в лог).
package mailer

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	htmltmpl "html/template"
	texttmpl "text/template"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// Mailer — общий интерфейс отправки писем.
type Mailer interface {
	Send(ctx context.Context, to, subject, htmlBody, textBody string) error
}

// VerificationData — данные для шаблона письма подтверждения.
type VerificationData struct {
	DisplayName string
	URL         string
}

// PasswordResetData — данные для шаблона письма сброса пароля.
type PasswordResetData struct {
	DisplayName string
	URL         string
}

// RenderVerification рендерит HTML и текстовое тела письма верификации.
func RenderVerification(data VerificationData) (htmlBody, textBody string, err error) {
	htmlBody, err = renderHTML("templates/verification.html.tmpl", data)
	if err != nil {
		return "", "", fmt.Errorf("mailer: html template: %w", err)
	}
	textBody, err = renderText("templates/verification.txt.tmpl", data)
	if err != nil {
		return "", "", fmt.Errorf("mailer: text template: %w", err)
	}
	return htmlBody, textBody, nil
}

// RenderPasswordReset рендерит HTML и текстовое тела письма сброса пароля.
func RenderPasswordReset(data PasswordResetData) (htmlBody, textBody string, err error) {
	htmlBody, err = renderHTML("templates/password_reset.html.tmpl", data)
	if err != nil {
		return "", "", fmt.Errorf("mailer: html template: %w", err)
	}
	textBody, err = renderText("templates/password_reset.txt.tmpl", data)
	if err != nil {
		return "", "", fmt.Errorf("mailer: text template: %w", err)
	}
	return htmlBody, textBody, nil
}

// renderHTML использует html/template — автоматически экранирует пользовательские данные в HTML.
func renderHTML(name string, data any) (string, error) {
	raw, err := templateFS.ReadFile(name)
	if err != nil {
		return "", err
	}
	tmpl, err := htmltmpl.New(name).Parse(string(raw))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err = tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func renderText(name string, data any) (string, error) {
	raw, err := templateFS.ReadFile(name)
	if err != nil {
		return "", err
	}
	tmpl, err := texttmpl.New(name).Parse(string(raw))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err = tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
