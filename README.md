# character-api

[![CI](https://github.com/Yu-be-shi/character-api-go/actions/workflows/ci.yml/badge.svg)](https://github.com/Yu-be-shi/character-api-go/actions/workflows/ci.yml)
[![CodeQL](https://github.com/Yu-be-shi/character-api-go/actions/workflows/codeql.yml/badge.svg)](https://github.com/Yu-be-shi/character-api-go/actions/workflows/codeql.yml)

キャラクターデータの CRUD を行うコアAPI。
ユーザー情報に一切依存しない、ピュアなキャラクターデータ管理サービス。

> 検索・ソート・フィルタは現状未実装（一覧は `?ids=` のバッチ取得と `?limit=&offset=` の
> ページングのみ）。名前検索・性別フィルタ・並べ替えは要件が出た時点で後方互換に追加する。

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

### エラーハンドリング

ドメインエラー（`ErrNotFound` / `ErrInvalidName` / `ErrInvalidGender` / race の `ErrNotFound` 等）を
`interfaces/http` の `mapCharErr` / `mapRaceErr` が HTTP ステータスへ変換する（404 / 400 / 422）。
**未分類のエラー（DB 障害など）は内部詳細をクライアントに漏らさず、`slog` でサーバーログに残したうえで
汎用 500 を返す**（`default` ケース）。

## テスト

```bash
make test          # go test ./...（CI と同じ）
go test ./internal/usecase/...   # ユースケースのみ
```

- **usecase 層**：リポジトリ interface の fake と `Clock` 注入でDB非依存にユニットテスト
  （`usecase/character`, `usecase/race`）。
- **interfaces/http 層**：`httptest` で実ルーター（`httpiface.New`）を起動し、APIキー認証（401）・
  パラメータ検証（400）・エラーマッピング（404/422）・正常系（201）をエンドツーエンドに検証
  （`internal/interfaces/http/router_test.go`）。
- インメモリ fake を使うため、テスト実行に PostgreSQL は不要。

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
| GET | /api/v1/characters | 一覧取得。`?ids=<uuid,...>` バッチ / `?limit=&offset=` ページング。レスポンスは `{ "items": [...], "total": <総件数> }` |
| POST | /api/v1/characters | 新規作成（race 不在は 422） |
| GET | /api/v1/characters/:id | 単件取得 |
| PUT | /api/v1/characters/:id | **全置換**（送らなかった任意項目は NULL になる＝値のクリアはこちら） |
| PATCH | /api/v1/characters/:id | **部分更新**（送った項目だけ変更。クリアは不可＝PUT を使う） |
| DELETE | /api/v1/characters/:id | 削除（論理削除: deleted_at を設定） |
| GET | /api/v1/races | 種族一覧取得 |
| POST | /api/v1/races | 種族新規作成 |
| GET | /api/v1/races/:id | 種族単件取得 |
| PUT | /api/v1/races/:id | 種族更新 |
| DELETE | /api/v1/races/:id | 種族削除（**使用中の種族は 409 Conflict**） |
| GET | /swagger/* | Swagger UI（OpenAPI） |

**ステータスコードの方針**：`404`=対象なし / `400`=形式・パラメータ不正 / `422`=参照先 race が存在しない /
`409`=使用中 race の削除 or 種族名の重複 / `412`=楽観ロックの版不一致 / `500`=未分類のサーバーエラー（詳細はログのみ）。

## 並行制御（楽観ロック）と冪等性

- **楽観ロック**：`core_characters.version`（DB の連番）でレコードのバージョンを管理。
  - GET / 作成・更新のレスポンスは `version`（body）と `ETag` ヘッダを返す。
  - PUT / PATCH で `If-Match: "<version>"` を送ると、版が一致するときだけ更新し +1 する。
    不一致（別の更新が先に入った）なら **412 Precondition Failed**。`If-Match` 省略時は無条件更新（後方互換）。
- **冪等性キー**：`POST /api/v1/characters` で `Idempotency-Key: <uuid>` を送ると、同一キーの再送は
  保存済みレスポンスを再生（`Idempotent-Replayed: true`）。処理中の同一キーは 409。
  - ストアは Redis（`REDIS_ADDR` 未設定なら機能無効＝ローカル/CI は Redis 不要）。
  - 実装はクリーンアーキの `infrastructure/idempotency`（Redis/メモリ実装）＋ Echo ミドルウェアで、
    ハンドラ/ユースケースを汚染しない。PUT/PATCH/DELETE は元々冪等なので対象外。

## 環境変数

| 変数名 | 必須 | デフォルト | 説明 |
|---|---|---|---|
| `DB_DSN` | ✓ | - | PostgreSQL DSN |
| `INTERNAL_API_KEY` | ✓ | - | サービス間認証キー |
| `PORT` | | 8080 | リスニングポート |
| `LOG_LEVEL` | | info | ログレベル |
| `CORS_ORIGINS` | | http://localhost:3000 | CORS 許可オリジン（カンマ区切り） |
| `REDIS_ADDR` | | （空＝無効） | 冪等性キー用 Redis のアドレス（例 `character-api-redis:6379`）。空なら冪等性機能を無効化 |
| `RATE_LIMIT_RPS` | | 0（無効） | `/api/v1` の IP あたり秒間リクエスト上限（無料・インメモリ）。0 で無効 |
