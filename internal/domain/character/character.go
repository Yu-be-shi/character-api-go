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
	ErrNotFound    = errors.New("character not found")
	ErrInvalidName = errors.New("invalid character name")
)

const (
	nameMinLen = 1
	nameMaxLen = 120
)

// Attributes はスキーマフレキシブルなオプションフィールドのマップ。
// リポジトリ層がJSONとして永続化する。
type Attributes map[string]any

// Character はキャラクターの集約ルート。
type Character struct {
	ID         uuid.UUID
	Name       string
	Attributes Attributes
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// New は UUID v7 IDを持つ新規キャラクターを生成する。
// ID生成をドメイン層に置くことで全エントリポイントで同一の方式を保証する。
func New(name string, attrs Attributes, now time.Time) (*Character, error) {
	name, err := normalizeName(name)
	if err != nil {
		return nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("character: generate id: %w", err)
	}
	if attrs == nil {
		attrs = Attributes{}
	}
	return &Character{
		ID:         id,
		Name:       name,
		Attributes: attrs,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

func (c *Character) Rename(name string, now time.Time) error {
	n, err := normalizeName(name)
	if err != nil {
		return err
	}
	c.Name = n
	c.UpdatedAt = now
	return nil
}

func (c *Character) ReplaceAttributes(attrs Attributes, now time.Time) {
	if attrs == nil {
		attrs = Attributes{}
	}
	c.Attributes = attrs
	c.UpdatedAt = now
}

func normalizeName(name string) (string, error) {
	n := strings.TrimSpace(name)
	l := utf8.RuneCountInString(n)
	if l < nameMinLen || l > nameMaxLen {
		return "", fmt.Errorf("%w: length must be %d..%d", ErrInvalidName, nameMinLen, nameMaxLen)
	}
	return n, nil
}
