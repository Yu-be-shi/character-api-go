package handler

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	chardomain "github.com/yu-be-shi/character-api/internal/domain/character"
	racedomain "github.com/yu-be-shi/character-api/internal/domain/race"
	"github.com/yu-be-shi/character-api/internal/interfaces/http/dto"
	usecase "github.com/yu-be-shi/character-api/internal/usecase/character"
)

type CharacterHandler struct {
	svc *usecase.Service
}

func NewCharacterHandler(svc *usecase.Service) *CharacterHandler {
	return &CharacterHandler{svc: svc}
}

func (h *CharacterHandler) Register(g *echo.Group) {
	g.GET("", h.List)
	g.POST("", h.Create)
	g.GET("/:id", h.Get)
	g.PUT("/:id", h.Update)
	g.DELETE("/:id", h.Delete)
}

// List godoc
// @Summary      キャラクター一覧取得
// @Tags         characters
// @Produce      json
// @Security     InternalAPIKey
// @Success      200  {array}   dto.CharacterResponse
// @Failure      500  {object}  dto.ErrorResponse
// @Router       /api/v1/characters [get]
func (h *CharacterHandler) List(c echo.Context) error {
	cs, err := h.svc.List(c.Request().Context())
	if err != nil {
		return mapCharErr(err)
	}
	return c.JSON(http.StatusOK, dto.FromDomainList(cs))
}

// Create godoc
// @Summary      キャラクター作成
// @Tags         characters
// @Accept       json
// @Produce      json
// @Security     InternalAPIKey
// @Param        body  body      dto.CreateCharacterRequest  true  "キャラクター作成リクエスト"
// @Success      201   {object}  dto.CharacterResponse
// @Failure      400   {object}  dto.ErrorResponse
// @Failure      422   {object}  dto.ErrorResponse
// @Failure      500   {object}  dto.ErrorResponse
// @Router       /api/v1/characters [post]
func (h *CharacterHandler) Create(c echo.Context) error {
	var req dto.CreateCharacterRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	if err := c.Validate(&req); err != nil {
		return err
	}
	out, err := h.svc.Create(c.Request().Context(), usecase.CreateInput{
		Name:        req.Name,
		Description: req.Description,
		RaceID:      req.RaceID,
		Gender:      chardomain.Gender(req.Gender),
		BirthDate:   req.BirthDate,
		BirthPlace:  req.BirthPlace,
		HeightCm:    req.HeightCm,
		WeightKg:    req.WeightKg,
		BodyFat:     req.BodyFat,
		SizeTop:     req.SizeTop,
		SizeMiddle:  req.SizeMiddle,
		SizeBottom:  req.SizeBottom,
	})
	if err != nil {
		return mapCharErr(err)
	}
	return c.JSON(http.StatusCreated, dto.FromDomain(out))
}

// Get godoc
// @Summary      キャラクター取得
// @Tags         characters
// @Produce      json
// @Security     InternalAPIKey
// @Param        id   path      string  true  "キャラクターID (UUID)"
// @Success      200  {object}  dto.CharacterResponse
// @Failure      400  {object}  dto.ErrorResponse
// @Failure      404  {object}  dto.ErrorResponse
// @Failure      500  {object}  dto.ErrorResponse
// @Router       /api/v1/characters/{id} [get]
func (h *CharacterHandler) Get(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Get(c.Request().Context(), id)
	if err != nil {
		return mapCharErr(err)
	}
	return c.JSON(http.StatusOK, dto.FromDomain(out))
}

// Update godoc
// @Summary      キャラクター更新
// @Tags         characters
// @Accept       json
// @Produce      json
// @Security     InternalAPIKey
// @Param        id    path      string                      true  "キャラクターID (UUID)"
// @Param        body  body      dto.UpdateCharacterRequest  true  "キャラクター更新リクエスト"
// @Success      200   {object}  dto.CharacterResponse
// @Failure      400   {object}  dto.ErrorResponse
// @Failure      404   {object}  dto.ErrorResponse
// @Failure      422   {object}  dto.ErrorResponse
// @Failure      500   {object}  dto.ErrorResponse
// @Router       /api/v1/characters/{id} [put]
func (h *CharacterHandler) Update(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	var req dto.UpdateCharacterRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	if err := c.Validate(&req); err != nil {
		return err
	}

	var gender *chardomain.Gender
	if req.Gender != nil {
		g := chardomain.Gender(*req.Gender)
		gender = &g
	}

	out, err := h.svc.Update(c.Request().Context(), id, usecase.UpdateInput{
		UpdateFields: chardomain.UpdateFields{
			Name:        req.Name,
			Description: req.Description,
			RaceID:      req.RaceID,
			Gender:      gender,
			BirthDate:   req.BirthDate,
			BirthPlace:  req.BirthPlace,
			HeightCm:    req.HeightCm,
			WeightKg:    req.WeightKg,
			BodyFat:     req.BodyFat,
			SizeTop:     req.SizeTop,
			SizeMiddle:  req.SizeMiddle,
			SizeBottom:  req.SizeBottom,
		},
	})
	if err != nil {
		return mapCharErr(err)
	}
	return c.JSON(http.StatusOK, dto.FromDomain(out))
}

// Delete godoc
// @Summary      キャラクター削除
// @Tags         characters
// @Security     InternalAPIKey
// @Param        id   path  string  true  "キャラクターID (UUID)"
// @Success      204
// @Failure      400  {object}  dto.ErrorResponse
// @Failure      404  {object}  dto.ErrorResponse
// @Failure      500  {object}  dto.ErrorResponse
// @Router       /api/v1/characters/{id} [delete]
func (h *CharacterHandler) Delete(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if err := h.svc.Delete(c.Request().Context(), id); err != nil {
		return mapCharErr(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func parseID(c echo.Context) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return uuid.Nil, echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	return id, nil
}

func mapCharErr(err error) error {
	switch {
	case errors.Is(err, chardomain.ErrNotFound):
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	case errors.Is(err, chardomain.ErrInvalidName),
		errors.Is(err, chardomain.ErrInvalidGender):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, racedomain.ErrNotFound):
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "race not found")
	default:
		return err
	}
}
