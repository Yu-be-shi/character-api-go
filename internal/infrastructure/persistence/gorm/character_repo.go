package gormrepo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	domain "github.com/yu-be-shi/character-api/internal/domain/character"
)

// coreCharacterModel は core_characters テーブルの GORM モデル。
// ドメイン層を ORM フリーに保つため domain.Character とは分離する。
type coreCharacterModel struct {
	ID          uuid.UUID `gorm:"primaryKey"`
	Name        string
	Description string
	RaceID      uuid.UUID
	Gender      string
	BirthDate   *time.Time
	BirthPlace  string
	HeightCm    *int16
	WeightKg    *int16
	BodyFat     *float32 `gorm:"column:body_fat_percentage"`
	SizeTop     *int16
	SizeMiddle  *int16
	SizeBottom  *int16
	Version     int64
	CreatedAt   time.Time  `gorm:"autoCreateTime:false"`
	UpdatedAt   time.Time  `gorm:"autoUpdateTime:false"`
	DeletedAt   *time.Time `gorm:"index"`

	// JOIN 結果（reads のみ）
	RaceName string `gorm:"->;column:race_name"`
}

func (coreCharacterModel) TableName() string { return "core_characters" }

func toCharacterModel(c *domain.Character) *coreCharacterModel {
	return &coreCharacterModel{
		ID:          c.ID,
		Name:        c.Name,
		Description: c.Description,
		RaceID:      c.RaceID,
		Gender:      string(c.Gender),
		BirthDate:   c.BirthDate,
		BirthPlace:  c.BirthPlace,
		HeightCm:    c.HeightCm,
		WeightKg:    c.WeightKg,
		BodyFat:     c.BodyFat,
		SizeTop:     c.SizeTop,
		SizeMiddle:  c.SizeMiddle,
		SizeBottom:  c.SizeBottom,
		Version:     c.Version,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
	}
}

