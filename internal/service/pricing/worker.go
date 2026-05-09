package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/pkg/crypto"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/google/uuid"
)

// ExecuteRecalcJob — обработчик BackgroundJobTypePriceRecalculation.
// Вызывается из cmd/worker/main.go в switch по job.JobType.
//
// Шаги:
//  1. Парсинг payload.
//  2. UpdatePlanStatus(processing).
//  3. Загружаем продукты (пакетно): либо переданные ProductIDs, либо все товары магазина с has_strategy=true.
//  4. Для каждого: получаем стратегию, конкурентов (LatestFreshPrice через ProductCompetitors),
//     вызываем Calculate, формируем PricePlanItem.
//  5. InsertItems batch-ом + UpdateStatus(applied).
func (s *Service) ExecuteRecalcJob(ctx context.Context, job *domain.BackgroundJob) error {
	if s.plans == nil || s.products == nil || s.strategies == nil {
		return fmt.Errorf("pricing worker: missing required repositories")
	}

	var payload domain.PriceRecalculationJobPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("recalc worker: payload parse: %w", err)
	}

	if err := s.plans.UpdateStatus(ctx, payload.PlanID, domain.PlanStatusProcessing); err != nil {
		return fmt.Errorf("recalc worker: mark processing: %w", err)
	}

	// Синхронизируем current_price для магазина если включена опция и есть устаревшие товары.
	if err := s.syncShopPricesIfStale(ctx, payload.ShopID, payload.RequestedByUserID); err != nil {
		// Не падаем — продолжаем со старыми ценами; ошибку только логируем.
		slog.Default().Warn("pricing: shop price sync failed",
			"shop_id", payload.ShopID, "error", err)
	}

	products, err := s.loadProductsForRecalc(ctx, payload)
	if err != nil {
		_ = s.plans.UpdateStatus(ctx, payload.PlanID, domain.PlanStatusFailed)
		return fmt.Errorf("recalc worker: load products: %w", err)
	}

	items := make([]*domain.PricePlanItem, 0, len(products))
	for _, p := range products {
		item := s.calculateItemForProduct(ctx, payload.RequestedByUserID, p)
		if item != nil {
			items = append(items, item)
		}
	}

	if err := s.plans.InsertItems(ctx, payload.PlanID, items); err != nil {
		_ = s.plans.UpdateStatus(ctx, payload.PlanID, domain.PlanStatusFailed)
		return fmt.Errorf("recalc worker: insert items: %w", err)
	}

	// Этап 6: расчёт окончен, но цены ещё не отправлены — статус "calculated".
	if err := s.plans.UpdateStatus(ctx, payload.PlanID, domain.PlanStatusCalculated); err != nil {
		return fmt.Errorf("recalc worker: mark calculated: %w", err)
	}

	// Auto-dispatch hook (Этап 6): если у магазина включён auto_update_enabled —
	// сразу enqueue dispatch-job. Best-effort: ошибки логируем, не падаем.
	if s.dispatcher != nil && s.shops != nil {
		shop, err := s.shops.GetByID(ctx, payload.ShopID, payload.RequestedByUserID)
		if err == nil && shop.AutoUpdateEnabled {
			if _, err := s.dispatcher.EnqueueDispatch(ctx, payload.RequestedByUserID, payload.PlanID); err != nil {
				slog.Default().Warn("pricing: auto-dispatch enqueue failed",
					"plan_id", payload.PlanID, "shop_id", payload.ShopID, "error", err)
			}
		}
	}
	return nil
}

func (s *Service) loadProductsForRecalc(ctx context.Context, p domain.PriceRecalculationJobPayload) ([]*domain.Product, error) {
	if len(p.ProductIDs) > 0 {
		out := make([]*domain.Product, 0, len(p.ProductIDs))
		for _, pid := range p.ProductIDs {
			prod, err := s.products.GetByIDForUser(ctx, p.RequestedByUserID, pid)
			if err != nil {
				continue // пропускаем недоступные товары (тенант/удалены)
			}
			out = append(out, prod)
		}
		return out, nil
	}

	// Весь магазин — берём все активные товары пользователя в этом магазине.
	shopID := p.ShopID
	res, err := s.products.List(ctx, p.RequestedByUserID, repository.ProductListFilter{
		ShopID:  &shopID,
		Status:  domain.ProductStatusActive,
		Page:    1,
		PerPage: maxRecalculateBatch,
	})
	if err != nil {
		return nil, err
	}
	return res.Items, nil
}

