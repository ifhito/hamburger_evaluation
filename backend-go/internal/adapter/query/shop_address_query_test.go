package query_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// TestShopQueryPrefecture は、ショップの一覧の都道府県の絞り込みと、住所の読み取りを、実 PostgreSQL で
// 確かめる。絞り込みは、どの並び順でも効き、keyword とは AND になる。
func TestShopQueryPrefecture(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	shopQuery := query.NewShopQuery(conn)

	insertShop := `INSERT INTO shops (name, status, prefecture_code, city, street_address) VALUES ($1, $2, $3, $4, $5) RETURNING id`
	tokyoBurger := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "Tokyo Burger", 1, 13, "渋谷区", "神南1-2-3")
	tokyoGrill := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "Tokyo Grill", 1, 13, "新宿区", "")
	dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "Osaka Burger", 1, 27, "大阪市北区", "")
	dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "Nowhere Burger", 1, nil, "", "")
	dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "Tokyo Pending", 0, 13, "", "") // 匿名には見えない

	anon := domain.ShopVisibilityFor(nil)
	tokyo := 13
	ids := func(t *testing.T, keyword string, prefectureCode *int, sort usecase.ShopSort) []string {
		t.Helper()
		listings, _, err := shopQuery.ListShops(ctx, anon, keyword, prefectureCode, sort, 100, 0)
		if err != nil {
			t.Fatalf("ListShops returned error: %v", err)
		}
		out := make([]string, 0, len(listings))
		for _, l := range listings {
			out = append(out, l.ID)
		}
		return sortedIDs(out...)
	}

	for _, sort := range []usecase.ShopSort{"", usecase.ShopSortNewest, usecase.ShopSortRating} {
		t.Run("東京都で絞り込むと、見える東京都のショップだけを返す(並び順 "+string(sort)+")", func(t *testing.T) {
			if got, want := ids(t, "", &tokyo, sort), sortedIDs(tokyoBurger, tokyoGrill); !reflect.DeepEqual(got, want) {
				t.Errorf("ids = %v, want %v", got, want)
			}
		})
	}

	t.Run("keyword と併せると、両方に一致するショップだけを返す", func(t *testing.T) {
		if got, want := ids(t, "Burger", &tokyo, ""), []string{tokyoBurger}; !reflect.DeepEqual(got, want) {
			t.Errorf("ids = %v, want %v", got, want)
		}
	})

	t.Run("都道府県を指定しなければ、住所のないショップも含めてすべて返す", func(t *testing.T) {
		if got := ids(t, "", nil, ""); len(got) != 4 {
			t.Errorf("ids = %v, want active な 4 件", got)
		}
	})

	t.Run("一覧と詳細は、住所を読み取る", func(t *testing.T) {
		listings, _, err := shopQuery.ListShops(ctx, anon, "Tokyo Burger", &tokyo, "", 100, 0)
		if err != nil || len(listings) != 1 {
			t.Fatalf("listings = %+v, err = %v", listings, err)
		}
		want := domain.ShopAddress{PrefectureCode: &tokyo, City: "渋谷区", StreetAddress: "神南1-2-3"}
		if !reflect.DeepEqual(listings[0].ShopAddress, want) {
			t.Errorf("一覧の住所 = %+v, want %+v", listings[0].ShopAddress, want)
		}
		detail, err := shopQuery.GetShopWithCreator(ctx, tokyoBurger)
		if err != nil {
			t.Fatalf("GetShopWithCreator returned error: %v", err)
		}
		if !reflect.DeepEqual(detail.ShopAddress, want) {
			t.Errorf("詳細の住所 = %+v, want %+v", detail.ShopAddress, want)
		}
	})
}
