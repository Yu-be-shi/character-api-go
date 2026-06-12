package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yu-be-shi/character-api/internal/infrastructure/idempotency"
	mw "github.com/yu-be-shi/character-api/internal/interfaces/http/middleware"
)

// newIdemApp は冪等ミドルウェア付きの POST /things を持つ Echo を返す。
// failFirst=true なら1回目の呼び出しだけエラーにする。
func newIdemApp(calls *int32, failFirst bool) *echo.Echo {
	e := echo.New()
	e.Use(mw.Idempotency(idempotency.NewMemoryStore()))
	e.POST("/things", func(c echo.Context) error {
		n := atomic.AddInt32(calls, 1)
		if failFirst && n == 1 {
			return echo.NewHTTPError(http.StatusInternalServerError, "boom")
		}
		return c.JSON(http.StatusCreated, echo.Map{"n": n})
	})
	return e
}

func postThing(e *echo.Echo, key string) *httptest.ResponseRecorder {
	return postBody(e, "/things", key, "{}")
}

func postBody(e *echo.Echo, path, key, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set(mw.IdempotencyKeyHeader, key)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestIdempotency_ReplaysSameKey(t *testing.T) {
	var calls int32
	e := newIdemApp(&calls, false)

	r1 := postThing(e, "k1")
	r2 := postThing(e, "k1")

	require.Equal(t, http.StatusCreated, r1.Code)
	require.Equal(t, http.StatusCreated, r2.Code)
	assert.Equal(t, int32(1), calls, "ハンドラは1回だけ実行される")
	assert.Equal(t, r1.Body.String(), r2.Body.String(), "同じレスポンスが再生される")
	assert.Equal(t, "true", r2.Header().Get("Idempotent-Replayed"))
}

func TestIdempotency_NoKey_AlwaysRuns(t *testing.T) {
	var calls int32
	e := newIdemApp(&calls, false)

	postThing(e, "")
	postThing(e, "")

	assert.Equal(t, int32(2), calls, "キー無しは毎回実行される")
}

func TestIdempotency_DifferentKeys_Run(t *testing.T) {
	var calls int32
	e := newIdemApp(&calls, false)

	postThing(e, "a")
	postThing(e, "b")

	assert.Equal(t, int32(2), calls)
}

func TestIdempotency_ReleasesOnError(t *testing.T) {
	var calls int32
	e := newIdemApp(&calls, true) // 1回目はエラー

	r1 := postThing(e, "k1")
	r2 := postThing(e, "k1") // 同じキーで再試行できる（予約は解放済み）

	assert.Equal(t, http.StatusInternalServerError, r1.Code)
	assert.Equal(t, http.StatusCreated, r2.Code)
	assert.Equal(t, int32(2), calls, "失敗後は同一キーでも再実行される")
}

func TestIdempotency_KeyIsScopedToEndpoint(t *testing.T) {
	var things, others int32
	e := echo.New()
	e.Use(mw.Idempotency(idempotency.NewMemoryStore()))
	e.POST("/things", func(c echo.Context) error {
		atomic.AddInt32(&things, 1)
		return c.JSON(http.StatusCreated, echo.Map{"kind": "thing"})
	})
	e.POST("/others", func(c echo.Context) error {
		atomic.AddInt32(&others, 1)
		return c.JSON(http.StatusCreated, echo.Map{"kind": "other"})
	})

	r1 := postBody(e, "/things", "k1", "{}")
	r2 := postBody(e, "/others", "k1", "{}") // 同じキーでも別エンドポイントは別物

	require.Equal(t, http.StatusCreated, r1.Code)
	require.Equal(t, http.StatusCreated, r2.Code)
	assert.Equal(t, int32(1), things)
	assert.Equal(t, int32(1), others, "別エンドポイントには再生されず実行される")
	assert.Contains(t, r2.Body.String(), "other", "別エンドポイントのレスポンスが再生されない")
}

func TestIdempotency_DifferentBodySameKey_Returns422(t *testing.T) {
	var calls int32
	e := newIdemApp(&calls, false)

	r1 := postBody(e, "/things", "k1", `{"name":"a"}`)
	r2 := postBody(e, "/things", "k1", `{"name":"b"}`) // キー使い回し＋別ボディ

	require.Equal(t, http.StatusCreated, r1.Code)
	assert.Equal(t, http.StatusUnprocessableEntity, r2.Code, "別ボディの再利用は黙って再生せず 422")
	assert.Equal(t, int32(1), calls)
}

func TestIdempotency_ReplayRestoresETag(t *testing.T) {
	var calls int32
	e := echo.New()
	e.Use(mw.Idempotency(idempotency.NewMemoryStore()))
	e.POST("/things", func(c echo.Context) error {
		atomic.AddInt32(&calls, 1)
		c.Response().Header().Set("ETag", `"1"`)
		return c.JSON(http.StatusCreated, echo.Map{"ok": true})
	})

	r1 := postBody(e, "/things", "k1", "{}")
	r2 := postBody(e, "/things", "k1", "{}")

	require.Equal(t, `"1"`, r1.Header().Get("ETag"))
	assert.Equal(t, `"1"`, r2.Header().Get("ETag"), "再生時も ETag が復元される（楽観ロック継続のため）")
	assert.Equal(t, int32(1), calls)
}

func TestIdempotency_ReleasesOnPanic(t *testing.T) {
	var calls int32
	store := idempotency.NewMemoryStore()
	e := echo.New()
	e.Use(echo.MiddlewareFunc(func(next echo.HandlerFunc) echo.HandlerFunc {
		// 本番構成と同じく Recover はミドルウェアの外側にある。
		return func(c echo.Context) error {
			defer func() {
				if r := recover(); r != nil {
					_ = c.JSON(http.StatusInternalServerError, echo.Map{"error": "panic"})
				}
			}()
			return next(c)
		}
	}))
	e.Use(mw.Idempotency(store))
	e.POST("/things", func(c echo.Context) error {
		if atomic.AddInt32(&calls, 1) == 1 {
			panic("boom")
		}
		return c.JSON(http.StatusCreated, echo.Map{"ok": true})
	})

	r1 := postBody(e, "/things", "k1", "{}")
	r2 := postBody(e, "/things", "k1", "{}") // panic 後も 409 にならず再試行できる

	require.Equal(t, http.StatusInternalServerError, r1.Code)
	assert.Equal(t, http.StatusCreated, r2.Code, "panic 時は予約が解放され再試行できる")
	assert.Equal(t, int32(2), calls)
}

// brokenStore は常にエラーを返す Store（fail-closed の検証用）。
type brokenStore struct{}

func (brokenStore) Begin(_ context.Context, _ string) (*idempotency.Result, bool, error) {
	return nil, false, errors.New("store down")
}
func (brokenStore) Complete(_ context.Context, _ string, _ idempotency.Result) error {
	return errors.New("store down")
}
func (brokenStore) Release(_ context.Context, _ string) error { return errors.New("store down") }

// pendingStore は常に「処理中」を返す Store（409 分岐の検証用）。
type pendingStore struct{}

func (pendingStore) Begin(_ context.Context, _ string) (*idempotency.Result, bool, error) {
	return nil, false, nil // 予約済み・結果未確定 = 処理中
}
func (pendingStore) Complete(_ context.Context, _ string, _ idempotency.Result) error { return nil }
func (pendingStore) Release(_ context.Context, _ string) error                        { return nil }

func TestIdempotency_InProgressSameKey_Returns409(t *testing.T) {
	var calls int32
	e := echo.New()
	e.Use(mw.Idempotency(pendingStore{}))
	e.POST("/things", func(c echo.Context) error {
		atomic.AddInt32(&calls, 1)
		return c.JSON(http.StatusCreated, echo.Map{"ok": true})
	})

	r := postBody(e, "/things", "k1", "{}")

	assert.Equal(t, http.StatusConflict, r.Code, "処理中の同一キーは 409")
	assert.Equal(t, int32(0), calls, "ハンドラは実行されない")
}

func TestIdempotency_StoreFailure_FailsClosed(t *testing.T) {
	var calls int32
	e := echo.New()
	e.Use(mw.Idempotency(brokenStore{}))
	e.POST("/things", func(c echo.Context) error {
		atomic.AddInt32(&calls, 1)
		return c.JSON(http.StatusCreated, echo.Map{"ok": true})
	})

	r1 := postBody(e, "/things", "k1", "{}")
	r2 := postBody(e, "/things", "k1", "{}")

	// 正しさ優先（fail-closed）: ストア障害時は 503 を返し、ハンドラを実行しない（二重作成防止）。
	require.Equal(t, http.StatusServiceUnavailable, r1.Code)
	require.Equal(t, http.StatusServiceUnavailable, r2.Code)
	assert.Equal(t, int32(0), calls, "ストア障害時は実行しない（fail-closed）")
}

func TestIdempotency_KeyTooLong_Returns400(t *testing.T) {
	var calls int32
	e := newIdemApp(&calls, false)

	r := postBody(e, "/things", strings.Repeat("x", 201), "{}")

	assert.Equal(t, http.StatusBadRequest, r.Code)
	assert.Equal(t, int32(0), calls)
}

// DB creation_token リプレイ（Idempotent-Replayed ヘッダ付き）は Redis に保存されない。
// Redis ミス後に DB 一意制約が既存行を返した場合、現リクエストの bodyHash で
// Redis を上書きすると元ボディの再送で 422 になる競合が起きるため保存しない。
func TestIdempotency_DBReplay_NotCached(t *testing.T) {
	var calls int32
	e := echo.New()
	e.Use(mw.Idempotency(idempotency.NewMemoryStore()))
	e.POST("/things", func(c echo.Context) error {
		atomic.AddInt32(&calls, 1)
		// DB リプレイを模倣: Idempotent-Replayed ヘッダをセットして 201 を返す。
		c.Response().Header().Set("Idempotent-Replayed", "true")
		return c.JSON(http.StatusCreated, echo.Map{"ok": true})
	})

	r1 := postBody(e, "/things", "k1", `{"name":"original"}`)
	require.Equal(t, http.StatusCreated, r1.Code)
	assert.Equal(t, "true", r1.Header().Get("Idempotent-Replayed"))

	// DB リプレイは Redis に保存されない（予約を解放する）ので、
	// 別のボディで再送しても 422 にならず再度ハンドラが実行される。
	r2 := postBody(e, "/things", "k1", `{"name":"different"}`)
	assert.Equal(t, http.StatusCreated, r2.Code, "DB リプレイは Redis を汚染しないため別ボディでも 422 にならない")
	assert.Equal(t, int32(2), calls, "Redis に保存されないため毎回ハンドラが実行される")
}

func TestIdempotency_Direct5xx_NotStored(t *testing.T) {
	var calls int32
	e := echo.New()
	e.Use(mw.Idempotency(idempotency.NewMemoryStore()))
	e.POST("/things", func(c echo.Context) error {
		// エラーを return せず直接 5xx を書き込むハンドラ。
		if atomic.AddInt32(&calls, 1) == 1 {
			return c.JSON(http.StatusServiceUnavailable, echo.Map{"error": "downstream down"})
		}
		return c.JSON(http.StatusCreated, echo.Map{"ok": true})
	})

	r1 := postBody(e, "/things", "k1", "{}")
	r2 := postBody(e, "/things", "k1", "{}")

	require.Equal(t, http.StatusServiceUnavailable, r1.Code)
	assert.Equal(t, http.StatusCreated, r2.Code, "直接書き込まれた 5xx は保存されず再試行できる")
	assert.Equal(t, int32(2), calls)
}