// calculateItemForProduct — расчёт одного PricePlanItem.
// Возвращает nil только если товар без стратегии (не включается в план).
func (s *Service) calculateItemForProduct(ctx context.Context, userID uuid.UUID, p *domain.Product) *domain.PricePlanItem {
	// Получаем стратегию через assignments (1 товар = 1 стратегия).
	strategy := s.lookupStrategyForProduct(ctx, userID, p)
	if strategy == nil {
		return nil // нет стратегии — не считаем
	}
	stratID := strategy.ID

	// min_interval_minutes — пропустить если последний пересчёт был недавно.
	if minInterval, ok := minIntervalFromConstraints(strategy.Constraints); ok && minInterval > 0 && s.plans != nil {
		latest, err := s.plans.LatestItemCreatedAt(ctx, p.ID)
		if err == nil && latest != nil {
			elapsed := time.Since(*latest)
			required := time.Duration(minInterval) * time.Minute
			if elapsed < required {
				return &domain.PricePlanItem{
					ProductID:     p.ID,
					StrategyID:    &stratID,
					CurrentPrice:  p.CurrentPrice,
					TargetPrice:   0,
					FinalPrice:    p.CurrentPrice,
					ConstraintHit: domain.ConstraintMinInterval,
					Status:        domain.PlanItemStatusSkipped,
					Error: fmt.Sprintf("%s: elapsed=%s required=%s",
						domain.ReasonMinIntervalNotElapsed, elapsed.Truncate(time.Second), required),
				}
			}
		}
	}

	competitorPrices := s.competitorPricesForProduct(ctx, userID, p.ID)

	res := Calculate(CalculateInput{
		Product:          p,
		Strategy:         strategy,
		CompetitorPrices: competitorPrices,
	})

	return &domain.PricePlanItem{
		ProductID:     p.ID,
		StrategyID:    &stratID,
		CurrentPrice:  p.CurrentPrice,
		TargetPrice:   res.TargetPrice,
		FinalPrice:    res.FinalPrice,
		ConstraintHit: res.ConstraintHit,
		Status:        res.Status,
		Error:         res.Error,
	}
}

// minIntervalFromConstraints извлекает min_interval_minutes из constraints JSONB.
func minIntervalFromConstraints(raw json.RawMessage) (int, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var c struct {
		MinIntervalMinutes *int `json:"min_interval_minutes"`
	}
	if err := json.Unmarshal(raw, &c); err != nil || c.MinIntervalMinutes == nil {
		return 0, false
	}
	return *c.MinIntervalMinutes, true
}

func (s *Service) lookupStrategyForProduct(ctx context.Context, userID uuid.UUID, p *domain.Product) *domain.Strategy {
	if !p.HasStrategy || s.assignments == nil {
		return nil
	}
	all, err := s.strategies.ListByUser(ctx, userID)
	if err != nil {
		return nil
	}
	for _, st := range all {
		ids, err := s.assignments.ListProductIDsByStrategy(ctx, userID, st.ID)
		if err != nil {
			continue
		}
		if slices.Contains(ids, p.ID) {
			return st
		}
	}
	return nil
}

// syncShopPricesIfStale проверяет товары магазина и если хотя бы один имеет
// last_synced_at старше priceMaxAge, вызывает marketplace.ListSKUs и обновляет
// все товары через UpsertImported. Включается опцией WithPriceSync.
//
// Метод безопасен к ошибкам: при сбое factory/decrypt/ListSKUs возвращает error,
// но caller продолжает работу с текущими (старыми) ценами.
func (s *Service) syncShopPricesIfStale(ctx context.Context, shopID, userID uuid.UUID) error {
	if s.priceMaxAge == 0 || len(s.factories) == 0 || s.shops == nil {
		return nil // sync выключен
	}
	shop, err := s.shops.GetByID(ctx, shopID, userID)
	if err != nil {
		return fmt.Errorf("get shop: %w", err)
	}

	// Проверяем нужна ли синхронизация: ищем хотя бы один stale-товар в магазине.
	products, err := s.products.List(ctx, userID, repository.ProductListFilter{
		ShopID: &shopID, Status: domain.ProductStatusActive,
		Page: 1, PerPage: maxRecalculateBatch,
	})
	if err != nil {
		return fmt.Errorf("list products: %w", err)
	}
	cutoff := time.Now().UTC().Add(-s.priceMaxAge)
	stale := false
	for _, p := range products.Items {
		if p.LastSyncedAt == nil || p.LastSyncedAt.Before(cutoff) {
			stale = true
			break
		}
	}
	if !stale {
		return nil
	}

	factory, ok := s.factories[shop.Marketplace]
	if !ok {
		return fmt.Errorf("no factory for marketplace %q", shop.Marketplace)
	}
	credsJSON, err := crypto.Decrypt(shop.CredentialsEncrypted, s.secret)
	if err != nil {
		return fmt.Errorf("decrypt creds: %w", err)
	}
	client, err := factory(shopID.String(), credsJSON)
	if err != nil {
		return fmt.Errorf("build adapter: %w", err)
	}
	skus, err := client.ListSKUs(ctx)
	if err != nil {
		return fmt.Errorf("list skus: %w", err)
	}

	rows := make([]repository.ProductImportRow, 0, len(skus))
	for _, sku := range skus {
		rows = append(rows, repository.ProductImportRow{
			ExternalSKU:  sku.ExternalSKU,
			Name:         sku.Name,
			CurrentPrice: sku.CurrentPrice,
			Currency:     sku.Currency,
			Status:       domain.ProductStatusActive,
		})
	}
	if _, err := s.products.UpsertImported(ctx, shopID, rows); err != nil {
		return fmt.Errorf("upsert: %w", err)
	}
	return nil
}

func (s *Service) competitorPricesForProduct(ctx context.Context, userID, productID uuid.UUID) []float64 {
	if s.competitors == nil {
		return nil
	}
	latest, err := s.competitors.LatestFreshPrice(ctx, userID, productID, s.competitorMaxAge)
	if err != nil || latest == nil || *latest <= 0 {
		return nil
	}
	return []float64{*latest}
}
