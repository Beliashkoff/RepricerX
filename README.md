# RepricerX

Веб-сервис автоматического управления ценами на маркетплейсах **Wildberries** и **Ozon**.
Продавец подключает магазины по API-ключам, импортирует SKU, задаёт стратегии ценообразования
(следование за конкурентом, поддержание маржи, фиксированная цена), а сервис собирает
рыночные данные, рассчитывает рекомендуемые цены в рамках ограничений (min/max, max change, маржа),
отправляет обновления в маркетплейсы и ведёт журнал изменений.

## Стек

| Слой       | Технологии |
|------------|------------|
| Backend    | Go 1.24, Gin, pgx/v5 (без ORM), robfig/cron v3 |
| Frontend   | React 18, Vite, TypeScript, TanStack Query |
| Хранилища  | PostgreSQL 16, Redis 7 |
| Миграции   | golang-migrate (чистый SQL) |
| Инфра      | Docker Compose, nginx (reverse proxy в prod) |
| Аутентификация | Серверные сессии в cookie, OAuth (VK ID, Яндекс ID) с PKCE |

## Архитектура

Два бинаря, общие `internal/` пакеты, общая БД и Redis:

- `cmd/api` — HTTP-сервер.
- `cmd/worker` — фоновый обработчик задач (импорт SKU, рассылка цен, ретраи).
- `cmd/scheduler` — cron-планировщик (рыночные данные, плановый пересчёт, ротация логов).
- `cmd/bot` — Telegram-бот для notifier-канала.
- `cmd/migrate`, `cmd/credbackfill` — служебные утилиты.

Слойка (сверху вниз):

```
transport/http/      handlers, middleware (auth, CSRF, rate-limit), DTO
service/             auth, shop, product, competitor, strategy, pricing,
                     dispatcher, audit, notifier — бизнес-логика
repository/*_pg.go   pgx-реализации интерфейсов
domain/              чистые структуры
integration/{wildberries,ozon}  адаптеры Marketplace (TestAuth, ListSKUs, UpdatePrice)
integration/oauth/{vkid,yandex} OAuth-провайдеры
internal/pkg/        crypto (AES-256-GCM), password (argon2id), token, mailer,
                     ratelimit, redislimit, dblock, auditlog, oauthstate
```

**Принципы:**
- Sentinel-ошибки репозитория → ошибки сервиса → HTTP-статусы в `transport/http/errors.go`. Репозиторные ошибки не утекают в хендлеры.
- API-ключи магазинов хранятся только в виде AES-256-GCM шифротекста; ключ — из `APP_SECRET_KEY`.
- CSRF: same-origin проверка `Origin` для всех мутаций (кроме публичной auth-группы).
- Rate-limit: per-shop token bucket; HTTP 429 от маркетплейса → `integration.ErrRateLimited` → `shopsvc.ErrRateLimited` → 429 наружу.
- Длинные операции (импорт, массовая рассылка цен) — через `background_jobs` (advisory lock), не в HTTP-хендлере.
- Добавление маркетплейса = новый адаптер + ключ в `MarketplaceFactory` map в `cmd/api/main.go`.

## Локальный запуск

```bash
docker compose up --build
# или
make up      # docker compose up -d --build
make logs
make down
```

После старта:

- Frontend: `http://localhost:5173`
- API: `http://localhost:8080`
- Liveness: `GET /healthz`
- Readiness: `GET /ready` (Postgres + Redis)
- Swagger UI: `http://localhost:8080/swagger/index.html`

Vite внутри Docker проксирует `/api` и `/healthz` на `http://api:8080`. Фронтенд монтируется из `./web`,
изменения подхватываются HMR без пересборки контейнера.

## Сборка и тесты

```bash
go build -o api ./cmd/api
go build -o worker ./cmd/worker

go test -race -cover ./...                                   # unit
go test -tags=integration -race -v ./tests/integration/...   # нужен DATABASE_URL

golangci-lint run ./...

make migrate    # применяет миграции напрямую, нужен DATABASE_URL
make swag       # перегенерация docs/ из swag-аннотаций
```

## Переменные окружения

Обязательные: `DATABASE_URL`, `APP_SECRET_KEY`, `REDIS_ADDR`.
Полный список — `internal/config/config.go`. Основные группы:

- **CORS/CSRF:** `ALLOWED_ORIGINS`, `TRUST_PROXY_HEADERS`.
- **Сессии:** `SESSION_IDLE_TTL` (24h), `SESSION_ABSOLUTE_TTL` (168h).
- **Почта:** `MAILER_MODE=log|smtp`, `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM`, `VERIFICATION_URL_BASE`, `PASSWORD_RESET_URL_BASE`.
- **Worker:** `WORKER_CONCURRENCY`, `WORKER_POLL_INTERVAL`, `WORKER_LOCK_TTL`, `WORKER_JOB_TIMEOUT`, `WORKER_MAX_ATTEMPTS`.
- **OAuth:** `OAUTH_VK_CLIENT_ID/SECRET`, `OAUTH_YANDEX_CLIENT_ID/SECRET`, `OAUTH_CALLBACK_BASE_URL`, `OAUTH_FRONTEND_BASE_URL`.
- **Telegram:** `TELEGRAM_BOT_TOKEN`, `TELEGRAM_BOT_START_URL`.
- **HTTP:** `MAX_BODY_BYTES` (1 MiB по умолчанию).
- **Dev-only:** `MOCK_MARKETPLACES=true` подменяет WB/Ozon на in-memory заглушки.

