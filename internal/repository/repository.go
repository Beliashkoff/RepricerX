package repository

import (
	"context"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/google/uuid"
)

type UsersRepository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetById(ctx context.Context, id uuid.UUID) (*domain.User, error)
}
