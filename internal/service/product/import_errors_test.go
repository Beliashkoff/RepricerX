package product

import (
	"strings"
	"testing"

	"github.com/Beliashkoff/RepricerX/internal/domain"
)

func TestPublicImportErrorMapsUnknownToStablePublicError(t *testing.T) {
	code, message := publicImportError("provider timeout: token=raw-secret")
	if code != importErrorUnknown {
		t.Fatalf("want %q, got %q", importErrorUnknown, code)
	}
	if message != publicImportErrors[importErrorUnknown] {
		t.Fatalf("want unknown public message, got %q", message)
	}
}

func TestPublicImportLogErrorsRewritesMessages(t *testing.T) {
	got := publicImportLogErrors([]domain.ImportLogError{
		{ExternalSKU: "SKU-1", Code: importErrorAdapter, Message: "raw provider error token=secret"},
		{ExternalSKU: "SKU-2", Code: "unexpected raw code", Message: "stack trace"},
	})

	if got[0].Code != importErrorAdapter || got[0].Message != publicImportErrors[importErrorAdapter] {
		t.Fatalf("adapter error was not normalized: %#v", got[0])
	}
	if got[1].Code != importErrorUnknown || got[1].Message != publicImportErrors[importErrorUnknown] {
		t.Fatalf("unknown error was not normalized: %#v", got[1])
	}
}

func TestRedactImportDiagnosticRemovesSensitiveData(t *testing.T) {
	input := `adapter failed: Authorization: Basic QWxhZGRpbjpvcGVuIHNlc2FtZQ== Cookie: session=secretcookie; refresh=secret-refresh url=https://api.example.test/accounts/SECRET-PATH/import?token=querysecret#frag payload={"credential":"full-payload","api_key":"AKIA123"} password=hunter2 client_id=my-client bearer SECRET-TOKEN`

	got := redactImportDiagnostic(input)
	for _, forbidden := range []string{
		"QWxhZGRpbjpvcGVuIHNlc2FtZQ==",
		"secretcookie",
		"secret-refresh",
		"SECRET-PATH",
		"querysecret",
		"full-payload",
		"AKIA123",
		"hunter2",
		"my-client",
		"SECRET-TOKEN",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("diagnostic leaked %q: %s", forbidden, got)
		}
	}
	if !strings.Contains(got, "[redacted]") {
		t.Fatalf("want redacted markers, got %q", got)
	}
}

func TestRedactImportDiagnosticDropsStackTraceLines(t *testing.T) {
	got := redactImportDiagnostic("provider timeout token=secret\npanic: stack trace\ninternal/file.go:12")
	if strings.Contains(got, "panic:") || strings.Contains(got, "internal/file.go") {
		t.Fatalf("diagnostic leaked stack trace: %s", got)
	}
	if strings.Contains(got, "secret") {
		t.Fatalf("diagnostic leaked secret: %s", got)
	}
}
