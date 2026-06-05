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
	usecase "github.com/yu-be-shi/character-api/internal/usecase/character"
)

type fakeRepo struct {
	store     map[uuid.UUID]*domain.Character
	saveErr   error
	updateErr error
	findErr   error
	deleteErr error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{store: map[uuid.UUID]*domain.Character{}}
}

func (r *fakeRepo) Save(_ context.Context, c *domain.Character) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	cp := *c
	r.store[c.ID] = &cp
	return nil
}

func (r *fakeRepo) Update(_ context.Context, c *domain.Character) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	if _, ok := r.store[c.ID]; !ok {
		return domain.ErrNotFound
	}
	cp := *c
	r.store[c.ID] = &cp
	return nil
}

func (r *fakeRepo) FindByID(_ context.Context, id uuid.UUID) (*domain.Character, error) {
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

func (r *fakeRepo) List(_ context.Context) ([]*domain.Character, error) {
	out := make([]*domain.Character, 0, len(r.store))
	for _, c := range r.store {
		cp := *c
		out = append(out, &cp)
	}
	return out, nil
}

func (r *fakeRepo) Delete(_ context.Context, id uuid.UUID) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	if _, ok := r.store[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.store, id)
	return nil
}

func fixedClock(t time.Time) usecase.Clock { return func() time.Time { return t } }

func TestService_Create(t *testing.T) {
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	svc := usecase.NewService(repo, fixedClock(now))

	c, err := svc.Create(context.Background(), usecase.CreateInput{
		Name:       "Alice",
		Attributes: domain.Attributes{"role": "mage"},
	})
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.Equal(t, "Alice", c.Name)
	assert.Equal(t, "mage", c.Attributes["role"])
	assert.Equal(t, now, c.CreatedAt)
	assert.NotEqual(t, uuid.Nil, c.ID)
}

func TestService_Create_InvalidName(t *testing.T) {
	svc := usecase.NewService(newFakeRepo(), nil)
	_, err := svc.Create(context.Background(), usecase.CreateInput{Name: "  "})
	require.ErrorIs(t, err, domain.ErrInvalidName)
}

func TestService_Create_UnicodeName(t *testing.T) {
	svc := usecase.NewService(newFakeRepo(), nil)
	name120 := strings.Repeat("あ", 120)
	c, err := svc.Create(context.Background(), usecase.CreateInput{Name: name120})
	require.NoError(t, err)
	assert.Equal(t, name120, c.Name)

	_, err = svc.Create(context.Background(), usecase.CreateInput{Name: strings.Repeat("あ", 121)})
	require.ErrorIs(t, err, domain.ErrInvalidName)
}

func TestService_Get_NotFound(t *testing.T) {
	svc := usecase.NewService(newFakeRepo(), nil)
	_, err := svc.Get(context.Background(), uuid.Must(uuid.NewV7()))
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestService_Update_PartialFields(t *testing.T) {
	t0 := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	repo := newFakeRepo()
	clock := &steppingClock{times: []time.Time{t0, t1}}
	svc := usecase.NewService(repo, clock.Now)

	c, err := svc.Create(context.Background(), usecase.CreateInput{Name: "Alice"})
	require.NoError(t, err)

	newName := "Alicia"
	updated, err := svc.Update(context.Background(), c.ID, usecase.UpdateInput{Name: &newName})
	require.NoError(t, err)
	assert.Equal(t, "Alicia", updated.Name)
	assert.Equal(t, t1, updated.UpdatedAt)
	assert.Equal(t, t0, updated.CreatedAt)
}

func TestService_Delete(t *testing.T) {
	repo := newFakeRepo()
	svc := usecase.NewService(repo, nil)

	c, err := svc.Create(context.Background(), usecase.CreateInput{Name: "Bob"})
	require.NoError(t, err)
	require.NoError(t, svc.Delete(context.Background(), c.ID))
	require.ErrorIs(t, svc.Delete(context.Background(), c.ID), domain.ErrNotFound)
}

func TestService_Create_RepoError(t *testing.T) {
	repo := newFakeRepo()
	repo.saveErr = errors.New("boom")
	svc := usecase.NewService(repo, nil)
	_, err := svc.Create(context.Background(), usecase.CreateInput{Name: "Bob"})
	require.Error(t, err)
}

func TestService_List(t *testing.T) {
	repo := newFakeRepo()
	svc := usecase.NewService(repo, nil)

	_, err := svc.Create(context.Background(), usecase.CreateInput{Name: "Alice"})
	require.NoError(t, err)
	_, err = svc.Create(context.Background(), usecase.CreateInput{Name: "Bob"})
	require.NoError(t, err)

	cs, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, cs, 2)
}

type steppingClock struct {
	times []time.Time
	i     int
}

func (s *steppingClock) Now() time.Time {
	t := s.times[s.i]
	if s.i < len(s.times)-1 {
		s.i++
	}
	return t
}
