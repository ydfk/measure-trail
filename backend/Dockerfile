FROM node:24-alpine AS web-builder

WORKDIR /src/web
COPY web/ ./
RUN if [ -f package.json ]; then \
      corepack enable; \
      if [ -f pnpm-lock.yaml ]; then pnpm install --frozen-lockfile && pnpm run build; \
      elif [ -f yarn.lock ]; then yarn install --immutable && yarn build; \
      elif [ -f package-lock.json ]; then npm ci && npm run build; \
      else npm install && npm run build; fi; \
    else \
      mkdir -p dist && cp placeholder.html dist/index.html; \
    fi

FROM golang:1.26-alpine AS api-builder

WORKDIR /src
RUN apk add --no-cache build-base git
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
RUN CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/measuretrail-api ./cmd

FROM alpine:3.23

RUN apk add --no-cache ca-certificates sqlite tzdata && addgroup -S measuretrail && adduser -S measuretrail -G measuretrail
WORKDIR /app
COPY --from=api-builder /out/measuretrail-api ./measuretrail-api
COPY --from=web-builder /src/web/dist ./web
COPY backend/scripts/backup-sqlite.sh backend/scripts/restore-sqlite.sh /app/scripts/
RUN chmod 0755 /app/scripts/*.sh && mkdir -p /app/backups /app/data /app/log && chown -R measuretrail:measuretrail /app
USER measuretrail
EXPOSE 21000
ENTRYPOINT ["./measuretrail-api"]
