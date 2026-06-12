package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	domain "github.com/yu-be-shi/character-api/internal/domain/character"
	"github.com/yu-be-shi/character-api/internal/infrastructure/persistence/postgres/sqlc"
)

type CharacterRepository struct {
	q *sqlc.Queries
}

func NewCharacterRepository(db sqlc.DBTX) *CharacterRepository {
	return &CharacterRepository{q: sqlc.New(db)}
}

var _ domain.Repository = (*CharacterRepository)(nil)

func (r *CharacterRepository) Save(ctx context.Context, c *domain.Character) error {
	bodyFat, err := pgNumeric(c.BodyFat)
	if err != nil {
		return fmt.Errorf("postgres: save character: %w", err)
	}
	rows, err := r.q.CreateCharacter(ctx, sqlc.CreateCharacterParams{
		ID:   c.ID,
		Name: c.Name,
		// 任意のテキスト項目は空文字を NULL に正規化して保存する
		// （複数 API が共有する DB で '' と NULL の意味を揺らさない）。
		Description:       pgTextOrNull(c.Description),
		RaceID:            c.RaceID,
		Gender:            sqlc.GenderEnum(c.Gender),
		BirthDate:         pgDate(c.BirthDate),
		BirthPlace:        pgTextOrNull(c.BirthPlace),
		HeightCm:          pgInt2(c.HeightCm),
		WeightKg:          pgInt2(c.WeightKg),
		BodyFatPercentage: bodyFat,
		SizeTop:           pgInt2(c.SizeTop),
		SizeMiddle:        pgInt2(c.SizeMiddle),
		SizeBottom:        pgInt2(c.SizeBottom),
		Version:           c.Version,
		// 作成は予約（pending）。confirmed_at は DB 既定の NULL のまま（明示指定しない）。
		CreationToken: pgTextPtr(c.CreationToken),
		CreatedAt:     pgTimestamptz(c.CreatedAt),
		UpdatedAt:     pgTimestamptz(c.UpdatedAt),
	})
	if err != nil {
		// 参照先 race が無いと FK 違反。事前 SELECT せずここでドメインエラーへ変換する。
		if code, ok := pgCode(err); ok && code == sqlStateForeignKey {
			return domain.ErrRaceNotFound
		}
		return fmt.Errorf("postgres: save character: %w", err)
	}
	if rows == 0 {
		// creation_token 重複（ON CONFLICT DO NOTHING で 0 行）＝既に作成済みの再送。
		// 既存行を引いて c へ反映し、冪等な再生（同一 id/状態を返す）にする。
		if c.CreationToken == nil {
			return fmt.Errorf("postgres: save character: no row inserted without creation token")
		}
		row, err := r.q.GetCharacterByToken(ctx, pgText(*c.CreationToken))
		if err != nil {
			return fmt.Errorf("postgres: save character: fetch existing by token: %w", err)
		}
		*c = *characterFromRow(sqlc.GetCharacterRow(row))
		c.Replayed = true // DB creation_token replay: Redis に誤った bodyHash を記録させない
		return nil
	}
	return nil
}

