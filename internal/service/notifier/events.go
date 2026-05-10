package notifier

import (
	"fmt"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/google/uuid"
)

// DispatchCounts — счётчики исходов в плане.
type DispatchCounts struct {
	Dispatched int
	Failed     int
	Skipped    int
	Pending    int
}

// NewDispatchCompletedEvent — итог отправки цен в МП.
func NewDispatchCompletedEvent(planID, shopID uuid.UUID, planStatus string, counts DispatchCounts) Event {
	severity := domain.NotificationSeverityInfo
	switch planStatus {
	case domain.PlanStatusFailed:
		severity = domain.NotificationSeverityError
	case domain.PlanStatusPartial:
		severity = domain.NotificationSeverityWarning
	}
	title := fmt.Sprintf("Отправка цен завершена: %d из %d", counts.Dispatched, counts.Dispatched+counts.Failed+counts.Skipped+counts.Pending)
	body := fmt.Sprintf("Применено: %d. Ошибок: %d. Пропущено: %d.",
		counts.Dispatched, counts.Failed, counts.Skipped)
	pid := planID
	sid := shopID
	return Event{
		Type:     domain.NotificationEventDispatchCompleted,
		Severity: severity,
		Title:    title,
		Body:     body,
		Data: map[string]any{
			"plan_id":     planID,
			"shop_id":     shopID,
			"plan_status": planStatus,
			"counts":      counts,
		},
		PlanID: &pid,
		ShopID: &sid,
	}
}

// RecalcCounts — счётчики результата пересчёта.
type RecalcCounts struct {
	Total      int
	Calculated int
	Skipped    int
	Errors     int
}

func NewRecalcCompletedEvent(planID, shopID uuid.UUID, counts RecalcCounts) Event {
	severity := domain.NotificationSeverityInfo
	if counts.Errors > 0 {
		severity = domain.NotificationSeverityWarning
	}
	title := fmt.Sprintf("Расчёт цен завершён: %d товаров", counts.Total)
	body := fmt.Sprintf("Рассчитано: %d. Пропущено: %d. Ошибок: %d.",
		counts.Calculated, counts.Skipped, counts.Errors)
	pid := planID
	sid := shopID
	return Event{
		Type:     domain.NotificationEventRecalcCompleted,
		Severity: severity,
		Title:    title,
		Body:     body,
		Data: map[string]any{
			"plan_id": planID,
			"shop_id": shopID,
			"counts":  counts,
		},
		PlanID: &pid,
		ShopID: &sid,
	}
}

// ImportCounts — результат импорта SKU.
type ImportCounts struct {
	Total   int
	Added   int
	Updated int
	Skipped int
	Failed  int
}

func NewImportCompletedEvent(importID, shopID uuid.UUID, counts ImportCounts) Event {
	severity := domain.NotificationSeverityInfo
	if counts.Failed > 0 {
		severity = domain.NotificationSeverityWarning
	}
	title := fmt.Sprintf("Импорт SKU завершён: %d товаров", counts.Total)
	body := fmt.Sprintf("Добавлено: %d. Обновлено: %d. Пропущено: %d. Ошибок: %d.",
		counts.Added, counts.Updated, counts.Skipped, counts.Failed)
	sid := shopID
	return Event{
		Type:     domain.NotificationEventImportCompleted,
		Severity: severity,
		Title:    title,
		Body:     body,
		Data: map[string]any{
			"import_id": importID,
			"shop_id":   shopID,
			"counts":    counts,
		},
		ShopID: &sid,
	}
}

// NewIntegrationErrorEvent — для 401/429/5xx из marketplace API.
func NewIntegrationErrorEvent(shopID uuid.UUID, operation string, httpStatus int, errText string) Event {
	severity := domain.NotificationSeverityError
	if httpStatus == 429 {
		severity = domain.NotificationSeverityWarning
	}
	title := "Ошибка интеграции с маркетплейсом"
	switch httpStatus {
	case 401, 403:
		title = "Маркетплейс отклонил ключ доступа"
	case 429:
		title = "Превышен лимит запросов к маркетплейсу"
	}
	body := fmt.Sprintf("Операция %q вернула статус %d. %s", operation, httpStatus, errText)
	sid := shopID
	return Event{
		Type:     domain.NotificationEventIntegrationError,
		Severity: severity,
		Title:    title,
		Body:     body,
		Data: map[string]any{
			"shop_id":     shopID,
			"operation":   operation,
			"http_status": httpStatus,
			"error_text":  errText,
		},
		ShopID:       &sid,
		DedupeWindow: 15 * time.Minute,
	}
}

func NewScheduledJobFailedEvent(jobName, errText string) Event {
	return Event{
		Type:     domain.NotificationEventScheduledJobFailed,
		Severity: domain.NotificationSeverityError,
		Title:    fmt.Sprintf("Cron-задача %q завершилась с ошибкой", jobName),
		Body:     errText,
		Data: map[string]any{
			"job_name":   jobName,
			"error_text": errText,
		},
		DedupeWindow: 15 * time.Minute,
	}
}

