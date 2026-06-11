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
| DB アクセス | sqlc（SQL からの型安全コード生成）+ pgx/v5 |
| DB | PostgreSQL（`character-db`） |
| マイグレーション | **持たない**（スキーマは `character-db` の Atlas が管理） |

## アーキテクチャ

クリーンアーキテクチャを採用。依存方向は一方向:

```
interfaces/http → usecase → domain ← infrastructure/persistence
```

### DB アクセス（sqlc）とスキーマ依存

永続化層は **sqlc**（`internal/infrastructure/persistence/postgres`）。SQL を書くと型安全な Go が
生成され、`domain.Repository` を実装する。ORM のリフレクションに頼らず、発行 SQL が明示的。

- **書き込みの不変条件は DB 側に集約**：楽観ロック（version 検査＋ +1）と論理削除は
  `character-db` の DB 関数 `update_character` / `soft_delete_character` を呼ぶだけ。
  関数は競合を SQLSTATE `CH412`、不在/削除済みを `CH404` で返し、`convert.go` が
  ドメインエラー（`ErrVersionConflict` / `ErrNotFound` 等）へ変換する。
- **スキーマの正は別リポジトリ**：`character-db` を **git submodule**（`third_party/character-db`）で
  特定コミットに固定し、sqlc はそこの `schema.sql` と `views/10_*.sql` から型を生成する。
  これによりローカルの並び順や起動中 DB に依存しない、宣言された純粋な依存になる。

```bash
git submodule update --init                 # 初回・clone 後
git -C third_party/character-db checkout <sha> && git add third_party/character-db  # スキーマ追従（ピン更新）
make sqlc                                    # 生成（docker の sqlc/sqlc。Go へのツール導入不要）
```

生成物（`internal/.../postgres/sqlc/*.go`）はコミットする。CI はビルド時に submodule 不要
（生成済みコードを使う）。submodule が要るのは `make sqlc` の再生成と統合テストのみ。

### エラーハンドリング

ドメインエラー（`ErrNotFound` / `ErrInvalidName` / `ErrInvalidGender` / race の `ErrNotFound` 等）を
`interfaces/http` の `mapCharErr` / `mapRaceErr` が HTTP ステータスへ変換する（404 / 400 / 422）。
**未分類のエラー（DB 障害など）は内部詳細をクライアントに漏らさず、`slog` でサーバーログに残したうえで
汎用 500 を返す**（`default` ケース）。

## テスト

```bash
make test          # go test -race ./...（CI と同じ）
go test ./internal/usecase/...   # ユースケースのみ
```

- **usecase 層**：リポジトリ interface の fake と `Clock` 注入でDB非依存にユニットテスト
  （`usecase/character`, `usecase/race`）。
- **interfaces/http 層**：`httptest` で実ルーター（`httpiface.New`）を起動し、APIキー認証（401）・
  パラメータ検証（400）・エラーマッピング（404/422）・正常系（201）をエンドツーエンドに検証
  （`internal/interfaces/http/router_test.go`）。
- インメモリ fake を使うため、通常のテスト実行に PostgreSQL は不要。
- **永続化層の統合テスト**（`-tags=integration`）：実 PostgreSQL に対し、submodule の実スキーマ＋
  DB 関数を適用して version 競合（CH412）・論理削除（CH404）・FK→ErrInUse・NUMERIC 往復・
  updated_at トリガーを検証する。`git submodule update --init` 済みであることと `TEST_DB_DSN`
  （無ければ `DB_DSN`）が前提。例: `go test -tags=integration ./internal/infrastructure/persistence/postgres/...`

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

**ステータスコードの方針**：`401`=API キー不一致 / `404`=対象なし / `400`=形式・パラメータ不正
（不正な `If-Match` 形式を含む） / `413`=リクエストボディ 1MB 超 / `422`=参照先 race が存在しない or
冪等キーの別ボディ再利用 / `409`=使用中 race の削除 or 種族名の重複 or 冪等キーが処理中 /
`412`=楽観ロックの版不一致 / `429`=レート制限（`RATE_LIMIT_RPS` 有効時） /
`500`=未分類のサーバーエラー（詳細はログのみ）。

> `/readyz` は認証不要で 1 リクエスト = 1 DB ping のため、LB のヘルスチェック以外には公開しない
> （本番では SG / LB 設定で到達元を絞る）。`/swagger/*` も同様に内部公開のみとする。

## 並行制御（楽観ロック）と冪等性

- **楽観ロック**：`core_characters.version`（DB の連番）でレコードのバージョンを管理。
  - GET / 作成・更新のレスポンスは `version`（body）と `ETag` ヘッダを返す。
  - PUT / PATCH で `If-Match: "<version>"` を送ると、版が一致するときだけ更新し +1 する。
    不一致（別の更新が先に入った）なら **412 Precondition Failed**。
  - **`If-Match` 省略時も無条件上書きにはならない**：読み取り時点の version を期待値として
    使うため、read-modify-write の間に他者の更新が入れば 412 が返り得る（lost update 防止）。
  - 版検査と +1 は DB 関数 `update_character`（character-db 側）が行う（複数 API で手順がズレない）。
- **冪等性キー**：`/api/v1` 配下の **すべての POST**（characters / races）で
  `Idempotency-Key`（200 文字以内）を送ると、同一キー・同一ボディの再送は保存済みレスポンスを
  再生する（`Idempotent-Replayed: true`。`ETag` / `Location` / `Content-Type` も復元）。
  - 同一キー・**異なるボディ**は 422（キーの誤用を黙って再生しない）。処理中の同一キーは 409。
  - キーはエンドポイント（メソッド + パス）にスコープされ、結果の保持期間（TTL）は 24 時間。
  - ストアは Redis（`REDIS_ADDR` 未設定なら機能無効＝ローカル/CI は Redis 不要）。
  - 抽象（`Store`）は `internal/domain/idempotency`、実装（Redis/メモリ）は
    `infrastructure/idempotency`。Echo ミドルウェアはハンドラ/ユースケースを汚染しない。
    PUT/PATCH/DELETE は元々冪等なので対象外。

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
