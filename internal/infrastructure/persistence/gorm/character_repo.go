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
	ID          uuid.UUID  `gorm:"primaryKey"`
	Name        string
	Description string
	RaceID      uuid.UUID
	Gender      string
	BirthDate   *time.Time
	BirthPlace  string
	HeightCm    *int16
	WeightKg    *int16
	BodyFat     *float32   `gorm:"column:body_fat_percentage"`
	SizeTop     *int16
	SizeMiddle  *int16
	SizeBottom  *int16
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
		return fmt.Errorf("gormrepo: save character: %w", err)
	}
	return nil
}

func (r *CharacterRepository) Update(ctx context.Context, c *domain.Character) error {
	m := toCharacterModel(c)
	res := r.db.WithContext(ctx).
		Model(m).
		Where("deleted_at IS NULL").
		Updates(m)
	if res.Error != nil {
		return fmt.Errorf("gormrepo: update character: %w", res.Error)
	}
	if res.RowsAffected == 0 {
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

func (r *CharacterRepository) List(ctx context.Context) ([]*domain.Character, error) {
	var ms []coreCharacterModel
	err := withRace(r.db.WithContext(ctx)).
		Order("core_characters.created_at ASC").
		Find(&ms).Error
	if err != nil {
		return nil, fmt.Errorf("gormrepo: list characters: %w", err)
	}
	out := make([]*domain.Character, 0, len(ms))
	for i := range ms {
		out = append(out, fromCharacterModel(&ms[i]))
	}
	return out, nil
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
