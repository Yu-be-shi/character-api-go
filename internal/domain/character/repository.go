package character

import (
	"context"

	"github.com/google/uuid"
)

// Repository はキャラクターの永続化契約。
// 実装は internal/infrastructure/persistence/ 配下に置く。
// メソッドシグネチャに GORM・SQL 型を含めない。
type Repository interface {
	Save(ctx context.Context, c *Character) error
	Update(ctx context.Context, c *Character) error
	FindByID(ctx context.Context, id uuid.UUID) (*Character, error)
	List(ctx context.Context) ([]*Character, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
