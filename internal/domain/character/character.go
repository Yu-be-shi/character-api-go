package character

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrNotFound      = errors.New("character not found")
	ErrInvalidName   = errors.New("invalid character name")
	ErrInvalidGender = errors.New("invalid gender")
)

const (
	nameMinLen = 1
	nameMaxLen = 100
)

type Gender string

const (
	GenderMale    Gender = "male"
	GenderFemale  Gender = "female"
	GenderOther   Gender = "other"
	GenderUnknown Gender = "unknown"
)

// AllGenders は許可される gender の唯一の定義。バリデーション等はここを参照し、
// 値の一覧を各所にハードコードしない（DTO の oneof 文字列の重複を排除する）。
var AllGenders = []Gender{GenderMale, GenderFemale, GenderOther, GenderUnknown}

func (g Gender) Valid() bool {
	for _, v := range AllGenders {
		if g == v {
			return true
		}
	}
	return false
}

type Character struct {
	ID          uuid.UUID
	Name        string
	Description string
	RaceID      uuid.UUID
	RaceName    string
	Gender      Gender
	BirthDate   *time.Time
	BirthPlace  string
	HeightCm    *int16
	WeightKg    *int16
	BodyFat     *float32
	SizeTop     *int16
	SizeMiddle  *int16
	SizeBottom  *int16
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func New(name, description string, raceID uuid.UUID, gender Gender, now time.Time) (*Character, error) {
	name, err := normalizeName(name)
	if err != nil {
		return nil, err
	}
	if !gender.Valid() {
		return nil, ErrInvalidGender
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("character: generate id: %w", err)
	}
	return &Character{
		ID:          id,
		Name:        name,
		Description: description,
		RaceID:      raceID,
		Gender:      gender,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (c *Character) Update(in UpdateFields, now time.Time) error {
	if in.Name != nil {
		n, err := normalizeName(*in.Name)
		if err != nil {
			return err
		}
		c.Name = n
	}
	if in.Description != nil {
		c.Description = *in.Description
	}
	if in.RaceID != nil {
		c.RaceID = *in.RaceID
	}
	if in.Gender != nil {
		if !in.Gender.Valid() {
			return ErrInvalidGender
		}
		c.Gender = *in.Gender
	}
	c.BirthDate = coalesce(in.BirthDate, c.BirthDate)
	c.BirthPlace = coalesceStr(in.BirthPlace, c.BirthPlace)
	c.HeightCm = coalesce(in.HeightCm, c.HeightCm)
	c.WeightKg = coalesce(in.WeightKg, c.WeightKg)
	c.BodyFat = coalesce(in.BodyFat, c.BodyFat)
	c.SizeTop = coalesce(in.SizeTop, c.SizeTop)
	c.SizeMiddle = coalesce(in.SizeMiddle, c.SizeMiddle)
	c.SizeBottom = coalesce(in.SizeBottom, c.SizeBottom)
	c.UpdatedAt = now
	return nil
}

// UpdateFields はすべてポインタ — nil は「変更しない」を意味する。
type UpdateFields struct {
	Name        *string
	Description *string
	RaceID      *uuid.UUID
	Gender      *Gender
	BirthDate   *time.Time
	BirthPlace  *string
	HeightCm    *int16
	WeightKg    *int16
	BodyFat     *float32
	SizeTop     *int16
	SizeMiddle  *int16
	SizeBottom  *int16
}

func normalizeName(name string) (string, error) {
	n := strings.TrimSpace(name)
	l := utf8.RuneCountInString(n)
	if l < nameMinLen || l > nameMaxLen {
		return "", fmt.Errorf("%w: length must be %d..%d", ErrInvalidName, nameMinLen, nameMaxLen)
	}
	return n, nil
}

func coalesce[T any](patch *T, current *T) *T {
	if patch != nil {
		return patch
	}
	return current
}

func coalesceStr(patch *string, current string) string {
	if patch != nil {
		return *patch
	}
	return current
}
