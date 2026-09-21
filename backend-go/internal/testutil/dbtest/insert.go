package dbtest

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

// InsertUUIDRow は sql（id を RETURN する必要がある）で insert し、生成された id を UUID の正規形の
// 文字列で返す。adapter/query と adapter/repository のテストが、互いに依存せずに fixture を用意する
// ための共通の道具である。
func InsertUUIDRow(ctx context.Context, t *testing.T, conn *pgx.Conn, sql string, args ...any) string {
	t.Helper()
	var id string
	if err := conn.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
		t.Fatalf("insert %q: %v", sql, err)
	}
	return id
}

// InsertUserRow は InsertUUIDRow の、users の id 用の別名である。
func InsertUserRow(ctx context.Context, t *testing.T, conn *pgx.Conn, sql string, args ...any) string {
	t.Helper()
	return InsertUUIDRow(ctx, t, conn, sql, args...)
}
