package middleware_test

import (
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
	req := httptest.NewRequest(http.MethodPost, "/things", strings.NewReader("{}"))
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
