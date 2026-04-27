package auth

import (
	"errors"
	"net/mail"
	"unicode"
)

var (
	ErrInvalidEmail = errors.New("invalid email")
	ErrWeakPassword = errors.New("weak password")
)

// validateEmail проверяет формат через net/mail — охватывает стандартный RFC 5322.
func validateEmail(email string) error {
	if _, err := mail.ParseAddress(email); err != nil {
		return ErrInvalidEmail
	}
	return nil
}

// validatePassword проверяет пароль по OWASP 2024:
//   - от 12 до 128 символов,
//   - содержит хотя бы одну букву и одну цифру,
//   - без непечатаемых управляющих символов (кроме пробела).
//
// Максимум 128 символов — защита argon2id от DoS-атак огромными входами.
func validatePassword(password string) error {
	runes := []rune(password)
	length := len(runes)
	if length < 12 || length > 128 {
		return ErrWeakPassword
	}

	var hasLetter, hasDigit bool
	for _, r := range runes {
		if unicode.IsControl(r) && r != ' ' {
			// Управляющие символы (кроме пробела) запрещены
			return ErrWeakPassword
		}
		if unicode.IsLetter(r) {
			hasLetter = true
		}
		if unicode.IsDigit(r) {
			hasDigit = true
		}
	}

	if !hasLetter || !hasDigit {
		return ErrWeakPassword
	}
	return nil
}
