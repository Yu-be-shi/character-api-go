# syntax=docker/dockerfile:1.7

# ── dev: Air によるホットリロード ────────────────────────────────────────────────
FROM golang:1.25-alpine AS dev
WORKDIR /app
RUN apk add --no-cache git ca-certificates \
    && go install github.com/air-verse/air@v1.52.3
COPY go.mod go.sum* ./
RUN go mod download || true
EXPOSE 8080
CMD ["sh", "-c", "go mod tidy && air -c .air.toml"]

# ── build: 静的バイナリの生成 ─────────────────────────────────────────────────────
FROM golang:1.25-alpine AS build
WORKDIR /app
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# ── runtime: 最小イメージ ─────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot AS runtime
WORKDIR /app
COPY --from=build /out/api /app/api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/api"]
# このバイナリは HTTP サーバーを起動するだけ（サブコマンドなし）。
# マイグレーションはこの API では行わない（スキーマ適用は character-db-migrate が担当）。
