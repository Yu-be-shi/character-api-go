package gormrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	domain "github.com/yu-be-shi/character-api/internal/domain/race"
)

type raceModel struct {
	ID   uuid.UUID `gorm:"primaryKey"`
	Name string    `gorm:"uniqueIndex"`
}

func (raceModel) TableName() string { return "races" }

func toRaceModel(r *domain.Race) *raceModel {
	return &raceModel{ID: r.ID, Name: r.Name}
}

func fromRaceModel(m *raceModel) *domain.Race {
	return &domain.Race{ID: m.ID, Name: m.Name}
}

type RaceRepository struct {
	db *gorm.DB
}

func NewRaceRepository(db *gorm.DB) *RaceRepository {
	return &RaceRepository{db: db}
}

var _ domain.Repository = (*RaceRepository)(nil)

func (r *RaceRepository) Save(ctx context.Context, race *domain.Race) error {
	m := toRaceModel(race)
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return domain.ErrDuplicate
		}
		return fmt.Errorf("gormrepo: save race: %w", err)
	}
	return nil
}

func (r *RaceRepository) Update(ctx context.Context, race *domain.Race) error {
	m := toRaceModel(race)
	res := r.db.WithContext(ctx).Save(m)
	if res.Error != nil {
		if errors.Is(res.Error, gorm.ErrDuplicatedKey) {
			return domain.ErrDuplicate
		}
		return fmt.Errorf("gormrepo: update race: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *RaceRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Race, error) {
	var m raceModel
	err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("gormrepo: find race by id: %w", err)
	}
	return fromRaceModel(&m), nil
}

func (r *RaceRepository) List(ctx context.Context) ([]*domain.Race, error) {
	var ms []raceModel
	if err := r.db.WithContext(ctx).Order("name ASC").Find(&ms).Error; err != nil {
		return nil, fmt.Errorf("gormrepo: list races: %w", err)
	}
	out := make([]*domain.Race, 0, len(ms))
	for i := range ms {
		out = append(out, fromRaceModel(&ms[i]))
	}
	return out, nil
}

func (r *RaceRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&raceModel{}, "id = ?", id)
	if res.Error != nil {
		// character から参照中の race は FK 制約（ON DELETE NO ACTION）で削除できない。
		if errors.Is(res.Error, gorm.ErrForeignKeyViolated) {
			return domain.ErrInUse
		}
		return fmt.Errorf("gormrepo: delete race: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
