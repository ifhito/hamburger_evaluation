package db_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
)

// TestShopClosedAtUpgrade は、作成時期により閉業日時の列が異なる既存DBからの更新を検証する。
// 空DBからの全適用だけでは、適用済みのCREATE TABLEを書き換えたときの追加漏れを検出できない。
func TestShopClosedAtUpgrade(t *testing.T) {
	for _, tt := range []struct {
		name      string
		hasColumn bool
	}{
		{"閉業日時の列がない既存DBを更新すると店舗一覧を取得できる", false},
		{"すでに閉業日時があるDBを更新しても日時と店舗データを保持する", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			conn, _ := dbtest.NewEmpty(t)
			ups, downs := dbtest.LoadMigrations(t)
			var before, after, rollback []string
			for _, file := range ups {
				if filepath.Base(file) < "000019_" {
					before = append(before, file)
				} else {
					after = append(after, file)
				}
			}
			for _, file := range downs {
				if strings.HasPrefix(filepath.Base(file), "000019_") {
					rollback = append(rollback, file)
				}
			}
			dbtest.Apply(ctx, t, conn, before)
			if tt.hasColumn {
				// 一時的に列を含んでいたCREATE TABLEから作られたDBも更新できる。
				if _, err := conn.Exec(ctx, "ALTER TABLE shops ADD COLUMN closed_at timestamptz"); err != nil {
					t.Fatal(err)
				}
			}
			id := dbtest.InsertUUIDRow(ctx, t, conn, "INSERT INTO shops (name, status, map_url) VALUES ('既存店', 1, 'https://maps.example/shop') RETURNING id")
			closedAt := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
			shops := query.NewShopQuery(conn)
			if tt.hasColumn {
				if _, err := conn.Exec(ctx, "UPDATE shops SET closed_at = $1 WHERE id = $2", closedAt, id); err != nil {
					t.Fatal(err)
				}
			} else {
				_, _, err := shops.ListShops(ctx, domain.ShopVisibility{}, "", "", 20, 0)
				var pgErr *pgconn.PgError
				if !errors.As(err, &pgErr) || pgErr.Code != "42703" {
					t.Fatalf("追加前の一覧エラー = %v, want undefined_column", err)
				}
			}
			dbtest.Apply(ctx, t, conn, after)
			items, more, err := shops.ListShops(ctx, domain.ShopVisibility{}, "", "", 20, 0)
			if err != nil {
				t.Fatalf("追加後の店舗一覧: %v", err)
			}
			if more || len(items) != 1 || items[0].ID != id || items[0].Name != "既存店" || items[0].MapURL == nil || *items[0].MapURL != "https://maps.example/shop" {
				t.Fatalf("既存店舗データが変わった: items=%+v hasMore=%v", items, more)
			}
			if tt.hasColumn {
				if items[0].ClosedAt == nil || !items[0].ClosedAt.Equal(closedAt) {
					t.Fatalf("閉業日時が変わった: %v", items[0].ClosedAt)
				}
			} else if items[0].ClosedAt != nil {
				t.Fatalf("既存店は営業中のままであること: %v", items[0].ClosedAt)
			}
			// downは列だけを取り消し、店舗行を残す。再適用できることも確認する。
			dbtest.Apply(ctx, t, conn, rollback)
			var exists bool
			if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'shops' AND column_name = 'closed_at')").Scan(&exists); err != nil || exists {
				t.Fatalf("down後の閉業日時の列: exists=%v err=%v", exists, err)
			}
			var count int
			if err := conn.QueryRow(ctx, "SELECT count(*) FROM shops WHERE id = $1", id).Scan(&count); err != nil || count != 1 {
				t.Fatalf("店舗行: count=%d err=%v", count, err)
			}
			dbtest.Apply(ctx, t, conn, after)
			if _, _, err := shops.ListShops(ctx, domain.ShopVisibility{}, "", "", 20, 0); err != nil {
				t.Fatalf("再適用後: %v", err)
			}
		})
	}
}
