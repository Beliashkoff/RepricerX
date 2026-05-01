package crypto

import (
	"bytes"
	"testing"
)

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	secret := "test-secret-key"
	plaintext := []byte(`{"api_key":"tok_test_12345"}`)

	ciphertext, err := Encrypt(plaintext, secret)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if bytes.Equal(ciphertext, plaintext) {
		t.Fatal("ciphertext must differ from plaintext")
	}

	got, err := Decrypt(ciphertext, secret)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("got %q, want %q", got, plaintext)
	}
}

func TestEncryptDecrypt_WrongKey(t *testing.T) {
	ciphertext, _ := Encrypt([]byte("secret data"), "key-a")
	_, err := Decrypt(ciphertext, "key-b")
	if err != ErrDecrypt {
		t.Fatalf("expected ErrDecrypt, got %v", err)
	}
}

func TestDecrypt_TooShort(t *testing.T) {
	_, err := Decrypt([]byte("short"), "key")
	if err != ErrDecrypt {
		t.Fatalf("expected ErrDecrypt, got %v", err)
	}
}

func TestEncrypt_UniqueNonces(t *testing.T) {
	c1, _ := Encrypt([]byte("same"), "key")
	c2, _ := Encrypt([]byte("same"), "key")
	if bytes.Equal(c1, c2) {
		t.Fatal("two encryptions of same plaintext must produce different ciphertexts")
	}
}