// ConstraintHits — счётчик сработавших ограничений в плане.
type ConstraintHits struct {
	MinPrice     int
	MaxPrice     int
	MaxChangePct int
	Other        int
}

func NewConstraintHitEvent(planID, shopID uuid.UUID, hits ConstraintHits) Event {
	pid := planID
	sid := shopID
	return Event{
		Type:     domain.NotificationEventConstraintHit,
		Severity: domain.NotificationSeverityWarning,
		Title:    "Стратегия упёрлась в ограничения",
		Body: fmt.Sprintf("Min: %d. Max: %d. MaxChange%%: %d. Другое: %d.",
			hits.MinPrice, hits.MaxPrice, hits.MaxChangePct, hits.Other),
		Data: map[string]any{
			"plan_id": planID,
			"shop_id": shopID,
			"hits":    hits,
		},
		PlanID: &pid,
		ShopID: &sid,
	}
}

func NewBusinessWarningNoCostEvent(productID uuid.UUID, externalSKU, productName string) Event {
	return Event{
		Type:     domain.NotificationEventBusinessWarningNoCost,
		Severity: domain.NotificationSeverityWarning,
		Title:    "Не указана себестоимость",
		Body:    fmt.Sprintf("Товар %s (%s) под стратегией маржи без cost_price.", productName, externalSKU),
		Data: map[string]any{
			"product_id":   productID,
			"external_sku": externalSKU,
			"product_name": productName,
		},
		DedupeWindow: 24 * time.Hour,
	}
}

func NewBusinessWarningNoCompetitorsEvent(productID uuid.UUID, externalSKU, productName string) Event {
	return Event{
		Type:     domain.NotificationEventBusinessWarningNoCompetitors,
		Severity: domain.NotificationSeverityWarning,
		Title:    "Нет данных о конкурентах",
		Body:    fmt.Sprintf("По товару %s (%s) >24ч нет цены конкурентов; стратегия может уйти в fallback.", productName, externalSKU),
		Data: map[string]any{
			"product_id":   productID,
			"external_sku": externalSKU,
			"product_name": productName,
		},
		DedupeWindow: 24 * time.Hour,
	}
}

func NewBusinessWarningPriceDriftEvent(productID uuid.UUID, externalSKU string, expected, actual float64) Event {
	return Event{
		Type:     domain.NotificationEventBusinessWarningPriceDrift,
		Severity: domain.NotificationSeverityWarning,
		Title:    "Цена в маркетплейсе разошлась с расчётной",
		Body:    fmt.Sprintf("SKU %s: расчётная %.2f, текущая %.2f.", externalSKU, expected, actual),
		Data: map[string]any{
			"product_id":   productID,
			"external_sku": externalSKU,
			"expected":     expected,
			"actual":       actual,
		},
		DedupeWindow: 24 * time.Hour,
	}
}

func NewCompetitorPriceDroppedEvent(productID uuid.UUID, externalSKU string, oldPrice, newPrice float64) Event {
	return Event{
		Type:     domain.NotificationEventCompetitorPriceDropped,
		Severity: domain.NotificationSeverityInfo,
		Title:    "Конкурент снизил цену",
		Body:    fmt.Sprintf("SKU %s: было %.2f → стало %.2f.", externalSKU, oldPrice, newPrice),
		Data: map[string]any{
			"product_id":   productID,
			"external_sku": externalSKU,
			"old_price":    oldPrice,
			"new_price":    newPrice,
		},
		DedupeWindow: 6 * time.Hour,
	}
}

func NewCompetitorAppearedEvent(productID uuid.UUID, externalSKU, competitorURL string, price float64) Event {
	return Event{
		Type:     domain.NotificationEventCompetitorAppeared,
		Severity: domain.NotificationSeverityInfo,
		Title:    "Появился новый конкурент",
		Body:    fmt.Sprintf("SKU %s: %s по цене %.2f.", externalSKU, competitorURL, price),
		Data: map[string]any{
			"product_id":     productID,
			"external_sku":   externalSKU,
			"competitor_url": competitorURL,
			"price":          price,
		},
		DedupeWindow: 24 * time.Hour,
	}
}

func NewMedianShiftedEvent(productID uuid.UUID, externalSKU string, oldMedian, newMedian float64) Event {
	return Event{
		Type:     domain.NotificationEventMedianShifted,
		Severity: domain.NotificationSeverityInfo,
		Title:    "Медиана конкурентов сильно сдвинулась",
		Body:    fmt.Sprintf("SKU %s: было %.2f → стало %.2f.", externalSKU, oldMedian, newMedian),
		Data: map[string]any{
			"product_id":   productID,
			"external_sku": externalSKU,
			"old_median":   oldMedian,
			"new_median":   newMedian,
		},
		DedupeWindow: 24 * time.Hour,
	}
}
