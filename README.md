# RepricerX
Курсовая работа 2 курса НИУ "ВШЭ" ФКН программной инженерии, студента Зуева Акима 
# Описание 
Проект посвящен умному изменению цены на маркетплейсах Wildberries и Ozon
С помощью данного сервиса продавец на данной площадке может выбрать разные стратегии для своего товара: 
- Следование за конкурентом
- Фиксирование константы

# Актуальный план
MVP для сдачи должен быть готов до 10.05.2026. Подробный технический план и текущий статус задач ведутся в `CLAUDE.md`.

# Локальный запуск
```bash
docker compose up --build
```

После старта доступны:
- фронтенд: `http://localhost:5173`
- API: `http://localhost:8080`
- `GET http://localhost:8080/healthz`
- `GET http://localhost:8080/ready`

Для локальной разработки `docker-compose.yaml` поднимает Postgres, Redis, API, worker и Vite-фронтенд.
Фронтенд монтируется из `./web`, поэтому изменения в React-коде применяются через Vite HMR без пересборки контейнера.
Внутри Docker Vite проксирует `/api` и `/healthz` в `http://api:8080`; при запуске фронтенда напрямую на хосте используется `http://localhost:8080`.

Полезные команды:
```bash
make up      # docker compose up -d --build
make logs    # логи всех dev-сервисов
make ps      # статус контейнеров
make down    # остановить dev-сервисы
```

# REST API

Базовый URL: `http://localhost:8080`  
Полное описание с полями запросов/ответов и кодами ошибок — [`docs/api_endpoints.docx`](docs/api_endpoints.docx).

## Сервисные

| Метод | Путь       | Авторизация | Описание                                       |
|-------|------------|-------------|------------------------------------------------|
| GET   | `/healthz` | —           | Liveness probe — 200 пока процесс запущен      |
| GET   | `/ready`   | —           | Readiness probe — проверяет Postgres и Redis   |

## Аутентификация `/api/auth`

| Метод | Путь                          | Авторизация | CSRF | Описание                                         |
|-------|-------------------------------|-------------|------|--------------------------------------------------|
| POST  | `/api/auth/register`          | —           | —    | Регистрация; отправляет письмо верификации        |
| POST  | `/api/auth/login`             | —           | —    | Вход; устанавливает HttpOnly-cookie `session_token` |
| POST  | `/api/auth/logout`            | cookie      | да   | Выход; инвалидирует сессию и удаляет cookie      |
| GET   | `/api/auth/me`                | cookie      | —    | Профиль текущего пользователя                    |
| PATCH | `/api/auth/me`                | cookie      | да   | Обновление `displayName`                         |
| GET   | `/api/auth/verify?token=...`  | —           | —    | Подтверждение email; редирект на фронтенд        |
| POST  | `/api/auth/verification/resend` | —         | —    | Повторная отправка письма верификации            |
| POST  | `/api/auth/password/forgot`  | —           | —    | Запрос ссылки сброса пароля; всегда возвращает общий ответ |
| POST  | `/api/auth/password/reset`   | —           | —    | Установка нового пароля по одноразовому токену   |
| GET   | `/api/auth/oauth/:provider/start`    | — | — | Начало OAuth-логина: редирект на форму согласия VK ID / Яндекс ID |
| GET   | `/api/auth/oauth/:provider/callback` | — | — | Callback провайдера; создаёт сессию или редиректит на `/link-oauth` |
| POST  | `/api/auth/oauth/link`               | — | — | Подтверждение привязки OAuth-аккаунта паролем               |

### OAuth: VK ID и Яндекс ID

В RepricerX поддерживаются вход и регистрация через **VK ID** и **Яндекс ID**. Поток — серверный Authorization Code Flow с PKCE (S256), state и PKCE-verifier хранятся в Redis с TTL 10 минут. БД-таблица `oauth_identities` связывает внешний `(provider, external_id)` с локальным пользователем (миграция `000015`).

**1. Регистрация приложений у провайдеров.** Без `client_id`/`client_secret` хендлер вернёт 503 — провайдеры опциональны.

- **VK ID** — https://id.vk.com/about/business/go: создайте «Веб-приложение» и в «Доверенные redirect URI» укажите `<OAUTH_CALLBACK_BASE_URL>/api/auth/oauth/vk/callback` (в dev: `http://localhost:8080/api/auth/oauth/vk/callback`). Запросите доступ к email. Скопируйте идентификатор приложения и защищённый ключ.
- **Яндекс ID** — https://oauth.yandex.ru/client/new: создайте «Веб-сервисы», тот же `redirect_uri`, доступы — «Доступ к email» (`login:email`) и «Доступ к логину, имени и фамилии, полу» (`login:info`).

