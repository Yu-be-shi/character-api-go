package race

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	Save(ctx context.Context, r *Race) error
	Update(ctx context.Context, r *Race) error
	FindByID(ctx context.Context, id uuid.UUID) (*Race, error)
	List(ctx context.Context) ([]*Race, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
