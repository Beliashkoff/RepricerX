package token

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestGenerate_ReturnsBase64URLNoPadding(t *testing.T) {
	pt, _, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// base64url без паддинга — не должно быть + / =
	if strings.ContainsAny(pt, "+/=") {
		t.Errorf("plaintext содержит символы не из base64url: %s", pt)
	}
	// Проверяем декодируемость и размер энтропии
	raw, err := base64.RawURLEncoding.DecodeString(pt)
	if err != nil {
		t.Fatalf("DecodeString: %v", err)
	}
	if len(raw) != tokenBytes {
		t.Errorf("ожидали %d байт, получили %d", tokenBytes, len(raw))
	}
}

func TestGenerate_Unique(t *testing.T) {
	pt1, h1, _ := Generate()
	pt2, h2, _ := Generate()
	if pt1 == pt2 {
		t.Error("два plaintext совпали — коллизия rand")
	}
	if h1 == h2 {
		t.Error("два hash совпали — коллизия")
	}
}

func TestHash_Deterministic(t *testing.T) {
	const pt = "test-plaintext-value"
	h1 := Hash(pt)
	h2 := Hash(pt)
	if h1 != h2 {
		t.Error("Hash недетерминирован")
	}
}

func TestHash_IsHex64Chars(t *testing.T) {
	h := Hash("anything")
	if len(h) != 64 {
		t.Errorf("ожидали 64 hex-символа (sha256), получили %d", len(h))
	}
}

func TestGenerate_HashMatchesPlaintext(t *testing.T) {
	pt, hashFromGenerate, _ := Generate()
	if Hash(pt) != hashFromGenerate {
		t.Error("Hash(plaintext) != hashHex из Generate")
	}
}
