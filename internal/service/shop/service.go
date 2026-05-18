// Package shop реализует бизнес-логику управления магазинами маркетплейсов.
package shop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/integration"
	"github.com/Beliashkoff/RepricerX/internal/pkg/crypto"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/google/uuid"
)

var (
	ErrShopNotFound           = errors.New("shop not found")
	ErrInvalidMarketplace     = errors.New("invalid marketplace")
	ErrInvalidCredentials     = errors.New("invalid marketplace credentials")
	ErrAuthFailed             = errors.New("marketplace auth failed")
	ErrRateLimited            = errors.New("shop: rate limited by marketplace")
	ErrMarketplaceUnavailable = errors.New("marketplace temporarily unavailable")
)

const maxCredentialsJSONBytes = 4 * 1024

// MarketplaceFactory создаёт клиент маркетплейса. shopID — ключ для per-shop rate limiting.
type MarketplaceFactory func(shopID string, credsJSON []byte) (integration.Marketplace, error)

// UpdatePatch содержит изменяемые поля магазина (nil = не менять).
type UpdatePatch struct {
	Name              *string
	Credentials       json.RawMessage
	AutoUpdateEnabled *bool
	ScheduleCron      *string
}

type Service struct {
	shops     repository.ShopsRepository
	intLog    repository.IntegrationLogRepository
	secret    string
	factories map[string]MarketplaceFactory
}

func New(
	shops repository.ShopsRepository,
	intLog repository.IntegrationLogRepository,
	secret string,
	factories map[string]MarketplaceFactory,
) *Service {
	return &Service{
		shops:     shops,
		intLog:    intLog,
		secret:    secret,
		factories: factories,
	}
}

// Create создаёт новый магазин со статусом draft.
// credsJSON — незашифрованный JSON с учётными данными (WBCredentials / OzonCredentials).
func (s *Service) Create(ctx context.Context, userID uuid.UUID, marketplace, name string, credsJSON json.RawMessage) (*domain.Shop, error) {
	if _, ok := s.factories[marketplace]; !ok {
		return nil, ErrInvalidMarketplace
	}
	normalizedCreds, err := normalizeCredentials(marketplace, credsJSON)
	if err != nil {
		return nil, err
	}

	encrypted, err := crypto.Encrypt(normalizedCreds, s.secret)
	if err != nil {
		return nil, fmt.Errorf("shop create: encrypt: %w", err)
	}

	now := time.Now().UTC()
	shop := &domain.Shop{
		ID:                   uuid.New(),
		UserID:               userID,
		Marketplace:          marketplace,
		Name:                 name,
		CredentialsEncrypted: encrypted,
		Status:               domain.ShopStatusDraft,
		AutoUpdateEnabled:    false,
		ScheduleCron:         "0 3 * * *",
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	if err := s.shops.Create(ctx, shop); err != nil {
		return nil, fmt.Errorf("shop create: %w", err)
	}
	return shop, nil
}

// List возвращает все магазины пользователя.
func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]*domain.Shop, error) {
	shops, err := s.shops.ListByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("shop list: %w", err)
	}
	return shops, nil
}

// Get возвращает магазин по ID (только если принадлежит пользователю).
func (s *Service) Get(ctx context.Context, userID, shopID uuid.UUID) (*domain.Shop, error) {
	shop, err := s.shops.GetByID(ctx, shopID, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrShopNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("shop get: %w", err)
	}
	return shop, nil
}

// Update изменяет поля магазина согласно патчу.
func (s *Service) Update(ctx context.Context, userID, shopID uuid.UUID, patch UpdatePatch) (*domain.Shop, error) {
	shop, err := s.Get(ctx, userID, shopID)
	if err != nil {
		return nil, err
	}

	if patch.Name != nil {
		shop.Name = *patch.Name
	}
	if len(patch.Credentials) > 0 {
		if _, ok := s.factories[shop.Marketplace]; !ok {
			return nil, ErrInvalidMarketplace
		}
		normalizedCreds, err := normalizeCredentials(shop.Marketplace, patch.Credentials)
		if err != nil {
			return nil, err
		}
		encrypted, err := crypto.Encrypt(normalizedCreds, s.secret)
		if err != nil {
			return nil, fmt.Errorf("shop update: encrypt: %w", err)
		}
		shop.CredentialsEncrypted = encrypted
	}
	if patch.AutoUpdateEnabled != nil {
		shop.AutoUpdateEnabled = *patch.AutoUpdateEnabled
	}
	if patch.ScheduleCron != nil {
		shop.ScheduleCron = *patch.ScheduleCron
	}
	shop.UpdatedAt = time.Now().UTC()

	if err := s.shops.Update(ctx, shop); err != nil {
		return nil, fmt.Errorf("shop update: %w", err)
	}
	return shop, nil
}

