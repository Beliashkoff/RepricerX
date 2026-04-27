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
// Если trustProxy=true, сначала смотрим X-Forwarded-For (берём самый левый, т. е. клиентский).
// В dev/без прокси trustProxy должен быть false — иначе клиент может подделать заголовок.
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
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// Берём самый левый IP — он ближайший к клиенту
			parts := strings.SplitN(xff, ",", 2)
			if ip := net.ParseIP(strings.TrimSpace(parts[0])); ip != nil {
				return ip
			}
		}
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			if ip := net.ParseIP(strings.TrimSpace(xri)); ip != nil {
				return ip
			}
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return net.ParseIP(r.RemoteAddr)
	}
	return net.ParseIP(host)
}
