package httpiface_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	store      map[uuid.UUID]*chardomain.Character
	raceValid  func(uuid.UUID) bool  // race の存在を模す（FK 違反 → ErrRaceNotFound）
	lastParams chardomain.ListParams // List に渡されたパラメータの検証用
}

func (r *fakeCharRepo) Save(_ context.Context, c *chardomain.Character) error {
	if r.raceValid != nil && !r.raceValid(c.RaceID) {
		return chardomain.ErrRaceNotFound
	}
	r.store[c.ID] = c
	return nil
}
func (r *fakeCharRepo) Confirm(_ context.Context, id uuid.UUID) (*chardomain.Character, error) {
	c, ok := r.store[id]
	if !ok {
		return nil, chardomain.ErrNotFound
	}
	return c, nil
}
func (r *fakeCharRepo) GC(_ context.Context, _ time.Duration) (int, error) { return 0, nil }
func (r *fakeCharRepo) Update(_ context.Context, c *chardomain.Character, expectedVersion *int64) (*chardomain.Character, error) {
	cur, ok := r.store[c.ID]
	if !ok {
		return nil, chardomain.ErrNotFound
	}
	if r.raceValid != nil && !r.raceValid(c.RaceID) {
		return nil, chardomain.ErrRaceNotFound
	}
	if expectedVersion != nil && cur.Version != *expectedVersion {
		return nil, chardomain.ErrVersionConflict
	}
	cp := *c
	cp.Version = cur.Version + 1
	r.store[c.ID] = &cp
	out := cp
	return &out, nil
}
func (r *fakeCharRepo) FindByID(_ context.Context, id uuid.UUID) (*chardomain.Character, error) {
	c, ok := r.store[id]
	if !ok {
		return nil, chardomain.ErrNotFound
	}
	return c, nil
}
func (r *fakeCharRepo) List(_ context.Context, p chardomain.ListParams) ([]*chardomain.Character, error) {
	r.lastParams = p
	out := make([]*chardomain.Character, 0, len(r.store))
	for _, c := range r.store {
		out = append(out, c)
	}
	return out, nil
}

func (r *fakeCharRepo) Count(_ context.Context, _ chardomain.ListParams) (int64, error) {
	return int64(len(r.store)), nil
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
	inUse func(uuid.UUID) bool // character から参照中かを模す（FK 違反 → ErrInUse）
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
	if r.inUse != nil && r.inUse(id) {
		return racedomain.ErrInUse
	}
	delete(r.store, id)
	return nil
}

// newTestServer はルーターと、seed したテスト用 race の ID を返す。
func newTestServer(t *testing.T) (*httptest.Server, uuid.UUID) {
	srv, raceID, _ := newTestServerWithRepo(t)
	return srv, raceID
}

