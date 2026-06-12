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
	g.POST("/:id/confirm", h.Confirm) // 予約パターン: 作成(pending)を確定(active)し可視化する
	g.GET("/:id", h.Get)
	g.PUT("/:id", h.Replace) // 全置換（省略した任意項目は NULL になる）
	g.PATCH("/:id", h.Patch) // 部分更新（送った項目だけ変更）
	g.DELETE("/:id", h.Delete)
}

// maxListLimit は ?limit= で指定できる最大件数（無制限取得を防ぐ）。
const maxListLimit = 500

// defaultListLimit は ?limit= 省略時の件数。未指定で全件返すとデータ増加に伴い
// 劣化するため、省略時も必ず上限を適用する（?ids= 指定時は ids の件数が上限）。
const defaultListLimit = 100

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
		// ids の個数にも上限を課す（URL 長の許す限り巨大な ANY クエリを打たせない）。
		if len(p.IDs) > maxListLimit {
			return p, echo.NewHTTPError(http.StatusBadRequest, "too many ids (max 500)")
		}
	}

	if raw := c.QueryParam("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxListLimit {
			return p, echo.NewHTTPError(http.StatusBadRequest, "limit must be 1..500")
		}
		p.Limit = n
	} else if p.IDs == nil {
		// ?ids= によるバッチ取得（件数は ids 自体が上限）以外は、limit 省略でも
		// 全件取得にならないようデフォルトを適用する。
		p.Limit = defaultListLimit
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
	// 作成の冪等トークン。Idempotency-Key ヘッダを永続的な二重作成防止に再利用する
	// （Redis の冪等性ミドルウェアとは独立に、DB の一意制約で同一作成を弾く）。
	var token *string
	if k := strings.TrimSpace(c.Request().Header.Get("Idempotency-Key")); k != "" {
		token = &k
	}
	out, err := h.svc.Create(c.Request().Context(), usecase.CreateInput{
		Name:          req.Name,
		Description:   req.Description,
		RaceID:        req.RaceID,
		Gender:        chardomain.Gender(req.Gender),
		BirthDate:     req.BirthDate,
		BirthPlace:    req.BirthPlace,
		HeightCm:      req.HeightCm,
		WeightKg:      req.WeightKg,
		BodyFat:       req.BodyFat,
		SizeTop:       req.SizeTop,
		SizeMiddle:    req.SizeMiddle,
		SizeBottom:    req.SizeBottom,
		CreationToken: token,
	})
	if err != nil {
		return mapCharErr(err)
	}
	if out.Replayed {
		// DB creation_token が既存行を返した（Redis ミス後の DB リプレイ）。
		// 冪等ミドルウェアに「この結果を Redis に保存しない」よう通知し、
		// Redis の bodyHash が現リクエストの値で上書きされるのを防ぐ。
		c.Response().Header().Set("Idempotent-Replayed", "true")
	}
	setETag(c, out.Version)
	return c.JSON(http.StatusCreated, dto.FromDomain(out))
}

// Confirm godoc
// @Summary      キャラクター予約の確定
// @Description  予約パターン: 作成直後の pending を確定（active）し、一覧/取得に出るようにする。冪等。
// @Tags         characters
// @Produce      json
// @Security     InternalAPIKey
// @Param        id   path      string  true  "キャラクターID (UUID)"
// @Success      200  {object}  dto.CharacterResponse
// @Failure      400  {object}  dto.ErrorResponse
// @Failure      404  {object}  dto.ErrorResponse
// @Failure      500  {object}  dto.ErrorResponse
// @Router       /api/v1/characters/{id}/confirm [post]
func (h *CharacterHandler) Confirm(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Confirm(c.Request().Context(), id)
	if err != nil {
		return mapCharErr(err)
	}
	setETag(c, out.Version)
	return c.JSON(http.StatusOK, dto.FromDomain(out))
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
	out, err := h.svc.Replace(c.Request().Context(), id, usecase.ReplaceInput{
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
// 省略 or "*" のときは nil を返す。nil の場合でも usecase 層が読み取り時点の
// version を期待値として使うため「無条件上書き」にはならない（lost update 防止。
// 競合すれば 412 が返り得る）。`"3"` 形式のみ受理する。
// 弱い検証子（W/"3"）は RFC 9110 §13.1.1 のとおり If-Match の強い比較では
// 決して一致しないため、412 を返す。
// https://www.rfc-editor.org/rfc/rfc9110#name-if-match
func parseIfMatch(c echo.Context) (*int64, error) {
	raw := strings.TrimSpace(c.Request().Header.Get("If-Match"))
	if raw == "" || raw == "*" {
		return nil, nil
	}
	if strings.HasPrefix(raw, "W/") {
		return nil, echo.NewHTTPError(http.StatusPreconditionFailed,
			"weak entity-tag never matches If-Match (RFC 9110)")
	}
	v, err := strconv.ParseInt(strings.Trim(raw, `"`), 10, 64)
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
	case errors.Is(err, chardomain.ErrCreationTokenConsumed):
		// 論理削除済み行が creation_token を保持し、再生行を引けない衝突。
		// クライアントは新しい Idempotency-Key で再試行する必要がある。
		return echo.NewHTTPError(http.StatusConflict, "creation token already consumed: retry with a new Idempotency-Key")
	default:
		// 未分類のエラー（DB 障害など）。詳細はサーバーログにのみ残し、
		// クライアントには内部情報を漏らさない汎用 500 を返す。
		slog.Error("unhandled character error", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
}
