package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/labstack/echo/v4"
)

// InternalAPIKeyHeader は内部サービス間認証に使う HTTP ヘッダー「名」（資格情報そのものではない）。
const InternalAPIKeyHeader = "X-Internal-API-Key" // #nosec G101 -- ヘッダ名であり秘密ではない

// InternalAPIKey は内部サービス（application 側）からのリクエストのみ許可するミドルウェア。
// 環境変数 INTERNAL_API_KEY と照合する。ブラウザからの直接アクセスはここで弾かれる。
// 比較はタイミング攻撃を避けるため定数時間で行い、鍵未設定（空）の場合は常に拒否する。
func InternalAPIKey(expectedKey string) echo.MiddlewareFunc {
	expected := []byte(expectedKey)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			got := []byte(c.Request().Header.Get(InternalAPIKeyHeader))
			if len(expected) == 0 || subtle.ConstantTimeCompare(got, expected) != 1 {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid or missing API key")
			}
			return next(c)
		}
	}
}
