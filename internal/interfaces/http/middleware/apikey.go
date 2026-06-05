package middleware

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

const internalAPIKeyHeader = "X-Internal-API-Key"

// InternalAPIKey は内部サービス（Repo2）からのリクエストのみ許可するミドルウェア。
// 環境変数 INTERNAL_API_KEY と照合する。
// ブラウザからの直接アクセスはこの手前で弾かれる。
func InternalAPIKey(expectedKey string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			key := c.Request().Header.Get(internalAPIKeyHeader)
			if key == "" || key != expectedKey {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid or missing API key")
			}
			return next(c)
		}
	}
}
