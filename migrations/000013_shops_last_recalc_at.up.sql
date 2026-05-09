-- Этап 7: scheduler. Поле для CAS-защиты от двойного запуска scheduledRecalc
-- между worker/scheduler-replicas.
--
-- Семантика last_recalc_at:
--   NULL          — магазин ещё ни разу не пересчитывался по расписанию.
--   <timestamp>   — момент когда scheduler в последний раз enqueue-ил
--                   recalc-job для этого магазина. Используется как
--                   baseline для cron.Next() и как expectedPrev в CAS-update.
--
-- НЕ путать с last_synced_at в products — это про последнюю синхронизацию
-- цен через ListSKUs (Этап 5).
ALTER TABLE shops ADD COLUMN IF NOT EXISTS last_recalc_at TIMESTAMP NULL;

CREATE INDEX IF NOT EXISTS idx_shops_last_recalc_at ON shops(last_recalc_at);
