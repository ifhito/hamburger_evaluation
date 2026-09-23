package query_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// TestBurgerQuery は、burger 詳細の読み取り（adapter/query の BurgerQuery）を、共有の dbtest の
// スキャフォールドを通じて実際の PostgreSQL に対して検証する（TEST_DATABASE_URL がなければスキップする）。
func TestBurgerQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	burgerQuery := query.NewBurgerQuery(conn)

	insertBurger := `INSERT INTO burgers (name) VALUES ($1) RETURNING id`
	cheese := dbtest.InsertUUIDRow(ctx, t, conn, insertBurger, "Cheese")
	veggie := dbtest.InsertUUIDRow(ctx, t, conn, insertBurger, "Veggie") // 統計行なし
	stale := dbtest.InsertUUIDRow(ctx, t, conn, insertBurger, "Stale")   // 統計行はあるが 0 件に戻った

	if _, err := conn.Exec(ctx,
		`INSERT INTO burger_stats (burger_id, review_count, average_rating, weighted_score, confidence, calculated_at)
		 VALUES ($1, 3, 4.2, 4.0, 0.8, now())`, cheese); err != nil {
		t.Fatalf("insert burger stats: %v", err)
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO burger_stats (burger_id, review_count, average_rating, weighted_score, confidence, calculated_at)
		 VALUES ($1, 0, 0, 0, 0, now())`, stale); err != nil {
		t.Fatalf("insert stale burger stats: %v", err)
	}

	t.Run("GetBurgerWithStats は保存された統計をそのまま返す", func(t *testing.T) {
		detail, err := burgerQuery.GetBurgerWithStats(ctx, cheese)
		if err != nil {
			t.Fatalf("GetBurgerWithStats returned error: %v", err)
		}
		count, avg, weighted := int64(3), 4.2, 4.0
		want := domain.BurgerDetail{ID: cheese, Name: "Cheese", ReviewCount: &count, AverageRating: &avg, WeightedScore: &weighted}
		if !reflect.DeepEqual(detail, want) {
			t.Errorf("detail = %+v, want %+v", detail, want)
		}
	})

	t.Run("GetBurgerWithStats は統計行がまだない burger の統計を nil にする", func(t *testing.T) {
		detail, err := burgerQuery.GetBurgerWithStats(ctx, veggie)
		if err != nil {
			t.Fatalf("GetBurgerWithStats returned error: %v", err)
		}
		want := domain.BurgerDetail{ID: veggie, Name: "Veggie"}
		if !reflect.DeepEqual(detail, want) {
			t.Errorf("detail = %+v, want %+v (nil stats)", detail, want)
		}
	})

	t.Run("GetBurgerWithStats は削除で 0 件に戻った burger の統計も nil にする(実際の値 0 と区別する)", func(t *testing.T) {
		detail, err := burgerQuery.GetBurgerWithStats(ctx, stale)
		if err != nil {
			t.Fatalf("GetBurgerWithStats returned error: %v", err)
		}
		want := domain.BurgerDetail{ID: stale, Name: "Stale"}
		if !reflect.DeepEqual(detail, want) {
			t.Errorf("detail = %+v, want %+v (nil stats)", detail, want)
		}
	})

	t.Run("GetBurgerWithStats は存在しない id に ErrBurgerNotFound を返す", func(t *testing.T) {
		if _, err := burgerQuery.GetBurgerWithStats(ctx, uid.N(9999)); !errors.Is(err, domain.ErrBurgerNotFound) {
			t.Fatalf("error = %v, want %v", err, domain.ErrBurgerNotFound)
		}
	})

	t.Run("ListBurgerShops は紐づく shop を作成の古い順で返す", func(t *testing.T) {
		insertShop := `INSERT INTO shops (name, status, created_at) VALUES ($1, $2, $3) RETURNING id`
		tOld := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "Old Diner", 1, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		tNew := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "New Diner", 1, time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC))
		mustLink := func(shopID string) {
			if _, err := conn.Exec(ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, shopID, cheese); err != nil {
				t.Fatalf("link shop %s burger %s: %v", shopID, cheese, err)
			}
		}
		mustLink(tNew)
		mustLink(tOld)

		shops, err := burgerQuery.ListBurgerShops(ctx, cheese)
		if err != nil {
			t.Fatalf("ListBurgerShops returned error: %v", err)
		}
		want := []domain.Shop{
			{ID: tOld, Name: "Old Diner", Status: domain.ShopStatusActive},
			{ID: tNew, Name: "New Diner", Status: domain.ShopStatusActive},
		}
		if !reflect.DeepEqual(shops, want) {
			t.Errorf("shops = %+v, want %+v", shops, want)
		}
	})

	t.Run("ListBurgerShops は紐づく shop がなければ空を返す", func(t *testing.T) {
		shops, err := burgerQuery.ListBurgerShops(ctx, veggie)
		if err != nil {
			t.Fatalf("ListBurgerShops returned error: %v", err)
		}
		if len(shops) != 0 {
			t.Errorf("shops = %+v, want empty", shops)
		}
	})
}
