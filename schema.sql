-- キャラクターDBのあるべきスキーマ定義
-- このファイルを編集して `make migration name=<説明>` を実行すると
-- migrations/ に差分SQLが自動生成される

CREATE TYPE gender_enum AS ENUM ('male', 'female', 'other', 'unknown');

CREATE TABLE races (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(50) NOT NULL UNIQUE
);

CREATE TABLE core_characters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    description TEXT,

    race_id UUID NOT NULL REFERENCES races(id),
    gender gender_enum NOT NULL DEFAULT 'unknown',

    birth_date DATE,
    birth_place VARCHAR(150),

    height_cm SMALLINT CHECK (height_cm > 0),
    weight_kg SMALLINT CHECK (weight_kg > 0),
    body_fat_percentage NUMERIC(4, 1) CHECK (body_fat_percentage >= 0 AND body_fat_percentage <= 100),

    size_top SMALLINT CHECK (size_top > 0),
    size_middle SMALLINT CHECK (size_middle > 0),
    size_bottom SMALLINT CHECK (size_bottom > 0),

    -- 楽観ロック用バージョン。版検査と +1 は DB 関数 update_character / soft_delete_character
    -- （views/10_character_write_functions.sql）が行い、各 API は関数を呼ぶだけ。
    version BIGINT NOT NULL DEFAULT 1,

    -- 作成の冪等トークン（消費者の Idempotency-Key 由来）。同一トークンの再作成は
    -- 一意インデックス（下記）が弾き、API は既存行を返す（Redis 非依存・永続的な二重作成防止）。
    -- DB 直アクセス/seed 等トークン無しの作成のため NULL 可（Postgres は複数 NULL を重複と見なさない）。
    creation_token TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- 予約パターン: 作成直後は NULL（pending=予約中）。消費者が所有を確定すると
    -- confirm_character が NOW() を立てる。可視データ（API の読み取り・更新・削除＝確定済み・生存行）は
    -- 確定済みのみを対象とし、確定されない予約は gc_unconfirmed_characters が物理回収する
    -- （消費者は origin を削除せず、未確定の回収は origin 自身の管轄で行う）。
    confirmed_at TIMESTAMPTZ,

    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_core_characters_race_id ON core_characters(race_id);

-- 一覧取得（API の List）は常に「確定済み かつ 生存行を created_at, id 順」で読む。
-- 未確定・削除済み行をスキャンしない部分インデックスでこのアクセスパスを直接支える。
-- ※ gender 単独のインデックスは ENUM 4 値の低カーディナリティで実用上使われないため持たない。
CREATE INDEX idx_core_characters_active_created_at
    ON core_characters (created_at, id) WHERE deleted_at IS NULL AND confirmed_at IS NOT NULL;

-- 未確定（pending）予約の TTL 回収（gc_unconfirmed_characters）のアクセスパスを支える。
CREATE INDEX idx_core_characters_unconfirmed
    ON core_characters (created_at) WHERE confirmed_at IS NULL;

-- 作成の冪等トークンの一意性。UNIQUE 制約ではなく一意インデックスにするのは、
-- ロック系 squawk ルール（既存除外の concurrent-index 系）に揃えるため。ON CONFLICT で利用する。
-- creation_token が NULL の行（DB 直/seed 等）は一意性の対象外（複数 NULL 可）。
CREATE UNIQUE INDEX idx_core_characters_creation_token
    ON core_characters (creation_token) WHERE creation_token IS NOT NULL;
