# syntax=docker/dockerfile:1.10

FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/import-icd11 ./cmd/import-icd11

FROM alpine:3.23

RUN apk add --no-cache ca-certificates curl tzdata && \
    addgroup -S nodus && \
    adduser -S -G nodus -h /app nodus && \
    install -d -o nodus -g nodus /app/logs

WORKDIR /app
COPY --from=build --chown=nodus:nodus /out/api /out/migrate /out/import-icd11 ./

USER nodus
EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=6 \
    CMD curl --fail --silent --show-error http://127.0.0.1:8080/health >/dev/null || exit 1

ENTRYPOINT ["/app/api"]
