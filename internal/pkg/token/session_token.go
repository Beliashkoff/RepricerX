// Package token генерирует и хеширует одноразовые токены (сессии, верификация email).
// Схема: plaintext уходит клиенту, в БД хранится только sha256(plaintext) в hex.
package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

const tokenBytes = 32 // 256 бит энтропии

// Generate создаёт случайный токен. Возвращает:
//   - plaintext — отправляется в cookie / письмо, нигде не сохраняется
//   - hashHex   — sha256(plaintext) в hex, кладётся в БД
func Generate() (plaintext, hashHex string, err error) {
	raw := make([]byte, tokenBytes)
	if _, err = rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("token: генерация случайных байт: %w", err)
	}

	plaintext = base64.RawURLEncoding.EncodeToString(raw)
	hashHex = Hash(plaintext)
	return plaintext, hashHex, nil
}

// Hash возвращает sha256(plaintext) в hex. Детерминирован — используется и для
// хранения при создании, и для lookup при валидации/logout.
func Hash(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
