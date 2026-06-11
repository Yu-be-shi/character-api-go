-- キャラクターの「書き込み不変条件」を DB 側に集約する関数群（冪等SQL。Atlas 管理外）。
--
-- なぜ関数にするか:
--   core_characters は複数の書き手（Go / TS / Python API・psql 直アクセス）が共有する。
--   「楽観ロック（version 確認＋ +1）」と「論理削除（物理 DELETE せず deleted_at を立てる）」は
--   DB のカラム制約だけでは強制できない “振る舞い” のため、各言語が素朴な UPDATE/DELETE を
--   書くと簡単に破られる（lost update・物理削除）。これらを関数に閉じ込め、各言語は生の
--   UPDATE/DELETE をやめて関数を呼ぶことで、不変条件を 1 箇所（DB）に一本化する。
--
-- 呼び出し側へのエラー契約（カスタム SQLSTATE。各言語はエラーの code で判別する）:
--   'CH404' … 対象が存在しない / 既に論理削除済み      → HTTP 404 にマップ
--   'CH412' … 楽観ロックのバージョン不一致（競合）      → HTTP 412 にマップ
--   ※ gender 不正・CHECK 違反・FK 違反などの “構造” の不変条件は引き続き DB の型/制約が
--      標準 SQLSTATE（22P02 / 23514 / 23503 等）で弾くため、ここでは扱わない。
--
-- 冪等性: DROP FUNCTION IF EXISTS（既知のシグネチャ）→ CREATE FUNCTION で何度適用しても安全。
--   CREATE OR REPLACE を使わないのは、RETURNS core_characters がテーブルに紐づく複合型で、
--   core_characters に列を追加すると「戻り値型を変更できない」エラーになるため。
--   引数を変更したときは DROP のシグネチャも合わせて更新すること（旧シグネチャの
--   オーバーロード残存は呼び出しの曖昧エラーや旧ロジック実行の原因になる）。
--   適用は psql -1（ファイル単位の単一トランザクション）なので、DROP→CREATE の間の
--   「関数が無い瞬間」が稼働中 API から見えることはない。

-- 楽観ロック付き全置換更新（PUT 相当）。
--   p_expected_version が NULL なら version 検査をスキップ（強制更新）。
--   PATCH（部分更新）は「呼び出し側で現在値を読み→マージし→この関数を呼ぶ」で実現する
--   （“どの項目を変えるか” は見せ方の話なのでアプリ側に残してよい。守るべき競合検知は DB に集約）。
DROP FUNCTION IF EXISTS update_character(
    UUID, BIGINT, VARCHAR, TEXT, UUID, gender_enum, DATE, VARCHAR,
    SMALLINT, SMALLINT, NUMERIC, SMALLINT, SMALLINT, SMALLINT
);
CREATE FUNCTION update_character(
    p_id                  UUID,
    p_expected_version    BIGINT,
    p_name                VARCHAR,
    p_description         TEXT,
    p_race_id             UUID,
    p_gender              gender_enum,
    p_birth_date          DATE,
    p_birth_place         VARCHAR,
    p_height_cm           SMALLINT,
    p_weight_kg           SMALLINT,
    p_body_fat_percentage NUMERIC,
    p_size_top            SMALLINT,
    p_size_middle         SMALLINT,
    p_size_bottom         SMALLINT
) RETURNS core_characters AS $$
DECLARE
    v_current core_characters;
    v_result  core_characters;
BEGIN
    -- 生存している行をロックして取得（FOR UPDATE で「検査→更新」の間の同時更新を直列化）。
    -- 予約パターン: 確定済み（confirmed_at IS NOT NULL）の生存行のみを対象にする。
    -- 未確定（pending）の予約は「まだ存在しない」ため update/soft_delete からは CH404 扱い。
    SELECT * INTO v_current
      FROM core_characters
     WHERE id = p_id AND deleted_at IS NULL AND confirmed_at IS NOT NULL
     FOR UPDATE;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'character % not found', p_id USING ERRCODE = 'CH404';
    END IF;

    IF p_expected_version IS NOT NULL AND v_current.version <> p_expected_version THEN
        RAISE EXCEPTION 'character % version conflict (expected %, actual %)',
            p_id, p_expected_version, v_current.version
            USING ERRCODE = 'CH412';
    END IF;

    UPDATE core_characters SET
        name                = p_name,
        description         = p_description,
        race_id             = p_race_id,
        gender              = p_gender,
        birth_date          = p_birth_date,
        birth_place         = p_birth_place,
        height_cm           = p_height_cm,
        weight_kg           = p_weight_kg,
        body_fat_percentage = p_body_fat_percentage,
        size_top            = p_size_top,
        size_middle         = p_size_middle,
        size_bottom         = p_size_bottom,
        version             = version + 1   -- updated_at は trg_core_characters_set_updated_at が更新
    WHERE id = p_id
    RETURNING * INTO v_result;

    RETURN v_result;
