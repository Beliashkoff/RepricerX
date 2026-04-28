.PHONY: up down build build-worker migrate test lint \
        prod-up prod-down prod-logs prod-ps

# --- dev ---

up:
	docker compose up -d --build

down:
	docker compose down

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

lint:
	golangci-lint run ./...

# --- production (запускать на сервере или через SSH) ---

prod-up:
	docker compose --env-file .env.prod -f docker-compose.prod.yaml up -d

prod-down:
	docker compose -f docker-compose.prod.yaml down

prod-logs:
	docker compose -f docker-compose.prod.yaml logs -f

prod-ps:
	docker compose -f docker-compose.prod.yaml ps
