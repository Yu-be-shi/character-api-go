.PHONY: dev test vet build generate sqlc

# ── 開発サーバー（Air ホットリロード） ──────────────────────────────────────────────
dev:
	air -c .air.toml

# ── テスト・静的解析 ─────────────────────────────────────────────────────────────
test:
	go test -race ./...

vet:
	go vet ./...

# ── ビルド ───────────────────────────────────────────────────────────────────────
build:
	CGO_ENABLED=0 go build -o bin/api ./cmd/api

# ── OpenAPI spec 生成 ─────────────────────────────────────────────────────────────
# swag はバージョン固定（CI のドリフト検知と出力を一致させるため）。
SWAG_VERSION := v1.16.4
generate:
	docker run --rm \
		-v $(shell pwd):/app \
		-w /app \
		golang:1.25-alpine \
		sh -c "go install github.com/swaggo/swag/cmd/swag@$(SWAG_VERSION) && swag init -g cmd/api/main.go -o docs --parseInternal"

# ── sqlc コード生成 ───────────────────────────────────────────────────────────────
# スキーマは third_party/character-db（vendoring した通常ファイル）から読む。CI のドリフト検知と一致させる。
SQLC_VERSION := 1.31.1
sqlc:
	docker run --rm -v "$(shell pwd):/src" -w /src sqlc/sqlc:$(SQLC_VERSION) generate
