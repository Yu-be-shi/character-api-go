//go:build integration

// 実 PostgreSQL に対するリポジトリ統合テスト。
// 通常の `go test ./...`（fake 使用のユニットテスト）からは build tag で除外され、
// `go test -tags=integration ./internal/infrastructure/persistence/postgres/...` で実行する。
// 接続先は TEST_DB_DSN（無ければ DB_DSN）。どちらも無ければ skip。
//
// スキーマは character-db を submodule で固定した third_party/ の実ファイル
// （schema.sql・views/00_set_updated_at.sql・views/10_character_write_functions.sql）を
// そのまま適用する。これにより楽観ロック・論理削除を担う DB 関数まで含めて本番同等で検証する。
package postgres_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	chardomain "github.com/yu-be-shi/character-api/internal/domain/character"
	racedomain "github.com/yu-be-shi/character-api/internal/domain/race"
	pgrepo "github.com/yu-be-shi/character-api/internal/infrastructure/persistence/postgres"
)

// repoRoot はこのテストファイルからリポジトリルートを求める（cwd に依存しない）。
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	// internal/infrastructure/persistence/postgres/ から 4 つ上がルート。
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..")
}

func setupPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		dsn = os.Getenv("DB_DSN")
	}
	if dsn == "" {
		t.Skip("TEST_DB_DSN / DB_DSN 未設定のため統合テストを skip")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	// 各テストはクリーンな public スキーマから始める。
	_, err = pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
	require.NoError(t, err)

	root := repoRoot(t)
	for _, rel := range []string{
		"third_party/character-db/schema.sql",
		"third_party/character-db/views/00_set_updated_at.sql",
		"third_party/character-db/views/10_character_write_functions.sql",
	} {
		sqlBytes, err := os.ReadFile(filepath.Join(root, rel))
		require.NoError(t, err, "submodule の SQL を読めること（git submodule update --init 済みか）: %s", rel)
		_, err = pool.Exec(ctx, string(sqlBytes))
		require.NoError(t, err, "apply %s", rel)
	}
	return pool
}

func seedRaceRow(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	r, err := racedomain.New("人間")
	require.NoError(t, err)
	require.NoError(t, pgrepo.NewRaceRepository(pool).Save(context.Background(), r))
	return r.ID
}

func newChar(t *testing.T, raceID uuid.UUID) *chardomain.Character {
	t.Helper()
	c, err := chardomain.New("アリス", "", raceID, chardomain.GenderFemale, time.Now())
	require.NoError(t, err)
	return c
}

func TestIntegration_SaveAndFind(t *testing.T) {
	pool := setupPool(t)
	repo := pgrepo.NewCharacterRepository(pool)
	raceID := seedRaceRow(t, pool)

	c := newChar(t, raceID)
	h := int16(170)
	c.HeightCm = &h
	bf := float32(12.5)
	c.BodyFat = &bf
	require.NoError(t, repo.Save(context.Background(), c))

	got, err := repo.FindByID(context.Background(), c.ID)
	require.NoError(t, err)
	assert.Equal(t, "アリス", got.Name)
	assert.Equal(t, "人間", got.RaceName) // JOIN で race 名が引ける
	require.NotNil(t, got.HeightCm)
	assert.EqualValues(t, 170, *got.HeightCm)
	require.NotNil(t, got.BodyFat)
	assert.InDelta(t, 12.5, *got.BodyFat, 0.001) // NUMERIC ⇄ float32 往復
	assert.EqualValues(t, 1, got.Version)

	// 空文字の任意テキスト項目は NULL に正規化されて保存される（'' と NULL を混在させない）。
	var descIsNull bool
	require.NoError(t, pool.QueryRow(context.Background(),
		"SELECT description IS NULL FROM core_characters WHERE id = $1", c.ID).Scan(&descIsNull))
	assert.True(t, descIsNull)
}

func TestIntegration_Save_FKViolation(t *testing.T) {
	pool := setupPool(t)
	repo := pgrepo.NewCharacterRepository(pool)

	c := newChar(t, uuid.Must(uuid.NewV7())) // 存在しない race
	err := repo.Save(context.Background(), c)
	assert.ErrorIs(t, err, chardomain.ErrRaceNotFound)
}

