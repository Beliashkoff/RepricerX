package auth

import (
	"strings"
	"testing"
)

func TestValidateEmail(t *testing.T) {
	valid := []string{
		"user@example.com",
		"user+tag@example.org",
		"a@b.ru",
	}
	for _, e := range valid {
		if err := validateEmail(e); err != nil {
			t.Errorf("ожидали valid для %q, получили: %v", e, err)
		}
	}

	invalid := []string{
		"",
		"notanemail",
		"@nodomain",
		"no@",
		"a b@example.com",
	}
	for _, e := range invalid {
		if err := validateEmail(e); err == nil {
			t.Errorf("ожидали invalid для %q", e)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	valid := []string{
		"Str0ngPass12",
		"abcdefghijk1",
		strings.Repeat("a", 127) + "1",
	}
	for _, p := range valid {
		if err := validatePassword(p); err != nil {
			t.Errorf("ожидали valid для пароля длиной %d, получили: %v", len([]rune(p)), err)
		}
	}

	invalid := []string{
		"short1",          // < 12
		"onlylettershere", // нет цифры
		"1234567890123",   // нет буквы
		strings.Repeat("a", 128) + "1", // > 128
		"ValidPass12\x01", // управляющий символ
	}
	for _, p := range invalid {
		if err := validatePassword(p); err == nil {
			t.Errorf("ожидали invalid для %q", p)
		}
	}
}

func TestValidatePassword_Boundary(t *testing.T) {
	// Ровно 8 — граница (валидно)
	if err := validatePassword("Abcdef1!"); err != nil {
		t.Errorf("8 символов должны проходить: %v", err)
	}

	// Ровно 128 — граница (валидно)
	pw128 := strings.Repeat("a", 127) + "1"
	if err := validatePassword(pw128); err != nil {
		t.Errorf("128 символов должны проходить: %v", err)
	}

	// 129 — выход за границу
	pw129 := strings.Repeat("a", 128) + "1"
	if err := validatePassword(pw129); err == nil {
		t.Error("129 символов должны отклоняться")
	}
}
