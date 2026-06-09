package httpiface

import (
	"context"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	echoSwagger "github.com/swaggo/echo-swagger"
	"golang.org/x/time/rate"

	_ "github.com/yu-be-shi/character-api/docs"
	"github.com/yu-be-shi/character-api/internal/config"
	chardomain "github.com/yu-be-shi/character-api/internal/domain/character"
	"github.com/yu-be-shi/character-api/internal/infrastructure/idempotency"
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

// newValidator は許可 gender をドメイン（chardomain.AllGenders）から取る
// カスタムバリデーション "gender" を登録する。DTO 側の oneof 文字列重複を排除する。
func newValidator() *validator.Validate {
	v := validator.New()
	_ = v.RegisterValidation("gender", func(fl validator.FieldLevel) bool {
		return chardomain.Gender(fl.Field().String()).Valid()
	})
	return v
}

// New は HTTP ルーターを構築する。pingDB は /readyz の依存チェックに使う（nil 可）。
// idemStore が非 nil のとき /api/v1 の POST に冪等性（Idempotency-Key）を適用する（nil 可）。
func New(cfg config.Config, charSvc *charUsecase.Service, raceSvc *raceUsecase.Service, pingDB func(context.Context) error, idemStore idempotency.Store) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Validator = &echoValidator{v: newValidator()}

	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(middleware.Logger())
	e.Use(middleware.BodyLimit("1M")) // 巨大リクエストボディを拒否（413）
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: cfg.CORSOrigins,
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowHeaders: []string{echo.HeaderContentType, apimw.InternalAPIKeyHeader, apimw.IdempotencyKeyHeader, "If-Match"},
	}))

	e.GET("/healthz", handler.Health)
	if pingDB != nil {
		e.GET("/readyz", handler.Ready(pingDB))
	}
	e.GET("/swagger/*", echoSwagger.WrapHandler)

	api := e.Group("/api/v1", apimw.InternalAPIKey(cfg.InternalAPIKey))
	if cfg.RateLimitRPS > 0 {
		// IP あたりのレート制限（無料・インメモリ）。0 で無効。
		api.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(rate.Limit(cfg.RateLimitRPS))))
	}
	if idemStore != nil {
		// POST のみ冪等化（ミドルウェア内で非 POST は素通し）。
		api.Use(apimw.Idempotency(idemStore))
	}
	handler.NewCharacterHandler(charSvc).Register(api.Group("/characters"))
	handler.NewRaceHandler(raceSvc).Register(api.Group("/races"))

	return e
}
