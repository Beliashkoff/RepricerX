package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Параметры argon2id зафиксированы здесь — PHC-строка хранит их вместе с хешем,
// поэтому смена параметров не сломает проверку старых хешей.
const (
	argonTime    = 2
	argonMemory  = 64 * 1024 // 64 MiB
	argonThreads = 2
	argonKeyLen  = 32
	argonSaltLen = 16
)

var (
	ErrInvalidHash   = errors.New("password: неверный формат PHC-строки")
	ErrWrongPassword = errors.New("password: неверный пароль")
)

// Hash хеширует пароль и возвращает PHC-строку для хранения в БД.
func Hash(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("password: генерация соли: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	// Формат PHC: $argon2id$v=19$m=65536,t=2,p=2$<salt_b64>$<hash_b64>
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argonMemory,
		argonTime,
		argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// Verify проверяет пароль против PHC-строки. Использует constant-time сравнение.
func Verify(password, phc string) (bool, error) {
	salt, storedHash, params, err := parsePHC(phc)
	if err != nil {
		return false, err
	}

	candidate := argon2.IDKey([]byte(password), salt, params.time, params.memory, params.threads, uint32(len(storedHash)))

	// subtle.ConstantTimeCompare защищает от timing-атак
	if subtle.ConstantTimeCompare(candidate, storedHash) != 1 {
		return false, nil
	}
	return true, nil
}

type phcParams struct {
	time    uint32
	memory  uint32
	threads uint8
}

func parsePHC(phc string) (salt, hash []byte, p phcParams, err error) {
	parts := strings.Split(phc, "$")
	// Ожидаем: ["", "argon2id", "v=19", "m=...,t=...,p=...", "<salt>", "<hash>"]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return nil, nil, p, ErrInvalidHash
	}

	var threads uint32
	if _, scanErr := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &threads); scanErr != nil {
		return nil, nil, p, ErrInvalidHash
	}
	p.threads = uint8(threads)

	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, p, ErrInvalidHash
	}

	hash, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, p, ErrInvalidHash
	}

	return salt, hash, p, nil
}
