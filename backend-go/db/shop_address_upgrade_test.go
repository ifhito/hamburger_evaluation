package db_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
)

// TestShopAddressUpgrade は、住所の列がない既存のDBに 000020 を当てたときの更新を検証する。
// 空のDBからの全適用だけでは、ショップの行がある既存のDBで ALTER が通るかを確かめられない。
func TestShopAddressUpgrade(t *testing.T) {
	t.Run("住所の列がない既存DBを更新すると既存の店は住所が未設定のまま一覧に出て、取り消しと再適用もできる", func(t *testing.T) {
		ctx := context.Background()
		conn, _ := dbtest.NewEmpty(t)
		ups, downs := dbtest.LoadMigrations(t)
		var before, after, rollback []string
		for _, file := range ups {
			if filepath.Base(file) < "000020_" {
				before = append(before, file)
			} else {
				after = append(after, file)
			}
		}
		for _, file := range downs {
			if filepath.Base(file) >= "000020_" {
				rollback = append(rollback, file)
			}
		}
		dbtest.Apply(ctx, t, conn, before)
		id := dbtest.InsertUUIDRow(ctx, t, conn, "INSERT INTO shops (name, status, map_url) VALUES ('既存店', 1, 'https://maps.example/shop') RETURNING id")

		dbtest.Apply(ctx, t, conn, after)
		shops := query.NewShopQuery(conn)
		items, more, err := shops.ListShops(ctx, domain.ShopVisibility{}, "", nil, "", 20, 0)
		if err != nil {
			t.Fatalf("追加後の店舗一覧: %v", err)
		}
		if more || len(items) != 1 || items[0].ID != id || items[0].Name != "既存店" || items[0].MapURL == nil || *items[0].MapURL != "https://maps.example/shop" {
			t.Fatalf("既存店舗データが変わった: items=%+v hasMore=%v", items, more)
		}
		var prefecture *int16
		var city, street string
		if err := conn.QueryRow(ctx, "SELECT prefecture_code, city, street_address FROM shops WHERE id = $1", id).Scan(&prefecture, &city, &street); err != nil {
			t.Fatal(err)
		}
		if prefecture != nil || city != "" || street != "" {
			t.Fatalf("既存店の住所は未設定であること: prefecture=%v city=%q street=%q", prefecture, city, street)
		}

		// down は住所の列・制約・索引だけを取り消し、店舗行を残す。
		dbtest.Apply(ctx, t, conn, rollback)
		var columns, constraints, indexes, rows int
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'shops' AND column_name IN ('prefecture_code', 'city', 'street_address')").Scan(&columns); err != nil {
			t.Fatal(err)
		}
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM pg_constraint WHERE conname IN ('shops_prefecture_code_range', 'shops_city_max_length', 'shops_street_address_max_length')").Scan(&constraints); err != nil {
			t.Fatal(err)
		}
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM pg_indexes WHERE schemaname = 'public' AND indexname = 'idx_shops_prefecture_code'").Scan(&indexes); err != nil {
			t.Fatal(err)
		}
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM shops WHERE id = $1", id).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if columns != 0 || constraints != 0 || indexes != 0 || rows != 1 {
			t.Fatalf("down後: 列=%d 制約=%d 索引=%d 店舗行=%d", columns, constraints, indexes, rows)
		}

		dbtest.Apply(ctx, t, conn, after)
		if _, _, err := shops.ListShops(ctx, domain.ShopVisibility{}, "", nil, "", 20, 0); err != nil {
			t.Fatalf("再適用後: %v", err)
		}
	})
}
