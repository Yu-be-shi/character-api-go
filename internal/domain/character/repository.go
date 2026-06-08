package character

import (
	"context"

	"github.com/google/uuid"
)

// ListParams は一覧取得の絞り込み・ページングを表す。
//   - IDs が非空: その ID のみを返す（バッチ取得。N+1 回避用）
//   - Limit > 0 : 最大件数。Offset と併用してページング
type ListParams struct {
	IDs    []uuid.UUID
	Limit  int
	Offset int
}

type Repository interface {
	Save(ctx context.Context, c *Character) error
	Update(ctx context.Context, c *Character) error
	FindByID(ctx context.Context, id uuid.UUID) (*Character, error)
	List(ctx context.Context, p ListParams) ([]*Character, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
