package netutil

import (
	"net/http/httptest"
	"testing"
)

func TestIPPrefix_IPv4(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.168.1.42:12345"

	got := IPPrefix(r, false)
	if got != "192.168.1.0/24" {
		t.Errorf("ожидали 192.168.1.0/24, получили %s", got)
	}
}

func TestIPPrefix_IPv6(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "[2001:db8:85a3::8a2e:370:7334]:443"

	got := IPPrefix(r, false)
	if got != "2001:db8:85a3::/64" {
		t.Errorf("ожидали 2001:db8:85a3::/64, получили %s", got)
	}
}

func TestIPPrefix_XForwardedFor_TrustProxy(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:80"
	r.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1")

	got := IPPrefix(r, true)
	if got != "203.0.113.0/24" {
		t.Errorf("ожидали 203.0.113.0/24, получили %s", got)
	}
}

func TestIPPrefix_XForwardedFor_NoTrust(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:80"
	r.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1")

	// При trustProxy=false заголовок игнорируется
	got := IPPrefix(r, false)
	if got != "10.0.0.0/24" {
		t.Errorf("ожидали 10.0.0.0/24, получили %s", got)
	}
}
