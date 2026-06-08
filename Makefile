.PHONY: dev test vet build generate

# ── 開発サーバー（Air ホットリロード） ──────────────────────────────────────────────
dev:
	air -c .air.toml

# ── テスト・静的解析 ─────────────────────────────────────────────────────────────
test:
	go test ./...

vet:
	go vet ./...

# ── ビルド ───────────────────────────────────────────────────────────────────────
build:
	CGO_ENABLED=0 go build -o bin/api ./cmd/api

# ── OpenAPI spec 生成 ─────────────────────────────────────────────────────────────
generate:
	docker run --rm \
		-v $(shell pwd):/app \
		-w /app \
		golang:1.23-alpine \
		sh -c "go install github.com/swaggo/swag/cmd/swag@latest && swag init -g cmd/api/main.go -o docs --parseInternal"
