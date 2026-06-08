package handler

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	domain "github.com/yu-be-shi/character-api/internal/domain/race"
	"github.com/yu-be-shi/character-api/internal/interfaces/http/dto"
	usecase "github.com/yu-be-shi/character-api/internal/usecase/race"
)

type RaceHandler struct {
	svc *usecase.Service
}

func NewRaceHandler(svc *usecase.Service) *RaceHandler {
	return &RaceHandler{svc: svc}
}

func (h *RaceHandler) Register(g *echo.Group) {
	g.GET("", h.List)
	g.POST("", h.Create)
	g.GET("/:id", h.Get)
	g.PUT("/:id", h.Update)
	g.DELETE("/:id", h.Delete)
}

// List godoc
// @Summary      種族一覧取得
// @Tags         races
// @Produce      json
// @Security     InternalAPIKey
// @Success      200  {array}   dto.RaceResponse
// @Failure      500  {object}  dto.ErrorResponse
// @Router       /api/v1/races [get]
func (h *RaceHandler) List(c echo.Context) error {
	rs, err := h.svc.List(c.Request().Context())
	if err != nil {
		return mapRaceErr(err)
	}
	return c.JSON(http.StatusOK, dto.RaceFromDomainList(rs))
}

// Create godoc
// @Summary      種族作成
// @Tags         races
// @Accept       json
// @Produce      json
// @Security     InternalAPIKey
// @Param        body  body      dto.CreateRaceRequest  true  "種族作成リクエスト"
// @Success      201   {object}  dto.RaceResponse
// @Failure      400   {object}  dto.ErrorResponse
// @Failure      409   {object}  dto.ErrorResponse
// @Failure      500   {object}  dto.ErrorResponse
// @Router       /api/v1/races [post]
func (h *RaceHandler) Create(c echo.Context) error {
	var req dto.CreateRaceRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	if err := c.Validate(&req); err != nil {
		return err
	}
	out, err := h.svc.Create(c.Request().Context(), usecase.CreateInput{Name: req.Name})
	if err != nil {
		return mapRaceErr(err)
	}
	return c.JSON(http.StatusCreated, dto.RaceFromDomain(out))
}

// Get godoc
// @Summary      種族取得
// @Tags         races
// @Produce      json
// @Security     InternalAPIKey
// @Param        id   path      string  true  "種族ID (UUID)"
// @Success      200  {object}  dto.RaceResponse
// @Failure      400  {object}  dto.ErrorResponse
// @Failure      404  {object}  dto.ErrorResponse
// @Failure      500  {object}  dto.ErrorResponse
// @Router       /api/v1/races/{id} [get]
func (h *RaceHandler) Get(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Get(c.Request().Context(), id)
	if err != nil {
		return mapRaceErr(err)
	}
	return c.JSON(http.StatusOK, dto.RaceFromDomain(out))
}

// Update godoc
// @Summary      種族更新
// @Tags         races
// @Accept       json
// @Produce      json
// @Security     InternalAPIKey
// @Param        id    path      string                 true  "種族ID (UUID)"
// @Param        body  body      dto.UpdateRaceRequest  true  "種族更新リクエスト"
// @Success      200   {object}  dto.RaceResponse
// @Failure      400   {object}  dto.ErrorResponse
// @Failure      404   {object}  dto.ErrorResponse
// @Failure      409   {object}  dto.ErrorResponse
// @Failure      500   {object}  dto.ErrorResponse
// @Router       /api/v1/races/{id} [put]
func (h *RaceHandler) Update(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	var req dto.UpdateRaceRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	if err := c.Validate(&req); err != nil {
		return err
	}
	out, err := h.svc.Update(c.Request().Context(), id, usecase.UpdateInput{Name: req.Name})
	if err != nil {
		return mapRaceErr(err)
	}
	return c.JSON(http.StatusOK, dto.RaceFromDomain(out))
}

// Delete godoc
// @Summary      種族削除
// @Tags         races
// @Security     InternalAPIKey
// @Param        id   path  string  true  "種族ID (UUID)"
// @Success      204
// @Failure      400  {object}  dto.ErrorResponse
// @Failure      404  {object}  dto.ErrorResponse
// @Failure      500  {object}  dto.ErrorResponse
// @Router       /api/v1/races/{id} [delete]
func (h *RaceHandler) Delete(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if err := h.svc.Delete(c.Request().Context(), id); err != nil {
		return mapRaceErr(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func mapRaceErr(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrInvalidName):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrDuplicate):
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	default:
		return err
	}
}
