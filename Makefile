.PHONY: up down logs ps build build-worker migrate test test-integration lint swag \
        prod-up prod-down prod-logs prod-ps

# Подтягиваем переменные из .env если файл существует
-include .env
export

# --- dev ---

up:
	docker compose up -d --build

down:
	docker compose down

logs:
	docker compose logs -f

ps:
	docker compose ps

build:
	go build -o api ./cmd/api

build-worker:
	go build -o worker ./cmd/worker

# Применяет миграции напрямую (без docker)
migrate:
	@test -n "$(DATABASE_URL)" || (echo "DATABASE_URL не задан" && exit 1)
	go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest \
		-path ./migrations -database "$(DATABASE_URL)" up

test:
	go test -race -cover ./...

test-integration:
	@test -n "$(DATABASE_URL)" || (echo "DATABASE_URL не задан" && exit 1)
	go test -tags=integration -race -v ./tests/integration/...

lint:
	golangci-lint run ./...

# Регенерирует docs/ из аннотаций. Запускать после изменения хендлеров.
swag:
	swag init -g cmd/api/main.go -o docs

# --- production (запускать на сервере или через SSH) ---

prod-up:
	docker compose --env-file .env.prod -f docker-compose.prod.yaml up -d

prod-down:
	docker compose -f docker-compose.prod.yaml down

prod-logs:
	docker compose -f docker-compose.prod.yaml logs -f

prod-ps:
	docker compose -f docker-compose.prod.yaml ps
