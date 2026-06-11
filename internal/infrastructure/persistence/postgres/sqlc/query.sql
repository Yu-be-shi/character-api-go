-- character / race の SQL。書き込みの不変条件（楽観ロック・論理削除）は
-- character-db 側の DB 関数 update_character / soft_delete_character に集約済みで、
-- ここではそれを呼ぶだけ。読み取りは races を JOIN して race 名を展開する。

-- name: CreateCharacter :exec
-- description / birth_place は「未設定」を NULL で表現する（空文字と NULL を混在させない。
-- 正規化は repo 層の pgTextOrNull が行う）。
INSERT INTO core_characters (
    id, name, description, race_id, gender, birth_date, birth_place,
    height_cm, weight_kg, body_fat_percentage, size_top, size_middle, size_bottom,
    version, created_at, updated_at
) VALUES (
    @id, @name, sqlc.narg(description)::text, @race_id, @gender,
    sqlc.narg(birth_date)::date, sqlc.narg(birth_place)::varchar,
    sqlc.narg(height_cm)::smallint, sqlc.narg(weight_kg)::smallint,
    sqlc.narg(body_fat_percentage)::numeric,
    sqlc.narg(size_top)::smallint, sqlc.narg(size_middle)::smallint, sqlc.narg(size_bottom)::smallint,
    @version, @created_at, @updated_at
);

-- name: GetCharacter :one
SELECT c.*, r.name AS race_name
FROM core_characters c
LEFT JOIN races r ON r.id = c.race_id
WHERE c.id = @id AND c.deleted_at IS NULL;

-- name: ListCharacters :many
SELECT c.*, r.name AS race_name
FROM core_characters c
LEFT JOIN races r ON r.id = c.race_id
WHERE c.deleted_at IS NULL
  AND (sqlc.narg(ids)::uuid[] IS NULL OR c.id = ANY(sqlc.narg(ids)::uuid[]))
-- id をタイブレークにして同時刻行でもページング順序を安定させる
ORDER BY c.created_at ASC, c.id ASC
LIMIT NULLIF(sqlc.arg(lim)::bigint, 0)
OFFSET sqlc.arg(off)::bigint;

-- name: CountCharacters :one
SELECT COUNT(*) FROM core_characters
WHERE deleted_at IS NULL
  AND (sqlc.narg(ids)::uuid[] IS NULL OR id = ANY(sqlc.narg(ids)::uuid[]));

-- name: UpdateCharacter :one
-- 楽観ロック(version 検査+1)と soft-delete フィルタは関数内で行う。
-- expected_version が NULL なら version 検査をスキップ（強制更新）。
-- 競合時は SQLSTATE 'CH412'、不在/削除済みは 'CH404' を RAISE する。
-- 関数の RETURNING（更新後の行）をそのまま返し、呼び出し側の再 SELECT
-- （別トランザクションになることによる競合窓・余分な往復）を無くす。
SELECT u.*, r.name AS race_name
FROM update_character(
    @id,
    sqlc.narg(expected_version)::bigint,
    @name,
    sqlc.narg(description)::text,
    @race_id,
    @gender,
    sqlc.narg(birth_date)::date,
    sqlc.narg(birth_place)::varchar,
    sqlc.narg(height_cm)::smallint,
    sqlc.narg(weight_kg)::smallint,
    sqlc.narg(body_fat_percentage)::numeric,
    sqlc.narg(size_top)::smallint,
    sqlc.narg(size_middle)::smallint,
    sqlc.narg(size_bottom)::smallint
) AS u
LEFT JOIN races r ON r.id = u.race_id;

-- name: SoftDeleteCharacter :exec
-- 物理 DELETE せず deleted_at を立てる。不在/削除済みは SQLSTATE 'CH404' を RAISE。
SELECT soft_delete_character(@id);

-- name: CreateRace :exec
INSERT INTO races (id, name) VALUES (@id, @name);

-- name: GetRace :one
SELECT * FROM races WHERE id = @id;

-- name: ListRaces :many
SELECT * FROM races ORDER BY name ASC;

-- name: UpdateRace :execrows
UPDATE races SET name = @name WHERE id = @id;

-- name: DeleteRace :execrows
DELETE FROM races WHERE id = @id;
