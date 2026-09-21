package dbtest

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

// InsertRow は sql（id を RETURN する必要がある）で insert し、新しい id を返す。
// adapter/query と adapter/repository のテストが、互いに依存せずに fixture を用意するための
// 共通の道具である。
func InsertRow(ctx context.Context, t *testing.T, conn *pgx.Conn, sql string, args ...any) int64 {
	t.Helper()
	var id int64
	if err := conn.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
		t.Fatalf("insert %q: %v", sql, err)
	}
	return id
}

// InsertUUIDRow は InsertRow と同様だが、id が UUID の表(users・shops・burgers)のために、
// 生成された id を UUID の正規形の文字列で返す。
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