**2. Переменные окружения.** Список см. в `.env.example`:

```
OAUTH_VK_CLIENT_ID, OAUTH_VK_CLIENT_SECRET
OAUTH_YANDEX_CLIENT_ID, OAUTH_YANDEX_CLIENT_SECRET
OAUTH_CALLBACK_BASE_URL=http://localhost:8080
OAUTH_FRONTEND_BASE_URL=http://localhost:5173
```

**3. Конфликт email.** Если провайдер возвращает email, который уже занят email/password-аккаунтом, callback вместо логина перенаправляет на `/link-oauth?token=…&email=…&provider=…`. Пользователь вводит пароль; backend проверяет его и создаёт связь `oauth_identities`, после чего выдаёт сессию.

> Все ошибки возвращаются в формате `{"error":{"code":"...","message":"..."}}`.  
> Мутирующие защищённые эндпоинты проверяют заголовок `Origin` (same-origin CSRF).

Для писем подтверждения и сброса пароля используется `MAILER_MODE=log` в dev или `MAILER_MODE=smtp` в prod. В dev-режиме письмо не отправляется наружу, а пишется в логи API, которые можно посмотреть через `make logs` или `docker compose logs api`. На сервере запуск идёт через `docker-compose.prod.yaml`; для реальной SMTP-отправки в `.env.prod` нужны `MAILER_MODE=smtp`, `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM`. Ссылка сброса пароля настраивается через `PASSWORD_RESET_URL_BASE`, например `https://example.com/reset-password`; токен добавляется в fragment (`#token=...`) и в prod не должен попадать в серверные логи.

## Магазины `/api/shops`

| Метод  | Путь                  | Авторизация | CSRF | Описание                                              |
|--------|-----------------------|-------------|------|-------------------------------------------------------|
| GET    | `/api/shops`          | cookie      | —    | Список магазинов текущего пользователя                |
| GET    | `/api/shops/:id`      | cookie      | —    | Один магазин (только свой)                            |
| POST   | `/api/shops`          | cookie      | да   | Создать магазин; credentials шифруются AES-256-GCM    |
| PATCH  | `/api/shops/:id`      | cookie      | да   | Обновить имя, credentials, расписание, auto-update    |
| DELETE | `/api/shops/:id`      | cookie      | да   | Удалить магазин                                       |
| POST   | `/api/shops/:id/test` | cookie      | да   | Проверить подключение к маркетплейсу; меняет статус   |

Поддерживаемые маркетплейсы: `wb` (Wildberries), `ozon` (Ozon).  
Поле `credentials` в теле запроса зависит от маркетплейса:
- **WB:** `{"api_key": "..."}`
- **Ozon:** `{"client_id": "...", "api_key": "..."}`

Статусы магазина: `draft` → `active` / `error` (после `/test`) → `disabled`.

# Для разработчиков
## Pull Request
### Структура коммита
<тип>(<область>): <короткое описание>
### Нейминг как в коммитах
### Доп. инфо:
- Один PR = Одна фича.
- Draft PR.
- Self-review. 
## Commit
### Структура коммита
При коммите изменений нужно руководствоваться данной структурой коммита: 
<тип>(<область>): <короткое описание>
<подробное описание (тело) - опционально> 
<метаданные (футер) - опционально>

### Нейминг в коммитах
| Тип      | Значение                                                     | Пример                                       |
| -------- | ------------------------------------------------------------ | -------------------------------------------- |
| feat     | Новая функциональность (feature)                             | feat: add dark mode support                  |
| fix      | Исправление ошибки (bug fix)                                 | fix: resolve crash on login screen           |
| docs     | Изменения в документации                                     | docs: update API references                  |
| style    | Правки стиля (пробелы, форматирование, точки с запятой)      | style: format code with Prettier             |
| refactor | Правка кода без исправления багов или добавления фич         | refactor: simplify user authentication logic |
| test     | Добавление или исправление тестов                            | test: add unit tests for User service        |
| chore    | Служебные задачи (обновление зависимостей, настройка сборки) | chore: update dependency versions            |

### Доп. инфо: 
- Один commit = Одна фича.
- Self-review 
