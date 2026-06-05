# character-api

キャラクターデータのCRUD・検索・ソート・フィルターを行うコアAPI。
ユーザー情報に一切依存しない、ピュアなキャラクターデータ管理サービス。

## 技術スタック

| 層 | 技術 |
|---|---|
| Web フレームワーク | Go + Echo v4 |
| ORM | GORM |
| DB | PostgreSQL（Repo3） |
| マイグレーション | 独自ランナー（`embed.FS` + SQL） |

## アーキテクチャ

クリーンアーキテクチャを採用。依存方向は一方向:

```
interfaces/http → usecase → domain ← infrastructure/persistence
```

## 起動方法

```bash
# 環境変数の準備
cp .env.example .env  # Repo4 の .env.example を参照

# ローカル開発（Repo4 の docker-compose から起動推奨）
docker compose up
```

## マイグレーション

マイグレーションは API サーバー起動時に自動実行される（`migrate subcommand` を Repo4 が init container として呼び出す）。

新規マイグレーションファイルの作成:

```bash
make migration name=add_description_to_characters
```

## サービス間認証

すべての `/api/v1/*` エンドポイントは `X-Internal-API-Key` ヘッダーを必須とする。
キーは環境変数 `INTERNAL_API_KEY` で設定する（Repo4 の `.env` で管理）。

## API エンドポイント

| Method | Path | 説明 |
|---|---|---|
| GET | /healthz | ヘルスチェック（認証不要） |
| GET | /api/v1/characters | 一覧取得 |
| POST | /api/v1/characters | 新規作成 |
| GET | /api/v1/characters/:id | 単件取得 |
| PUT | /api/v1/characters/:id | 更新 |
| DELETE | /api/v1/characters/:id | 削除 |

## 環境変数

| 変数名 | 必須 | デフォルト | 説明 |
|---|---|---|---|
| `DB_DSN` | ✓ | - | PostgreSQL DSN |
| `INTERNAL_API_KEY` | ✓ | - | サービス間認証キー |
| `PORT` | | 8080 | リスニングポート |
| `LOG_LEVEL` | | info | ログレベル |
| `CORS_ORIGINS` | | http://localhost:3000 | CORS 許可オリジン（カンマ区切り） |
