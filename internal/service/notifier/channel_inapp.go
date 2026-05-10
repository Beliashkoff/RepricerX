package notifier

import (
	"context"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/google/uuid"
)

// inAppChannel — канал «лежит в БД, отдадим через API».
// Регистрируется всегда; никаких внешних вызовов не делает: notifier.Service
// атомарно создаёт notification, и этого достаточно — UI забирает через
// GET /api/notifications.
type inAppChannel struct{}

// NewInAppChannel — конструктор для регистрации в notifier.Service.
func NewInAppChannel() Channel { return inAppChannel{} }

func (inAppChannel) Name() string { return domain.NotificationChannelInApp }

func (inAppChannel) Deliver(_ context.Context, _ *domain.Notification, _ *domain.NotificationDelivery) error {
	return nil
}

func (inAppChannel) DigestDeliver(_ context.Context, _ uuid.UUID, _ []*domain.Notification) error {
	return ErrDigestNotSupported
}
