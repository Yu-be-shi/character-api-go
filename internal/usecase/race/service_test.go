package race_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "github.com/yu-be-shi/character-api/internal/domain/race"
	usecase "github.com/yu-be-shi/character-api/internal/usecase/race"
)

// --- fake repository ---

type fakeRaceRepo struct {
	store     map[uuid.UUID]*domain.Race
	saveErr   error
	updateErr error
	findErr   error
	deleteErr error
}

func newFakeRaceRepo() *fakeRaceRepo {
	return &fakeRaceRepo{store: map[uuid.UUID]*domain.Race{}}
}

func (r *fakeRaceRepo) Save(_ context.Context, race *domain.Race) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	cp := *race
	r.store[race.ID] = &cp
	return nil
}

func (r *fakeRaceRepo) Update(_ context.Context, race *domain.Race) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	if _, ok := r.store[race.ID]; !ok {
		return domain.ErrNotFound
	}
	cp := *race
	r.store[race.ID] = &cp
	return nil
}

func (r *fakeRaceRepo) FindByID(_ context.Context, id uuid.UUID) (*domain.Race, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	race, ok := r.store[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *race
	return &cp, nil
}

func (r *fakeRaceRepo) List(_ context.Context) ([]*domain.Race, error) {
	out := make([]*domain.Race, 0, len(r.store))
	for _, race := range r.store {
		cp := *race
		out = append(out, &cp)
	}
	return out, nil
}

func (r *fakeRaceRepo) Delete(_ context.Context, id uuid.UUID) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	if _, ok := r.store[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.store, id)
	return nil
}

// --- tests ---

func TestService_Create(t *testing.T) {
	repo := newFakeRaceRepo()
	svc := usecase.NewService(repo)

	got, err := svc.Create(context.Background(), usecase.CreateInput{Name: "エルフ"})
	require.NoError(t, err)
	assert.Equal(t, "エルフ", got.Name)
	assert.NotEqual(t, uuid.Nil, got.ID)
	// 永続化されていること
	stored, err := repo.FindByID(context.Background(), got.ID)
	require.NoError(t, err)
	assert.Equal(t, "エルフ", stored.Name)
}

func TestService_Create_TrimsName(t *testing.T) {
	repo := newFakeRaceRepo()
	svc := usecase.NewService(repo)

	got, err := svc.Create(context.Background(), usecase.CreateInput{Name: "  ドワーフ  "})
	require.NoError(t, err)
	assert.Equal(t, "ドワーフ", got.Name)
}

func TestService_Create_InvalidName(t *testing.T) {
	repo := newFakeRaceRepo()
	svc := usecase.NewService(repo)

	t.Run("empty", func(t *testing.T) {
		_, err := svc.Create(context.Background(), usecase.CreateInput{Name: "   "})
		assert.ErrorIs(t, err, domain.ErrInvalidName)
	})
	t.Run("too long", func(t *testing.T) {
		_, err := svc.Create(context.Background(), usecase.CreateInput{Name: strings.Repeat("あ", 51)})
		assert.ErrorIs(t, err, domain.ErrInvalidName)
	})
}

func TestService_Create_DuplicatePropagates(t *testing.T) {
	repo := newFakeRaceRepo()
	repo.saveErr = domain.ErrDuplicate
	svc := usecase.NewService(repo)

	_, err := svc.Create(context.Background(), usecase.CreateInput{Name: "人間"})
	assert.ErrorIs(t, err, domain.ErrDuplicate)
}

func TestService_Update(t *testing.T) {
	repo := newFakeRaceRepo()
	svc := usecase.NewService(repo)

	created, err := svc.Create(context.Background(), usecase.CreateInput{Name: "獣人"})
	require.NoError(t, err)

	updated, err := svc.Update(context.Background(), created.ID, usecase.UpdateInput{Name: "竜族"})
	require.NoError(t, err)
	assert.Equal(t, "竜族", updated.Name)

	stored, err := repo.FindByID(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, "竜族", stored.Name)
}

func TestService_Update_NotFound(t *testing.T) {
	repo := newFakeRaceRepo()
	svc := usecase.NewService(repo)

	_, err := svc.Update(context.Background(), uuid.New(), usecase.UpdateInput{Name: "天使"})
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestService_Update_InvalidName(t *testing.T) {
	repo := newFakeRaceRepo()
	svc := usecase.NewService(repo)
	created, err := svc.Create(context.Background(), usecase.CreateInput{Name: "妖精"})
	require.NoError(t, err)

	_, err = svc.Update(context.Background(), created.ID, usecase.UpdateInput{Name: ""})
	assert.ErrorIs(t, err, domain.ErrInvalidName)
}

func TestService_Delete(t *testing.T) {
	repo := newFakeRaceRepo()
	svc := usecase.NewService(repo)
	created, err := svc.Create(context.Background(), usecase.CreateInput{Name: "ゴブリン"})
	require.NoError(t, err)

	require.NoError(t, svc.Delete(context.Background(), created.ID))

	_, err = repo.FindByID(context.Background(), created.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestService_Delete_NotFound(t *testing.T) {
	repo := newFakeRaceRepo()
	svc := usecase.NewService(repo)

	err := svc.Delete(context.Background(), uuid.New())
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestService_Delete_WrapsUnknownError(t *testing.T) {
	repo := newFakeRaceRepo()
	repo.deleteErr = errors.New("db down")
	svc := usecase.NewService(repo)

	err := svc.Delete(context.Background(), uuid.New())
	require.Error(t, err)
	assert.False(t, errors.Is(err, domain.ErrNotFound))
}