func newTestServerWithRepo(t *testing.T) (*httptest.Server, uuid.UUID, *fakeCharRepo) {
	t.Helper()
	charRepo := &fakeCharRepo{store: map[uuid.UUID]*chardomain.Character{}}
	raceRepo := &fakeRaceRepo{store: map[uuid.UUID]*racedomain.Race{}}

	race, err := racedomain.New("人間")
	require.NoError(t, err)
	raceRepo.store[race.ID] = race

	// race の存在/参照中を2つの store を突き合わせて模す。
	charRepo.raceValid = func(id uuid.UUID) bool { _, ok := raceRepo.store[id]; return ok }
	raceRepo.inUse = func(id uuid.UUID) bool {
		for _, c := range charRepo.store {
			if c.RaceID == id {
				return true
			}
		}
		return false
	}

	charSvc := charusecase.NewService(charRepo, nil)
	raceSvc := raceusecase.NewService(raceRepo)

	cfg := config.Config{
		InternalAPIKey: testAPIKey,
		CORSOrigins:    []string{"http://localhost:3000"},
	}
	e := httpiface.New(cfg, charSvc, raceSvc, nil, nil)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	return srv, race.ID, charRepo
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

// createCharacter は POST でキャラを作り、生成された id を返すヘルパー。
func createCharacter(t *testing.T, srv *httptest.Server, raceID uuid.UUID) string {
	t.Helper()
	body := `{"name":"アリス","raceId":"` + raceID.String() + `","gender":"female","heightCm":170}`
	res := do(t, srv, http.MethodPost, "/api/v1/characters", testAPIKey, body)
	require.Equal(t, http.StatusCreated, res.StatusCode)
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
	require.NotEmpty(t, created.ID)
	return created.ID
}

func TestListCharacters_ReturnsItemsAndTotal(t *testing.T) {
	srv, raceID := newTestServer(t)
	createCharacter(t, srv, raceID)

	res := do(t, srv, http.MethodGet, "/api/v1/characters", testAPIKey, "")
	require.Equal(t, http.StatusOK, res.StatusCode)
	var out struct {
		Items []map[string]any `json:"items"`
		Total int64            `json:"total"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&out))
	assert.Len(t, out.Items, 1)
	assert.Equal(t, int64(1), out.Total)
}

func TestReplaceCharacter_PUT(t *testing.T) {
	srv, raceID := newTestServer(t)
	id := createCharacter(t, srv, raceID)

	// heightCm を省略して PUT（全置換）→ 200。クリアされる挙動は usecase テストで検証済み。
	body := `{"name":"アリス改","raceId":"` + raceID.String() + `","gender":"female"}`
	res := do(t, srv, http.MethodPut, "/api/v1/characters/"+id, testAPIKey, body)
	assert.Equal(t, http.StatusOK, res.StatusCode)
}

func TestPatchCharacter(t *testing.T) {
	srv, raceID := newTestServer(t)
	id := createCharacter(t, srv, raceID)

	body := `{"name":"アリス部分更新"}`
	res := do(t, srv, http.MethodPatch, "/api/v1/characters/"+id, testAPIKey, body)
	assert.Equal(t, http.StatusOK, res.StatusCode)
}

func TestReplaceCharacter_RaceNotFound(t *testing.T) {
	srv, raceID := newTestServer(t)
	id := createCharacter(t, srv, raceID)

	body := `{"name":"アリス","raceId":"` + uuid.NewString() + `","gender":"female"}`
	res := do(t, srv, http.MethodPut, "/api/v1/characters/"+id, testAPIKey, body)
	assert.Equal(t, http.StatusUnprocessableEntity, res.StatusCode)
}

func TestDeleteRace_InUse_Conflict(t *testing.T) {
	srv, raceID := newTestServer(t)
	createCharacter(t, srv, raceID) // この race を使用中にする

	res := do(t, srv, http.MethodDelete, "/api/v1/races/"+raceID.String(), testAPIKey, "")
	assert.Equal(t, http.StatusConflict, res.StatusCode)
}

// putWithIfMatch は If-Match 付きで PUT する。
func putWithIfMatch(t *testing.T, srv *httptest.Server, path, ifMatch, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, srv.URL+path, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-API-Key", testAPIKey)
	req.Header.Set("If-Match", ifMatch)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

func TestReplaceCharacter_VersionConflict(t *testing.T) {
	srv, raceID := newTestServer(t)
	id := createCharacter(t, srv, raceID) // 版は 1

	body := `{"name":"x","raceId":"` + raceID.String() + `","gender":"female"}`
	res := putWithIfMatch(t, srv, "/api/v1/characters/"+id, `"999"`, body)
	assert.Equal(t, http.StatusPreconditionFailed, res.StatusCode)
}

func TestBodyLimit_RejectsLargeBody(t *testing.T) {
	srv, raceID := newTestServer(t)
	big := strings.Repeat("a", (1<<20)+1024) // 1MB 超
	body := `{"name":"` + big + `","raceId":"` + raceID.String() + `","gender":"female"}`
	res := do(t, srv, http.MethodPost, "/api/v1/characters", testAPIKey, body)
	assert.Equal(t, http.StatusRequestEntityTooLarge, res.StatusCode)
}

func TestListCharacters_DefaultLimitApplied(t *testing.T) {
	srv, raceID, repo := newTestServerWithRepo(t)
	createCharacter(t, srv, raceID)

	// limit 省略 → デフォルト 100 が適用される（全件取得にならない）。
	res := do(t, srv, http.MethodGet, "/api/v1/characters", testAPIKey, "")
	require.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, 100, repo.lastParams.Limit)

	// ?ids= のバッチ取得時はデフォルト limit を適用しない（ids 自体が上限）。
	res = do(t, srv, http.MethodGet, "/api/v1/characters?ids="+uuid.NewString(), testAPIKey, "")
	require.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, 0, repo.lastParams.Limit)

	// 明示指定はそのまま通る。
	res = do(t, srv, http.MethodGet, "/api/v1/characters?limit=42", testAPIKey, "")
	require.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, 42, repo.lastParams.Limit)

	// 上限超過は 400。
	res = do(t, srv, http.MethodGet, "/api/v1/characters?limit=501", testAPIKey, "")
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
}

func TestReplaceCharacter_WeakIfMatch_Rejected(t *testing.T) {
	srv, raceID := newTestServer(t)
	id := createCharacter(t, srv, raceID)

	// 弱い検証子は If-Match の強い比較では一致しない（RFC 9110）→ 412。
	body := `{"name":"x","raceId":"` + raceID.String() + `","gender":"female"}`
	res := putWithIfMatch(t, srv, "/api/v1/characters/"+id, `W/"1"`, body)
	assert.Equal(t, http.StatusPreconditionFailed, res.StatusCode)
}

func TestReplaceCharacter_MatchingVersion_BumpsETag(t *testing.T) {
	srv, raceID := newTestServer(t)
	id := createCharacter(t, srv, raceID) // 版は 1

	body := `{"name":"x","raceId":"` + raceID.String() + `","gender":"female"}`
	res := putWithIfMatch(t, srv, "/api/v1/characters/"+id, `"1"`, body)
	require.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, `"2"`, res.Header.Get("ETag")) // 更新で版が +1
}
