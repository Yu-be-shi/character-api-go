package character

import (
	"context"
	"time"

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
	// Save は予約（pending）として作成する。c.CreationToken が非 nil で同一トークンの
	// 行が既にあれば、二重作成せず既存行を c に反映する（永続的な冪等作成）。
	Save(ctx context.Context, c *Character) error
	// Confirm は予約（pending）を確定（active）して可視化する。冪等。
	// 不在 / 論理削除 / TTL 回収済みは ErrNotFound。
	Confirm(ctx context.Context, id uuid.UUID) (*Character, error)
	// GC は確定されなかった予約（pending）のうち maxAge より古いものを物理回収する。
	// 戻り値は削除件数。可視データ（確定済み）には触れない。
	GC(ctx context.Context, maxAge time.Duration) (int, error)
	// Update は c の状態を永続化し、永続化後の最新状態（version +1・updated_at は
	// DB 側で確定した値）を返す。expectedVersion が非 nil のときは楽観ロックとして
	// version 一致を条件にし、不一致なら ErrVersionConflict を返す。
	Update(ctx context.Context, c *Character, expectedVersion *int64) (*Character, error)
	FindByID(ctx context.Context, id uuid.UUID) (*Character, error)
	List(ctx context.Context, p ListParams) ([]*Character, error)
	// Count は List と同じ絞り込み条件（IDs・論理削除）での総件数を返す。
	// Limit/Offset は無視する（ページャの total 表示用）。
	Count(ctx context.Context, p ListParams) (int64, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
