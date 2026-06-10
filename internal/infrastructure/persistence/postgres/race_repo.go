package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	domain "github.com/yu-be-shi/character-api/internal/domain/race"
	"github.com/yu-be-shi/character-api/internal/infrastructure/persistence/postgres/sqlc"
)

type RaceRepository struct {
	q *sqlc.Queries
}

func NewRaceRepository(db sqlc.DBTX) *RaceRepository {
	return &RaceRepository{q: sqlc.New(db)}
}

var _ domain.Repository = (*RaceRepository)(nil)

func (r *RaceRepository) Save(ctx context.Context, race *domain.Race) error {
	err := r.q.CreateRace(ctx, sqlc.CreateRaceParams{ID: race.ID, Name: race.Name})
	if err != nil {
		if code, ok := pgCode(err); ok && code == sqlStateUnique {
			return domain.ErrDuplicate
		}
		return fmt.Errorf("postgres: save race: %w", err)
	}
	return nil
}

func (r *RaceRepository) Update(ctx context.Context, race *domain.Race) error {
	rows, err := r.q.UpdateRace(ctx, sqlc.UpdateRaceParams{ID: race.ID, Name: race.Name})
	if err != nil {
		if code, ok := pgCode(err); ok && code == sqlStateUnique {
			return domain.ErrDuplicate
		}
		return fmt.Errorf("postgres: update race: %w", err)
	}
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *RaceRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Race, error) {
	row, err := r.q.GetRace(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("postgres: find race by id: %w", err)
	}
	return &domain.Race{ID: row.ID, Name: row.Name}, nil
}

func (r *RaceRepository) List(ctx context.Context) ([]*domain.Race, error) {
	rows, err := r.q.ListRaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: list races: %w", err)
	}
	out := make([]*domain.Race, 0, len(rows))
	for _, row := range rows {
		out = append(out, &domain.Race{ID: row.ID, Name: row.Name})
	}
	return out, nil
}

// Delete は races を物理削除。character から参照中なら FK 違反 → ErrInUse。
func (r *RaceRepository) Delete(ctx context.Context, id uuid.UUID) error {
	rows, err := r.q.DeleteRace(ctx, id)
	if err != nil {
		if code, ok := pgCode(err); ok && code == sqlStateForeignKey {
			return domain.ErrInUse
		}
		return fmt.Errorf("postgres: delete race: %w", err)
	}
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}
