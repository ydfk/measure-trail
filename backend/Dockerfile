FROM golang:1.26-alpine AS builder

WORKDIR /src
RUN apk add --no-cache build-base git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/measuretrail-api ./cmd

FROM alpine:3.23

RUN apk add --no-cache ca-certificates sqlite tzdata && addgroup -S measuretrail && adduser -S measuretrail -G measuretrail
WORKDIR /app
COPY --from=builder /out/measuretrail-api ./measuretrail-api
COPY scripts/backup-sqlite.sh scripts/restore-sqlite.sh /app/scripts/
RUN chmod 0755 /app/scripts/*.sh && mkdir -p /app/backups /app/data /app/log && chown -R measuretrail:measuretrail /app
USER measuretrail
EXPOSE 21000
ENTRYPOINT ["./measuretrail-api"]
