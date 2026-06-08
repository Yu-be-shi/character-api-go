package character

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	domain "github.com/yu-be-shi/character-api/internal/domain/character"
	racedomain "github.com/yu-be-shi/character-api/internal/domain/race"
)

type Clock func() time.Time

type Service struct {
	repo     domain.Repository
	raceRepo racedomain.Repository
	clock    Clock
}

func NewService(repo domain.Repository, raceRepo racedomain.Repository, clock Clock) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{repo: repo, raceRepo: raceRepo, clock: clock}
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
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*domain.Character, error) {
	if _, err := s.raceRepo.FindByID(ctx, in.RaceID); err != nil {
		if errors.Is(err, racedomain.ErrNotFound) {
			return nil, fmt.Errorf("usecase create character: %w", racedomain.ErrNotFound)
		}
		return nil, fmt.Errorf("usecase create character: %w", err)
	}
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

	if err := s.repo.Save(ctx, c); err != nil {
		return nil, fmt.Errorf("usecase create character: %w", err)
	}
	return c, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*domain.Character, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *Service) List(ctx context.Context, p domain.ListParams) ([]*domain.Character, error) {
	return s.repo.List(ctx, p)
}

type UpdateInput struct {
	domain.UpdateFields
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, in UpdateInput) (*domain.Character, error) {
	if in.RaceID != nil {
		if _, err := s.raceRepo.FindByID(ctx, *in.RaceID); err != nil {
			if errors.Is(err, racedomain.ErrNotFound) {
				return nil, fmt.Errorf("usecase update character: %w", racedomain.ErrNotFound)
			}
			return nil, fmt.Errorf("usecase update character: %w", err)
		}
	}
	c, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := c.Update(in.UpdateFields, s.now()); err != nil {
		return nil, err
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

func (s *Service) now() time.Time { return s.clock().UTC() }
