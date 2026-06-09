package handler

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

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
	g.PUT("/:id", h.Replace) // 全置換（省略した任意項目は NULL になる）
	g.PATCH("/:id", h.Patch) // 部分更新（送った項目だけ変更）
	g.DELETE("/:id", h.Delete)
}

// maxListLimit は ?limit= で指定できる最大件数（無制限取得を防ぐ）。
const maxListLimit = 500

// List godoc
// @Summary      キャラクター一覧取得
// @Description  ?ids=<uuid,...> でバッチ取得、?limit= / ?offset= でページング
// @Tags         characters
// @Produce      json
// @Security     InternalAPIKey
// @Param        ids     query     string  false  "カンマ区切りの UUID（指定時はその ID のみ返す）"
// @Param        limit   query     int     false  "最大件数（1..500）"
// @Param        offset  query     int     false  "オフセット"
// @Success      200  {object}  dto.CharacterListResponse
// @Failure      400  {object}  dto.ErrorResponse
// @Failure      500  {object}  dto.ErrorResponse
// @Router       /api/v1/characters [get]
func (h *CharacterHandler) List(c echo.Context) error {
	p, err := parseListParams(c)
	if err != nil {
		return err
	}
	cs, total, err := h.svc.List(c.Request().Context(), p)
	if err != nil {
		return mapCharErr(err)
	}
	return c.JSON(http.StatusOK, dto.NewCharacterListResponse(cs, total))
}

func parseListParams(c echo.Context) (chardomain.ListParams, error) {
	var p chardomain.ListParams

	if raw := strings.TrimSpace(c.QueryParam("ids")); raw != "" {
		p.IDs = []uuid.UUID{} // 空でない ids が来たら「絞り込みあり」を明示
		for _, s := range strings.Split(raw, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			id, err := uuid.Parse(s)
			if err != nil {
				return p, echo.NewHTTPError(http.StatusBadRequest, "invalid id in ids")
			}
			p.IDs = append(p.IDs, id)
		}
	}

	if raw := c.QueryParam("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxListLimit {
			return p, echo.NewHTTPError(http.StatusBadRequest, "limit must be 1..500")
		}
		p.Limit = n
	}
	if raw := c.QueryParam("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return p, echo.NewHTTPError(http.StatusBadRequest, "offset must be >= 0")
		}
		p.Offset = n
	}
	return p, nil
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
	setETag(c, out.Version)
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
	setETag(c, out.Version)
	return c.JSON(http.StatusOK, dto.FromDomain(out))
}

// Replace godoc
// @Summary      キャラクター更新（全置換）
// @Description  PUT は全置換。送らなかった任意項目は NULL になる（値のクリアはこちらで行う）。
// @Tags         characters
// @Accept       json
// @Produce      json
// @Security     InternalAPIKey
// @Param        id        path    string                      true   "キャラクターID (UUID)"
// @Param        If-Match  header  string                      false  "楽観ロック: 期待バージョン（GET の ETag 値）。不一致なら 412"
// @Param        body      body    dto.CreateCharacterRequest  true   "キャラクター全置換リクエスト"
// @Success      200   {object}  dto.CharacterResponse
// @Failure      400   {object}  dto.ErrorResponse
// @Failure      404   {object}  dto.ErrorResponse
// @Failure      412   {object}  dto.ErrorResponse
// @Failure      422   {object}  dto.ErrorResponse
// @Failure      500   {object}  dto.ErrorResponse
// @Router       /api/v1/characters/{id} [put]
func (h *CharacterHandler) Replace(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	expectedVersion, err := parseIfMatch(c)
	if err != nil {
		return err
	}
	var req dto.CreateCharacterRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	if err := c.Validate(&req); err != nil {
		return err
	}
	out, err := h.svc.Replace(c.Request().Context(), id, usecase.CreateInput{
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
	}, expectedVersion)
	if err != nil {
		return mapCharErr(err)
	}
	setETag(c, out.Version)
	return c.JSON(http.StatusOK, dto.FromDomain(out))
}

// Patch godoc
// @Summary      キャラクター更新（部分更新）
// @Description  PATCH は部分更新。送った項目だけ変更し、省略した項目は据え置く（クリアは PUT を使う）。
// @Tags         characters
// @Accept       json
// @Produce      json
// @Security     InternalAPIKey
// @Param        id        path    string                      true   "キャラクターID (UUID)"
// @Param        If-Match  header  string                      false  "楽観ロック: 期待バージョン（GET の ETag 値）。不一致なら 412"
// @Param        body      body    dto.UpdateCharacterRequest  true   "キャラクター部分更新リクエスト"
// @Success      200   {object}  dto.CharacterResponse
// @Failure      400   {object}  dto.ErrorResponse
// @Failure      404   {object}  dto.ErrorResponse
// @Failure      412   {object}  dto.ErrorResponse
// @Failure      422   {object}  dto.ErrorResponse
// @Failure      500   {object}  dto.ErrorResponse
// @Router       /api/v1/characters/{id} [patch]
func (h *CharacterHandler) Patch(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	expectedVersion, err := parseIfMatch(c)
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
	}, expectedVersion)
	if err != nil {
		return mapCharErr(err)
	}
	setETag(c, out.Version)
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

// parseIfMatch は If-Match ヘッダから期待バージョン（楽観ロック）を取り出す。
// 省略 or "*" のときは nil（無条件更新＝後方互換）。`"3"` や `W/"3"` 形式を許容する。
func parseIfMatch(c echo.Context) (*int64, error) {
	raw := strings.TrimSpace(c.Request().Header.Get("If-Match"))
	if raw == "" || raw == "*" {
		return nil, nil
	}
	raw = strings.TrimPrefix(raw, "W/")
	raw = strings.Trim(raw, `"`)
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "invalid If-Match header")
	}
	return &v, nil
}

// setETag は現在のバージョンを ETag ヘッダに載せる（クライアントは次回 If-Match に使う）。
func setETag(c echo.Context, version int64) {
	c.Response().Header().Set("ETag", fmt.Sprintf("%q", strconv.FormatInt(version, 10)))
}

func mapCharErr(err error) error {
	switch {
	case errors.Is(err, chardomain.ErrNotFound):
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	case errors.Is(err, chardomain.ErrInvalidName),
		errors.Is(err, chardomain.ErrInvalidGender):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, chardomain.ErrRaceNotFound),
		errors.Is(err, racedomain.ErrNotFound):
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "race not found")
	case errors.Is(err, chardomain.ErrVersionConflict):
		// 楽観ロック：別の更新が先に入った。クライアントは再取得して再試行する。
		return echo.NewHTTPError(http.StatusPreconditionFailed, "version conflict: resource was modified")
	default:
		// 未分類のエラー（DB 障害など）。詳細はサーバーログにのみ残し、
		// クライアントには内部情報を漏らさない汎用 500 を返す。
		slog.Error("unhandled character error", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
}
