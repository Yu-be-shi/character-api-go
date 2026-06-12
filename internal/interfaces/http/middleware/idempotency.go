package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/yu-be-shi/character-api/internal/domain/idempotency"
)

// storeOpTimeout はレスポンス確定後のストア操作（Complete / Release）の上限時間。
// リクエスト context から切り離して使うため、無制限にしない。
const storeOpTimeout = 5 * time.Second

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
//   - ストア障害時は正しさ優先で 503 を返す（fail-closed）。二重作成を防ぐため素通ししない。
//     （作成は加えて DB の creation_token 一意制約でも保護され、ストア不在でも二重作成しない。）
//   - 成功時のみ結果を保存し、ハンドラのエラー・panic・直接書き込まれた 5xx の場合は
//     defer で予約を解放して同一キーで再試行できるようにする（panic は外側の Recover
//     まで巻き戻る途中でこの defer が実行される）。
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
				// fail-closed: ストア障害時は二重作成を防ぐため拒否する（正しさ優先）。
				slog.Warn("idempotency store unavailable; rejecting (fail-closed)", "key", storeKey, "error", err)
				return echo.NewHTTPError(http.StatusServiceUnavailable, "idempotency store temporarily unavailable")
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
			// レスポンス送信後にクライアントが切断するとリクエスト context は cancel
			// されるため、ストア操作は WithoutCancel で切り離す（cancel に巻き込まれて
			// 予約が解放されず、再送が 409 になり続けるのを防ぐ）。
			completed := false
			defer func() {
				if completed {
					return
				}
				rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), storeOpTimeout)
				defer cancel()
				if err := store.Release(rctx, storeKey); err != nil {
					slog.Warn("idempotency release failed", "key", storeKey, "error", err)
				}
			}()

			rec := &recorder{ResponseWriter: c.Response().Writer, buf: &bytes.Buffer{}, status: http.StatusOK}
			c.Response().Writer = rec

			if err := next(c); err != nil {
				return err
			}
			// ハンドラがエラーを return せず直接 5xx を書き込んだ場合も確定結果として
			// 保存しない（サーバー都合の失敗を 24h 再生し続けないため。defer が予約を
			// 解放するので同一キーで再試行できる）。
			if rec.status >= http.StatusInternalServerError {
				return nil
			}
			// DB creation_token リプレイ（Redis ミス後に DB 一意制約が発動した場合）は保存しない。
			// 保存すると現リクエストの bodyHash で上書きされ、元ボディと異なるリクエストが
			// 先に DB リプレイを踏んだ場合に元ボディの再送で 422 になる競合を引き起こす。
			// DB リプレイは永続的（creation_token 一意制約）なので Redis キャッシュは不要。
			if c.Response().Header().Get("Idempotent-Replayed") == "true" {
				return nil // defer が予約を解放: 次のリクエストも同様に DB リプレイを経由
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
			// レスポンスは送信済みのためリクエスト context は既に不要。クライアント
			// 切断による cancel でここが失敗すると「201 を受け取ったのに保存されず、
			// 再送で二重作成される」窓ができるため、WithoutCancel で切り離す。
			cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), storeOpTimeout)
			defer cancel()
			if err := store.Complete(cctx, storeKey, res); err != nil {
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
