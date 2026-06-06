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

// --- fakes ---

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

func (r *fakeCharRepo) Update(_ context.Context, c *domain.Character) error {
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

func (r *fakeCharRepo) List(_ context.Context) ([]*domain.Character, error) {
	out := make([]*domain.Character, 0, len(r.store))
	for _, c := range r.store {
		cp := *c
		out = append(out, &cp)
	}
	return out, nil
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

type fakeRaceRepo struct {
	store   map[uuid.UUID]*racedomain.Race
	findErr error
}

func newFakeRaceRepo(races ...*racedomain.Race) *fakeRaceRepo {
	r := &fakeRaceRepo{store: map[uuid.UUID]*racedomain.Race{}}
	for _, race := range races {
		cp := *race
		r.store[race.ID] = &cp
	}
	return r
}

func (r *fakeRaceRepo) Save(_ context.Context, race *racedomain.Race) error {
	cp := *race
	r.store[race.ID] = &cp
	return nil
}

func (r *fakeRaceRepo) Update(_ context.Context, race *racedomain.Race) error {
	r.store[race.ID] = race
	return nil
}

func (r *fakeRaceRepo) FindByID(_ context.Context, id uuid.UUID) (*racedomain.Race, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	race, ok := r.store[id]
	if !ok {
		return nil, racedomain.ErrNotFound
	}
	cp := *race
	return &cp, nil
}

func (r *fakeRaceRepo) List(_ context.Context) ([]*racedomain.Race, error) {
	out := make([]*racedomain.Race, 0, len(r.store))
	for _, race := range r.store {
		cp := *race
		out = append(out, &cp)
	}
	return out, nil
}

func (r *fakeRaceRepo) Delete(_ context.Context, id uuid.UUID) error {
	delete(r.store, id)
	return nil
}

// --- helpers ---

func fixedClock(t time.Time) usecase.Clock { return func() time.Time { return t } }

func seedRace(t *testing.T) *racedomain.Race {
	t.Helper()
	r, err := racedomain.New("Human")
	require.NoError(t, err)
	return r
}

func newSvc(charRepo *fakeCharRepo, raceRepo *fakeRaceRepo, clock usecase.Clock) *usecase.Service {
	return usecase.NewService(charRepo, raceRepo, clock)
}

// --- tests ---

func TestService_Create(t *testing.T) {
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	race := seedRace(t)
	raceRepo := newFakeRaceRepo(race)
	charRepo := newFakeCharRepo()
	svc := newSvc(charRepo, raceRepo, fixedClock(now))

	c, err := svc.Create(context.Background(), usecase.CreateInput{
		Name:   "Alice",
		RaceID: race.ID,
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
	race := seedRace(t)
	svc := newSvc(newFakeCharRepo(), newFakeRaceRepo(race), nil)
	_, err := svc.Create(context.Background(), usecase.CreateInput{
		Name:   "  ",
		RaceID: race.ID,
		Gender: domain.GenderUnknown,
	})
	require.ErrorIs(t, err, domain.ErrInvalidName)
}

func TestService_Create_InvalidGender(t *testing.T) {
	race := seedRace(t)
	svc := newSvc(newFakeCharRepo(), newFakeRaceRepo(race), nil)
	_, err := svc.Create(context.Background(), usecase.CreateInput{
		Name:   "Alice",
		RaceID: race.ID,
		Gender: domain.Gender("invalid"),
	})
	require.ErrorIs(t, err, domain.ErrInvalidGender)
}

func TestService_Create_UnicodeName(t *testing.T) {
	race := seedRace(t)
	svc := newSvc(newFakeCharRepo(), newFakeRaceRepo(race), nil)
	name100 := strings.Repeat("あ", 100)
	c, err := svc.Create(context.Background(), usecase.CreateInput{
		Name:   name100,
		RaceID: race.ID,
		Gender: domain.GenderUnknown,
	})
	require.NoError(t, err)
	assert.Equal(t, name100, c.Name)

	_, err = svc.Create(context.Background(), usecase.CreateInput{
		Name:   strings.Repeat("あ", 101),
		RaceID: race.ID,
		Gender: domain.GenderUnknown,
	})
	require.ErrorIs(t, err, domain.ErrInvalidName)
}

func TestService_Create_RaceNotFound(t *testing.T) {
	svc := newSvc(newFakeCharRepo(), newFakeRaceRepo(), nil)
	_, err := svc.Create(context.Background(), usecase.CreateInput{
		Name:   "Alice",
		RaceID: uuid.Must(uuid.NewV7()),
		Gender: domain.GenderFemale,
	})
	require.ErrorIs(t, err, racedomain.ErrNotFound)
}

func TestService_Get_NotFound(t *testing.T) {
	svc := newSvc(newFakeCharRepo(), newFakeRaceRepo(), nil)
	_, err := svc.Get(context.Background(), uuid.Must(uuid.NewV7()))
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestService_Update_PartialFields(t *testing.T) {
	t0 := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	race := seedRace(t)
	raceRepo := newFakeRaceRepo(race)
	charRepo := newFakeCharRepo()
	clock := &steppingClock{times: []time.Time{t0, t1}}
	svc := newSvc(charRepo, raceRepo, clock.Now)

	c, err := svc.Create(context.Background(), usecase.CreateInput{
		Name:   "Alice",
		RaceID: race.ID,
		Gender: domain.GenderFemale,
	})
	require.NoError(t, err)

	newName := "Alicia"
	updated, err := svc.Update(context.Background(), c.ID, usecase.UpdateInput{
		UpdateFields: domain.UpdateFields{Name: &newName},
	})
	require.NoError(t, err)
	assert.Equal(t, "Alicia", updated.Name)
	assert.Equal(t, t1, updated.UpdatedAt)
	assert.Equal(t, t0, updated.CreatedAt)
}

func TestService_Delete(t *testing.T) {
	race := seedRace(t)
	raceRepo := newFakeRaceRepo(race)
	charRepo := newFakeCharRepo()
	svc := newSvc(charRepo, raceRepo, nil)

	c, err := svc.Create(context.Background(), usecase.CreateInput{
		Name:   "Bob",
		RaceID: race.ID,
		Gender: domain.GenderMale,
	})
	require.NoError(t, err)
	require.NoError(t, svc.Delete(context.Background(), c.ID))
	require.ErrorIs(t, svc.Delete(context.Background(), c.ID), domain.ErrNotFound)
}

func TestService_Create_RepoError(t *testing.T) {
	race := seedRace(t)
	raceRepo := newFakeRaceRepo(race)
	charRepo := newFakeCharRepo()
	charRepo.saveErr = errors.New("boom")
	svc := newSvc(charRepo, raceRepo, nil)
	_, err := svc.Create(context.Background(), usecase.CreateInput{
		Name:   "Bob",
		RaceID: race.ID,
		Gender: domain.GenderMale,
	})
	require.Error(t, err)
}

func TestService_List(t *testing.T) {
	race := seedRace(t)
	raceRepo := newFakeRaceRepo(race)
	charRepo := newFakeCharRepo()
	svc := newSvc(charRepo, raceRepo, nil)

	for _, name := range []string{"Alice", "Bob"} {
		_, err := svc.Create(context.Background(), usecase.CreateInput{
			Name:   name,
			RaceID: race.ID,
			Gender: domain.GenderUnknown,
		})
		require.NoError(t, err)
	}

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