END;
$$ LANGUAGE plpgsql;

-- 論理削除。物理 DELETE を各言語に書かせない。既に削除済み / 不在なら CH404。
--   p_expected_version が非 NULL なら update_character と同じ CH412 契約で
--   版一致を検査する（「編集中の他者が消した/消された」競合の検知に使える）。
--   省略（NULL）時は無条件削除＝既存呼び出しと後方互換。
DROP FUNCTION IF EXISTS soft_delete_character(UUID);
DROP FUNCTION IF EXISTS soft_delete_character(UUID, BIGINT);
CREATE FUNCTION soft_delete_character(
    p_id               UUID,
    p_expected_version BIGINT DEFAULT NULL
)
RETURNS core_characters AS $$
DECLARE
    v_current core_characters;
    v_result  core_characters;
BEGIN
    -- 予約パターン: 確定済み（confirmed_at IS NOT NULL）の生存行のみを対象にする。
    -- 未確定（pending）の予約は「まだ存在しない」ため update/soft_delete からは CH404 扱い。
    SELECT * INTO v_current
      FROM core_characters
     WHERE id = p_id AND deleted_at IS NULL AND confirmed_at IS NOT NULL
     FOR UPDATE;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'character % not found or already deleted', p_id
            USING ERRCODE = 'CH404';
    END IF;

    IF p_expected_version IS NOT NULL AND v_current.version <> p_expected_version THEN
        RAISE EXCEPTION 'character % version conflict (expected %, actual %)',
            p_id, p_expected_version, v_current.version
            USING ERRCODE = 'CH412';
    END IF;

    UPDATE core_characters
       SET deleted_at = NOW()
     WHERE id = p_id
     RETURNING * INTO v_result;

    RETURN v_result;
END;
$$ LANGUAGE plpgsql;

-- 予約の確定（reservation パターン）。作成直後の pending 行に所有者が紐づいた後、
-- 消費者がこれを呼んで confirmed_at を立て、可視データ（API の読み取り＝確定済み・生存行）に昇格させる。
-- 冪等: 再送（既に確定済み）でも updated_at を動かさず現状の行を返す。
-- 不在 / 論理削除済み / TTL 回収済みは CH404（消費者は確定不能を検知して自分のリンクを掃除する）。
DROP FUNCTION IF EXISTS confirm_character(UUID);
CREATE FUNCTION confirm_character(p_id UUID)
RETURNS core_characters AS $$
DECLARE
    v_result core_characters;
BEGIN
    -- 未確定のものだけ確定する（confirmed_at IS NULL）。確定できれば updated_at トリガーが動く。
    UPDATE core_characters
       SET confirmed_at = NOW()
     WHERE id = p_id AND deleted_at IS NULL AND confirmed_at IS NULL
     RETURNING * INTO v_result;

    IF FOUND THEN
        RETURN v_result;
    END IF;

    -- 既に確定済みなら冪等成功として現状の行を返す（updated_at を動かさない）。
    SELECT * INTO v_result
      FROM core_characters
     WHERE id = p_id AND deleted_at IS NULL AND confirmed_at IS NOT NULL;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'character % not found (unconfirmed reservation may have been reclaimed)', p_id
            USING ERRCODE = 'CH404';
    END IF;

    RETURN v_result;
END;
$$ LANGUAGE plpgsql;

-- 確定されなかった予約（pending）の TTL 回収。物理 DELETE するが、対象は「未確定の予約」
-- のみで、可視データ（確定済み）には一切触れない＝origin 自身の管轄での後始末。
-- 消費者が origin を削除しなくて済むようにするための関数で、API のスイーパーが定期的に呼ぶ。
-- 戻り値は削除件数（ログ・監視用）。
DROP FUNCTION IF EXISTS gc_unconfirmed_characters(INTERVAL);
CREATE FUNCTION gc_unconfirmed_characters(p_age INTERVAL)
RETURNS integer AS $$
DECLARE
    v_count integer;
BEGIN
    DELETE FROM core_characters
     WHERE confirmed_at IS NULL
       AND created_at < NOW() - p_age;
    GET DIAGNOSTICS v_count = ROW_COUNT;
    RETURN v_count;
END;
$$ LANGUAGE plpgsql;
