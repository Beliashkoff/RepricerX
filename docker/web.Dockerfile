# web.Dockerfile — сборка React/Vite фронтенда.
# Пока web/ директории нет — создаёт placeholder index.html.
#
# Когда появится web/:
#   1. Раскомментировать блок "# Production build".
#   2. Удалить блок "# Placeholder".

FROM node:22-alpine AS builder

WORKDIR /app

# Production build (раскомментировать когда появится web/)
# COPY web/package.json web/package-lock.json ./
# RUN npm ci --prefer-offline
# COPY web/ .
# RUN npm run build

# Placeholder — минимальный index.html пока нет фронтенда
RUN mkdir -p dist && printf '<!DOCTYPE html>\n<html lang="ru">\n<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>AutoPrice</title></head>\n<body style="font-family:sans-serif;display:flex;align-items:center;justify-content:center;height:100vh;margin:0">\n<div style="text-align:center"><h1>AutoPrice</h1><p>Frontend в разработке. API работает.</p><a href="/healthz">/healthz</a></div>\n</body></html>\n' > dist/index.html

FROM alpine:3.21

WORKDIR /app

COPY --from=builder /app/dist ./dist

# Копирует файлы в shared volume при старте, затем ждёт
CMD ["sh", "-c", "cp -r /app/dist/. /dist/ && echo 'web: dist copied to volume' && tail -f /dev/null"]
