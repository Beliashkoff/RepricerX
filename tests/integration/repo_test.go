//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/pkg/token"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/google/uuid"
)

// --- UsersRepository ---

func TestUsersRepo_CreateAndGet(t *testing.T) {
	truncate(t)
	repo := repository.NewUsersRepository(testPool)
	ctx := context.Background()

	user := &domain.User{
		ID:           uuid.New(),
		Email:        "repo@example.com",
		PasswordHash: "hash",
		DisplayName:  "Repo User",
		Status:       domain.UserStatusPending,
	}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByEmail(ctx, user.Email)
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if got.ID != user.ID {
		t.Fatalf("want id %v, got %v", user.ID, got.ID)
	}
	if got.Status != domain.UserStatusPending {
		t.Fatalf("want status %q, got %q", domain.UserStatusPending, got.Status)
	}

	byID, err := repo.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if byID.Email != user.Email {
		t.Fatalf("want email %q, got %q", user.Email, byID.Email)
	}
}

func TestUsersRepo_DuplicateEmail(t *testing.T) {
	truncate(t)
	repo := repository.NewUsersRepository(testPool)
	ctx := context.Background()

	user := &domain.User{
		ID: uuid.New(), Email: "dup@example.com",
		PasswordHash: "h", DisplayName: "U", Status: domain.UserStatusPending,
	}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	dup := &domain.User{
		ID: uuid.New(), Email: "dup@example.com",
		PasswordHash: "h", DisplayName: "U2", Status: domain.UserStatusPending,
	}
	err := repo.Create(ctx, dup)
	if err != repository.ErrEmailTaken {
		t.Fatalf("want ErrEmailTaken, got %v", err)
	}
}

