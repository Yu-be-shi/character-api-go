package character

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	domain "github.com/yu-be-shi/character-api/internal/domain/character"
)

// Clock は現在時刻を返す関数型。テストで差し替えられる。
type Clock func() time.Time

type Service struct {
	repo  domain.Repository
	clock Clock
}

func NewService(repo domain.Repository, clock Clock) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{repo: repo, clock: clock}
}

type CreateInput struct {
	Name       string
	Attributes domain.Attributes
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*domain.Character, error) {
	c, err := domain.New(in.Name, in.Attributes, s.now())
	if err != nil {
		return nil, err
	}
	if err := s.repo.Save(ctx, c); err != nil {
		return nil, fmt.Errorf("usecase create character: %w", err)
	}
	return c, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*domain.Character, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]*domain.Character, error) {
	return s.repo.List(ctx)
}

type UpdateInput struct {
	Name       *string
	Attributes *domain.Attributes
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, in UpdateInput) (*domain.Character, error) {
	c, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	now := s.now()
	if in.Name != nil {
		if err := c.Rename(*in.Name, now); err != nil {
			return nil, err
		}
	}
	if in.Attributes != nil {
		c.ReplaceAttributes(*in.Attributes, now)
	}
	if err := s.repo.Update(ctx, c); err != nil {
		return nil, fmt.Errorf("usecase update character: %w", err)
	}
	return c, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return err
		}
		return fmt.Errorf("usecase delete character: %w", err)
	}
	return nil
}

func (s *Service) now() time.Time {
	return s.clock().UTC()
}
