// Package main реализует инструмент повторного шифрования credentials магазинов.
// Миграция 000003 сохраняла credentials как plaintext (convert_to UTF8), а runtime
// ожидает AES-256-GCM ciphertext. Эта утилита детектирует plaintext-строки и
// зашифровывает их с APP_SECRET_KEY.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/pkg/crypto"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BackfillResult содержит статистику одного прогона backfill.
type BackfillResult struct {
	Total    int
	Skipped  int // уже зашифрованы
	Migrated int // успешно зашифрованы
	Failed   int // ошибка при обработке
}

type shopRow struct {
	id                   string
	marketplace          string
	credentialsEncrypted []byte
}

// RunBackfill обрабатывает все строки в таблице shops:
// уже зашифрованные пропускает, plaintext-строки шифрует и обновляет в БД.
// Идемпотентен — безопасно запускать повторно.
func RunBackfill(ctx context.Context, pool *pgxpool.Pool, secret string) (BackfillResult, error) {
	rows, err := pool.Query(ctx,
		"SELECT id::text, marketplace, credentials_encrypted FROM shops ORDER BY created_at")
	if err != nil {
		return BackfillResult{}, fmt.Errorf("query shops: %w", err)
	}
	defer rows.Close()

	var shops []shopRow
	for rows.Next() {
		var s shopRow
		if err := rows.Scan(&s.id, &s.marketplace, &s.credentialsEncrypted); err != nil {
			return BackfillResult{}, fmt.Errorf("scan row: %w", err)
		}
		shops = append(shops, s)
	}
	if err := rows.Err(); err != nil {
		return BackfillResult{}, fmt.Errorf("iterate rows: %w", err)
	}

	var result BackfillResult
	result.Total = len(shops)

	for _, shop := range shops {
		normalized, skip, err := normalizeCredentials(shop.marketplace, shop.credentialsEncrypted, secret)
		if err != nil {
			slog.Error("normalize credentials",
				"shop_id", shop.id, "marketplace", shop.marketplace, "error", err)
			result.Failed++
			continue
		}
		if skip {
			slog.Debug("skip: already encrypted", "shop_id", shop.id)
			result.Skipped++
			continue
		}

		encrypted, err := encryptAndVerify(normalized, secret)
		if err != nil {
			slog.Error("encrypt credentials",
				"shop_id", shop.id, "marketplace", shop.marketplace, "error", err)
			result.Failed++
			continue
		}

		if _, err := pool.Exec(ctx,
			"UPDATE shops SET credentials_encrypted = $1, updated_at = NOW() WHERE id = $2::uuid",
			encrypted, shop.id,
		); err != nil {
			slog.Error("update shop", "shop_id", shop.id, "error", err)
			result.Failed++
			continue
		}

		slog.Info("migrated", "shop_id", shop.id, "marketplace", shop.marketplace)
		result.Migrated++
	}

	return result, nil
}

// normalizeCredentials возвращает:
//   - (nil, true, nil)      — данные уже зашифрованы, строку надо пропустить
//   - (json, false, nil)    — plaintext нормализован до JSON, готов к шифрованию
//   - (nil, false, error)   — некорректные данные, строку надо пометить failed
func normalizeCredentials(marketplace string, raw []byte, secret string) ([]byte, bool, error) {
	// Сначала проверяем: данные уже зашифрованы?
	if _, err := crypto.Decrypt(raw, secret); err == nil {
		return nil, true, nil
	} else if !errors.Is(err, crypto.ErrDecrypt) {
		return nil, false, fmt.Errorf("unexpected decrypt error: %w", err)
	}

	// Данные — plaintext. Нормализуем до корректного JSON.
	switch marketplace {
	case domain.MarketplaceWB:
		return normalizeWB(raw)
	case domain.MarketplaceOzon:
		return normalizeOzon(raw)
	default:
		return nil, false, fmt.Errorf("unknown marketplace: %q", marketplace)
	}
}

// normalizeWB обрабатывает два варианта WB credentials, которые мог создать 000003:
//   - convert_to(api_token_wb, 'UTF8')                          → raw token bytes ("tok_abc123")
//   - convert_to(jsonb_build_object('api_key', ...)::text, ...) → JSON bytes ({"api_key":"..."})
func normalizeWB(raw []byte) ([]byte, bool, error) {
	var creds domain.WBCredentials
	if json.Unmarshal(raw, &creds) == nil && creds.APIKey != "" {
		// Уже в JSON-формате {"api_key":"..."}
		return raw, false, nil
	}
	// Сырой токен-строка — оборачиваем в JSON
	token := string(raw)
	if token == "" {
		return nil, false, fmt.Errorf("wb: empty api_token_wb")
	}
	wrapped, err := json.Marshal(domain.WBCredentials{APIKey: token})
	if err != nil {
		return nil, false, fmt.Errorf("wb: marshal credentials: %w", err)
	}
	return wrapped, false, nil
}

// normalizeOzon обрабатывает Ozon credentials: 000003 всегда сохранял JSON.
func normalizeOzon(raw []byte) ([]byte, bool, error) {
	var creds domain.OzonCredentials
	if err := json.Unmarshal(raw, &creds); err != nil {
		return nil, false, fmt.Errorf("ozon: cannot parse as JSON: %w", err)
	}
	if creds.ClientID == "" || creds.APIKey == "" {
		return nil, false, fmt.Errorf("ozon: empty client_id or api_key after parse")
	}
	return raw, false, nil
}

// encryptAndVerify шифрует credJSON и сразу проверяет, что decrypt работает.
func encryptAndVerify(credJSON []byte, secret string) ([]byte, error) {
	encrypted, err := crypto.Encrypt(credJSON, secret)
	if err != nil {
		return nil, fmt.Errorf("encrypt: %w", err)
	}
	if _, err := crypto.Decrypt(encrypted, secret); err != nil {
		return nil, fmt.Errorf("post-encrypt verify: %w", err)
	}
	return encrypted, nil
}