func TestUsersRepo_GetByEmail_NotFound(t *testing.T) {
	truncate(t)
	repo := repository.NewUsersRepository(testPool)

	_, err := repo.GetByEmail(context.Background(), "nobody@example.com")
	if err != repository.ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestUsersRepo_UpdateDisplayName(t *testing.T) {
	truncate(t)
	repo := repository.NewUsersRepository(testPool)
	ctx := context.Background()

	user := &domain.User{
		ID: uuid.New(), Email: "upd@example.com",
		PasswordHash: "h", DisplayName: "Old", Status: domain.UserStatusPending,
	}
	repo.Create(ctx, user)

	if err := repo.UpdateDisplayName(ctx, user.ID, "New Name"); err != nil {
		t.Fatalf("UpdateDisplayName: %v", err)
	}

	got, _ := repo.GetByID(ctx, user.ID)
	if got.DisplayName != "New Name" {
		t.Fatalf("want %q, got %q", "New Name", got.DisplayName)
	}
}

func TestUsersRepo_FailedLoginTracking(t *testing.T) {
	truncate(t)
	repo := repository.NewUsersRepository(testPool)
	ctx := context.Background()

	user := &domain.User{
		ID: uuid.New(), Email: "lock@example.com",
		PasswordHash: "h", DisplayName: "L", Status: domain.UserStatusActive,
	}
	repo.Create(ctx, user)

	lockout := time.Now().Add(15 * time.Minute)
	if err := repo.RegisterFailedLogin(ctx, user.ID, 3, &lockout); err != nil {
		t.Fatalf("RegisterFailedLogin: %v", err)
	}

	got, _ := repo.GetByID(ctx, user.ID)
	if got.FailedLoginCount != 3 {
		t.Fatalf("want FailedLoginCount=3, got %d", got.FailedLoginCount)
	}
	if got.LockoutUntil == nil {
		t.Fatal("want LockoutUntil set, got nil")
	}

	if err := repo.ResetFailedLogin(ctx, user.ID); err != nil {
		t.Fatalf("ResetFailedLogin: %v", err)
	}
	got, _ = repo.GetByID(ctx, user.ID)
	if got.FailedLoginCount != 0 {
		t.Fatalf("want FailedLoginCount=0 after reset, got %d", got.FailedLoginCount)
	}
	if got.LockoutUntil != nil {
		t.Fatal("want LockoutUntil nil after reset")
	}
}

func TestUsersRepo_UpdateStatus(t *testing.T) {
	truncate(t)
	repo := repository.NewUsersRepository(testPool)
	ctx := context.Background()

	user := &domain.User{
		ID: uuid.New(), Email: "status@example.com",
		PasswordHash: "h", DisplayName: "S", Status: domain.UserStatusPending,
	}
	repo.Create(ctx, user)

	if err := repo.UpdateStatus(ctx, user.ID, domain.UserStatusActive); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	got, _ := repo.GetByID(ctx, user.ID)
	if got.Status != domain.UserStatusActive {
		t.Fatalf("want status %q, got %q", domain.UserStatusActive, got.Status)
	}
}

// --- SessionsRepository ---

func TestSessionsRepo_CreateAndGet(t *testing.T) {
	truncate(t)
	usersRepo := repository.NewUsersRepository(testPool)
	sessRepo := repository.NewSessionsRepository(testPool)
	ctx := context.Background()

	user := &domain.User{
		ID: uuid.New(), Email: "sess@example.com",
		PasswordHash: "h", DisplayName: "S", Status: domain.UserStatusActive,
	}
	usersRepo.Create(ctx, user)

	_, hashHex, _ := token.Generate()
	now := time.Now()
	sess := &domain.Session{
		ID:                uuid.New(),
		UserID:            user.ID,
		TokenHash:         hashHex,
		CreatedAt:         now,
		LastSeenAt:        now,
		IdleExpiresAt:     now.Add(24 * time.Hour),
		AbsoluteExpiresAt: now.Add(7 * 24 * time.Hour),
		UserAgent:         "test-agent",
		IPPrefix:          "127.0.0.0/24",
	}
	if err := sessRepo.Create(ctx, sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := sessRepo.GetByTokenHash(ctx, hashHex)
	if err != nil {
		t.Fatalf("GetByTokenHash: %v", err)
	}
	if got.UserID != user.ID {
		t.Fatalf("want UserID %v, got %v", user.ID, got.UserID)
	}
}

func TestSessionsRepo_ExpiredSession(t *testing.T) {
	truncate(t)
	usersRepo := repository.NewUsersRepository(testPool)
	sessRepo := repository.NewSessionsRepository(testPool)
	ctx := context.Background()

	user := &domain.User{
		ID: uuid.New(), Email: "exp@example.com",
		PasswordHash: "h", DisplayName: "E", Status: domain.UserStatusActive,
	}
	usersRepo.Create(ctx, user)

	_, hashHex, _ := token.Generate()
	past := time.Now().Add(-time.Hour)
	sess := &domain.Session{
		ID:                uuid.New(),
		UserID:            user.ID,
		TokenHash:         hashHex,
		CreatedAt:         past,
		LastSeenAt:        past,
		IdleExpiresAt:     past, // already expired
		AbsoluteExpiresAt: past,
		UserAgent:         "agent",
		IPPrefix:          "127.0.0.0/24",
	}
	sessRepo.Create(ctx, sess)

	_, err := sessRepo.GetByTokenHash(ctx, hashHex)
	if err != repository.ErrNotFound {
		t.Fatalf("want ErrNotFound for expired session, got %v", err)
	}
}

func TestSessionsRepo_DeleteByTokenHash(t *testing.T) {
	truncate(t)
	usersRepo := repository.NewUsersRepository(testPool)
	sessRepo := repository.NewSessionsRepository(testPool)
	ctx := context.Background()

	user := &domain.User{
		ID: uuid.New(), Email: "del@example.com",
		PasswordHash: "h", DisplayName: "D", Status: domain.UserStatusActive,
	}
	usersRepo.Create(ctx, user)

	_, hashHex, _ := token.Generate()
	now := time.Now()
	sessRepo.Create(ctx, &domain.Session{
		ID:                uuid.New(),
		UserID:            user.ID,
		TokenHash:         hashHex,
		CreatedAt:         now,
		LastSeenAt:        now,
		IdleExpiresAt:     now.Add(time.Hour),
		AbsoluteExpiresAt: now.Add(time.Hour),
	})

	if err := sessRepo.DeleteByTokenHash(ctx, hashHex); err != nil {
		t.Fatalf("DeleteByTokenHash: %v", err)
	}
	_, err := sessRepo.GetByTokenHash(ctx, hashHex)
	if err != repository.ErrNotFound {
		t.Fatalf("want ErrNotFound after delete, got %v", err)
	}
}

func TestSessionsRepo_TouchIdleIfNeeded(t *testing.T) {
	truncate(t)
	usersRepo := repository.NewUsersRepository(testPool)
	sessRepo := repository.NewSessionsRepository(testPool)
	ctx := context.Background()

	user := &domain.User{
		ID: uuid.New(), Email: "touch@example.com",
		PasswordHash: "h", DisplayName: "T", Status: domain.UserStatusActive,
	}
	usersRepo.Create(ctx, user)

	// Session expiring in 6 hours (< 12h threshold → should be extended)
	_, hashHex, _ := token.Generate()
	now := time.Now()
	sessID := uuid.New()
	sessRepo.Create(ctx, &domain.Session{
		ID:                sessID,
		UserID:            user.ID,
		TokenHash:         hashHex,
		CreatedAt:         now,
		LastSeenAt:        now,
		IdleExpiresAt:     now.Add(6 * time.Hour),
		AbsoluteExpiresAt: now.Add(7 * 24 * time.Hour),
	})

	newIdle, err := sessRepo.TouchIdleIfNeeded(ctx, sessID, now.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("TouchIdleIfNeeded: %v", err)
	}
	if newIdle == nil {
		t.Fatal("expected idle TTL to be extended (session < 12h from expiry), got nil")
	}
}

func TestSessionsRepo_TouchIdleNotNeeded(t *testing.T) {
	truncate(t)
	usersRepo := repository.NewUsersRepository(testPool)
	sessRepo := repository.NewSessionsRepository(testPool)
	ctx := context.Background()

	user := &domain.User{
		ID: uuid.New(), Email: "notouch@example.com",
		PasswordHash: "h", DisplayName: "N", Status: domain.UserStatusActive,
	}
	usersRepo.Create(ctx, user)

	// Session expiring in 20 hours (> 12h threshold → should NOT be extended)
	_, hashHex, _ := token.Generate()
	now := time.Now()
	sessID := uuid.New()
	sessRepo.Create(ctx, &domain.Session{
		ID:                sessID,
		UserID:            user.ID,
		TokenHash:         hashHex,
		CreatedAt:         now,
		LastSeenAt:        now,
		IdleExpiresAt:     now.Add(20 * time.Hour),
		AbsoluteExpiresAt: now.Add(7 * 24 * time.Hour),
	})

	newIdle, err := sessRepo.TouchIdleIfNeeded(ctx, sessID, now.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("TouchIdleIfNeeded: %v", err)
	}
	if newIdle != nil {
		t.Fatal("expected no extension (session > 12h from expiry), got non-nil")
	}
}

// --- EmailVerificationsRepository ---

func TestEmailVerificationsRepo_CreateAndGet(t *testing.T) {
	truncate(t)
	usersRepo := repository.NewUsersRepository(testPool)
	verRepo := repository.NewEmailVerificationsRepository(testPool)
	ctx := context.Background()

	user := &domain.User{
		ID: uuid.New(), Email: "ver@example.com",
		PasswordHash: "h", DisplayName: "V", Status: domain.UserStatusPending,
	}
	usersRepo.Create(ctx, user)

	plaintext, hashHex, _ := token.Generate()
	expiresAt := time.Now().Add(24 * time.Hour)
	if err := verRepo.Create(ctx, user.ID, hashHex, expiresAt); err != nil {
		t.Fatalf("Create: %v", err)
	}

	id, userID, err := verRepo.GetUnusedValid(ctx, token.Hash(plaintext))
	if err != nil {
		t.Fatalf("GetUnusedValid: %v", err)
	}
	if userID != user.ID {
		t.Fatalf("want userID %v, got %v", user.ID, userID)
	}

	if err := verRepo.MarkUsed(ctx, id); err != nil {
		t.Fatalf("MarkUsed: %v", err)
	}

	// Token no longer valid after being used
	_, _, err = verRepo.GetUnusedValid(ctx, token.Hash(plaintext))
	if err != repository.ErrNotFound {
		t.Fatalf("want ErrNotFound after MarkUsed, got %v", err)
	}
}

func TestEmailVerificationsRepo_InvalidatePending(t *testing.T) {
	truncate(t)
	usersRepo := repository.NewUsersRepository(testPool)
	verRepo := repository.NewEmailVerificationsRepository(testPool)
	ctx := context.Background()

	user := &domain.User{
		ID: uuid.New(), Email: "inv@example.com",
		PasswordHash: "h", DisplayName: "I", Status: domain.UserStatusPending,
	}
	usersRepo.Create(ctx, user)

	_, hashHex, _ := token.Generate()
	verRepo.Create(ctx, user.ID, hashHex, time.Now().Add(time.Hour))

	if err := verRepo.InvalidatePending(ctx, user.ID); err != nil {
		t.Fatalf("InvalidatePending: %v", err)
	}

	// All pending tokens for user should now be invalid
	_, _, err := verRepo.GetUnusedValid(ctx, hashHex)
	if err != repository.ErrNotFound {
		t.Fatalf("want ErrNotFound after InvalidatePending, got %v", err)
	}
}