В prod (`ENVIRONMENT=prod`): обязательны `MAILER_MODE=smtp`, HTTPS-`VERIFICATION_URL_BASE`/`PASSWORD_RESET_URL_BASE`, `MOCK_MARKETPLACES=false`.

## REST API

Базовый URL: `http://localhost:8080`.
Полный справочник — Swagger UI (`/swagger/index.html`) и [`docs/api_endpoints.docx`](docs/api_endpoints.docx).

### Сервисные

| Метод | Путь       | Описание                                       |
|-------|------------|------------------------------------------------|
| GET   | `/healthz` | Liveness — 200 пока процесс запущен            |
| GET   | `/ready`   | Readiness — проверяет Postgres и Redis         |

### Аутентификация `/api/auth`

| Метод | Путь                                  | Auth   | CSRF | Описание |
|-------|---------------------------------------|--------|------|----------|
| POST  | `/register`                           | —      | —    | Регистрация; отправляет письмо верификации |
| POST  | `/login`                              | —      | —    | Вход; ставит HttpOnly-cookie `rx_session` |
| POST  | `/logout`                             | cookie | да   | Выход |
| GET   | `/me`                                 | cookie | —    | Профиль текущего пользователя |
| PATCH | `/me`                                 | cookie | да   | Обновление `displayName` |
| GET   | `/verify?token=...`                   | —      | —    | Подтверждение email; редирект на фронтенд |
| POST  | `/verification/resend`                | —      | —    | Повторная отправка письма верификации |
| POST  | `/password/forgot`                    | —      | —    | Запрос ссылки сброса пароля |
| POST  | `/password/reset`                     | —      | —    | Установка нового пароля по токену |
| GET   | `/oauth/:provider/start`              | —      | —    | OAuth (vk \| yandex): редирект на форму согласия |
| GET   | `/oauth/:provider/callback`           | —      | —    | OAuth callback: создаёт сессию или редиректит на `/link-oauth` |
| POST  | `/oauth/link`                         | —      | —    | Подтверждение привязки OAuth к существующему email/password-аккаунту |

**OAuth (VK ID, Яндекс ID).** Серверный Authorization Code Flow + PKCE (S256). State и PKCE-verifier хранятся в Redis с TTL 10 минут; внешние идентичности — в `oauth_identities` (миграция `000015`). При конфликте email с существующим email/password-аккаунтом callback редиректит на `/link-oauth?token=…&email=…&provider=…` — пользователь вводит пароль, backend создаёт связь и выдаёт сессию. Без `client_id`/`secret` соответствующий провайдер недоступен (503). Оба провайдера требуют HTTPS в redirect URI; адаптер VK ID сам передаёт обязательный `device_id` из callback URL.

### Магазины `/api/shops`

| Метод  | Путь            | CSRF | Описание |
|--------|-----------------|------|----------|
| GET    | `/`             | —    | Список магазинов пользователя |
| GET    | `/:id`          | —    | Один магазин |
| POST   | `/`             | да   | Создать; credentials шифруются AES-256-GCM |
| PATCH  | `/:id`          | да   | Имя, credentials, расписание, auto-update |
| DELETE | `/:id`          | да   | Удалить магазин |
| POST   | `/:id/test`     | да   | Проверка подключения; меняет статус |
| POST   | `/:id/run-now`  | да   | Ручной триггер пересчёта |
| POST   | `/:id/products/import` | да | Запуск фоновой джобы импорта SKU |
| POST   | `/:id/products` | да   | Ручное добавление продукта |

Маркетплейсы: `wb`, `ozon`. Поле `credentials`:
- **WB:** `{"api_key": "..."}`
- **Ozon:** `{"client_id": "...", "api_key": "..."}`

Статусы магазина: `draft` → `active` / `error` (после `/test`) → `disabled`.

### Прочее

- `/api/products`, `/api/products/export`, `/api/products/:id/competitors`, `/api/imports/:id` — каталог и импорт.
- `/api/strategies`, `/api/strategies/:id/assignments` — стратегии и назначения.
- `/api/pricing/simulate`, `/api/pricing/recalculate`, `/api/price-plans`, `/api/price-plans/:id/dispatch` — движок цен и план изменений.
- `/api/audit/price-changes`, `/api/audit/price-changes.csv`, `/api/reports/summary` — журнал и отчёты.
- `/api/notifications/*` — уведомления, настройки каналов (UI, Telegram, webhooks).

Все ошибки — в формате `{"error":{"code":"...","message":"..."}}`. Мутирующие защищённые эндпоинты требуют same-origin `Origin`.

## Production

```bash
make prod-up      # docker compose --env-file .env.prod -f docker-compose.prod.yaml up -d
make prod-logs
make prod-down
```

Подробнее — [`docs/production-deploy.md`](docs/production-deploy.md).

## Конвенции

### Commit

```
<тип>(<область>): <короткое описание>
[опциональное тело]
[опциональный футер]
```

| Тип      | Когда |
|----------|-------|
| feat     | Новая функциональность |
| fix      | Исправление ошибки |
| docs     | Только документация |
| style    | Форматирование, без изменения логики |
| refactor | Рефакторинг без изменения поведения |
| test     | Тесты |
| chore    | Зависимости, конфиги, сборка |

Один коммит = одна фича. Self-review обязателен.

### Pull Request

- Один PR = одна фича.
- Draft PR до готовности.
- Self-review перед запросом ревью.
- Нейминг такой же, как у коммитов.
