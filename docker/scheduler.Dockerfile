# scheduler.Dockerfile — robfig/cron планировщик (Этап 7).
# Запускает периодические задачи: scheduledRecalc per-shop, competitorRefresh,
# cleanupHourly (sessions + retention 30/180d), stalePlanCleanup.

FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /scheduler ./cmd/scheduler

FROM alpine:3.21

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /scheduler /app/scheduler

ENTRYPOINT ["/app/scheduler"]
