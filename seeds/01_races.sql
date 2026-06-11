-- races マスタの初期データ（冪等シード）。
--   ・core_characters.race_id は NOT NULL 必須で、アプリの新規作成フォームは
--     races 一覧（ドロップダウン）に依存する。races が空だとキャラを1件も作成できないため、
--     最低限のマスタをここで投入する。
--   ・ON CONFLICT (name) DO NOTHING により何度適用しても重複しない（冪等）。
--     races.name には UNIQUE 制約がある（schema.sql 参照）。
--
-- ※ テーブル定義（schema.sql）は Atlas が管理するが、データ（行）は Atlas の差分計算対象外。
--   ビュー/関数（views/*.sql）と同じく Atlas 管理外の冪等SQLとして character-db-migrate が
--   `atlas migrate apply`（テーブル）→ `views/*.sql` の後に適用する。
--   マスタを追加・変更したいときはこのファイルを編集する（行の削除は全 API への影響を確認のうえ慎重に）。
INSERT INTO races (name) VALUES
    ('人間'),
    ('エルフ'),
    ('ドワーフ'),
    ('獣人'),
    ('竜族')
ON CONFLICT (name) DO NOTHING;
