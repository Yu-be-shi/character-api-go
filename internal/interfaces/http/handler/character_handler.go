package handler

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	domain "github.com/yu-be-shi/character-api/internal/domain/character"
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

func (h *CharacterHandler) List(c echo.Context) error {
	cs, err := h.svc.List(c.Request().Context())
	if err != nil {
		return mapErr(err)
	}
	return c.JSON(http.StatusOK, dto.FromDomainList(cs))
}

func (h *CharacterHandler) Create(c echo.Context) error {
	var req dto.CreateCharacterRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	if err := c.Validate(&req); err != nil {
		return err
	}
	out, err := h.svc.Create(c.Request().Context(), usecase.CreateInput{
		Name:       req.Name,
		Attributes: req.Attributes,
	})
	if err != nil {
		return mapErr(err)
	}
	return c.JSON(http.StatusCreated, dto.FromDomain(out))
}

func (h *CharacterHandler) Get(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Get(c.Request().Context(), id)
	if err != nil {
		return mapErr(err)
	}
	return c.JSON(http.StatusOK, dto.FromDomain(out))
}

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
	out, err := h.svc.Update(c.Request().Context(), id, usecase.UpdateInput{
		Name:       req.Name,
		Attributes: req.Attributes,
	})
	if err != nil {
		return mapErr(err)
	}
	return c.JSON(http.StatusOK, dto.FromDomain(out))
}

func (h *CharacterHandler) Delete(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if err := h.svc.Delete(c.Request().Context(), id); err != nil {
		return mapErr(err)
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

func mapErr(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrInvalidName):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	default:
		return err
	}
}
