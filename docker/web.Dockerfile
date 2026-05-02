# web.Dockerfile — multi-stage сборка React/Vite фронтенда

FROM node:22-alpine AS builder

WORKDIR /app

COPY web/package.json web/package-lock.json* ./
RUN npm ci --prefer-offline

COPY web/ .

# Build args для Clerk
ARG VITE_CLERK_PUBLISHABLE_KEY
ENV VITE_CLERK_PUBLISHABLE_KEY=$VITE_CLERK_PUBLISHABLE_KEY

RUN npm run build

# Финальный образ — только статика, копируется в shared volume
FROM alpine:3.21

WORKDIR /app

COPY --from=builder /app/dist ./dist

CMD ["sh", "-c", "cp -r /app/dist/. /dist/ && echo 'web: dist copied to volume' && tail -f /dev/null"]
