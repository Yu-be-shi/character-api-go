package httpiface_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yu-be-shi/character-api/internal/config"
	chardomain "github.com/yu-be-shi/character-api/internal/domain/character"
	racedomain "github.com/yu-be-shi/character-api/internal/domain/race"
	httpiface "github.com/yu-be-shi/character-api/internal/interfaces/http"
	charusecase "github.com/yu-be-shi/character-api/internal/usecase/character"
	raceusecase "github.com/yu-be-shi/character-api/internal/usecase/race"
)

const testAPIKey = "test-internal-key"

// --- fakes（永続化はインメモリ。ハンドラ→usecase→domain の経路を実機に近い形で検証する）---

type fakeCharRepo struct {
	store map[uuid.UUID]*chardomain.Character
}

func (r *fakeCharRepo) Save(_ context.Context, c *chardomain.Character) error {
	r.store[c.ID] = c
	return nil
}
func (r *fakeCharRepo) Update(_ context.Context, c *chardomain.Character) error {
	if _, ok := r.store[c.ID]; !ok {
		return chardomain.ErrNotFound
	}
	r.store[c.ID] = c
	return nil
}
func (r *fakeCharRepo) FindByID(_ context.Context, id uuid.UUID) (*chardomain.Character, error) {
	c, ok := r.store[id]
	if !ok {
		return nil, chardomain.ErrNotFound
	}
	return c, nil
}
func (r *fakeCharRepo) List(_ context.Context, _ chardomain.ListParams) ([]*chardomain.Character, error) {
	out := make([]*chardomain.Character, 0, len(r.store))
	for _, c := range r.store {
		out = append(out, c)
	}
	return out, nil
}
func (r *fakeCharRepo) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := r.store[id]; !ok {
		return chardomain.ErrNotFound
	}
	delete(r.store, id)
	return nil
}

type fakeRaceRepo struct {
	store map[uuid.UUID]*racedomain.Race
}

func (r *fakeRaceRepo) Save(_ context.Context, race *racedomain.Race) error {
	r.store[race.ID] = race
	return nil
}
func (r *fakeRaceRepo) Update(_ context.Context, race *racedomain.Race) error { return nil }
func (r *fakeRaceRepo) FindByID(_ context.Context, id uuid.UUID) (*racedomain.Race, error) {
	race, ok := r.store[id]
	if !ok {
		return nil, racedomain.ErrNotFound
	}
	return race, nil
}
func (r *fakeRaceRepo) List(_ context.Context) ([]*racedomain.Race, error) {
	out := make([]*racedomain.Race, 0, len(r.store))
	for _, race := range r.store {
		out = append(out, race)
	}
	return out, nil
}
func (r *fakeRaceRepo) Delete(_ context.Context, id uuid.UUID) error {
	delete(r.store, id)
	return nil
}

// newTestServer はルーターと、seed したテスト用 race の ID を返す。
func newTestServer(t *testing.T) (*httptest.Server, uuid.UUID) {
	t.Helper()
	charRepo := &fakeCharRepo{store: map[uuid.UUID]*chardomain.Character{}}
	raceRepo := &fakeRaceRepo{store: map[uuid.UUID]*racedomain.Race{}}

	race, err := racedomain.New("人間")
	require.NoError(t, err)
	raceRepo.store[race.ID] = race

	charSvc := charusecase.NewService(charRepo, raceRepo, nil)
	raceSvc := raceusecase.NewService(raceRepo)

	cfg := config.Config{
		InternalAPIKey: testAPIKey,
		CORSOrigins:    []string{"http://localhost:3000"},
	}
	e := httpiface.New(cfg, charSvc, raceSvc, nil)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	return srv, race.ID
}

func do(t *testing.T, srv *httptest.Server, method, path, key, body string) *http.Response {
	t.Helper()
	var r *strings.Reader
	if body != "" {
		r = strings.NewReader(body)
	} else {
		r = strings.NewReader("")
	}
	req, err := http.NewRequest(method, srv.URL+path, r)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("X-Internal-API-Key", key)
	}
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

func TestHealthz_NoAuthRequired(t *testing.T) {
	srv, _ := newTestServer(t)
	res := do(t, srv, http.MethodGet, "/healthz", "", "")
	assert.Equal(t, http.StatusOK, res.StatusCode)
}

func TestAPIKey_RejectsMissingAndWrong(t *testing.T) {
	srv, _ := newTestServer(t)

	t.Run("missing", func(t *testing.T) {
		res := do(t, srv, http.MethodGet, "/api/v1/characters", "", "")
		assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})
	t.Run("wrong", func(t *testing.T) {
		res := do(t, srv, http.MethodGet, "/api/v1/characters", "nope", "")
		assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})
	t.Run("valid", func(t *testing.T) {
		res := do(t, srv, http.MethodGet, "/api/v1/characters", testAPIKey, "")
		assert.Equal(t, http.StatusOK, res.StatusCode)
	})
}

func TestGetCharacter_InvalidUUID(t *testing.T) {
	srv, _ := newTestServer(t)
	res := do(t, srv, http.MethodGet, "/api/v1/characters/not-a-uuid", testAPIKey, "")
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
}

func TestListCharacters_InvalidIDsParam(t *testing.T) {
	srv, _ := newTestServer(t)
	res := do(t, srv, http.MethodGet, "/api/v1/characters?ids=not-a-uuid", testAPIKey, "")
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
}

func TestGetCharacter_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	res := do(t, srv, http.MethodGet, "/api/v1/characters/"+uuid.NewString(), testAPIKey, "")
	assert.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestCreateCharacter_Success(t *testing.T) {
	srv, raceID := newTestServer(t)
	body := `{"name":"アリス","raceId":"` + raceID.String() + `","gender":"female"}`
	res := do(t, srv, http.MethodPost, "/api/v1/characters", testAPIKey, body)
	assert.Equal(t, http.StatusCreated, res.StatusCode)
}

func TestCreateCharacter_RaceNotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	body := `{"name":"アリス","raceId":"` + uuid.NewString() + `","gender":"female"}`
	res := do(t, srv, http.MethodPost, "/api/v1/characters", testAPIKey, body)
	assert.Equal(t, http.StatusUnprocessableEntity, res.StatusCode)
}

func TestCreateCharacter_InvalidGender(t *testing.T) {
	srv, raceID := newTestServer(t)
	body := `{"name":"アリス","raceId":"` + raceID.String() + `","gender":"alien"}`
	res := do(t, srv, http.MethodPost, "/api/v1/characters", testAPIKey, body)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
}

func TestCreateCharacter_MissingName(t *testing.T) {
	srv, raceID := newTestServer(t)
	body := `{"raceId":"` + raceID.String() + `","gender":"female"}`
	res := do(t, srv, http.MethodPost, "/api/v1/characters", testAPIKey, body)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
}
