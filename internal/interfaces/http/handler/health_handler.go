package handler

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"
)

// Health godoc
// @Summary      Liveness（プロセス生存）。依存に触れない。
// @Tags         health
// @Produce      json
// @Success      200  {object}  map[string]string
// @Router       /healthz [get]
func Health(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// Ready は Readiness（依存込みで処理可能か）。DB に ping して可否を返す。
// liveness（/healthz）と分離し、DB 断時に 503 を返してロードバランサから外せるようにする。
//
// @Summary      Readiness（DB 接続確認込み）
// @Tags         health
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Router       /readyz [get]
func Ready(ping func(context.Context) error) echo.HandlerFunc {
	return func(c echo.Context) error {
		if err := ping(c.Request().Context()); err != nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "database not ready")
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
	}
}
