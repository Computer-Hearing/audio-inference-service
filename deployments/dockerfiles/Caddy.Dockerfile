FROM node:22-alpine AS frontend
WORKDIR /src
COPY frontend/ ./
RUN npm ci && npm run build

FROM caddy:alpine
COPY --from=frontend /src/dist /srv
EXPOSE 80
