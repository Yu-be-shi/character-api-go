-- v1_characters: 複数 API（Go / Python 等）や DB 直アクセスの利用者が共有する
-- 安定読み取りコントラクト（v1）。
--   ・soft-delete（deleted_at IS NULL）でフィルタ済みの行のみを返す
--   ・races を JOIN して race 名を展開する
--   ・gender に応じた size_top のラベルを付与する
--
-- ※ ビュー/関数は Atlas（無料版）の宣言的差分では扱えないため、テーブルとは分離し、
--   ここで「あるべき定義」を冪等に管理する。CREATE OR REPLACE なので何度適用しても安全。
--   character-db-migrate が `atlas migrate apply`（テーブル）の後にこれらを適用する。
--   列の並び替え・型変更など OR REPLACE で不可能な変更が必要なときは、先頭で
--   `DROP VIEW IF EXISTS v1_characters;` を明示してから作り直すこと。
CREATE OR REPLACE VIEW v1_characters AS
SELECT
    c.id,
    c.name,
    c.description,
    r.name AS race,
    c.gender,
    c.birth_date,
    c.birth_place,
    c.height_cm,
    c.weight_kg,
    c.body_fat_percentage,
    CASE
        WHEN c.gender = 'female' THEN 'バスト'
        WHEN c.gender = 'male'   THEN 'チェスト'
        ELSE '胸囲'
    END AS size_top_label,
    c.size_top    AS top_value,
    c.size_middle AS waist_value,
    c.size_bottom AS hip_value,
    c.created_at,
    -- 楽観ロック（update_character の p_expected_version）に必要。
    -- ビュー利用者がテーブル直読みに戻らなくて済むよう、読み取りコントラクトに含める。
    c.version,
    c.updated_at
FROM core_characters c
JOIN races r ON c.race_id = r.id
-- 確定済み（予約パターンの active）かつ生存行のみを公開する。未確定（pending）の
-- 予約は「まだ存在しないデータ」として読み取りコントラクトに出さない。
WHERE c.deleted_at IS NULL AND c.confirmed_at IS NOT NULL;
