// Package netutil содержит утилиты для работы с сетевыми адресами.
package netutil

import (
	"net"
	"net/http"
	"strings"
)

// IPPrefix возвращает сетевой префикс IP-адреса клиента (/24 для IPv4, /64 для IPv6).
// Используется для fingerprint-сессий: мягкое обнаружение смены сети без инвалидации.
//
// Если trustProxy=true, forwarded headers используются только когда непосредственный peer
// выглядит как доверенный reverse proxy. X-Forwarded-For разбирается справа налево:
// клиентским считается первый адрес до trusted proxy hops, а не spoofable leftmost значение.
// В dev/без прокси trustProxy должен быть false.
func IPPrefix(r *http.Request, trustProxy bool) string {
	ip := extractIP(r, trustProxy)
	if ip == nil {
		return ""
	}

	if ip.To4() != nil {
		// IPv4: маскируем до /24 (первые три октета)
		masked := ip.Mask(net.CIDRMask(24, 32))
		return masked.String() + "/24"
	}

	// IPv6: маскируем до /64
	masked := ip.Mask(net.CIDRMask(64, 128))
	return masked.String() + "/64"
}

func extractIP(r *http.Request, trustProxy bool) net.IP {
	remoteIP := remoteAddrIP(r)
	if !trustProxy || !isTrustedProxyIP(remoteIP) {
		return remoteIP
	}

	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip := clientIPFromXForwardedFor(xff); ip != nil {
			return ip
		}
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if ip := net.ParseIP(strings.TrimSpace(xri)); ip != nil {
			return ip
		}
	}

	return remoteIP
}

func remoteAddrIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return net.ParseIP(r.RemoteAddr)
	}
	return net.ParseIP(host)
}

func clientIPFromXForwardedFor(value string) net.IP {
	parts := strings.Split(value, ",")
	var rightmostValid net.IP
	for i := len(parts) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(parts[i]))
		if ip == nil {
			continue
		}
		if rightmostValid == nil {
			rightmostValid = ip
		}
		if !isTrustedProxyIP(ip) {
			return ip
		}
	}
	return rightmostValid
}

func isTrustedProxyIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}
