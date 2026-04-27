package auth

import (
	"testing"
	"time"
)

func TestLockoutDuration(t *testing.T) {
	cases := []struct {
		count    int
		wantNil  bool
		wantMins int
	}{
		{0, true, 0},
		{4, true, 0},
		{5, false, 5},
		{9, false, 5},
		{10, false, 15},
		{19, false, 15},
		{20, false, 60},
		{100, false, 60},
	}
	for _, c := range cases {
		d := lockoutDuration(c.count)
		if c.wantNil && d != nil {
			t.Errorf("count=%d: ожидали nil, получили %v", c.count, *d)
		}
		if !c.wantNil {
			if d == nil {
				t.Errorf("count=%d: ожидали %d мин, получили nil", c.count, c.wantMins)
				continue
			}
			got := int(d.Minutes())
			if got != c.wantMins {
				t.Errorf("count=%d: ожидали %d мин, получили %d мин", c.count, c.wantMins, got)
			}
		}
	}
}

func TestLockoutUntil(t *testing.T) {
	now := time.Now()
	// До порога — nil
	if lockoutUntil(4, now) != nil {
		t.Error("count=4 должен возвращать nil")
	}
	// На пороге — moment + 5 мин
	u := lockoutUntil(5, now)
	if u == nil {
		t.Fatal("count=5 не должен возвращать nil")
	}
	diff := u.Sub(now)
	if diff < 4*time.Minute || diff > 6*time.Minute {
		t.Errorf("count=5: ожидали ~5 мин, получили %v", diff)
	}
}
