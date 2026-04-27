package password

import (
	"strings"
	"testing"
)

func TestHash_ProducesPHCFormat(t *testing.T) {
	phc, err := Hash("CorrectHorse42")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if !strings.HasPrefix(phc, "$argon2id$") {
		t.Errorf("ожидался PHC-префикс $argon2id$, получили: %s", phc)
	}
}

func TestVerify_Roundtrip(t *testing.T) {
	const pw = "CorrectHorse42"
	phc, err := Hash(pw)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	ok, err := Verify(pw, phc)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !ok {
		t.Error("Verify вернул false для верного пароля")
	}
}

func TestVerify_WrongPassword(t *testing.T) {
	phc, _ := Hash("CorrectHorse42")
	ok, err := Verify("WrongPassword1", phc)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if ok {
		t.Error("Verify вернул true для неверного пароля")
	}
}

func TestVerify_InvalidPHC(t *testing.T) {
	_, err := Verify("any", "not-a-phc-string")
	if err == nil {
		t.Error("ожидали ошибку для невалидного PHC")
	}
}

// Разные хеши одного пароля — salt случайный
func TestHash_DifferentSaltsEachCall(t *testing.T) {
	h1, _ := Hash("SamePassword1")
	h2, _ := Hash("SamePassword1")
	if h1 == h2 {
		t.Error("два хеша одного пароля не должны совпадать")
	}
}
