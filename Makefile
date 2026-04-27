.PHONY: up down build migrate test lint

up:
	docker compose up -d --build

down:
	docker compose down

build:
	go build -o api ./cmd/api

# Применяет миграции напрямую (без docker)
migrate:
	@test -n "$(DATABASE_URL)" || (echo "DATABASE_URL не задан" && exit 1)
	go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest \
		-path ./migrations -database "$(DATABASE_URL)" up

test:
	go test -race -cover ./...

lint:
	golangci-lint run ./...
