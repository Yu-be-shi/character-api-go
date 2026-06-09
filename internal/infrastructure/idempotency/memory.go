package idempotency

import (
	"context"
	"sync"
)

// MemoryStore はインメモリの Store 実装（ローカル単一プロセス・テスト用。TTL は持たない）。
// 複数インスタンスでは共有されないため本番では Redis 実装を使うこと。
type MemoryStore struct {
	mu   sync.Mutex
	data map[string]*Result // キーあり & 値nil = 処理中 / 値あり = 完了
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: map[string]*Result{}}
}

func (s *MemoryStore) Begin(_ context.Context, key string) (*Result, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.data[key]; !exists {
		s.data[key] = nil // 予約（処理中）
		return nil, true, nil
	}
	return s.data[key], false, nil
}

func (s *MemoryStore) Complete(_ context.Context, key string, res Result) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := res
	s.data[key] = &r
	return nil
}

func (s *MemoryStore) Release(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}
