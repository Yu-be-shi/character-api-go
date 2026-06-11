package httpiface

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	echoSwagger "github.com/swaggo/echo-swagger"
	"golang.org/x/time/rate"

	_ "github.com/yu-be-shi/character-api/docs"
	"github.com/yu-be-shi/character-api/internal/config"
	chardomain "github.com/yu-be-shi/character-api/internal/domain/character"
	"github.com/yu-be-shi/character-api/internal/domain/idempotency"
	"github.com/yu-be-shi/character-api/internal/interfaces/http/handler"
	apimw "github.com/yu-be-shi/character-api/internal/interfaces/http/middleware"
	charUsecase "github.com/yu-be-shi/character-api/internal/usecase/character"
	raceUsecase "github.com/yu-be-shi/character-api/internal/usecase/race"
)

type echoValidator struct {
	v *validator.Validate
}

// Validate は検証エラーを「フィールド名（json タグ）→ 失敗した検証ルール」の
// 構造化された詳細に変換して返す。validator の生メッセージは Go の構造体名
// （`CreateCharacterRequest.Name` 等）を含み内部実装が漏れるため、そのまま返さない。
func (ev *echoValidator) Validate(i any) error {
	err := ev.v.Struct(i)
	if err == nil {
		return nil
	}
	var verrs validator.ValidationErrors
	if errors.As(err, &verrs) {
		details := make(map[string]string, len(verrs))
		for _, fe := range verrs {
			details[fe.Field()] = fmt.Sprintf("failed on '%s' validation", fe.Tag())
		}
		return echo.NewHTTPError(http.StatusBadRequest, map[string]any{
			"message": "validation failed",
			"details": details,
		})
	}
	return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
}

// newValidator は許可 gender をドメイン（chardomain.AllGenders）から取る
// カスタムバリデーション "gender" を登録する。DTO 側の oneof 文字列重複を排除する。
// エラー詳細のフィールド名は Go のフィールド名ではなく json タグ名を使う。
func newValidator() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "" || name == "-" {
			return fld.Name
		}
		return name
	})
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
	// CORS はデバッグ用途（Swagger UI 等）の最小許可。X-Internal-API-Key は
	// サーバー間共有シークレットでありブラウザに渡してはならないため、
	// 意図的に AllowHeaders へ含めない（ブラウザから鍵付きで呼ぶ構成を公認しない）。
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: cfg.CORSOrigins,
		AllowMethods: []string{http.MethodGet, http.MethodOptions},
		AllowHeaders: []string{echo.HeaderContentType},
	}))

	e.GET("/healthz", handler.Health)
	if pingDB != nil {
		e.GET("/readyz", handler.Ready(pingDB))
	}
	e.GET("/swagger/*", echoSwagger.WrapHandler)

	api := e.Group("/api/v1")
	if cfg.RateLimitRPS > 0 {
		// IP あたりのレート制限（無料・インメモリ）。0 で無効。
		// 認証より「前」に置く: 後ろに置くと不正キーでの総当たり（401 連打）が
		// レート制限を一切受けない。Burst は RPS<1 の設定でも最低 1 を保証する
		// （int(0.5)=0 だと全リクエストが 429 になるため）。
		burst := int(cfg.RateLimitRPS)
		if burst < 1 {
			burst = 1
		}
		api.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStoreWithConfig(
			middleware.RateLimiterMemoryStoreConfig{Rate: rate.Limit(cfg.RateLimitRPS), Burst: burst},
		)))
	}
	api.Use(apimw.InternalAPIKey(cfg.InternalAPIKey))
	if idemStore != nil {
		// POST のみ冪等化（ミドルウェア内で非 POST は素通し）。
		api.Use(apimw.Idempotency(idemStore))
	}
	handler.NewCharacterHandler(charSvc).Register(api.Group("/characters"))
	handler.NewRaceHandler(raceSvc).Register(api.Group("/races"))

	return e
}
