package character_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "github.com/yu-be-shi/character-api/internal/domain/character"
	racedomain "github.com/yu-be-shi/character-api/internal/domain/race"
	usecase "github.com/yu-be-shi/character-api/internal/usecase/character"
)

// --- fake repository（character サービスは race リポジトリに依存しない。
// race 不在は Save が domain.ErrRaceNotFound を返すことで表現する）---

type fakeCharRepo struct {
	store     map[uuid.UUID]*domain.Character
	saveErr   error
	updateErr error
	findErr   error
	deleteErr error
}

func newFakeCharRepo() *fakeCharRepo {
	return &fakeCharRepo{store: map[uuid.UUID]*domain.Character{}}
}

func (r *fakeCharRepo) Save(_ context.Context, c *domain.Character) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	cp := *c
	r.store[c.ID] = &cp
	return nil
}

func (r *fakeCharRepo) Update(_ context.Context, c *domain.Character, expectedVersion *int64) (*domain.Character, error) {
	if r.updateErr != nil {
		return nil, r.updateErr
	}
	cur, ok := r.store[c.ID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	if expectedVersion != nil && cur.Version != *expectedVersion {
		return nil, domain.ErrVersionConflict
	}
	cp := *c
	cp.Version = cur.Version + 1 // DB 関数（update_character）相当でバージョンを増やす
	r.store[c.ID] = &cp
	out := cp
	return &out, nil
}

func (r *fakeCharRepo) FindByID(_ context.Context, id uuid.UUID) (*domain.Character, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	c, ok := r.store[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *c
	return &cp, nil
}

func (r *fakeCharRepo) matched(p domain.ListParams) []*domain.Character {
	want := map[uuid.UUID]bool{}
	for _, id := range p.IDs {
		want[id] = true
	}
	out := make([]*domain.Character, 0, len(r.store))
	for _, c := range r.store {
		if len(p.IDs) > 0 && !want[c.ID] {
			continue
		}
		cp := *c
		out = append(out, &cp)
	}
	return out
}

func (r *fakeCharRepo) List(_ context.Context, p domain.ListParams) ([]*domain.Character, error) {
	if p.IDs != nil && len(p.IDs) == 0 {
		return []*domain.Character{}, nil
	}
	return r.matched(p), nil
}

func (r *fakeCharRepo) Count(_ context.Context, p domain.ListParams) (int64, error) {
	if p.IDs != nil && len(p.IDs) == 0 {
		return 0, nil
	}
	return int64(len(r.matched(p))), nil
}

func (r *fakeCharRepo) Delete(_ context.Context, id uuid.UUID) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	if _, ok := r.store[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.store, id)
	return nil
}

// --- helpers ---

func fixedClock(t time.Time) usecase.Clock { return func() time.Time { return t } }

func newRaceID(t *testing.T) uuid.UUID {
	t.Helper()
	r, err := racedomain.New("Human")
	require.NoError(t, err)
	return r.ID
}

func newSvc(charRepo *fakeCharRepo, clock usecase.Clock) *usecase.Service {
	return usecase.NewService(charRepo, clock)
}

func int16p(v int16) *int16 { return &v }

// --- tests ---

func TestService_Create(t *testing.T) {
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	raceID := newRaceID(t)
	svc := newSvc(newFakeCharRepo(), fixedClock(now))

	c, err := svc.Create(context.Background(), usecase.CreateInput{
		Name:   "Alice",
		RaceID: raceID,
		Gender: domain.GenderFemale,
	})
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.Equal(t, "Alice", c.Name)
	assert.Equal(t, domain.GenderFemale, c.Gender)
	assert.Equal(t, now, c.CreatedAt)
	assert.NotEqual(t, uuid.Nil, c.ID)
}

func TestService_Create_InvalidName(t *testing.T) {
	svc := newSvc(newFakeCharRepo(), nil)
	_, err := svc.Create(context.Background(), usecase.CreateInput{
		Name:   "  ",
		RaceID: newRaceID(t),
		Gender: domain.GenderUnknown,
	})
	require.ErrorIs(t, err, domain.ErrInvalidName)
}

func TestService_Create_InvalidGender(t *testing.T) {
	svc := newSvc(newFakeCharRepo(), nil)
	_, err := svc.Create(context.Background(), usecase.CreateInput{
		Name:   "Alice",
		RaceID: newRaceID(t),
		Gender: domain.Gender("invalid"),
	})
	require.ErrorIs(t, err, domain.ErrInvalidGender)
}

func TestService_Create_UnicodeName(t *testing.T) {
	svc := newSvc(newFakeCharRepo(), nil)
	name100 := strings.Repeat("あ", 100)
	c, err := svc.Create(context.Background(), usecase.CreateInput{
		Name:   name100,
		RaceID: newRaceID(t),
		Gender: domain.GenderUnknown,
	})
	require.NoError(t, err)
	assert.Equal(t, name100, c.Name)

	_, err = svc.Create(context.Background(), usecase.CreateInput{
		Name:   strings.Repeat("あ", 101),
		RaceID: newRaceID(t),
		Gender: domain.GenderUnknown,
	})
	require.ErrorIs(t, err, domain.ErrInvalidName)
}

// race 不在は永続化時の FK 違反（repo が domain.ErrRaceNotFound を返す）として表現される。
func TestService_Create_RaceNotFound(t *testing.T) {
	charRepo := newFakeCharRepo()
	charRepo.saveErr = domain.ErrRaceNotFound
	svc := newSvc(charRepo, nil)
	_, err := svc.Create(context.Background(), usecase.CreateInput{
		Name:   "Alice",
		RaceID: uuid.Must(uuid.NewV7()),
		Gender: domain.GenderFemale,
	})
	require.ErrorIs(t, err, domain.ErrRaceNotFound)
}

func TestService_Get_NotFound(t *testing.T) {
	svc := newSvc(newFakeCharRepo(), nil)
	_, err := svc.Get(context.Background(), uuid.Must(uuid.NewV7()))
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestService_Update_PartialFields(t *testing.T) {
	t0 := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	charRepo := newFakeCharRepo()
	svc := newSvc(charRepo, fixedClock(t0))

	c, err := svc.Create(context.Background(), usecase.CreateInput{
		Name:   "Alice",
		RaceID: newRaceID(t),
		Gender: domain.GenderFemale,
	})
	require.NoError(t, err)

	newName := "Alicia"
	updated, err := svc.Update(context.Background(), c.ID, usecase.UpdateInput{
		UpdateFields: domain.UpdateFields{Name: &newName},
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, "Alicia", updated.Name)
	assert.Equal(t, t0, updated.CreatedAt)        // created_at は変わらない
	assert.Equal(t, c.Version+1, updated.Version) // 更新で版が +1 される（DB 確定値）
	// updated_at は DB トリガーが確定する値のため、fake では検証しない（統合テストで担保）。
}

func TestService_Update_VersionConflict(t *testing.T) {
	charRepo := newFakeCharRepo()
	svc := newSvc(charRepo, nil)
	c, err := svc.Create(context.Background(), usecase.CreateInput{
		Name:   "Alice",
		RaceID: newRaceID(t),
		Gender: domain.GenderFemale,
	})
	require.NoError(t, err)

	wrong := c.Version + 99 // 古い/誤った版を指定
	name := "X"
	_, err = svc.Update(context.Background(), c.ID, usecase.UpdateInput{
		UpdateFields: domain.UpdateFields{Name: &name},
	}, &wrong)
	require.ErrorIs(t, err, domain.ErrVersionConflict)
}

// PUT（全置換）は送られなかった任意項目をクリアする。
func TestService_Replace_ClearsOmittedFields(t *testing.T) {
	charRepo := newFakeCharRepo()
	svc := newSvc(charRepo, nil)
	raceID := newRaceID(t)

	created, err := svc.Create(context.Background(), usecase.CreateInput{
		Name:     "Alice",
		RaceID:   raceID,
		Gender:   domain.GenderFemale,
		HeightCm: int16p(170),
	})
	require.NoError(t, err)
	require.NotNil(t, created.HeightCm)

	// HeightCm を省略して全置換 → クリアされる。
	replaced, err := svc.Replace(context.Background(), created.ID, usecase.CreateInput{
		Name:   "Alice2",
		RaceID: raceID,
		Gender: domain.GenderFemale,
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, "Alice2", replaced.Name)
	assert.Nil(t, replaced.HeightCm)
	assert.Equal(t, created.CreatedAt, replaced.CreatedAt)
	assert.Equal(t, created.Version+1, replaced.Version)
}

func TestService_Replace_NotFound(t *testing.T) {
	svc := newSvc(newFakeCharRepo(), nil)
	_, err := svc.Replace(context.Background(), uuid.Must(uuid.NewV7()), usecase.CreateInput{
		Name:   "X",
		RaceID: newRaceID(t),
		Gender: domain.GenderUnknown,
	}, nil)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestService_Delete(t *testing.T) {
	charRepo := newFakeCharRepo()
	svc := newSvc(charRepo, nil)

	c, err := svc.Create(context.Background(), usecase.CreateInput{
		Name:   "Bob",
		RaceID: newRaceID(t),
		Gender: domain.GenderMale,
	})
	require.NoError(t, err)
	require.NoError(t, svc.Delete(context.Background(), c.ID))
	require.ErrorIs(t, svc.Delete(context.Background(), c.ID), domain.ErrNotFound)
}

func TestService_Create_RepoError(t *testing.T) {
	charRepo := newFakeCharRepo()
	charRepo.saveErr = errors.New("boom")
	svc := newSvc(charRepo, nil)
	_, err := svc.Create(context.Background(), usecase.CreateInput{
		Name:   "Bob",
		RaceID: newRaceID(t),
		Gender: domain.GenderMale,
	})
	require.Error(t, err)
}

func TestService_List_ReturnsItemsAndTotal(t *testing.T) {
	charRepo := newFakeCharRepo()
	svc := newSvc(charRepo, nil)
	raceID := newRaceID(t)

	for _, name := range []string{"Alice", "Bob"} {
		_, err := svc.Create(context.Background(), usecase.CreateInput{
			Name:   name,
			RaceID: raceID,
			Gender: domain.GenderUnknown,
		})
		require.NoError(t, err)
	}

	cs, total, err := svc.List(context.Background(), domain.ListParams{})
	require.NoError(t, err)
	assert.Len(t, cs, 2)
	assert.Equal(t, int64(2), total)
}
