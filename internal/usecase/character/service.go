package character

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	domain "github.com/yu-be-shi/character-api/internal/domain/character"
)

type Clock func() time.Time

type Service struct {
	repo  domain.Repository
	clock Clock
}

// NewService は character ユースケースを生成する。
// race_id の存在確認は事前 SELECT せず、永続化時の FK 制約違反を
// domain.ErrRaceNotFound として扱う（race リポジトリへの依存は持たない）。
func NewService(repo domain.Repository, clock Clock) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{repo: repo, clock: clock}
}

type CreateInput struct {
	Name        string
	Description string
	RaceID      uuid.UUID
	Gender      domain.Gender
	BirthDate   *time.Time
	BirthPlace  string
	HeightCm    *int16
	WeightKg    *int16
	BodyFat     *float32
	SizeTop     *int16
	SizeMiddle  *int16
	SizeBottom  *int16
	// CreationToken は作成の冪等トークン（消費者の Idempotency-Key。任意）。
	// ※ ReplaceInput(=CreateInput) を ReplaceFields へ変換する箇所があるため、
	//   PUT には無いこのフィールドは toReplaceFields で明示的に落とす。
	CreationToken *string
}

// Create は予約（pending）としてキャラクターを作成する。可視化するには所有者を紐づけた後に
// Confirm を呼ぶ（予約パターン）。CreationToken が指定され同一トークンの行が既にあれば、
// 二重作成せず既存の状態を返す（永続的な冪等作成）。
func (s *Service) Create(ctx context.Context, in CreateInput) (*domain.Character, error) {
	c, err := domain.New(in.Name, in.Description, in.RaceID, in.Gender, s.now())
	if err != nil {
		return nil, err
	}
	c.BirthDate = in.BirthDate
	c.BirthPlace = in.BirthPlace
	c.HeightCm = in.HeightCm
	c.WeightKg = in.WeightKg
	c.BodyFat = in.BodyFat
	c.SizeTop = in.SizeTop
	c.SizeMiddle = in.SizeMiddle
	c.SizeBottom = in.SizeBottom
	c.CreationToken = in.CreationToken

	// 事前 SELECT せず保存。race が無ければ FK 違反 → domain.ErrRaceNotFound が返る。
	if err := s.repo.Save(ctx, c); err != nil {
		return nil, fmt.Errorf("usecase create character: %w", err)
	}
	return c, nil
}

// Confirm は予約（pending）を確定（active）し可視化する。冪等。
func (s *Service) Confirm(ctx context.Context, id uuid.UUID) (*domain.Character, error) {
	c, err := s.repo.Confirm(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("usecase confirm character: %w", err)
	}
	return c, nil
}

// SweepUnconfirmed は確定されなかった予約（pending）のうち maxAge より古いものを回収する。
// 戻り値は削除件数。API のバックグラウンドスイーパーが定期的に呼ぶ。
func (s *Service) SweepUnconfirmed(ctx context.Context, maxAge time.Duration) (int, error) {
	n, err := s.repo.GC(ctx, maxAge)
	if err != nil {
		return 0, fmt.Errorf("usecase sweep unconfirmed characters: %w", err)
	}
	return n, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*domain.Character, error) {
	c, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("usecase get character: %w", err)
	}
	return c, nil
}

// List は items と、Limit/Offset を無視した総件数 total を返す（ページャ用）。
// items と total は別クエリ（別スナップショット）のため瞬間的に不整合になり得るが、
// ページャ用途では許容する。
func (s *Service) List(ctx context.Context, p domain.ListParams) ([]*domain.Character, int64, error) {
	items, err := s.repo.List(ctx, p)
	if err != nil {
		return nil, 0, fmt.Errorf("usecase list characters: %w", err)
	}
	total, err := s.repo.Count(ctx, p)
	if err != nil {
		return nil, 0, fmt.Errorf("usecase count characters: %w", err)
	}
	return items, total, nil
}

// ReplaceInput は PUT（全置換）の入力。本体フィールドは CreateInput と同一
// （PUT は「作成時と同じ表現で丸ごと置き換える」セマンティクスのため）。
// CreationToken は作成専用なので PUT では無視される（toReplaceFields で落とす）。
type ReplaceInput = CreateInput

// toReplaceFields は ReplaceInput(=CreateInput) を全置換用の ReplaceFields へ写像する。
// 作成専用の CreationToken は持ち込まない（PUT に冪等トークンの概念は無い）。
func toReplaceFields(in ReplaceInput) domain.ReplaceFields {
	return domain.ReplaceFields{
		Name:        in.Name,
		Description: in.Description,
		RaceID:      in.RaceID,
		Gender:      in.Gender,
		BirthDate:   in.BirthDate,
		BirthPlace:  in.BirthPlace,
		HeightCm:    in.HeightCm,
		WeightKg:    in.WeightKg,
		BodyFat:     in.BodyFat,
		SizeTop:     in.SizeTop,
		SizeMiddle:  in.SizeMiddle,
		SizeBottom:  in.SizeBottom,
	}
}

// Replace は PUT（全置換）。送られなかった任意項目はクリアされる。
// expectedVersion が非 nil なら楽観ロック（版不一致は ErrVersionConflict）。
// nil（If-Match 省略）の場合も、読み取り時点の version を期待値として使い、
// FindByID と Update の間に入った他者の更新を黙って上書きしない（lost update 防止）。
// 返り値は repo.Update が返す永続化後の最新状態（version / updated_at は DB 確定値）。
func (s *Service) Replace(ctx context.Context, id uuid.UUID, in ReplaceInput, expectedVersion *int64) (*domain.Character, error) {
	c, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if expectedVersion == nil {
		v := c.Version
		expectedVersion = &v
	}
	if err := c.Replace(toReplaceFields(in)); err != nil {
		return nil, err
	}
	updated, err := s.repo.Update(ctx, c, expectedVersion)
	if err != nil {
		return nil, fmt.Errorf("usecase replace character: %w", err)
	}
	return updated, nil
}

type UpdateInput struct {
	domain.UpdateFields
}

// Update は PATCH（部分更新）。nil のフィールドは変更しない（クリアはできない＝全消しは PUT を使う）。
// expectedVersion が非 nil なら楽観ロック（版不一致は ErrVersionConflict）。
// nil（If-Match 省略）の場合も、読み取り時点の version を期待値として使い、
// 「読んでマージして全置換」の間に入った他者の更新を黙って失わない（lost update 防止）。
func (s *Service) Update(ctx context.Context, id uuid.UUID, in UpdateInput, expectedVersion *int64) (*domain.Character, error) {
	c, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if expectedVersion == nil {
		v := c.Version
		expectedVersion = &v
	}
	if err := c.Update(in.UpdateFields); err != nil {
		return nil, err
	}
	updated, err := s.repo.Update(ctx, c, expectedVersion)
	if err != nil {
		return nil, fmt.Errorf("usecase update character: %w", err)
	}
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("usecase delete character: %w", err)
	}
	return nil
}

func (s *Service) now() time.Time { return s.clock().UTC() }
