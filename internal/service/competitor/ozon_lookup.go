package competitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
)

type HTTPBasedOzonLookup struct {
	http *http.Client
}

func NewHTTPBasedOzonLookup() *HTTPBasedOzonLookup {
	return &HTTPBasedOzonLookup{http: &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: validateOzonRedirect,
	}}
}

func (l *HTTPBasedOzonLookup) Lookup(ctx context.Context, target OzonTarget) (LookupResult, error) {
	if target.URL == "" {
		return LookupResult{}, ErrInvalidTarget
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.URL, nil)
	if err != nil {
		return LookupResult{}, ErrInvalidTarget
	}
	req.Header.Set("User-Agent", "RepricerX/1.0 competitor-price-check")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := l.http.Do(req)
	if err != nil {
		return LookupResult{}, fmt.Errorf("%w: request", ErrRefreshFailed)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return LookupResult{}, fmt.Errorf("%w: redirect", ErrRefreshFailed)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return LookupResult{}, fmt.Errorf("%w: rate_limited", ErrRefreshFailed)
	}
	if resp.StatusCode == http.StatusNotFound {
		return LookupResult{Availability: domain.CompetitorAvailabilityNotFound, Source: "public_ozon"}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return LookupResult{}, fmt.Errorf("%w: status", ErrRefreshFailed)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return LookupResult{}, fmt.Errorf("%w: read", ErrRefreshFailed)
	}
	return parseOzonPricePage(body)
}

var (
	jsonLDPattern = regexp.MustCompile(`(?is)<script[^>]+type=["']application/ld\+json["'][^>]*>(.*?)</script>`)
	pricePattern  = regexp.MustCompile(`(?i)"price"\s*:\s*"?([0-9]+(?:[.,][0-9]+)?)"?`)
)

func parseOzonPricePage(body []byte) (LookupResult, error) {
	text := string(body)
	for _, match := range jsonLDPattern.FindAllStringSubmatch(text, -1) {
		price, availability := parseJSONLDPrice(match[1])
		if validPrice(price) {
			return LookupResult{Price: price, Availability: availability, Source: "public_ozon"}, nil
		}
	}
	if match := pricePattern.FindStringSubmatch(text); len(match) == 2 {
		price, err := parsePrice(match[1])
		if err == nil && validPrice(&price) {
			return LookupResult{Price: &price, Availability: domain.CompetitorAvailabilityUnknown, Source: "public_ozon"}, nil
		}
	}
	if strings.Contains(text, "OutOfStock") {
		return LookupResult{Availability: domain.CompetitorAvailabilityOutOfStock, Source: "public_ozon"}, nil
	}
	return LookupResult{}, fmt.Errorf("%w: parse", ErrRefreshFailed)
}

func parseJSONLDPrice(raw string) (*float64, string) {
	var payload any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &payload); err != nil {
		return nil, domain.CompetitorAvailabilityUnknown
	}
	return findPrice(payload), findAvailability(payload)
}

func findPrice(value any) *float64 {
	switch v := value.(type) {
	case map[string]any:
		if price, ok := priceFromAny(v["price"]); ok {
			return &price
		}
		if offer, ok := v["offers"]; ok {
			return findPrice(offer)
		}
		for _, nested := range v {
			if price := findPrice(nested); price != nil {
				return price
			}
		}
	case []any:
		for _, item := range v {
			if price := findPrice(item); price != nil {
				return price
			}
		}
	}
	return nil
}

func priceFromAny(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case string:
		price, err := parsePrice(v)
		return price, err == nil
	default:
		return 0, false
	}
}

func parsePrice(raw string) (float64, error) {
	clean := strings.ReplaceAll(strings.TrimSpace(raw), ",", ".")
	if clean == "" {
		return 0, errors.New("empty price")
	}
	return strconv.ParseFloat(clean, 64)
}

func findAvailability(value any) string {
	switch v := value.(type) {
	case map[string]any:
		if raw, ok := v["availability"].(string); ok {
			if strings.Contains(raw, "InStock") {
				return domain.CompetitorAvailabilityAvailable
			}
			if strings.Contains(raw, "OutOfStock") {
				return domain.CompetitorAvailabilityOutOfStock
			}
		}
		for _, nested := range v {
			if availability := findAvailability(nested); availability != domain.CompetitorAvailabilityUnknown {
				return availability
			}
		}
	case []any:
		for _, item := range v {
			if availability := findAvailability(item); availability != domain.CompetitorAvailabilityUnknown {
				return availability
			}
		}
	}
	return domain.CompetitorAvailabilityUnknown
}

func validateOzonRedirect(req *http.Request, _ []*http.Request) error {
	if req.URL.Scheme != "https" {
		return http.ErrUseLastResponse
	}
	host := strings.ToLower(req.URL.Hostname())
	if host != "ozon.ru" && host != "www.ozon.ru" {
		return http.ErrUseLastResponse
	}
	return nil
}
