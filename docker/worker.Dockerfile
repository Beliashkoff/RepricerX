# worker.Dockerfile — фоновый worker (планировщик, обработка задач).
# Сейчас cmd/worker/main.go — stub, контейнер сразу завершается.
# restart: unless-stopped в compose перезапускает его без последствий.
# Заработает автоматически когда появится реализация.

FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /worker ./cmd/worker

FROM alpine:3.21

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /worker /app/worker
COPY --from=builder /app/migrations /app/migrations

ENTRYPOINT ["/app/worker"]