// Confirm は予約（pending）を確定（active）する。冪等。
// 不在 / 論理削除 / TTL 回収済みは CH404 → ErrNotFound（呼び出し側は確定不能を検知できる）。
func (r *CharacterRepository) Confirm(ctx context.Context, id uuid.UUID) (*domain.Character, error) {
	row, err := r.q.ConfirmCharacter(ctx, id)
	if err != nil {
		if code, ok := pgCode(err); ok && code == sqlStateNotFound {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("postgres: confirm character: %w", err)
	}
	return characterFromRow(sqlc.GetCharacterRow(row)), nil
}

// GC は確定されなかった予約（pending）を maxAge より古いものに限り物理回収する。
// 戻り値は削除件数。origin 自身の後始末で、可視データ（確定済み）には触れない。
func (r *CharacterRepository) GC(ctx context.Context, maxAge time.Duration) (int, error) {
	secs := int32(maxAge.Seconds())
	if secs < 0 {
		secs = 0
	}
	n, err := r.q.GCUnconfirmedCharacters(ctx, secs)
	if err != nil {
		return 0, fmt.Errorf("postgres: gc unconfirmed characters: %w", err)
	}
	return int(n), nil
}

// Update は楽観ロック・全列上書き・version+1 を DB 関数 update_character に委ねる。
// expectedVersion が非 nil なら version 一致を条件にし、不一致は CH412 → ErrVersionConflict。
// 関数の RETURNING（更新後の行）をそのまま返すため、更新と読み取りが同一文で原子的に行われる
// （更新成功後の再 SELECT による競合窓・余分な往復が無い）。
func (r *CharacterRepository) Update(ctx context.Context, c *domain.Character, expectedVersion *int64) (*domain.Character, error) {
	bodyFat, err := pgNumeric(c.BodyFat)
	if err != nil {
		return nil, fmt.Errorf("postgres: update character: %w", err)
	}
	row, err := r.q.UpdateCharacter(ctx, sqlc.UpdateCharacterParams{
		ID:                c.ID,
		ExpectedVersion:   pgInt8(expectedVersion),
		Name:              c.Name,
		Description:       pgTextOrNull(c.Description),
		RaceID:            c.RaceID,
		Gender:            sqlc.GenderEnum(c.Gender),
		BirthDate:         pgDate(c.BirthDate),
		BirthPlace:        pgTextOrNull(c.BirthPlace),
		HeightCm:          pgInt2(c.HeightCm),
		WeightKg:          pgInt2(c.WeightKg),
		BodyFatPercentage: bodyFat,
		SizeTop:           pgInt2(c.SizeTop),
		SizeMiddle:        pgInt2(c.SizeMiddle),
		SizeBottom:        pgInt2(c.SizeBottom),
	})
	if err != nil {
		if code, ok := pgCode(err); ok {
			switch code {
			case sqlStateVersionConflict:
				return nil, domain.ErrVersionConflict
			case sqlStateNotFound:
				return nil, domain.ErrNotFound
			case sqlStateForeignKey:
				return nil, domain.ErrRaceNotFound
			}
		}
		return nil, fmt.Errorf("postgres: update character: %w", err)
	}
	return characterFromRow(sqlc.GetCharacterRow(row)), nil
}

func (r *CharacterRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Character, error) {
	row, err := r.q.GetCharacter(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("postgres: find character by id: %w", err)
	}
	return characterFromRow(row), nil
}

func (r *CharacterRepository) List(ctx context.Context, p domain.ListParams) ([]*domain.Character, error) {
	// IDs が空スライス（非 nil）のときは「該当なし」。全件返さないため DB に問い合わせない。
	if p.IDs != nil && len(p.IDs) == 0 {
		return []*domain.Character{}, nil
	}
	rows, err := r.q.ListCharacters(ctx, sqlc.ListCharactersParams{
		Ids: p.IDs, // nil なら $1 IS NULL で全件
		Off: int64(p.Offset),
		Lim: int64(p.Limit), // 0 なら NULLIF で LIMIT 無し
	})
	if err != nil {
		return nil, fmt.Errorf("postgres: list characters: %w", err)
	}
	out := make([]*domain.Character, 0, len(rows))
	for _, row := range rows {
		out = append(out, characterFromRow(sqlc.GetCharacterRow(row)))
	}
	return out, nil
}

func (r *CharacterRepository) Count(ctx context.Context, p domain.ListParams) (int64, error) {
	if p.IDs != nil && len(p.IDs) == 0 {
		return 0, nil
	}
	n, err := r.q.CountCharacters(ctx, p.IDs)
	if err != nil {
		return 0, fmt.Errorf("postgres: count characters: %w", err)
	}
	return n, nil
}

// Delete は論理削除（soft_delete_character）。不在/削除済みは CH404 → ErrNotFound。
func (r *CharacterRepository) Delete(ctx context.Context, id uuid.UUID) error {
	err := r.q.SoftDeleteCharacter(ctx, id)
	if err != nil {
		if code, ok := pgCode(err); ok && code == sqlStateNotFound {
			return domain.ErrNotFound
		}
		return fmt.Errorf("postgres: delete character: %w", err)
	}
	return nil
}

// characterFromRow はクエリ結果行 → ドメインの組み立て。
// GetCharacterRow / ListCharactersRow / UpdateCharacterRow はフィールド構成が同一のため、
// 呼び出し側で sqlc.GetCharacterRow(row) の struct conversion を介して共通化する
// （フィールドがずれるとコンパイルエラーで検知できる）。
func characterFromRow(row sqlc.GetCharacterRow) *domain.Character {
	return &domain.Character{
		ID:          row.ID,
		Name:        row.Name,
		Description: textString(row.Description),
		RaceID:      row.RaceID,
		RaceName:    textString(row.RaceName),
		Gender:      domain.Gender(row.Gender),
		BirthDate:   datePtr(row.BirthDate),
		BirthPlace:  textString(row.BirthPlace),
		HeightCm:    int2Ptr(row.HeightCm),
		WeightKg:    int2Ptr(row.WeightKg),
		BodyFat:     numericPtr(row.BodyFatPercentage),
		SizeTop:     int2Ptr(row.SizeTop),
		SizeMiddle:  int2Ptr(row.SizeMiddle),
		SizeBottom:  int2Ptr(row.SizeBottom),
		Version:     row.Version,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
}
