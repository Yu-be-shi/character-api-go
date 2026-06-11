// Package idempotency は冪等キーのストア実装を提供する。
// インターフェース（ポート）は internal/domain/idempotency に定義されており、
// ここには Redis 実装（本番）とインメモリ実装（ローカル/テスト）だけを置く。
package idempotency

import domain "github.com/yu-be-shi/character-api/internal/domain/idempotency"

// 実装が満たすべきポートの別名（利用側は domain 側を import すること）。
type (
	Result = domain.Result
	Store  = domain.Store
)

// コンパイル時にポートを満たすことを確認する。
var (
	_ Store = (*MemoryStore)(nil)
	_ Store = (*RedisStore)(nil)
)
