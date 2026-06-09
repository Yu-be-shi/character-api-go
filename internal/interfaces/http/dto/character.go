package dto

import (
	"time"

	"github.com/google/uuid"

	domain "github.com/yu-be-shi/character-api/internal/domain/character"
)

type CharacterResponse struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	RaceID      uuid.UUID  `json:"raceId"`
	RaceName    string     `json:"race"`
	Gender      string     `json:"gender"`
	BirthDate   *time.Time `json:"birthDate,omitempty"`
	BirthPlace  string     `json:"birthPlace,omitempty"`
	HeightCm    *int16     `json:"heightCm,omitempty"`
	WeightKg    *int16     `json:"weightKg,omitempty"`
	BodyFat     *float32   `json:"bodyFatPercentage,omitempty"`
	SizeTop     *int16     `json:"sizeTop,omitempty"`
	SizeMiddle  *int16     `json:"sizeMiddle,omitempty"`
	SizeBottom  *int16     `json:"sizeBottom,omitempty"`
	Version     int64      `json:"version"` // 楽観ロック用。更新時に If-Match で送り返す
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

func FromDomain(c *domain.Character) CharacterResponse {
	return CharacterResponse{
		ID:          c.ID,
		Name:        c.Name,
		Description: c.Description,
		RaceID:      c.RaceID,
		RaceName:    c.RaceName,
		Gender:      string(c.Gender),
		BirthDate:   c.BirthDate,
		BirthPlace:  c.BirthPlace,
		HeightCm:    c.HeightCm,
		WeightKg:    c.WeightKg,
		BodyFat:     c.BodyFat,
		SizeTop:     c.SizeTop,
		SizeMiddle:  c.SizeMiddle,
		SizeBottom:  c.SizeBottom,
		Version:     c.Version,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
	}
}

func FromDomainList(cs []*domain.Character) []CharacterResponse {
	out := make([]CharacterResponse, 0, len(cs))
	for _, c := range cs {
		out = append(out, FromDomain(c))
	}
	return out
}

// CharacterListResponse は一覧のレスポンス。items（このページの配列）と
// total（Limit/Offset を無視した総件数）を返し、フロントがページャを作れるようにする。
type CharacterListResponse struct {
	Items []CharacterResponse `json:"items"`
	Total int64               `json:"total"`
}

func NewCharacterListResponse(cs []*domain.Character, total int64) CharacterListResponse {
	return CharacterListResponse{Items: FromDomainList(cs), Total: total}
}

type CreateCharacterRequest struct {
	Name        string     `json:"name"        validate:"required,min=1,max=100"`
	Description string     `json:"description" validate:"omitempty,max=5000"`
	RaceID      uuid.UUID  `json:"raceId"      validate:"required"`
	Gender      string     `json:"gender"      validate:"required,gender"`
	BirthDate   *time.Time `json:"birthDate"   validate:"omitempty"`
	BirthPlace  string     `json:"birthPlace"  validate:"omitempty,max=150"`
	HeightCm    *int16     `json:"heightCm"    validate:"omitempty,min=1"`
	WeightKg    *int16     `json:"weightKg"    validate:"omitempty,min=1"`
	BodyFat     *float32   `json:"bodyFatPercentage" validate:"omitempty,min=0,max=100"`
	SizeTop     *int16     `json:"sizeTop"     validate:"omitempty,min=1"`
	SizeMiddle  *int16     `json:"sizeMiddle"  validate:"omitempty,min=1"`
	SizeBottom  *int16     `json:"sizeBottom"  validate:"omitempty,min=1"`
}

type UpdateCharacterRequest struct {
	Name        *string    `json:"name"        validate:"omitempty,min=1,max=100"`
	Description *string    `json:"description" validate:"omitempty,max=5000"`
	RaceID      *uuid.UUID `json:"raceId"      validate:"omitempty"`
	Gender      *string    `json:"gender"      validate:"omitempty,gender"`
	BirthDate   *time.Time `json:"birthDate"   validate:"omitempty"`
	BirthPlace  *string    `json:"birthPlace"  validate:"omitempty,max=150"`
	HeightCm    *int16     `json:"heightCm"    validate:"omitempty,min=1"`
	WeightKg    *int16     `json:"weightKg"    validate:"omitempty,min=1"`
	BodyFat     *float32   `json:"bodyFatPercentage" validate:"omitempty,min=0,max=100"`
	SizeTop     *int16     `json:"sizeTop"     validate:"omitempty,min=1"`
	SizeMiddle  *int16     `json:"sizeMiddle"  validate:"omitempty,min=1"`
	SizeBottom  *int16     `json:"sizeBottom"  validate:"omitempty,min=1"`
}

type ErrorResponse struct {
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}
