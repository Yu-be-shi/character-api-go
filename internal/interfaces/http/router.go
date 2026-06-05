package httpiface

import (
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/yu-be-shi/character-api/internal/config"
	"github.com/yu-be-shi/character-api/internal/interfaces/http/handler"
	apimw "github.com/yu-be-shi/character-api/internal/interfaces/http/middleware"
	usecase "github.com/yu-be-shi/character-api/internal/usecase/character"
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

func New(cfg config.Config, charSvc *usecase.Service) *echo.Echo {
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

	// ヘルスチェックは認証不要（Repo4 の healthcheck が叩く）
	e.GET("/healthz", handler.Health)

	// /api/v1 配下はすべてサービス間認証を要求
	api := e.Group("/api/v1", apimw.InternalAPIKey(cfg.InternalAPIKey))
	handler.NewCharacterHandler(charSvc).Register(api.Group("/characters"))

	return e
}
