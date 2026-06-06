package character

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	Save(ctx context.Context, c *Character) error
	Update(ctx context.Context, c *Character) error
	FindByID(ctx context.Context, id uuid.UUID) (*Character, error)
	List(ctx context.Context) ([]*Character, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
