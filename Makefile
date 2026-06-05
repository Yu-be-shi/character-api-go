.PHONY: dev test vet build migration

MIGRATIONS_DIR := internal/infrastructure/persistence/gorm/migrations/postgres

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

# ── マイグレーションファイル生成 ─────────────────────────────────────────────────
# 使用例: make migration name=add_description_to_characters
migration:
ifndef name
	$(error 使用方法: make migration name=<説明>)
endif
	$(eval N    := $(shell ls $(MIGRATIONS_DIR)/*.up.sql 2>/dev/null | wc -l | tr -d ' '))
	$(eval NEXT := $(shell printf "%04d" $$(($(N) + 1))))
	$(eval FILE := $(MIGRATIONS_DIR)/$(NEXT)_$(name))
	@touch $(FILE).up.sql $(FILE).down.sql
	@echo "作成: $(FILE).up.sql"
	@echo "作成: $(FILE).down.sql"