func fromCharacterModel(m *coreCharacterModel) *domain.Character {
	return &domain.Character{
		ID:          m.ID,
		Name:        m.Name,
		Description: m.Description,
		RaceID:      m.RaceID,
		RaceName:    m.RaceName,
		Gender:      domain.Gender(m.Gender),
		BirthDate:   m.BirthDate,
		BirthPlace:  m.BirthPlace,
		HeightCm:    m.HeightCm,
		WeightKg:    m.WeightKg,
		BodyFat:     m.BodyFat,
		SizeTop:     m.SizeTop,
		SizeMiddle:  m.SizeMiddle,
		SizeBottom:  m.SizeBottom,
		Version:     m.Version,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

// withRace は races テーブルを JOIN して race_name を取得するクエリを返す。
func withRace(db *gorm.DB) *gorm.DB {
	return db.
		Select("core_characters.*, races.name AS race_name").
		Joins("LEFT JOIN races ON races.id = core_characters.race_id").
		Where("core_characters.deleted_at IS NULL")
}

type CharacterRepository struct {
	db *gorm.DB
}

func NewCharacterRepository(db *gorm.DB) *CharacterRepository {
	return &CharacterRepository{db: db}
}

var _ domain.Repository = (*CharacterRepository)(nil)

func (r *CharacterRepository) Save(ctx context.Context, c *domain.Character) error {
	m := toCharacterModel(c)
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		// 参照先 race が無いと FK 違反になる。事前 SELECT せず、ここでドメインエラーへ変換する。
		if errors.Is(err, gorm.ErrForeignKeyViolated) {
			return domain.ErrRaceNotFound
		}
		return fmt.Errorf("gormrepo: save character: %w", err)
	}
	return nil
}

// updateAssignments は UPDATE で書き込む列の集合（map なので nil/空も NULL/空として確実に反映される。
// GORM の Updates(struct) はゼロ値をスキップするため map を使う）。version は +1 し、id /
// created_at / deleted_at は対象外。updated_at は DB トリガーでも更新されるが明示しておく。
func updateAssignments(c *domain.Character) map[string]any {
	return map[string]any{
		"name":                c.Name,
		"description":         c.Description,
		"race_id":             c.RaceID,
		"gender":              string(c.Gender),
		"birth_date":          c.BirthDate,
		"birth_place":         c.BirthPlace,
		"height_cm":           c.HeightCm,
		"weight_kg":           c.WeightKg,
		"body_fat_percentage": c.BodyFat,
		"size_top":            c.SizeTop,
		"size_middle":         c.SizeMiddle,
		"size_bottom":         c.SizeBottom,
		"updated_at":          c.UpdatedAt,
		"version":             gorm.Expr("version + 1"),
	}
}

// Update は domain.Character の現在状態（PUT=全置換 / PATCH=部分適用済み）を全列上書きで永続化する。
// expectedVersion が非 nil なら version 一致を条件にし、不一致は ErrVersionConflict。
func (r *CharacterRepository) Update(ctx context.Context, c *domain.Character, expectedVersion *int64) error {
	q := r.db.WithContext(ctx).
		Model(&coreCharacterModel{}).
		Where("id = ? AND deleted_at IS NULL", c.ID)
	if expectedVersion != nil {
		q = q.Where("version = ?", *expectedVersion)
	}
	res := q.Updates(updateAssignments(c))
	if res.Error != nil {
		if errors.Is(res.Error, gorm.ErrForeignKeyViolated) {
			return domain.ErrRaceNotFound
		}
		return fmt.Errorf("gormrepo: update character: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		// version 指定時は「不在」か「版不一致」かを区別する（不一致の検出は競合時のみの追加クエリ）。
		if expectedVersion != nil {
			var n int64
			if err := r.db.WithContext(ctx).Model(&coreCharacterModel{}).
				Where("id = ? AND deleted_at IS NULL", c.ID).Count(&n).Error; err != nil {
				return fmt.Errorf("gormrepo: update character (conflict check): %w", err)
			}
			if n > 0 {
				return domain.ErrVersionConflict
			}
		}
		return domain.ErrNotFound
	}
	return nil
}

func (r *CharacterRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Character, error) {
	var m coreCharacterModel
	err := withRace(r.db.WithContext(ctx)).
		First(&m, "core_characters.id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("gormrepo: find character by id: %w", err)
	}
	return fromCharacterModel(&m), nil
}

func (r *CharacterRepository) List(ctx context.Context, p domain.ListParams) ([]*domain.Character, error) {
	// IDs 指定で空スライスのときは「該当なし」を意味するので空を返す（全件返さない）。
	if p.IDs != nil && len(p.IDs) == 0 {
		return []*domain.Character{}, nil
	}
	q := withRace(r.db.WithContext(ctx)).Order("core_characters.created_at ASC")
	if len(p.IDs) > 0 {
		q = q.Where("core_characters.id IN ?", p.IDs)
	}
	if p.Limit > 0 {
		q = q.Limit(p.Limit)
	}
	if p.Offset > 0 {
		q = q.Offset(p.Offset)
	}
	var ms []coreCharacterModel
	if err := q.Find(&ms).Error; err != nil {
		return nil, fmt.Errorf("gormrepo: list characters: %w", err)
	}
	out := make([]*domain.Character, 0, len(ms))
	for i := range ms {
		out = append(out, fromCharacterModel(&ms[i]))
	}
	return out, nil
}

// Count は List と同じ絞り込み（論理削除・IDs）での総件数を返す（Limit/Offset は無視）。
func (r *CharacterRepository) Count(ctx context.Context, p domain.ListParams) (int64, error) {
	if p.IDs != nil && len(p.IDs) == 0 {
		return 0, nil
	}
	q := r.db.WithContext(ctx).
		Model(&coreCharacterModel{}).
		Where("deleted_at IS NULL")
	if len(p.IDs) > 0 {
		q = q.Where("id IN ?", p.IDs)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return 0, fmt.Errorf("gormrepo: count characters: %w", err)
	}
	return n, nil
}

// Delete は論理削除（deleted_at を現在時刻に設定）。
func (r *CharacterRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).
		Model(&coreCharacterModel{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Update("deleted_at", gorm.Expr("NOW()"))
	if res.Error != nil {
		return fmt.Errorf("gormrepo: delete character: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
