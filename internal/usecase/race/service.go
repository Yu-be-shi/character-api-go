package race

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	domain "github.com/yu-be-shi/character-api/internal/domain/race"
)

type Service struct {
	repo domain.Repository
}

func NewService(repo domain.Repository) *Service {
	return &Service{repo: repo}
}

type CreateInput struct {
	Name string
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*domain.Race, error) {
	r, err := domain.New(in.Name)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Save(ctx, r); err != nil {
		return nil, fmt.Errorf("usecase create race: %w", err)
	}
	return r, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*domain.Race, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]*domain.Race, error) {
	return s.repo.List(ctx)
}

type UpdateInput struct {
	Name string
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, in UpdateInput) (*domain.Race, error) {
	r, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := r.Rename(in.Name); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, r); err != nil {
		return nil, fmt.Errorf("usecase update race: %w", err)
	}
	return r, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return err
		}
		return fmt.Errorf("usecase delete race: %w", err)
	}
	return nil
}
