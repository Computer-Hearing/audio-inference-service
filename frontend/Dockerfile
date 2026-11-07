# Двухступенчатая сборка фронтенда инференса.
#
# 1) node — устанавливает зависимости и собирает dist/inference.js
#    (один файл: код, React и стили внутри, см. vite.config.ts);
# 2) caddy — лёгкий образ, в который попадает только готовый inference.js
#    и Caddyfile. Node, исходники и node_modules в итоговый образ не попадают.
#
# Сборка из папки frontend/:  docker build -t inference-frontend .

# --- Ступень 1: сборка ---
FROM node:22-alpine AS build
WORKDIR /app

# Сначала только файлы зависимостей — этот слой кешируется и не
# пересобирается, пока package*.json не изменились
COPY package.json package-lock.json ./
RUN npm ci

COPY . .
RUN npm run build

# --- Ступень 2: раздача ---
FROM caddy:2-alpine
COPY Caddyfile /etc/caddy/Caddyfile
COPY --from=build /app/dist /srv
EXPOSE 80
