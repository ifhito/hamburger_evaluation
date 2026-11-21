package infra

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

type fakeSchemaRow struct {
	noSchema           bool
	missingTables      []string
	unprivilegedTables []string
	missingColumns     []string
	hasPartialEmailIdx bool
	err                error
}

func (r fakeSchemaRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	schema := "public"
	if r.noSchema {
		schema = ""
	}
	*(dest[0].(*string)) = schema
	*(dest[1].(*[]string)) = r.missingTables
	*(dest[2].(*[]string)) = r.unprivilegedTables
	*(dest[3].(*[]string)) = r.missingColumns
	*(dest[4].(*bool)) = r.hasPartialEmailIdx
	return nil
}

type fakeHealthDB struct {
	pingErr error
	rows    []fakeSchemaRow
	pings   int
	queries int
}

func (f *fakeHealthDB) Ping(context.Context) error {
	f.pings++
	return f.pingErr
}

func (f *fakeHealthDB) QueryRow(context.Context, string, ...any) pgx.Row {
	i := f.queries
	f.queries++
	if i >= len(f.rows) {
		return fakeSchemaRow{hasPartialEmailIdx: true}
	}
	return f.rows[i]
}

func TestHealthChecker(t *testing.T) {
	ctx := context.Background()

	t.Run("Ping 自体が失敗したときはスキーマ問い合わせに進まずエラーを返す", func(t *testing.T) {
		boom := errors.New("connection refused")
		db := &fakeHealthDB{pingErr: boom}
		hc := NewHealthChecker(db)
		if err := hc.Ping(ctx); !errors.Is(err, boom) {
			t.Fatalf("Ping error = %v, want %v", err, boom)
		}
		if db.queries != 0 {
			t.Errorf("queries = %d, want 0", db.queries)
		}
	})

	t.Run("スキーマ不可視・テーブル不足・権限不足・列不足・インデックス未適用の間は失敗し、解消後に成功して以降はキャッシュする", func(t *testing.T) {
		db := &fakeHealthDB{
			rows: []fakeSchemaRow{
				{noSchema: true},
				{missingTables: []string{"user_sessions"}, hasPartialEmailIdx: false},
				{unprivilegedTables: []string{"user_sessions"}, hasPartialEmailIdx: true},
				{missingColumns: []string{"shops.closed_at"}, hasPartialEmailIdx: true},
				{hasPartialEmailIdx: false},
				{hasPartialEmailIdx: true},
			},
		}
		hc := NewHealthChecker(db)

		// 1 回目: search_path に利用可能なスキーマがない場合
		if err := hc.Ping(ctx); err == nil || !strings.Contains(err.Error(), "no accessible schema in search_path") {
			t.Fatalf("no-schema Ping error = %v, want no accessible schema diagnostic", err)
		}
		// 2 回目: users は存在するが user_sessions と部分一意インデックスの両方が未適用なら、両方まとめて診断に出す
		err := hc.Ping(ctx)
		if err == nil || !strings.Contains(err.Error(), "missing tables: user_sessions") || !strings.Contains(err.Error(), "missing partial unique index: users_email_lower_key") {
			t.Fatalf("combined missing table + index error = %v, want both reported", err)
		}

		for _, wantSubstr := range []string{
			"insufficient table privileges: user_sessions",
			"missing columns: shops.closed_at",
			"missing partial unique index: users_email_lower_key",
		} {
			err := hc.Ping(ctx)
			if err == nil || !strings.Contains(err.Error(), wantSubstr) {
				t.Fatalf("Ping error = %v, want substring %q", err, wantSubstr)
			}
		}

		// マイグレーション適用後は成功し、2 回目以降はカタログを再問い合わせせず Ping だけを行う。
		for i := 0; i < 3; i++ {
			if err := hc.Ping(ctx); err != nil {
				t.Fatalf("pass %d: Ping returned error: %v", i, err)
			}
		}
		if db.queries != 6 {
			t.Errorf("queries = %d, want 6 (cached after first clean schema check)", db.queries)
		}
		if db.pings != 8 {
			t.Errorf("pings = %d, want 8", db.pings)
		}
	})
}
