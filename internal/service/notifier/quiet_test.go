package notifier

import (
	"testing"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
)

func TestIsInQuietHours(t *testing.T) {
	start := 22
	end := 8
	settings := &domain.UserChannelSettings{QuietHoursStart: &start, QuietHoursEnd: &end}

	cases := []struct {
		name string
		hour int
		want bool
	}{
		{"before midnight", 23, true},
		{"after midnight", 7, true},
		{"at end", 8, false},
		{"daytime", 12, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsInQuietHours(time.Date(2026, 5, 10, tc.hour, 0, 0, 0, time.UTC), settings)
			if got != tc.want {
				t.Fatalf("IsInQuietHours hour=%d got %v want %v", tc.hour, got, tc.want)
			}
		})
	}
}

func TestIsInQuietHoursSameStartEndDisabled(t *testing.T) {
	start := 8
	end := 8
	got := IsInQuietHours(time.Date(2026, 5, 10, 8, 0, 0, 0, time.UTC),
		&domain.UserChannelSettings{QuietHoursStart: &start, QuietHoursEnd: &end})
	if got {
		t.Fatal("same start/end must disable quiet hours")
	}
}
