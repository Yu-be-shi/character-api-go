#!/bin/sh
# character-db-migrate の実行スクリプト。
#   1) atlas migrate apply … テーブル・ENUM・インデックス（Atlas 管理の宣言的マイグレーション）
#   2) psql で views/*.sql を冪等適用 … ビュー・関数（CREATE OR REPLACE 等。Atlas 管理外）
#   3) psql で seeds/*.sql を冪等適用 … マスタ初期データ（ON CONFLICT DO NOTHING 等。Atlas 管理外）
#
# DB 接続先は次の優先順で取得する:
#   1. command 引数の `--url=...`（ローカル compose はこの形で渡す）
#   2. 環境変数 DB_DSN（本番 ECS。Secrets Manager の url キーを注入する。
#      ECS の exec 形式 command は $(VAR) をシェル展開しないため、引数ではなく
#      環境変数で受け取る必要がある）
# いずれも atlas が解釈できる URL 形式（postgres://...）であること。
set -e

DB_URL=""
for arg in "$@"; do
	case "$arg" in
		--url=*) DB_URL="${arg#--url=}" ;;
	esac
done
if [ -z "$DB_URL" ] && [ -n "${DB_DSN:-}" ]; then
	DB_URL="$DB_DSN"
	set -- --url="$DB_URL" "$@"
fi
if [ -z "$DB_URL" ]; then
	echo "migrate-entrypoint: --url=<DSN> 引数か DB_DSN 環境変数が必要です" >&2
	exit 1
fi

echo "==> [1/3] atlas migrate apply（テーブル等）"
atlas migrate apply --dir "file:///migrations" "$@"

# -1（single transaction）: ファイル単位でアトミックに適用する。
# DROP FUNCTION → CREATE FUNCTION を含むファイルでも「関数が存在しない瞬間」を
# 稼働中の API に見せない。
echo "==> [2/3] ビュー/関数の冪等適用（views/*.sql）"
for f in /views/*.sql; do
	[ -e "$f" ] || continue   # views が空でも失敗しない
	echo "    -> $f"
	psql "$DB_URL" -v ON_ERROR_STOP=1 -1 -f "$f"
done

echo "==> [3/3] マスタ初期データの冪等適用（seeds/*.sql）"
for f in /seeds/*.sql; do
	[ -e "$f" ] || continue   # seeds が空でも失敗しない
	echo "    -> $f"
	psql "$DB_URL" -v ON_ERROR_STOP=1 -1 -f "$f"
done

echo "==> マイグレーション完了"
