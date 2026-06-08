# character-api

キャラクターデータのCRUD・検索・ソート・フィルターを行うコアAPI。
ユーザー情報に一切依存しない、ピュアなキャラクターデータ管理サービス。

## 技術スタック

| 層 | 技術 |
|---|---|
| Web フレームワーク | Go + Echo v4 |
| ORM | GORM |
| DB | PostgreSQL（`character-db`） |
| マイグレーション | **持たない**（スキーマは `character-db` の Atlas が管理） |

## アーキテクチャ

クリーンアーキテクチャを採用。依存方向は一方向:

```
interfaces/http → usecase → domain ← infrastructure/persistence
```

## 起動方法

```bash
# ローカル開発（推奨）: API 専用インフラの docker-compose から起動する。
# 事前に character-db-infra を起動して character-db-net / DB を用意しておくこと。
cd ../character-api-go-infra && docker compose up --build -d

# Air によるホットリロード単体開発
make dev
```

## マイグレーション

この API はマイグレーションを **持たない**。スキーマ（テーブル・ビュー・ENUM）は
`character-db` リポジトリの宣言的定義（`schema.sql`）と Atlas マイグレーションが
唯一の正であり、適用は `character-db-migrate` サービスが行う。

スキーマを変更したいときは `character-db/` 側で `make migration` / `make hash` を実行する。
カラムの削除・リネームは、この API を含む全 API が対応済みになってから行うこと。

## サービス間認証

すべての `/api/v1/*` エンドポイントは `X-Internal-API-Key` ヘッダーを必須とする。
キーは環境変数 `INTERNAL_API_KEY` で設定し、application 側の `CHARACTER_API_KEY` と
同じ値にする（不一致だと 401 を返す）。

## API エンドポイント

| Method | Path | 説明 |
|---|---|---|
| GET | /healthz | Liveness（認証不要・依存に触れない） |
| GET | /readyz | Readiness（認証不要・DB ping 込み。NG 時 503） |
| GET | /api/v1/characters | 一覧取得（`?ids=<uuid,...>` でバッチ取得 / `?limit=&offset=` でページング） |
| POST | /api/v1/characters | 新規作成 |
| GET | /api/v1/characters/:id | 単件取得 |
| PUT | /api/v1/characters/:id | 更新 |
| DELETE | /api/v1/characters/:id | 削除（論理削除: deleted_at を設定） |
| GET | /api/v1/races | 種族一覧取得 |
| POST | /api/v1/races | 種族新規作成 |
| GET | /api/v1/races/:id | 種族単件取得 |
| PUT | /api/v1/races/:id | 種族更新 |
| DELETE | /api/v1/races/:id | 種族削除 |
| GET | /swagger/* | Swagger UI（OpenAPI） |

## 環境変数

| 変数名 | 必須 | デフォルト | 説明 |
|---|---|---|---|
| `DB_DSN` | ✓ | - | PostgreSQL DSN |
| `INTERNAL_API_KEY` | ✓ | - | サービス間認証キー |
| `PORT` | | 8080 | リスニングポート |
| `LOG_LEVEL` | | info | ログレベル |
| `CORS_ORIGINS` | | http://localhost:3000 | CORS 許可オリジン（カンマ区切り） |
