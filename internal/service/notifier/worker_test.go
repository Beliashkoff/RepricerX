package notifier

import (
	"testing"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/google/uuid"
)

func TestDigestPendingItemsSkipsFinalizedDeliveries(t *testing.T) {
	pendingID := uuid.New()
	sentID := uuid.New()
	skippedID := uuid.New()
	pendingNotificationID := uuid.New()

	deliveries := []*domain.NotificationDelivery{
		{ID: sentID, Status: domain.NotificationDeliveryStatusSent},
		{ID: pendingID, Status: domain.NotificationDeliveryStatusQueuedDigest},
		{ID: skippedID, Status: domain.NotificationDeliveryStatusSkipped},
	}
	notifications := []*domain.Notification{
		{ID: uuid.New(), Title: "sent"},
		{ID: pendingNotificationID, Title: "pending"},
		{ID: uuid.New(), Title: "skipped"},
	}

	ids, items := digestPendingItems(deliveries, notifications)
	if len(ids) != 1 || ids[0] != pendingID {
		t.Fatalf("pending ids = %v, want only %s", ids, pendingID)
	}
	if len(items) != 1 || items[0].ID != pendingNotificationID {
		t.Fatalf("pending notifications = %v, want only %s", items, pendingNotificationID)
	}
}
