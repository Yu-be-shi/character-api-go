package gormrepo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	domain "github.com/yu-be-shi/character-api/internal/domain/character"
)

// characterModel は GORM 永続化モデル。
// domain.Character と分離することでドメイン層をORMフリーに保つ。
type characterModel struct {
	ID         uuid.UUID      `gorm:"primaryKey"`
	Name       string
	Attributes datatypes.JSON
	CreatedAt  time.Time `gorm:"autoCreateTime:false"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime:false"`
}

func (characterModel) TableName() string { return "characters" }

func toModel(c *domain.Character) (*characterModel, error) {
	attrs := c.Attributes
	if attrs == nil {
		attrs = domain.Attributes{}
	}
	raw, err := json.Marshal(attrs)
	if err != nil {
		return nil, fmt.Errorf("gormrepo: marshal attributes: %w", err)
	}
	return &characterModel{
		ID:         c.ID,
		Name:       c.Name,
		Attributes: datatypes.JSON(raw),
		CreatedAt:  c.CreatedAt,
		UpdatedAt:  c.UpdatedAt,
	}, nil
}

func fromModel(m *characterModel) (*domain.Character, error) {
	attrs := domain.Attributes{}
	if len(m.Attributes) > 0 {
		if err := json.Unmarshal(m.Attributes, &attrs); err != nil {
			return nil, fmt.Errorf("gormrepo: unmarshal attributes: %w", err)
		}
	}
	return &domain.Character{
		ID:         m.ID,
		Name:       m.Name,
		Attributes: attrs,
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
	}, nil
}

type CharacterRepository struct {
	db *gorm.DB
}

func NewCharacterRepository(db *gorm.DB) *CharacterRepository {
	return &CharacterRepository{db: db}
}

var _ domain.Repository = (*CharacterRepository)(nil)

func (r *CharacterRepository) Save(ctx context.Context, c *domain.Character) error {
	m, err := toModel(c)
	if err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return fmt.Errorf("gormrepo: save: %w", err)
	}
	return nil
}

func (r *CharacterRepository) Update(ctx context.Context, c *domain.Character) error {
	m, err := toModel(c)
	if err != nil {
		return err
	}
	res := r.db.WithContext(ctx).Save(m)
	if res.Error != nil {
		return fmt.Errorf("gormrepo: update: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *CharacterRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Character, error) {
	var m characterModel
	err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("gormrepo: find by id: %w", err)
	}
	return fromModel(&m)
}

func (r *CharacterRepository) List(ctx context.Context) ([]*domain.Character, error) {
	var ms []characterModel
	if err := r.db.WithContext(ctx).Order("created_at ASC").Find(&ms).Error; err != nil {
		return nil, fmt.Errorf("gormrepo: list: %w", err)
	}
	out := make([]*domain.Character, 0, len(ms))
	for i := range ms {
		c, err := fromModel(&ms[i])
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func (r *CharacterRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&characterModel{}, "id = ?", id)
	if res.Error != nil {
		return fmt.Errorf("gormrepo: delete: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
