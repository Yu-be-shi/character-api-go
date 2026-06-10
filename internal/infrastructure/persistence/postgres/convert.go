package postgres

import (
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// character-db の DB 関数が RAISE するカスタム SQLSTATE と、標準の制約違反コード。
// これらを見てドメインエラーへ変換する（ドライバ非依存にドメイン層を保つ）。
const (
	sqlStateNotFound        = "CH404" // update/soft_delete_character: 不在 or 論理削除済み
	sqlStateVersionConflict = "CH412" // update_character: 楽観ロック競合
	sqlStateForeignKey      = "23503" // foreign_key_violation
	sqlStateUnique          = "23505" // unique_violation
)

// pgCode は pgx のサーバエラーから SQLSTATE を取り出す。
func pgCode(err error) (string, bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code, true
	}
	return "", false
}

// --- ドメイン型 ⇄ pgtype 変換 ---

func pgText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }

func textString(t pgtype.Text) string {
	if t.Valid {
		return t.String
	}
	return ""
}

func pgInt2(p *int16) pgtype.Int2 {
	if p == nil {
		return pgtype.Int2{}
	}
	return pgtype.Int2{Int16: *p, Valid: true}
}

func int2Ptr(v pgtype.Int2) *int16 {
	if !v.Valid {
		return nil
	}
	x := v.Int16
	return &x
}

func pgInt8(p *int64) pgtype.Int8 {
	if p == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *p, Valid: true}
}

func pgDate(p *time.Time) pgtype.Date {
	if p == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *p, Valid: true}
}

func datePtr(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	t := d.Time
	return &t
}

func pgTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// pgNumeric は *float32 を NUMERIC へ。nil は NULL。文字列経由で変換する。
func pgNumeric(p *float32) pgtype.Numeric {
	var n pgtype.Numeric
	if p == nil {
		return n // Valid=false → NULL
	}
	if err := n.Scan(strconv.FormatFloat(float64(*p), 'f', -1, 32)); err != nil {
		return pgtype.Numeric{}
	}
	return n
}

func numericPtr(n pgtype.Numeric) *float32 {
	if !n.Valid {
		return nil
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return nil
	}
	v := float32(f.Float64)
	return &v
}
