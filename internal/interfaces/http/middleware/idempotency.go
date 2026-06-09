package middleware

import (
	"bytes"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/yu-be-shi/character-api/internal/infrastructure/idempotency"
)

// IdempotencyKeyHeader はクライアントが送る冪等キーのヘッダ名。
const IdempotencyKeyHeader = "Idempotency-Key"

// Idempotency は POST に Idempotency-Key 冪等性を付与するミドルウェア。
//   - 対象は POST のみ（PUT/PATCH/DELETE は元々冪等）。
//   - キーが無いリクエストは素通し（任意機能）。
//   - 同一キーの再送は保存済みレスポンスを再生（Idempotent-Replayed: true）。
//   - 処理中の同一キーは 409。
//   - ストア障害時は可用性優先で通常処理（fail-open）。
//   - 成功時のみ結果を保存し、ハンドラがエラーを返したら予約を解放して再試行可能にする。
func Idempotency(store idempotency.Store) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Request().Method != http.MethodPost {
				return next(c)
			}
			key := c.Request().Header.Get(IdempotencyKeyHeader)
			if key == "" {
				return next(c)
			}
			ctx := c.Request().Context()

			prev, isNew, err := store.Begin(ctx, key)
			if err != nil {
				return next(c) // fail-open
			}
			if !isNew {
				if prev != nil {
					c.Response().Header().Set("Idempotent-Replayed", "true")
					return c.Blob(prev.Status, echo.MIMEApplicationJSONCharsetUTF8, prev.Body)
				}
				return echo.NewHTTPError(http.StatusConflict, "a request with this Idempotency-Key is already in progress")
			}

			rec := &recorder{ResponseWriter: c.Response().Writer, buf: &bytes.Buffer{}, status: http.StatusOK}
			c.Response().Writer = rec

			if err := next(c); err != nil {
				// 失敗は確定結果を保存せず予約解放（同一キーで再試行できるように）。
				_ = store.Release(ctx, key)
				return err
			}
			_ = store.Complete(ctx, key, idempotency.Result{Status: rec.status, Body: rec.buf.Bytes()})
			return nil
		}
	}
}

// recorder はレスポンスのステータスと本文を記録しつつ下流へ書き込む。
type recorder struct {
	http.ResponseWriter
	buf    *bytes.Buffer
	status int
}

func (r *recorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	r.buf.Write(b)
	return r.ResponseWriter.Write(b)
}
