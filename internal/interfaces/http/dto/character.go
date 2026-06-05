package dto

import (
	"time"

	"github.com/google/uuid"

	domain "github.com/yu-be-shi/character-api/internal/domain/character"
)

type CharacterResponse struct {
	ID         uuid.UUID         `json:"id"`
	Name       string            `json:"name"`
	Attributes domain.Attributes `json:"attributes"`
	CreatedAt  time.Time         `json:"createdAt"`
	UpdatedAt  time.Time         `json:"updatedAt"`
}

func FromDomain(c *domain.Character) CharacterResponse {
	attrs := c.Attributes
	if attrs == nil {
		attrs = domain.Attributes{}
	}
	return CharacterResponse{
		ID:         c.ID,
		Name:       c.Name,
		Attributes: attrs,
		CreatedAt:  c.CreatedAt,
		UpdatedAt:  c.UpdatedAt,
	}
}

func FromDomainList(cs []*domain.Character) []CharacterResponse {
	out := make([]CharacterResponse, 0, len(cs))
	for _, c := range cs {
		out = append(out, FromDomain(c))
	}
	return out
}

type CreateCharacterRequest struct {
	Name       string            `json:"name"       validate:"required,min=1,max=120"`
	Attributes domain.Attributes `json:"attributes" validate:"omitempty"`
}

type UpdateCharacterRequest struct {
	Name       *string            `json:"name,omitempty"       validate:"omitempty,min=1,max=120"`
	Attributes *domain.Attributes `json:"attributes,omitempty" validate:"omitempty"`
}

type ErrorResponse struct {
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}
