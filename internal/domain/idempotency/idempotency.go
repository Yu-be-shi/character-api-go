// Package idempotency は冪等キー（Idempotency-Key）の予約・結果保存のポート（インターフェース）を定義する。
//
// 依存方向の規約（interfaces/http → usecase → domain ← infrastructure）を守るため、
// 抽象は domain 層に置き、実装（Redis / インメモリ）は infrastructure/idempotency が提供する。
// 利用側（HTTP ミドルウェア）はこのポートだけに依存し、実装を差し替え可能にする
// （persistence が domain.Repository を実装するのと同じ構図）。
package idempotency

import "context"

// Result は冪等キーに紐づけて保存する完了レスポンス（再送時に再生する）。
//   - BodyHash: リクエストボディの SHA-256。同一キーで異なるボディが来たことを
//     検知するために保存する（黙って別リクエストの結果を再生しない）。
//   - Header: 再生時に復元するレスポンスヘッダ（ETag / Content-Type 等）。
type Result struct {
	Status   int               `json:"status"`
	Body     []byte            `json:"body"`
	BodyHash string            `json:"body_hash"`
	Header   map[string]string `json:"header,omitempty"`
}

// Store は冪等キーの予約・結果保存・解放を行う。
type Store interface {
	// Begin は key を予約する。
	//   - 新規に予約できた   : (nil, true, nil)
	//   - 既に完了結果がある : (result, false, nil)  ← 再送なので再生する
	//   - 既に処理中(結果未確定): (nil, false, nil)  ← 二重実行中なので 409
	Begin(ctx context.Context, key string) (*Result, bool, error)
	// Complete は処理結果を保存する（以後の同一キーは再生される）。
	Complete(ctx context.Context, key string, res Result) error
	// Release は予約を解放する（処理が失敗したとき。同一キーで再試行できるようにする）。
	Release(ctx context.Context, key string) error
}
