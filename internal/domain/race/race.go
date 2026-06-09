package race

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrNotFound    = errors.New("race not found")
	ErrInvalidName = errors.New("invalid race name")
	ErrDuplicate   = errors.New("race name already exists")
	// ErrInUse は使用中（character から参照されている）の race を削除しようとしたとき返す。
	// 外部キー制約（ON DELETE NO ACTION）の違反をこれに変換する。
	ErrInUse = errors.New("race is in use")
)

const (
	nameMinLen = 1
	nameMaxLen = 50
)

type Race struct {
	ID   uuid.UUID
	Name string
}

func New(name string) (*Race, error) {
	name, err := normalizeName(name)
	if err != nil {
		return nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("race: generate id: %w", err)
	}
	return &Race{ID: id, Name: name}, nil
}

func (r *Race) Rename(name string) error {
	n, err := normalizeName(name)
	if err != nil {
		return err
	}
	r.Name = n
	return nil
}

func normalizeName(name string) (string, error) {
	n := strings.TrimSpace(name)
	l := utf8.RuneCountInString(n)
	if l < nameMinLen || l > nameMaxLen {
		return "", fmt.Errorf("%w: length must be %d..%d", ErrInvalidName, nameMinLen, nameMaxLen)
	}
	return n, nil
}
