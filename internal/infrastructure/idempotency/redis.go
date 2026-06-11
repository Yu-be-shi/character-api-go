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

// pendingTTL は予約（処理中マーカー）の生存期間。
// プロセスクラッシュ等で Release されなかった予約がこの時間で自動失効し、
// 同一キーが結果 TTL（24h）の間 409 になり続けるのを防ぐ。
// 1 リクエストの処理時間より十分長く、結果 TTL より十分短くする。
const pendingTTL = 5 * time.Minute

// keyPrefix は character-api 専有 Redis 内での名前空間。
const keyPrefix = "idem:"

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
	key = keyPrefix + key
	// SET key PENDING NX EX pendingTTL ＝ 「無ければ予約」を原子的に行う。
	ok, err := s.client.SetNX(ctx, key, pendingMarker, s.pendingTTL()).Result()
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
	// 確定結果はフル TTL で保持する（再送はこの期間中に再生される）。
	return s.client.Set(ctx, keyPrefix+key, b, s.ttl).Err()
}

func (s *RedisStore) Release(ctx context.Context, key string) error {
	return s.client.Del(ctx, keyPrefix+key).Err()
}

// pendingTTL は予約の TTL（結果 TTL が短く設定されていればそちらに合わせる）。
func (s *RedisStore) pendingTTL() time.Duration {
	if s.ttl < pendingTTL {
		return s.ttl
	}
	return pendingTTL
}