func normalizeCredentials(marketplace string, raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 || len(raw) > maxCredentialsJSONBytes {
		return nil, ErrInvalidCredentials
	}
	switch marketplace {
	case domain.MarketplaceWB:
		var creds domain.WBCredentials
		if err := decodeStrictCredentials(raw, &creds); err != nil {
			return nil, ErrInvalidCredentials
		}
		creds.APIKey = strings.TrimSpace(creds.APIKey)
		if creds.APIKey == "" {
			return nil, ErrInvalidCredentials
		}
		return json.Marshal(creds)
	case domain.MarketplaceOzon:
		var creds domain.OzonCredentials
		if err := decodeStrictCredentials(raw, &creds); err != nil {
			return nil, ErrInvalidCredentials
		}
		creds.ClientID = strings.TrimSpace(creds.ClientID)
		creds.APIKey = strings.TrimSpace(creds.APIKey)
		if creds.ClientID == "" || creds.APIKey == "" {
			return nil, ErrInvalidCredentials
		}
		return json.Marshal(creds)
	default:
		return nil, ErrInvalidMarketplace
	}
}

func decodeStrictCredentials(raw []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ErrInvalidCredentials
	}
	return nil
}

// Delete удаляет магазин.
func (s *Service) Delete(ctx context.Context, userID, shopID uuid.UUID) error {
	err := s.shops.Delete(ctx, shopID, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrShopNotFound
	}
	return err
}

// TestConnection проверяет подключение к маркетплейсу и обновляет статус магазина.
func (s *Service) TestConnection(ctx context.Context, userID, shopID uuid.UUID) error {
	shop, err := s.Get(ctx, userID, shopID)
	if err != nil {
		return err
	}

	factory, ok := s.factories[shop.Marketplace]
	if !ok {
		return ErrInvalidMarketplace
	}

	credsJSON, err := crypto.Decrypt(shop.CredentialsEncrypted, s.secret)
	if err != nil {
		if errors.Is(err, crypto.ErrDecrypt) {
			return fmt.Errorf("shop %s: credentials not encrypted — run cmd/credbackfill: %w", shop.ID, err)
		}
		return fmt.Errorf("shop test: decrypt: %w", err)
	}

	client, err := factory(shop.ID.String(), credsJSON)
	if err != nil {
		return fmt.Errorf("shop test: build client: %w", err)
	}

	corrID := uuid.New()
	testErr := client.TestAuth(ctx)
	now := time.Now().UTC()

	newStatus := domain.ShopStatusActive
	logEntry := &domain.IntegrationLogEntry{
		ID:            uuid.New(),
		ShopID:        &shop.ID,
		Operation:     "test_auth",
		CorrelationID: corrID,
		CreatedAt:     now,
	}

	if testErr != nil {
		newStatus = domain.ShopStatusError
		switch {
		case errors.Is(testErr, integration.ErrUnauthorized):
			logEntry.ErrorText = "auth_failed"
			testErr = ErrAuthFailed
		case errors.Is(testErr, integration.ErrRateLimited):
			logEntry.ErrorText = "marketplace_rate_limited"
			testErr = ErrRateLimited
		case errors.Is(testErr, integration.ErrUnexpectedStatus):
			logEntry.ErrorText = "marketplace_unavailable"
			testErr = ErrMarketplaceUnavailable
		default:
			logEntry.ErrorText = "marketplace_test_failed"
		}
	}

	_ = s.shops.UpdateStatus(ctx, shop.ID, newStatus, now)
	_ = s.intLog.Create(ctx, logEntry)

	return testErr
}
