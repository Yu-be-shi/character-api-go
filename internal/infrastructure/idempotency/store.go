// Package idempotency は冪等キー（Idempotency-Key）の予約・結果保存を行うストアを提供する。
// インターフェースに対して Redis 実装（本番）とインメモリ実装（ローカル/テスト）を用意し、
// 利用側（HTTP ミドルウェア）は実装を差し替え可能にする。
package idempotency

import "context"

// Result は冪等キーに紐づけて保存する完了レスポンス（再送時に再生する）。
type Result struct {
	Status int    `json:"status"`
	Body   []byte `json:"body"`
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
