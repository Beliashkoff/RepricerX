package product

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"github.com/Beliashkoff/RepricerX/internal/domain"
)

const (
	importErrorAdapter     = "adapter_error"
	importErrorCredentials = "credentials_error"
	importErrorUpsert      = "upsert_failed"
	importErrorValidation  = "validation_error"
	importErrorUnknown     = "unknown_error"

	importErrorDuplicateSKU  = "duplicate_sku"
	importErrorInvalidSKU    = "invalid_sku"
	importErrorLimitExceeded = "limit_exceeded"
	importErrorTooMany       = "too_many_errors"
)

var publicImportErrors = map[string]string{
	importErrorAdapter:     "Import failed because the external shop adapter returned an error.",
	importErrorCredentials: "Import failed because shop credentials are invalid or expired.",
	importErrorUpsert:      "Import failed while saving product data.",
	importErrorValidation:  "Import failed because some product data was invalid.",
	importErrorUnknown:     "Import failed due to an unexpected error.",

	importErrorDuplicateSKU:  "duplicate SKU in import payload",
	importErrorInvalidSKU:    "invalid imported SKU data",
	importErrorLimitExceeded: "import row limit exceeded",
	importErrorTooMany:       "additional import errors were omitted",
}

var (
	importDiagnosticURLPattern        = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s<>"']+`)
	importDiagnosticHeaderPattern     = regexp.MustCompile(`(?i)\b(authorization|proxy-authorization|cookie|set-cookie)\b\s*[:=]\s*[^,\r\n]+`)
	importDiagnosticSecretPattern     = regexp.MustCompile(`(?i)\b(access[_-]?token|api[_-]?key|body|client[_-]?id|credentials?|password|payload|refresh[_-]?token|request|response|secret|token)\b\s*[:=]\s*("[^"]*"|'[^']*'|\{[^}]*\}|\[[^\]]*\]|[^\s,;&]+)`)
	importDiagnosticJSONSecretPattern = regexp.MustCompile(`(?i)"(access[_-]?token|api[_-]?key|authorization|client[_-]?id|cookie|credentials?|password|refresh[_-]?token|secret|set-cookie|token)"\s*:\s*("[^"]*"|[^,\}\]\s]+)`)
	importDiagnosticBearerPattern     = regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/=-]+`)
	importDiagnosticBasicPattern      = regexp.MustCompile(`(?i)\bbasic\s+[a-z0-9._~+/=-]+`)
)

func publicImportError(code string) (string, string) {
	if message, ok := publicImportErrors[code]; ok {
		return code, message
	}
	return importErrorUnknown, publicImportErrors[importErrorUnknown]
}

func publicImportLogErrors(errs []domain.ImportLogError) []domain.ImportLogError {
	if len(errs) == 0 {
		return errs
	}
	out := make([]domain.ImportLogError, 0, len(errs))
	for _, err := range errs {
		code, message := publicImportError(err.Code)
		out = append(out, domain.ImportLogError{
			ExternalSKU: err.ExternalSKU,
			Code:        code,
			Message:     message,
		})
	}
	return out
}

func redactImportDiagnostic(value string) string {
	value = firstImportDiagnosticLine(strings.TrimSpace(value))
	if value == "" {
		return "internal diagnostic unavailable"
	}
	value = importDiagnosticURLPattern.ReplaceAllStringFunc(value, redactDiagnosticURL)
	value = importDiagnosticBearerPattern.ReplaceAllString(value, "Bearer [redacted]")
	value = importDiagnosticBasicPattern.ReplaceAllString(value, "Basic [redacted]")
	value = importDiagnosticHeaderPattern.ReplaceAllString(value, "$1=[redacted]")
	value = importDiagnosticJSONSecretPattern.ReplaceAllString(value, `"$1":"[redacted]"`)
	value = importDiagnosticSecretPattern.ReplaceAllString(value, "$1=[redacted]")
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return "internal diagnostic unavailable"
	}
	return truncateImportDiagnostic(value, 300)
}

func firstImportDiagnosticLine(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	for _, line := range strings.Split(value, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func redactDiagnosticURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" {
		return "[redacted-url]"
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return parsed.Scheme + "://[redacted]"
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	if parsed.Host == "" {
		return parsed.Scheme + "://[redacted]"
	}
	parsed.Path = "/[redacted]"
	parsed.RawPath = ""
	return parsed.String()
}

func truncateImportDiagnostic(value string, maxRunes int) string {
	for i := range value {
		if maxRunes == 0 {
			if i == 0 {
				return ""
			}
			return strings.TrimSpace(value[:i]) + "..."
		}
		maxRunes--
	}
	return value
}
