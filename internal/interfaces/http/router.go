package httpiface

import (
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/yu-be-shi/character-api/internal/config"
	"github.com/yu-be-shi/character-api/internal/interfaces/http/handler"
	apimw "github.com/yu-be-shi/character-api/internal/interfaces/http/middleware"
	charUsecase "github.com/yu-be-shi/character-api/internal/usecase/character"
	raceUsecase "github.com/yu-be-shi/character-api/internal/usecase/race"
)

type echoValidator struct {
	v *validator.Validate
}

func (ev *echoValidator) Validate(i any) error {
	if err := ev.v.Struct(i); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return nil
}

func New(cfg config.Config, charSvc *charUsecase.Service, raceSvc *raceUsecase.Service) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Validator = &echoValidator{v: validator.New()}

	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(middleware.Logger())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: cfg.CORSOrigins,
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowHeaders: []string{echo.HeaderContentType, echo.HeaderAuthorization},
	}))

	e.GET("/healthz", handler.Health)

	api := e.Group("/api/v1", apimw.InternalAPIKey(cfg.InternalAPIKey))
	handler.NewCharacterHandler(charSvc).Register(api.Group("/characters"))
	handler.NewRaceHandler(raceSvc).Register(api.Group("/races"))

	return e
}
