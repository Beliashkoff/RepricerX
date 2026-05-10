package notifier

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestWebhookSignature(t *testing.T) {
	body := []byte(`{"event_type":"webhook_test"}`)
	got := webhookSignature("secret", body)

	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if got != want {
		t.Fatalf("signature got %q want %q", got, want)
	}
}
