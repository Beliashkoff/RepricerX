package main

import (
	"encoding/json"
	"testing"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/pkg/crypto"
)

const testSecret = "test-secret-key-for-backfill"

// ---------------------------------------------------------------------------
// normalizeCredentials — WB
// ---------------------------------------------------------------------------

func TestNormalizeCredentials_WB_RawToken(t *testing.T) {
	raw := []byte("tok_abc123_wildberries")

	normalized, skip, err := normalizeCredentials(domain.MarketplaceWB, raw, testSecret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skip {
		t.Fatal("raw token must not be skipped")
	}

	var creds domain.WBCredentials
	if err := json.Unmarshal(normalized, &creds); err != nil {
		t.Fatalf("normalized result is not valid JSON: %v — got: %s", err, normalized)
	}
	if creds.APIKey != "tok_abc123_wildberries" {
		t.Errorf("api_key: got %q, want %q", creds.APIKey, "tok_abc123_wildberries")
	}
}

func TestNormalizeCredentials_WB_AlreadyJSON(t *testing.T) {
	raw := []byte(`{"api_key":"tok_abc123"}`)

	normalized, skip, err := normalizeCredentials(domain.MarketplaceWB, raw, testSecret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skip {
		t.Fatal("plaintext JSON must not be skipped")
	}

	var creds domain.WBCredentials
	if err := json.Unmarshal(normalized, &creds); err != nil || creds.APIKey != "tok_abc123" {
		t.Errorf("unexpected result %s: %v", normalized, err)
	}
}

func TestNormalizeCredentials_WB_EmptyToken(t *testing.T) {
	// Пустой токен — должна быть ошибка, а не тихое шифрование пустой строки.
	raw := []byte("")

	_, _, err := normalizeCredentials(domain.MarketplaceWB, raw, testSecret)
	if err == nil {
		t.Fatal("expected error for empty WB token, got nil")
	}
}

// ---------------------------------------------------------------------------
// normalizeCredentials — Ozon
// ---------------------------------------------------------------------------

func TestNormalizeCredentials_Ozon_JSON(t *testing.T) {
	raw := []byte(`{"client_id":"12345","api_key":"ozon_secret"}`)

	normalized, skip, err := normalizeCredentials(domain.MarketplaceOzon, raw, testSecret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skip {
		t.Fatal("plaintext Ozon JSON must not be skipped")
	}

	var creds domain.OzonCredentials
	if err := json.Unmarshal(normalized, &creds); err != nil {
		t.Fatalf("not valid OzonCredentials JSON: %v", err)
	}
	if creds.ClientID != "12345" || creds.APIKey != "ozon_secret" {
		t.Errorf("unexpected creds: %+v", creds)
	}
}

func TestNormalizeCredentials_Ozon_EmptyCreds(t *testing.T) {
	// Магазин мигрирован с пустыми ключами — ошибка, не тихое шифрование.
	raw := []byte(`{"client_id":"","api_key":""}`)

	_, _, err := normalizeCredentials(domain.MarketplaceOzon, raw, testSecret)
	if err == nil {
		t.Fatal("expected error for empty Ozon credentials, got nil")
	}
}

func TestNormalizeCredentials_Ozon_NotJSON(t *testing.T) {
	raw := []byte("not-json-at-all")

	_, _, err := normalizeCredentials(domain.MarketplaceOzon, raw, testSecret)
	if err == nil {
		t.Fatal("expected error for non-JSON Ozon credentials")
	}
}

// ---------------------------------------------------------------------------
// normalizeCredentials — уже зашифровано
// ---------------------------------------------------------------------------

func TestNormalizeCredentials_AlreadyEncrypted_WB(t *testing.T) {
	original := []byte(`{"api_key":"tok_already_encrypted"}`)
	encrypted, err := crypto.Encrypt(original, testSecret)
	if err != nil {
		t.Fatalf("setup encrypt: %v", err)
	}

	_, skip, err := normalizeCredentials(domain.MarketplaceWB, encrypted, testSecret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !skip {
		t.Fatal("already-encrypted data must be skipped")
	}
}

func TestNormalizeCredentials_AlreadyEncrypted_Ozon(t *testing.T) {
	original := []byte(`{"client_id":"123","api_key":"secret"}`)
	encrypted, _ := crypto.Encrypt(original, testSecret)

	_, skip, err := normalizeCredentials(domain.MarketplaceOzon, encrypted, testSecret)
	if err != nil || !skip {
		t.Fatalf("err=%v skip=%v — expected skip=true, err=nil", err, skip)
	}
}

// ---------------------------------------------------------------------------
// normalizeCredentials — неизвестный маркетплейс
// ---------------------------------------------------------------------------

func TestNormalizeCredentials_UnknownMarketplace(t *testing.T) {
	_, _, err := normalizeCredentials("unknown_mp", []byte("data"), testSecret)
	if err == nil {
		t.Fatal("expected error for unknown marketplace")
	}
}

// ---------------------------------------------------------------------------
// encryptAndVerify
// ---------------------------------------------------------------------------

func TestEncryptAndVerify_RoundTrip(t *testing.T) {
	payload := []byte(`{"api_key":"tok_round_trip"}`)

	encrypted, err := encryptAndVerify(payload, testSecret)
	if err != nil {
		t.Fatalf("encryptAndVerify: %v", err)
	}

	decrypted, err := crypto.Decrypt(encrypted, testSecret)
	if err != nil {
		t.Fatalf("decrypt after encryptAndVerify: %v", err)
	}
	if string(decrypted) != string(payload) {
		t.Errorf("got %q, want %q", decrypted, payload)
	}
}

func TestEncryptAndVerify_Idempotent(t *testing.T) {
	// Два вызова должны давать разные ciphertext (уникальный nonce), но оба декодируемые.
	payload := []byte(`{"api_key":"tok"}`)
	e1, _ := encryptAndVerify(payload, testSecret)
	e2, _ := encryptAndVerify(payload, testSecret)

	if string(e1) == string(e2) {
		t.Error("два encrypt одного plaintext должны давать разные ciphertext")
	}

	d1, err1 := crypto.Decrypt(e1, testSecret)
	d2, err2 := crypto.Decrypt(e2, testSecret)
	if err1 != nil || err2 != nil {
		t.Fatalf("decrypt: err1=%v err2=%v", err1, err2)
	}
	if string(d1) != string(payload) || string(d2) != string(payload) {
		t.Error("оба decrypt должны давать исходный plaintext")
	}
}
