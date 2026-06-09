//go:build integration

// 実 PostgreSQL に対するリポジトリ統合テスト。
// 通常の `go test ./...`（fake 使用のユニットテスト）からは build tag で除外され、
// `go test -tags=integration ./internal/infrastructure/persistence/gorm/...` で実行する。
// 接続先は TEST_DB_DSN（無ければ DB_DSN）。どちらも無ければ skip。
//
// version 条件付き UPDATE・FK 違反→ErrRaceNotFound・論理削除・updated_at トリガーなど、
// fake では検証できない「実 SQL/DB 挙動」を確認する。スキーマは character-db を模した DDL を
// テスト内で適用する（character-db は別リポジトリのため）。
package gormrepo_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	chardomain "github.com/yu-be-shi/character-api/internal/domain/character"
	racedomain "github.com/yu-be-shi/character-api/internal/domain/race"
	gormrepo "github.com/yu-be-shi/character-api/internal/infrastructure/persistence/gorm"
)

const schemaDDL = `
DO $$ BEGIN
  CREATE TYPE gender_enum AS ENUM ('male','female','other','unknown');
EXCEPTION WHEN duplicate_object THEN null; END $$;

CREATE TABLE IF NOT EXISTS races (
  id UUID PRIMARY KEY,
  name VARCHAR(50) NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS core_characters (
  id UUID PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  description TEXT,
  race_id UUID NOT NULL REFERENCES races(id),
  gender gender_enum NOT NULL DEFAULT 'unknown',
  birth_date DATE,
  birth_place VARCHAR(150),
  height_cm SMALLINT,
  weight_kg SMALLINT,
  body_fat_percentage NUMERIC(4,1),
  size_top SMALLINT,
  size_middle SMALLINT,
  size_bottom SMALLINT,
  version BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ
);

CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN NEW.updated_at = NOW(); RETURN NEW; END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_set_updated_at ON core_characters;
CREATE TRIGGER trg_set_updated_at BEFORE UPDATE ON core_characters
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
`

func setupDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		dsn = os.Getenv("DB_DSN")
	}
	if dsn == "" {
		t.Skip("TEST_DB_DSN / DB_DSN 未設定のため統合テストを skip")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:         gormlogger.Default.LogMode(gormlogger.Silent),
		TranslateError: true,
	})
	require.NoError(t, err)
	require.NoError(t, db.Exec(schemaDDL).Error)
	// 各テストはクリーンな状態から始める。
	require.NoError(t, db.Exec("TRUNCATE core_characters, races CASCADE").Error)
	return db
}

func seedRaceRow(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	r, err := racedomain.New("人間")
	require.NoError(t, err)
	require.NoError(t, gormrepo.NewRaceRepository(db).Save(context.Background(), r))
	return r.ID
}

func newChar(t *testing.T, raceID uuid.UUID) *chardomain.Character {
	t.Helper()
	c, err := chardomain.New("アリス", "", raceID, chardomain.GenderFemale, time.Now())
	require.NoError(t, err)
	return c
}

func TestIntegration_SaveAndFind(t *testing.T) {
	db := setupDB(t)
	repo := gormrepo.NewCharacterRepository(db)
	raceID := seedRaceRow(t, db)

	c := newChar(t, raceID)
	h := int16(170)
	c.HeightCm = &h
	require.NoError(t, repo.Save(context.Background(), c))

	got, err := repo.FindByID(context.Background(), c.ID)
	require.NoError(t, err)
	assert.Equal(t, "アリス", got.Name)
	assert.Equal(t, "人間", got.RaceName) // JOIN で race 名が引ける
	require.NotNil(t, got.HeightCm)
	assert.EqualValues(t, 170, *got.HeightCm)
	assert.EqualValues(t, 1, got.Version)
}

func TestIntegration_Save_FKViolation(t *testing.T) {
	db := setupDB(t)
	repo := gormrepo.NewCharacterRepository(db)

	c := newChar(t, uuid.Must(uuid.NewV7())) // 存在しない race
	err := repo.Save(context.Background(), c)
	assert.ErrorIs(t, err, chardomain.ErrRaceNotFound)
}

func TestIntegration_Update_VersionAndClear(t *testing.T) {
	db := setupDB(t)
	repo := gormrepo.NewCharacterRepository(db)
	raceID := seedRaceRow(t, db)

	c := newChar(t, raceID)
	h := int16(170)
	c.HeightCm = &h
	require.NoError(t, repo.Save(context.Background(), c))

	// height をクリア（nil）し、正しい version で更新 → 成功・version +1・height NULL。
	c.HeightCm = nil
	c.Name = "アリス改"
	v1 := int64(1)
	require.NoError(t, repo.Update(context.Background(), c, &v1))

	got, err := repo.FindByID(context.Background(), c.ID)
	require.NoError(t, err)
	assert.Equal(t, "アリス改", got.Name)
	assert.Nil(t, got.HeightCm)
	assert.EqualValues(t, 2, got.Version)

	// 古い version(1) で再更新 → 競合。
	err = repo.Update(context.Background(), c, &v1)
	assert.ErrorIs(t, err, chardomain.ErrVersionConflict)
}

func TestIntegration_SoftDeleteAndCount(t *testing.T) {
	db := setupDB(t)
	repo := gormrepo.NewCharacterRepository(db)
	raceID := seedRaceRow(t, db)

	c := newChar(t, raceID)
	require.NoError(t, repo.Save(context.Background(), c))

	n, err := repo.Count(context.Background(), chardomain.ListParams{})
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)

	require.NoError(t, repo.Delete(context.Background(), c.ID))

	_, err = repo.FindByID(context.Background(), c.ID)
	assert.ErrorIs(t, err, chardomain.ErrNotFound) // 論理削除済みは見えない

	n, err = repo.Count(context.Background(), chardomain.ListParams{})
	require.NoError(t, err)
	assert.EqualValues(t, 0, n)
}

func TestIntegration_UpdatedAtTrigger(t *testing.T) {
	db := setupDB(t)
	repo := gormrepo.NewCharacterRepository(db)
	raceID := seedRaceRow(t, db)

	c := newChar(t, raceID)
	require.NoError(t, repo.Save(context.Background(), c))
	created, err := repo.FindByID(context.Background(), c.ID)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)
	// updated_at を一切セットしない生 UPDATE。トリガーが NOW() に更新するはず。
	require.NoError(t, db.Exec("UPDATE core_characters SET name = 'x' WHERE id = ?", c.ID).Error)

	after, err := repo.FindByID(context.Background(), c.ID)
	require.NoError(t, err)
	assert.True(t, after.UpdatedAt.After(created.UpdatedAt), "トリガーで updated_at が進む")
}
