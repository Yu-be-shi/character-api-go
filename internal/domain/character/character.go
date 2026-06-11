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
	// ErrRaceNotFound は参照先の race が存在しないとき（外部キー違反）に返す。
	// 作成・更新時に race_id の存在を事前 SELECT せず、DB の FK 制約違反をこれに変換する。
	ErrRaceNotFound = errors.New("referenced race not found")
	// ErrVersionConflict は楽観ロックの版不一致（別の更新が先に入った）ときに返す。
	ErrVersionConflict = errors.New("version conflict")
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
	Version     int64 // 楽観ロック用。更新のたびに DB 側で +1 される
	CreatedAt   time.Time
	UpdatedAt   time.Time
	// CreationToken は作成の冪等トークン（消費者の Idempotency-Key 由来。任意）。
	// 同一トークンの再作成は DB 側の一意制約が弾き、既存行が返る（Redis 非依存の二重作成防止）。
	CreationToken *string
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
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// Update は PATCH（部分更新）のマージ。version / updated_at は DB 側
// （update_character 関数とトリガー）が確定するため、ここでは触らない。
func (c *Character) Update(in UpdateFields) error {
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
	return nil
}

// ReplaceFields は PUT（全置換）用。送られなかった任意項目は明示的に未設定（NULL/空）にする。
// UpdateFields（PATCH・部分更新）と異なり「変更しない」概念は無く、常に全項目を上書きする。
type ReplaceFields struct {
	Name        string
	Description string
	RaceID      uuid.UUID
	Gender      Gender
	BirthDate   *time.Time
	BirthPlace  string
	HeightCm    *int16
	WeightKg    *int16
	BodyFat     *float32
	SizeTop     *int16
	SizeMiddle  *int16
	SizeBottom  *int16
}

// Replace は PUT セマンティクス（全置換）。id / created_at / deleted_at 以外を丸ごと上書きし、
// 任意項目の nil/空はそのまま未設定にする（＝既存値をクリアできる）。
// version / updated_at は DB 側が確定するため、ここでは触らない。
func (c *Character) Replace(f ReplaceFields) error {
	name, err := normalizeName(f.Name)
	if err != nil {
		return err
	}
	if !f.Gender.Valid() {
		return ErrInvalidGender
	}
	c.Name = name
	c.Description = f.Description
	c.RaceID = f.RaceID
	c.Gender = f.Gender
	c.BirthDate = f.BirthDate
	c.BirthPlace = f.BirthPlace
	c.HeightCm = f.HeightCm
	c.WeightKg = f.WeightKg
	c.BodyFat = f.BodyFat
	c.SizeTop = f.SizeTop
	c.SizeMiddle = f.SizeMiddle
	c.SizeBottom = f.SizeBottom
	return nil
}

// UpdateFields はすべてポインタ — nil は「変更しない」を意味する（PATCH・部分更新）。
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
