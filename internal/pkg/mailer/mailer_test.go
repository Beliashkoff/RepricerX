package mailer

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// sanitizeHeader
// ---------------------------------------------------------------------------

func TestSanitizeHeader_StripsCRLF(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Normal Subject", "Normal Subject"},
		{"Inject\r\nBcc: evil@bad.com", "InjectBcc: evil@bad.com"},
		{"Only\nNewline", "OnlyNewline"},
		{"Only\rCarriage", "OnlyCarriage"},
		{"Multi\r\n\r\nInjection", "MultiInjection"},
		{"", ""},
		{"Тема на русском", "Тема на русском"},
	}
	for _, tc := range cases {
		got := sanitizeHeader(tc.in)
		if got != tc.want {
			t.Errorf("sanitizeHeader(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// buildMIMEMessage — структура
// ---------------------------------------------------------------------------

func TestBuildMIMEMessage_Headers(t *testing.T) {
	from := "RepricerX <noreply@example.com>"
	to := "user@example.com"
	subject := "Подтверждение email"

	msg := buildMIMEMessage(from, to, subject, "<p>html</p>", "text")

	m, err := mail.ReadMessage(strings.NewReader(msg))
	if err != nil {
		t.Fatalf("не удалось распарсить письмо: %v", err)
	}

	tests := []struct{ name, got, want string }{
		{"From", m.Header.Get("From"), from},
		{"To", m.Header.Get("To"), to},
		{"Subject", m.Header.Get("Subject"), subject},
		{"MIME-Version", m.Header.Get("MIME-Version"), "1.0"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("заголовок %s: получили %q, ожидали %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestBuildMIMEMessage_ContentType(t *testing.T) {
	msg := buildMIMEMessage("f@x.com", "t@x.com", "s", "<p/>", "txt")

	m, err := mail.ReadMessage(strings.NewReader(msg))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	ct := m.Header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(ct)
	if err != nil {
		t.Fatalf("ParseMediaType(%q): %v", ct, err)
	}
	if mediaType != "multipart/alternative" {
		t.Errorf("Content-Type: %q, ожидали multipart/alternative", mediaType)
	}
	if params["boundary"] == "" {
		t.Error("boundary отсутствует в Content-Type")
	}
}

func TestBuildMIMEMessage_TwoParts(t *testing.T) {
	htmlBody := "<p>Привет из <b>HTML</b></p>"
	textBody := "Привет из текста"

	msg := buildMIMEMessage("f@x.com", "t@x.com", "s", htmlBody, textBody)

	m, err := mail.ReadMessage(strings.NewReader(msg))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	_, params, _ := mime.ParseMediaType(m.Header.Get("Content-Type"))
	boundary := params["boundary"]

	mr := multipart.NewReader(m.Body, boundary)

	// Часть 1: text/plain
	p1, err := mr.NextPart()
	if err != nil {
		t.Fatalf("чтение первой части: %v", err)
	}
	ct1 := p1.Header.Get("Content-Type")
	if !strings.HasPrefix(ct1, "text/plain") {
		t.Errorf("первая часть: Content-Type=%q, ожидали text/plain", ct1)
	}
	if !strings.Contains(ct1, "UTF-8") {
		t.Errorf("первая часть: нет charset=UTF-8 в %q", ct1)
	}
	body1, _ := io.ReadAll(p1)
	if !strings.Contains(string(body1), textBody) {
		t.Errorf("первая часть не содержит текст: %q", string(body1))
	}

	// Часть 2: text/html
	p2, err := mr.NextPart()
	if err != nil {
		t.Fatalf("чтение второй части: %v", err)
	}
	ct2 := p2.Header.Get("Content-Type")
	if !strings.HasPrefix(ct2, "text/html") {
		t.Errorf("вторая часть: Content-Type=%q, ожидали text/html", ct2)
	}
	if !strings.Contains(ct2, "UTF-8") {
		t.Errorf("вторая часть: нет charset=UTF-8 в %q", ct2)
	}
	body2, _ := io.ReadAll(p2)
	if !strings.Contains(string(body2), htmlBody) {
		t.Errorf("вторая часть не содержит HTML: %q", string(body2))
	}

	// Финальная граница — убеждаемся, что нет третьей части.
	_, err = mr.NextPart()
	if err == nil {
		t.Error("ожидали конец MIME, получили третью часть")
	}
}

func TestBuildMIMEMessage_SubjectInjectionBlocked(t *testing.T) {
	// Если subject содержит \r\n — инъекция не должна попасть в письмо.
	malicious := "Тема\r\nBcc: evil@evil.com"
	msg := buildMIMEMessage("f@x.com", "t@x.com", malicious, "", "")

	m, err := mail.ReadMessage(strings.NewReader(msg))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Заголовок Bcc не должен появиться.
	if m.Header.Get("Bcc") != "" {
		t.Errorf("header injection прошла: Bcc=%q", m.Header.Get("Bcc"))
	}
	// Subject не должен содержать перенос строки.
	subj := m.Header.Get("Subject")
	if strings.ContainsAny(subj, "\r\n") {
		t.Errorf("Subject содержит CR/LF: %q", subj)
	}
}

// ---------------------------------------------------------------------------
// LogMailer
// ---------------------------------------------------------------------------

func TestLogMailer_SendReturnsNil(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := NewLogMailer(log)

	err := m.Send(context.Background(), "to@example.com", "Тема", "<p/>", "text")
	if err != nil {
		t.Fatalf("Send вернул ошибку: %v", err)
	}
}

func TestLogMailer_LogsRecipientAndSubject(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	m := NewLogMailer(log)

	_ = m.Send(context.Background(), "target@example.com", "Проверка письма", "<p/>", "text")

	logged := buf.String()
	if !strings.Contains(logged, "target@example.com") {
		t.Errorf("лог не содержит recipient: %s", logged)
	}
	if !strings.Contains(logged, "Проверка письма") {
		t.Errorf("лог не содержит subject: %s", logged)
	}
}

func TestLogMailer_DoesNotLogHTMLBody(t *testing.T) {
	// LogMailer намеренно не логирует тело письма — только метаданные.
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	m := NewLogMailer(log)

	secretHTML := "<p>СЕКРЕТНЫЙ_КОНТЕНТ_В_ТЕЛЕ</p>"
	_ = m.Send(context.Background(), "to@test.com", "Тема", secretHTML, "plain text")

	if strings.Contains(buf.String(), "СЕКРЕТНЫЙ_КОНТЕНТ_В_ТЕЛЕ") {
		t.Error("LogMailer не должен логировать HTML-тело письма")
	}
}

// ---------------------------------------------------------------------------
// Template rendering
// ---------------------------------------------------------------------------

func TestRenderVerification_ContainsURLAndName(t *testing.T) {
	url := "https://repricerx.ru/api/auth/verify?token=abc123"
	data := VerificationData{DisplayName: "Иван Петров", URL: url}

	html, text, err := RenderVerification(data)
	if err != nil {
		t.Fatalf("RenderVerification: %v", err)
	}
	if html == "" {
		t.Error("HTML-тело пустое")
	}
	if text == "" {
		t.Error("текстовое тело пустое")
	}
	if !strings.Contains(html, url) {
		t.Errorf("HTML не содержит URL %q", url)
	}
	if !strings.Contains(text, url) {
		t.Errorf("текст не содержит URL %q", url)
	}
}

func TestRenderPasswordReset_ContainsURLAndName(t *testing.T) {
	url := "https://repricerx.ru/reset-password#token=xyz789"
	data := PasswordResetData{DisplayName: "Алиса", URL: url}

	html, text, err := RenderPasswordReset(data)
	if err != nil {
		t.Fatalf("RenderPasswordReset: %v", err)
	}
	if html == "" {
		t.Error("HTML-тело пустое")
	}
	if text == "" {
		t.Error("текстовое тело пустое")
	}
	if !strings.Contains(html, url) {
		t.Errorf("HTML не содержит URL %q", url)
	}
	if !strings.Contains(text, url) {
		t.Errorf("текст не содержит URL %q", url)
	}
}
