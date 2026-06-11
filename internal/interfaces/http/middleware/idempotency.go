package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/yu-be-shi/character-api/internal/infrastructure/idempotency"
)

// IdempotencyKeyHeader はクライアントが送る冪等キーのヘッダ名。
const IdempotencyKeyHeader = "Idempotency-Key"

// maxIdempotencyKeyLen はキー長の上限（ストアのキー肥大を防ぐ）。
const maxIdempotencyKeyLen = 200

// replayHeaders は再生時に復元するレスポンスヘッダ。
// ETag を含めないと、再送クライアントが楽観ロック（If-Match）を継続できない。
var replayHeaders = []string{echo.HeaderContentType, "ETag", "Location"}

// Idempotency は POST に Idempotency-Key 冪等性を付与するミドルウェア。
//   - 対象は POST のみ（PUT/PATCH/DELETE は元々冪等）。
//   - キーが無いリクエストは素通し（任意機能）。
//   - キーはエンドポイント（メソッド + ルートパス）にスコープする。同一キーを
//     別エンドポイントへ送っても、別リソースのレスポンスが再生されることはない。
//   - 同一キー・同一ボディの再送は保存済みレスポンスを再生（Idempotent-Replayed: true）。
//   - 同一キー・異なるボディは 422（キーの誤用を黙って再生しない）。
//   - 処理中の同一キーは 409。
//   - ストア障害時は可用性優先で通常処理（fail-open）。
//   - 成功時のみ結果を保存し、ハンドラのエラー・panic 時は defer で予約を解放して
//     同一キーで再試行できるようにする（panic は外側の Recover まで巻き戻る途中で
//     この defer が実行される）。
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
			if len(key) > maxIdempotencyKeyLen {
				return echo.NewHTTPError(http.StatusBadRequest, "Idempotency-Key is too long")
			}
			ctx := c.Request().Context()

			storeKey := c.Request().Method + " " + c.Path() + " " + key
			bodyHash, err := hashRequestBody(c)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, "failed to read request body")
			}

			prev, isNew, err := store.Begin(ctx, storeKey)
			if err != nil {
				return next(c) // fail-open
			}
			if !isNew {
				if prev == nil {
					return echo.NewHTTPError(http.StatusConflict, "a request with this Idempotency-Key is already in progress")
				}
				if prev.BodyHash != bodyHash {
					return echo.NewHTTPError(http.StatusUnprocessableEntity, "Idempotency-Key was reused with a different request body")
				}
				return replay(c, prev)
			}

			// 未完了（エラー・panic・保存失敗）なら予約を解放して再試行可能にする。
			completed := false
			defer func() {
				if completed {
					return
				}
				if err := store.Release(ctx, storeKey); err != nil {
					slog.Warn("idempotency release failed", "key", storeKey, "error", err)
				}
			}()

			rec := &recorder{ResponseWriter: c.Response().Writer, buf: &bytes.Buffer{}, status: http.StatusOK}
			c.Response().Writer = rec

			if err := next(c); err != nil {
				return err
			}
			res := idempotency.Result{
				Status:   rec.status,
				Body:     rec.buf.Bytes(),
				BodyHash: bodyHash,
				Header:   map[string]string{},
			}
			for _, k := range replayHeaders {
				if v := c.Response().Header().Get(k); v != "" {
					res.Header[k] = v
				}
			}
			if err := store.Complete(ctx, storeKey, res); err != nil {
				// レスポンスは送信済みなのでクライアントにはエラーを返さない。
				// 保存失敗は「再送時に再生されない」だけで、defer が予約を解放する。
				slog.Warn("idempotency complete failed", "key", storeKey, "error", err)
				return nil
			}
			completed = true
			return nil
		}
	}
}

// replay は保存済みレスポンスを（ETag 等のヘッダ込みで）再生する。
func replay(c echo.Context, prev *idempotency.Result) error {
	for k, v := range prev.Header {
		c.Response().Header().Set(k, v)
	}
	c.Response().Header().Set("Idempotent-Replayed", "true")
	contentType := prev.Header[echo.HeaderContentType]
	if contentType == "" {
		contentType = echo.MIMEApplicationJSONCharsetUTF8
	}
	return c.Blob(prev.Status, contentType, prev.Body)
}

// hashRequestBody はボディを読み切って SHA-256 を返し、ハンドラが再読できるよう復元する。
// ボディサイズは BodyLimit ミドルウェアで上限済み。
func hashRequestBody(c echo.Context) (string, error) {
	req := c.Request()
	if req.Body == nil {
		sum := sha256.Sum256(nil)
		return hex.EncodeToString(sum[:]), nil
	}
	b, err := io.ReadAll(req.Body)
	if err != nil {
		return "", err
	}
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(b))
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
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
