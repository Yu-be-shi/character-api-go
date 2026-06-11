.PHONY: dev test vet build generate sqlc migration hash lint

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
# スキーマは自リポジトリの schema.sql / views/ から読む。CI のドリフト検知と一致させる。
SQLC_VERSION := 1.31.1
sqlc:
	docker run --rm -v "$(shell pwd):/src" -w /src sqlc/sqlc:$(SQLC_VERSION) generate

# ── スキーマ管理（Atlas + 冪等SQL のハイブリッド） ───────────────────────────────────
# 役割分担（Atlas 無料版はビュー/関数/トリガーと migrate lint が Pro 専用のため）:
#   - テーブル・ENUM・インデックス … Atlas（宣言的差分）。唯一の正は schema.sql
#   - ビュー・関数・トリガー       … views/*.sql に冪等SQL（CREATE OR REPLACE 等）
#   - マスタ初期データ             … seeds/*.sql に冪等SQL（INSERT ... ON CONFLICT 等）
#   - 安全リンタ                   … make lint（squawk・無料）
# 適用は character-db-migrate（Dockerfile.migrate）が「atlas apply → psql で views → seeds」を行う。
#
# 手順: schema.sql を編集 → make migration name=<説明>（差分生成）→ make hash。
#       ビュー/関数/初期データは views/*.sql・seeds/*.sql を直接編集（冪等SQL）。
ATLAS := $(shell which atlas 2>/dev/null)
HOST_DEV_URL := docker://postgres/16/dev?search_path=public
SQUAWK_IMG := node:20-slim
ATLAS_IMG  := arigaio/atlas:1.2.2
PG_IMAGE   := postgres:16
NET        := atlas-migrate-net
PG_NAME    := atlas-dev-pg
DEV_DB_URL := postgres://atlas:atlas@$(PG_NAME):5432/dev?sslmode=disable&search_path=public
SQUAWK_VERSION := 2.57.0

migration:
ifndef name
	$(error 使用方法: make migration name=<説明>)
endif
ifdef ATLAS
	atlas migrate diff $(name) \
		--dir "file://migrations" \
		--to "file://schema.sql" \
		--dev-url "$(HOST_DEV_URL)"
else
	@echo "▶ atlas CLI 未検出 → 使い捨て Postgres を起動して atlas をコンテナ実行します"
	@set -e; \
	cleanup() { \
		docker rm -f $(PG_NAME) >/dev/null 2>&1 || true; \
		docker network rm $(NET) >/dev/null 2>&1 || true; \
	}; \
	trap cleanup EXIT INT TERM; \
	docker network create $(NET) >/dev/null 2>&1 || true; \
	docker run -d --rm --name $(PG_NAME) --network $(NET) \
		-e POSTGRES_USER=atlas -e POSTGRES_PASSWORD=atlas -e POSTGRES_DB=dev \
		$(PG_IMAGE) >/dev/null; \
	echo "▶ 一時 Postgres の起動を待機中..."; \
	for i in $$(seq 1 60); do \
		if docker exec $(PG_NAME) psql -U atlas -d dev -c 'select 1' >/dev/null 2>&1; then break; fi; \
		sleep 1; \
	done; \
	if ! docker exec $(PG_NAME) psql -U atlas -d dev -c 'select 1' >/dev/null 2>&1; then \
		echo "✗ 一時 Postgres が 60 秒以内に起動しませんでした" >&2; exit 1; \
	fi; \
	docker run --rm --network $(NET) -v "$(PWD):/workspace" -w /workspace \
		$(ATLAS_IMG) migrate diff $(name) \
		--dir "file://migrations" \
		--to "file://schema.sql" \
		--dev-url "$(DEV_DB_URL)"
endif

hash:
ifdef ATLAS
	atlas migrate hash --dir "file://migrations"
else
	docker run --rm -v "$(PWD):/workspace" -w /workspace \
		$(ATLAS_IMG) migrate hash --dir "file://migrations"
endif

# lint: マイグレーション/ビュー/シードの SQL を squawk で安全点検（無料）。除外理由は squawk.toml。
lint:
	docker run --rm -v "$(PWD):/work" -w /work $(SQUAWK_IMG) \
		sh -c "npx --yes squawk-cli@$(SQUAWK_VERSION) -c squawk.toml migrations/*.sql views/*.sql seeds/*.sql"
