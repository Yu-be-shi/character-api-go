package dto

import (
	"github.com/google/uuid"

	domain "github.com/yu-be-shi/character-api/internal/domain/race"
)

type RaceResponse struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

func RaceFromDomain(r *domain.Race) RaceResponse {
	return RaceResponse{ID: r.ID, Name: r.Name}
}

func RaceFromDomainList(rs []*domain.Race) []RaceResponse {
	out := make([]RaceResponse, 0, len(rs))
	for _, r := range rs {
		out = append(out, RaceFromDomain(r))
	}
	return out
}

type CreateRaceRequest struct {
	Name string `json:"name" validate:"required,min=1,max=50"`
}

type UpdateRaceRequest struct {
	Name string `json:"name" validate:"required,min=1,max=50"`
}