func TestIntegration_Update_VersionAndClear(t *testing.T) {
	pool := setupPool(t)
	repo := pgrepo.NewCharacterRepository(pool)
	raceID := seedRaceRow(t, pool)

	c := newChar(t, raceID)
	h := int16(170)
	c.HeightCm = &h
	require.NoError(t, repo.Save(context.Background(), c))

	// height をクリア（nil）し、正しい version で更新 → 成功・version +1・height NULL。
	// Update は update_character の RETURNING（更新後の行）をそのまま返す。
	c.HeightCm = nil
	c.Name = "アリス改"
	v1 := int64(1)
	updated, err := repo.Update(context.Background(), c, &v1)
	require.NoError(t, err)
	assert.Equal(t, "アリス改", updated.Name)
	assert.Nil(t, updated.HeightCm)
	assert.EqualValues(t, 2, updated.Version)
	assert.Equal(t, "人間", updated.RaceName) // RETURNING 行にも race 名が JOIN される

	// 古い version(1) で再更新 → 競合（CH412 → ErrVersionConflict）。
	_, err = repo.Update(context.Background(), c, &v1)
	assert.ErrorIs(t, err, chardomain.ErrVersionConflict)
}

func TestIntegration_Update_NotFound(t *testing.T) {
	pool := setupPool(t)
	repo := pgrepo.NewCharacterRepository(pool)
	raceID := seedRaceRow(t, pool)

	c := newChar(t, raceID) // 未保存（存在しない）
	_, err := repo.Update(context.Background(), c, nil)
	assert.ErrorIs(t, err, chardomain.ErrNotFound)
}

func TestIntegration_SoftDeleteAndCount(t *testing.T) {
	pool := setupPool(t)
	repo := pgrepo.NewCharacterRepository(pool)
	raceID := seedRaceRow(t, pool)

	c := newChar(t, raceID)
	require.NoError(t, repo.Save(context.Background(), c))

	n, err := repo.Count(context.Background(), chardomain.ListParams{})
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)

	require.NoError(t, repo.Delete(context.Background(), c.ID))

	_, err = repo.FindByID(context.Background(), c.ID)
	assert.ErrorIs(t, err, chardomain.ErrNotFound) // 論理削除済みは見えない

	// 再削除は CH404 → ErrNotFound。
	err = repo.Delete(context.Background(), c.ID)
	assert.ErrorIs(t, err, chardomain.ErrNotFound)

	n, err = repo.Count(context.Background(), chardomain.ListParams{})
	require.NoError(t, err)
	assert.EqualValues(t, 0, n)
}

func TestIntegration_UpdatedAtTrigger(t *testing.T) {
	pool := setupPool(t)
	repo := pgrepo.NewCharacterRepository(pool)
	raceID := seedRaceRow(t, pool)

	c := newChar(t, raceID)
	require.NoError(t, repo.Save(context.Background(), c))
	created, err := repo.FindByID(context.Background(), c.ID)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)
	// updated_at を一切セットしない生 UPDATE。トリガーが NOW() に更新するはず。
	_, err = pool.Exec(context.Background(), "UPDATE core_characters SET name = 'x' WHERE id = $1", c.ID)
	require.NoError(t, err)

	after, err := repo.FindByID(context.Background(), c.ID)
	require.NoError(t, err)
	assert.True(t, after.UpdatedAt.After(created.UpdatedAt), "トリガーで updated_at が進む")
}

func TestIntegration_RaceInUse(t *testing.T) {
	pool := setupPool(t)
	charRepo := pgrepo.NewCharacterRepository(pool)
	raceRepo := pgrepo.NewRaceRepository(pool)
	raceID := seedRaceRow(t, pool)

	c := newChar(t, raceID)
	require.NoError(t, charRepo.Save(context.Background(), c))

	// character から参照中の race は削除できない（FK 違反 → ErrInUse）。
	err := raceRepo.Delete(context.Background(), raceID)
	assert.ErrorIs(t, err, racedomain.ErrInUse)
}
