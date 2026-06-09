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
	// Update は c の状態を永続化する。expectedVersion が非 nil のときは
	// 楽観ロックとして version 一致を条件にし、不一致なら ErrVersionConflict を返す。
	Update(ctx context.Context, c *Character, expectedVersion *int64) error
	FindByID(ctx context.Context, id uuid.UUID) (*Character, error)
	List(ctx context.Context, p ListParams) ([]*Character, error)
	// Count は List と同じ絞り込み条件（IDs・論理削除）での総件数を返す。
	// Limit/Offset は無視する（ページャの total 表示用）。
	Count(ctx context.Context, p ListParams) (int64, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
