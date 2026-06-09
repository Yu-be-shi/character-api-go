package idempotency

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// pendingMarker は「予約済み・処理中（結果未確定）」を表す番兵値。
const pendingMarker = "__PENDING__"

// RedisStore は Redis を使う Store 実装。`SET NX` で予約を原子的に行う。
type RedisStore struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisStore(addr string, ttl time.Duration) *RedisStore {
	return &RedisStore{
		client: redis.NewClient(&redis.Options{Addr: addr}),
		ttl:    ttl,
	}
}

// Ping は起動時の疎通確認用。
func (s *RedisStore) Ping(ctx context.Context) error { return s.client.Ping(ctx).Err() }

func (s *RedisStore) Begin(ctx context.Context, key string) (*Result, bool, error) {
	// SET key PENDING NX EX ttl ＝ 「無ければ予約」を原子的に行う。
	ok, err := s.client.SetNX(ctx, key, pendingMarker, s.ttl).Result()
	if err != nil {
		return nil, false, err
	}
	if ok {
		return nil, true, nil // 新規予約
	}
	val, err := s.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil // 直前に失効。処理中扱い
	}
	if err != nil {
		return nil, false, err
	}
	if val == pendingMarker {
		return nil, false, nil // 処理中
	}
	var res Result
	if err := json.Unmarshal([]byte(val), &res); err != nil {
		return nil, false, err
	}
	return &res, false, nil
}

func (s *RedisStore) Complete(ctx context.Context, key string, res Result) error {
	b, err := json.Marshal(res)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, key, b, s.ttl).Err()
}

func (s *RedisStore) Release(ctx context.Context, key string) error {
	return s.client.Del(ctx, key).Err()
}
