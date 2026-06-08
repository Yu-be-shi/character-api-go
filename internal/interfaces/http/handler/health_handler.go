package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// Health godoc
// @Summary      ヘルスチェック
// @Tags         health
// @Produce      json
// @Success      200  {object}  map[string]string
// @Router       /healthz [get]
func Health(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
