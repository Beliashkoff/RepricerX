-- Этап 6: Dispatcher (отправка цен в маркетплейсы).
-- Расширяем существующие ENUM-ы новыми статусами, чтобы разделить
-- "расчёт" (Этап 5) и "отправку" (Этап 6) в жизненном цикле плана.
--
-- Семантика:
--   plan_status:
--     'pending'     — план создан, ждёт обработки
--     'processing'  — расчёт идёт (внутри recalc-job)
--     'calculated'  — НОВОЕ. Расчёт завершён, ждёт отправки в МП
--     'dispatching' — НОВОЕ. Идёт отправка цен в маркетплейс
--     'applied'     — отправка завершена успешно (был "расчёт окончен", теперь "отправка ок")
--     'partial'     — НОВОЕ. Часть цен отправлена, часть с ошибкой
--     'failed'      — терминальная ошибка
--     'cancelled'   — отменён пользователем
--
--   plan_item_status:
--     'pending'    — рассчитан, ждёт отправки
--     'applied'    — (legacy) совпадал с calculated; новые items не используют
--     'dispatched' — НОВОЕ. Цена реально отправлена в МП и принята
--     'skipped'    — пропущен (constraint hit, missing_cost, ...)
--     'failed'     — отправка упала (после retry)
--
-- ВАЖНО: ALTER TYPE ADD VALUE в Postgres 12+ можно вне транзакции.
-- golang-migrate выполняет каждый файл как одну транзакцию, но в Postgres 16
-- это работает.

ALTER TYPE plan_status ADD VALUE IF NOT EXISTS 'calculated';
ALTER TYPE plan_status ADD VALUE IF NOT EXISTS 'dispatching';
ALTER TYPE plan_status ADD VALUE IF NOT EXISTS 'partial';

ALTER TYPE plan_item_status ADD VALUE IF NOT EXISTS 'dispatched';
