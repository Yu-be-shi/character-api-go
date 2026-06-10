package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

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
	err := r.q.CreateCharacter(ctx, sqlc.CreateCharacterParams{
		ID:                c.ID,
		Name:              c.Name,
		Description:       pgText(c.Description),
		RaceID:            c.RaceID,
		Gender:            sqlc.GenderEnum(c.Gender),
		BirthDate:         pgDate(c.BirthDate),
		BirthPlace:        pgText(c.BirthPlace),
		HeightCm:          pgInt2(c.HeightCm),
		WeightKg:          pgInt2(c.WeightKg),
		BodyFatPercentage: pgNumeric(c.BodyFat),
		SizeTop:           pgInt2(c.SizeTop),
		SizeMiddle:        pgInt2(c.SizeMiddle),
		SizeBottom:        pgInt2(c.SizeBottom),
		Version:           c.Version,
		CreatedAt:         pgTimestamptz(c.CreatedAt),
		UpdatedAt:         pgTimestamptz(c.UpdatedAt),
	})
	if err != nil {
		// 参照先 race が無いと FK 違反。事前 SELECT せずここでドメインエラーへ変換する。
		if code, ok := pgCode(err); ok && code == sqlStateForeignKey {
			return domain.ErrRaceNotFound
		}
		return fmt.Errorf("postgres: save character: %w", err)
	}
	return nil
}

// Update は楽観ロック・全列上書き・version+1 を DB 関数 update_character に委ねる。
// expectedVersion が非 nil なら version 一致を条件にし、不一致は CH412 → ErrVersionConflict。
func (r *CharacterRepository) Update(ctx context.Context, c *domain.Character, expectedVersion *int64) error {
	err := r.q.UpdateCharacter(ctx, sqlc.UpdateCharacterParams{
		ID:                c.ID,
		ExpectedVersion:   pgInt8(expectedVersion),
		Name:              c.Name,
		Description:       c.Description,
		RaceID:            c.RaceID,
		Gender:            sqlc.GenderEnum(c.Gender),
		BirthDate:         pgDate(c.BirthDate),
		BirthPlace:        c.BirthPlace,
		HeightCm:          pgInt2(c.HeightCm),
		WeightKg:          pgInt2(c.WeightKg),
		BodyFatPercentage: pgNumeric(c.BodyFat),
		SizeTop:           pgInt2(c.SizeTop),
		SizeMiddle:        pgInt2(c.SizeMiddle),
		SizeBottom:        pgInt2(c.SizeBottom),
	})
	if err != nil {
		if code, ok := pgCode(err); ok {
			switch code {
			case sqlStateVersionConflict:
				return domain.ErrVersionConflict
			case sqlStateNotFound:
				return domain.ErrNotFound
			case sqlStateForeignKey:
				return domain.ErrRaceNotFound
			}
		}
		return fmt.Errorf("postgres: update character: %w", err)
	}
	return nil
}

func (r *CharacterRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Character, error) {
	row, err := r.q.GetCharacter(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("postgres: find character by id: %w", err)
	}
	return characterFromRow(
		row.ID, row.Name, row.Description, row.RaceID, row.Gender,
		row.BirthDate, row.BirthPlace, row.HeightCm, row.WeightKg, row.BodyFatPercentage,
		row.SizeTop, row.SizeMiddle, row.SizeBottom, row.Version,
		row.CreatedAt, row.UpdatedAt, row.RaceName,
	), nil
}

func (r *CharacterRepository) List(ctx context.Context, p domain.ListParams) ([]*domain.Character, error) {
	// IDs が空スライス（非 nil）のときは「該当なし」。全件返さないため DB に問い合わせない。
	if p.IDs != nil && len(p.IDs) == 0 {
		return []*domain.Character{}, nil
	}
	rows, err := r.q.ListCharacters(ctx, sqlc.ListCharactersParams{
		Ids: p.IDs, // nil なら $1 IS NULL で全件
		Off: int32(p.Offset),
		Lim: int32(p.Limit), // 0 なら NULLIF で LIMIT 無し
	})
	if err != nil {
		return nil, fmt.Errorf("postgres: list characters: %w", err)
	}
	out := make([]*domain.Character, 0, len(rows))
	for _, row := range rows {
		out = append(out, characterFromRow(
			row.ID, row.Name, row.Description, row.RaceID, row.Gender,
			row.BirthDate, row.BirthPlace, row.HeightCm, row.WeightKg, row.BodyFatPercentage,
			row.SizeTop, row.SizeMiddle, row.SizeBottom, row.Version,
			row.CreatedAt, row.UpdatedAt, row.RaceName,
		))
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

// characterFromRow は GetCharacterRow / ListCharactersRow（同一フィールド）共通の組み立て。
func characterFromRow(
	id uuid.UUID, name string, description pgtype.Text, raceID uuid.UUID, gender sqlc.GenderEnum,
	birthDate pgtype.Date, birthPlace pgtype.Text, heightCm, weightKg pgtype.Int2,
	bodyFat pgtype.Numeric, sizeTop, sizeMiddle, sizeBottom pgtype.Int2, version int64,
	createdAt, updatedAt pgtype.Timestamptz, raceName pgtype.Text,
) *domain.Character {
	return &domain.Character{
		ID:          id,
		Name:        name,
		Description: textString(description),
		RaceID:      raceID,
		RaceName:    textString(raceName),
		Gender:      domain.Gender(gender),
		BirthDate:   datePtr(birthDate),
		BirthPlace:  textString(birthPlace),
		HeightCm:    int2Ptr(heightCm),
		WeightKg:    int2Ptr(weightKg),
		BodyFat:     numericPtr(bodyFat),
		SizeTop:     int2Ptr(sizeTop),
		SizeMiddle:  int2Ptr(sizeMiddle),
		SizeBottom:  int2Ptr(sizeBottom),
		Version:     version,
		CreatedAt:   createdAt.Time,
		UpdatedAt:   updatedAt.Time,
	}
}
