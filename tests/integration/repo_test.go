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

	lockout := time.Now().UTC().Add(15 * time.Minute)
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
	now := time.Now().UTC()
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
	// Use UTC explicitly: TIMESTAMP column has no timezone info,
	// so comparing with Postgres now() (always UTC) requires UTC input.
	past := time.Now().UTC().UTC().Add(-time.Hour)
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
	now := time.Now().UTC()
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
	now := time.Now().UTC()
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
	now := time.Now().UTC()
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
	expiresAt := time.Now().UTC().Add(24 * time.Hour)
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
	verRepo.Create(ctx, user.ID, hashHex, time.Now().UTC().Add(time.Hour))

	if err := verRepo.InvalidatePending(ctx, user.ID); err != nil {
		t.Fatalf("InvalidatePending: %v", err)
	}

	// All pending tokens for user should now be invalid
	_, _, err := verRepo.GetUnusedValid(ctx, hashHex)
	if err != repository.ErrNotFound {
		t.Fatalf("want ErrNotFound after InvalidatePending, got %v", err)
	}
}

// --- PasswordResetTokensRepository ---

func TestPasswordResetTokensRepo_ConsumeSingleUseAndRevokesSessions(t *testing.T) {
	truncate(t)
	usersRepo := repository.NewUsersRepository(testPool)
	sessRepo := repository.NewSessionsRepository(testPool)
	resetRepo := repository.NewPasswordResetTokensRepository(testPool)
	ctx := context.Background()

	user := &domain.User{
		ID: uuid.New(), Email: "resetrepo@example.com",
		PasswordHash: "old-hash", DisplayName: "R", Status: domain.UserStatusActive,
	}
	if err := usersRepo.Create(ctx, user); err != nil {
		t.Fatalf("Create user: %v", err)
	}
	lockout := time.Now().UTC().Add(time.Hour)
	if err := usersRepo.RegisterFailedLogin(ctx, user.ID, 5, &lockout); err != nil {
		t.Fatalf("RegisterFailedLogin: %v", err)
	}

	_, sessionHash, _ := token.Generate()
	now := time.Now().UTC()
	if err := sessRepo.Create(ctx, &domain.Session{
		ID:                uuid.New(),
		UserID:            user.ID,
		TokenHash:         sessionHash,
		CreatedAt:         now,
		LastSeenAt:        now,
		IdleExpiresAt:     now.Add(time.Hour),
		AbsoluteExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("Create session: %v", err)
	}

	plaintext, resetHash, _ := token.Generate()
	if err := resetRepo.Issue(ctx, user.ID, resetHash, now.Add(time.Hour)); err != nil {
		t.Fatalf("Issue: %v", err)
	}

	userID, revoked, err := resetRepo.Consume(ctx, token.Hash(plaintext), "new-hash")
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if userID != user.ID {
		t.Fatalf("want userID %v, got %v", user.ID, userID)
	}
	if revoked != 1 {
		t.Fatalf("want 1 revoked session, got %d", revoked)
	}

	got, err := usersRepo.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.PasswordHash != "new-hash" {
		t.Fatalf("want new password hash, got %q", got.PasswordHash)
	}
	if got.FailedLoginCount != 0 || got.LockoutUntil != nil {
		t.Fatalf("failed login state was not reset: count=%d lockout=%v", got.FailedLoginCount, got.LockoutUntil)
	}
	if _, err = sessRepo.GetByTokenHash(ctx, sessionHash); err != repository.ErrNotFound {
		t.Fatalf("want session revoked, got %v", err)
	}

	_, _, err = resetRepo.Consume(ctx, token.Hash(plaintext), "another-hash")
	if err != repository.ErrNotFound {
		t.Fatalf("want ErrNotFound on token reuse, got %v", err)
	}
}

func TestPasswordResetTokensRepo_IssueInvalidatesOlderPending(t *testing.T) {
	truncate(t)
	usersRepo := repository.NewUsersRepository(testPool)
	resetRepo := repository.NewPasswordResetTokensRepository(testPool)
	ctx := context.Background()

	user := &domain.User{
		ID: uuid.New(), Email: "resetissue@example.com",
		PasswordHash: "h", DisplayName: "R", Status: domain.UserStatusActive,
	}
	if err := usersRepo.Create(ctx, user); err != nil {
		t.Fatalf("Create user: %v", err)
	}

	oldPlain, oldHash, _ := token.Generate()
	newPlain, newHash, _ := token.Generate()
	if err := resetRepo.Issue(ctx, user.ID, oldHash, time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("Issue old: %v", err)
	}
	if err := resetRepo.Issue(ctx, user.ID, newHash, time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("Issue new: %v", err)
	}

	_, _, err := resetRepo.Consume(ctx, token.Hash(oldPlain), "old-consume")
	if err != repository.ErrNotFound {
		t.Fatalf("want old token invalidated, got %v", err)
	}
	if _, _, err = resetRepo.Consume(ctx, token.Hash(newPlain), "new-consume"); err != nil {
		t.Fatalf("new token should remain valid: %v", err)
	}
}

// TestEmailVerificationsRepo_ConsumeAndActivate проверяет атомарность и защитные условия.
func TestEmailVerificationsRepo_ConsumeAndActivate(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	usersRepo := repository.NewUsersRepository(testPool)
	verRepo := repository.NewEmailVerificationsRepository(testPool)

	// pending пользователь + валидный токен → активируется, токен становится использованным.
	pendingUser := &domain.User{
		ID: uuid.New(), Email: "consume-activate@example.com",
		PasswordHash: "h", DisplayName: "CA", Status: domain.UserStatusPending,
	}
	if err := usersRepo.Create(ctx, pendingUser); err != nil {
		t.Fatalf("Create pending user: %v", err)
	}
	_, hashHex, _ := token.Generate()
	if err := verRepo.Create(ctx, pendingUser.ID, hashHex, time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("Create token: %v", err)
	}

	userID, err := verRepo.ConsumeAndActivate(ctx, hashHex)
	if err != nil {
		t.Fatalf("ConsumeAndActivate (pending): %v", err)
	}
	if userID != pendingUser.ID {
		t.Fatalf("want userID %v, got %v", pendingUser.ID, userID)
	}
	got, _ := usersRepo.GetByID(ctx, pendingUser.ID)
	if got.Status != domain.UserStatusActive {
		t.Fatalf("want status active after consume, got %q", got.Status)
	}

	// повторный вызов с тем же токеном → ErrNotFound (токен уже использован).
	if _, err = verRepo.ConsumeAndActivate(ctx, hashHex); err != repository.ErrNotFound {
		t.Fatalf("want ErrNotFound on token reuse, got %v", err)
	}

	// заблокированный пользователь + валидный токен → ErrNotFound, статус не меняется.
	blockedUser := &domain.User{
		ID: uuid.New(), Email: "blocked-consume@example.com",
		PasswordHash: "h", DisplayName: "BC", Status: domain.UserStatusBlocked,
	}
	if err := usersRepo.Create(ctx, blockedUser); err != nil {
		t.Fatalf("Create blocked user: %v", err)
	}
	_, blockedHash, _ := token.Generate()
	if err := verRepo.Create(ctx, blockedUser.ID, blockedHash, time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("Create blocked token: %v", err)
	}
	if _, err = verRepo.ConsumeAndActivate(ctx, blockedHash); err != repository.ErrNotFound {
		t.Fatalf("want ErrNotFound for blocked user, got %v", err)
	}
	gotBlocked, _ := usersRepo.GetByID(ctx, blockedUser.ID)
	if gotBlocked.Status != domain.UserStatusBlocked {
		t.Fatalf("blocked user status must not change, got %q", gotBlocked.Status)
	}
}

func TestPasswordResetTokensRepo_RejectsExpiredAndBlockedUser(t *testing.T) {
	truncate(t)
	usersRepo := repository.NewUsersRepository(testPool)
	resetRepo := repository.NewPasswordResetTokensRepository(testPool)
	ctx := context.Background()

	expiredUser := &domain.User{
		ID: uuid.New(), Email: "expired-reset@example.com",
		PasswordHash: "h", DisplayName: "R", Status: domain.UserStatusActive,
	}
	if err := usersRepo.Create(ctx, expiredUser); err != nil {
		t.Fatalf("Create expired user: %v", err)
	}
	expiredPlain, expiredHash, _ := token.Generate()
	if err := resetRepo.Issue(ctx, expiredUser.ID, expiredHash, time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatalf("Issue expired: %v", err)
	}
	if _, _, err := resetRepo.Consume(ctx, token.Hash(expiredPlain), "new-hash"); err != repository.ErrNotFound {
		t.Fatalf("want ErrNotFound for expired token, got %v", err)
	}

	blockedUser := &domain.User{
		ID: uuid.New(), Email: "blocked-reset@example.com",
		PasswordHash: "h", DisplayName: "R", Status: domain.UserStatusActive,
	}
	if err := usersRepo.Create(ctx, blockedUser); err != nil {
		t.Fatalf("Create blocked user: %v", err)
	}
	blockedPlain, blockedHash, _ := token.Generate()
	if err := resetRepo.Issue(ctx, blockedUser.ID, blockedHash, time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("Issue blocked: %v", err)
	}
	if err := usersRepo.UpdateStatus(ctx, blockedUser.ID, domain.UserStatusBlocked); err != nil {
		t.Fatalf("UpdateStatus blocked: %v", err)
	}
	if _, _, err := resetRepo.Consume(ctx, token.Hash(blockedPlain), "new-hash"); err != repository.ErrNotFound {
		t.Fatalf("want ErrNotFound for blocked user, got %v", err)
	}
}
